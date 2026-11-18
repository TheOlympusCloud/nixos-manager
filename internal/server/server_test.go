package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheOlympusCloud/nixos-manager/internal/auth"
	"github.com/TheOlympusCloud/nixos-manager/internal/core"
	"github.com/TheOlympusCloud/nixos-manager/internal/mcpserver"
)

func TestMain(m *testing.M) {
	if os.Getenv("FAKE_NIXD") == "1" {
		fakeNixd()
		return
	}
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_COUNT")
	os.Exit(m.Run())
}

// fakeNixd answers initialize, then asks for its configuration the way nixd
// does and reports what it got back as a notification.
func fakeNixd() {
	if f := os.Getenv("FAKE_NIXD_PIDFILE"); f != "" {
		os.WriteFile(f, []byte(strconv.Itoa(os.Getpid())), 0o644)
	}
	br := bufio.NewReader(os.Stdin)
	send := func(v any) {
		b, _ := json.Marshal(v)
		os.Stdout.WriteString("Content-Length: " + strconv.Itoa(len(b)) + "\r\n\r\n" + string(b))
	}
	for {
		msg, err := readFrame(br)
		if err != nil {
			return
		}
		var m map[string]any
		json.Unmarshal(msg, &m)
		switch {
		case m["method"] == "initialize":
			caps := m["params"].(map[string]any)["capabilities"].(map[string]any)
			send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": map[string]any{"capabilities": map[string]any{}, "sawConfigCapability": caps["workspace"].(map[string]any)["configuration"]}})
			send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]any{"section": "nixd"}}}})
		case m["id"] == float64(99):
			send(map[string]any{"jsonrpc": "2.0", "method": "test/config", "params": m["result"]})
		}
	}
}

type env struct {
	srv    *httptest.Server
	client *http.Client
	auth   *auth.Auth
	svc    *core.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo, err := core.OpenRepo(filepath.Join(t.TempDir(), "repo"), "", "Test User", "test@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	svc := core.NewService(repo)
	nix := filepath.Join(t.TempDir(), "nix")
	os.WriteFile(nix, []byte("#!/bin/sh\necho \"nix $*\"\n"), 0o755)
	svc.Jobs.Nix = nix
	hash, _ := auth.HashPassword("pw")
	a, err := auth.New(hash, filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	s := &Server{Svc: svc, Auth: a, MCP: mcpserver.Handler(mcpserver.New(svc, "test")), Nixd: []string{exe},
		UI: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>app")}, "assets/app.js": {Data: []byte("js")}}}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &env{srv: srv, client: &http.Client{Jar: jar}, auth: a, svc: svc}
}

// do sends a request; a non-nil body is sent as JSON. It decodes JSON replies into out.
func (e *env) do(t *testing.T, method, path string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (e *env) login(t *testing.T) {
	t.Helper()
	if code := e.do(t, "POST", "/api/login", map[string]string{"password": "pw"}, nil); code != 204 {
		t.Fatalf("login: %d", code)
	}
}

func TestEverythingNeedsLogin(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/api/tree", "/api/status", "/api/info", "/api/tokens", "/api/lsp"} {
		if code := e.do(t, "GET", p, nil, nil); code != 401 {
			t.Errorf("GET %s without login: %d", p, code)
		}
	}
	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/login", map[string]string{"password": "nope"}, &er); code != 401 || er.Error == "" {
		t.Fatalf("wrong password: %d %+v", code, er)
	}
	e.login(t)
	if code := e.do(t, "GET", "/api/tree", nil, nil); code != 200 {
		t.Fatalf("tree after login: %d", code)
	}
	e.do(t, "POST", "/api/logout", struct{}{}, nil)
	if code := e.do(t, "GET", "/api/tree", nil, nil); code != 401 {
		t.Fatalf("tree after logout: %d", code)
	}
}

func TestNonJSONWritesRefused(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	resp, err := e.client.Post(e.srv.URL+"/api/mkdir", "application/x-www-form-urlencoded", strings.NewReader("path=x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Fatalf("form post: %d", resp.StatusCode)
	}
}

func TestSecurityHeadersAndCookie(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/login", strings.NewReader(`{"password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing CSP")
	}
	c := resp.Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", c)
	}
}

func TestFileAndCommitFlowOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	var w struct{ Hash string }
	if code := e.do(t, "PUT", "/api/file", map[string]string{"path": "flake.nix", "content": "{ }\n", "base_hash": ""}, &w); code != 200 || w.Hash == "" {
		t.Fatalf("create: %d", code)
	}
	if code := e.do(t, "PUT", "/api/file", map[string]string{"path": "flake.nix", "content": "x", "base_hash": "old"}, nil); code != 409 {
		t.Fatalf("stale save: %d", code)
	}
	if code := e.do(t, "PUT", "/api/file", map[string]string{"path": "flake.nix", "content": "{ a = 1; }\n", "base_hash": w.Hash}, nil); code != 200 {
		t.Fatalf("save: %d", code)
	}
	var f struct{ Content string }
	e.do(t, "GET", "/api/file?path=flake.nix", nil, &f)
	if f.Content != "{ a = 1; }\n" {
		t.Fatalf("read back %q", f.Content)
	}
	if code := e.do(t, "GET", "/api/file?path=../x", nil, nil); code != 400 {
		t.Fatalf("escape: %d", code)
	}

	e.do(t, "POST", "/api/stage", map[string]any{"paths": []string{}}, nil)
	if code := e.do(t, "POST", "/api/commit", map[string]any{"message": "feat: flake"}, nil); code != 412 {
		t.Fatalf("unchecked commit: %d", code)
	}
	var job struct{ ID string }
	if code := e.do(t, "POST", "/api/jobs", map[string]any{"kind": "check", "targets": []string{"nixosConfigurations.web.config.system.build.toplevel"}}, &job); code != 200 {
		t.Fatalf("submit: %d", code)
	}

	// The event stream replays the output and ends with done.
	resp, err := e.client.Get(e.srv.URL + "/api/jobs/" + job.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "event: output") || !strings.Contains(string(body), "event: done\ndata: {\"state\":\"passed\"}") {
		t.Fatalf("events:\n%s", body)
	}

	var st core.Status
	e.do(t, "GET", "/api/status", nil, &st)
	if !st.StagedChecked {
		t.Fatal("status not checked after passing job")
	}
	var c struct{ SHA string }
	if code := e.do(t, "POST", "/api/commit", map[string]any{"message": "feat: flake"}, &c); code != 200 || c.SHA == "" {
		t.Fatalf("commit: %d", code)
	}
	var log struct{ Commits []core.Commit }
	e.do(t, "GET", "/api/log?path=flake.nix", nil, &log)
	if len(log.Commits) != 1 || log.Commits[0].SHA != c.SHA {
		t.Fatalf("log %+v", log)
	}
	var info map[string]string
	e.do(t, "GET", "/api/info", nil, &info)
	if info["author"] != "Test User <test@example.invalid>" || info["branch"] != "main" {
		t.Fatalf("info %+v", info)
	}
}

func TestTokensAndMCPOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	var tok struct{ ID, Token string }
	if code := e.do(t, "POST", "/api/tokens", map[string]string{"name": "argus"}, &tok); code != 200 || tok.Token == "" {
		t.Fatalf("create token: %d", code)
	}

	connect := func(secret string) (*mcp.ClientSession, error) {
		hc := &http.Client{Transport: bearer{secret}}
		tr := &mcp.StreamableClientTransport{Endpoint: e.srv.URL + "/mcp", HTTPClient: hc}
		return mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(context.Background(), tr, nil)
	}
	if _, err := connect("wrong"); err == nil {
		t.Fatal("MCP accepted a wrong token")
	}
	cs, err := connect(tok.Token)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "write_file", Arguments: map[string]any{"path": "a.nix", "content": "1"}})
	if err != nil || res.IsError {
		t.Fatalf("write over MCP: %v %+v", err, res)
	}
	cs.Close()
	if _, _, err := e.svc.Read("a.nix"); err != nil {
		t.Fatal("MCP write not visible to the service")
	}

	if code := e.do(t, "DELETE", "/api/tokens/"+tok.ID, struct{}{}, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if _, err := connect(tok.Token); err == nil {
		t.Fatal("revoked token still works")
	}
	// A session cookie is not enough for /mcp.
	if code := e.do(t, "POST", "/mcp", map[string]any{}, nil); code != 401 {
		t.Fatalf("mcp with cookie only: %d", code)
	}
}

type bearer struct{ secret string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.secret)
	return http.DefaultTransport.RoundTrip(r)
}

func TestStaticUI(t *testing.T) {
	e := newEnv(t)
	for path, want := range map[string]string{"/": "app", "/history/abc": "app", "/assets/app.js": "js"} {
		resp, err := http.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), want) {
			t.Errorf("%s: %q", path, b)
		}
	}
}

func TestLSPBridgeAnswersConfiguration(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	t.Setenv("FAKE_NIXD", "1") // inherited by the re-executed test binary acting as nixd
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, _ := http.NewRequest("GET", e.srv.URL, nil)
	hdr := http.Header{"Cookie": {cookieHeader(e.client.Jar.Cookies(u.URL))}}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/lsp", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	c.Write(ctx, websocket.MessageText, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`))
	var gotInit, gotConfig bool
	for !gotInit || !gotConfig {
		_, msg, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		s := string(msg)
		switch {
		case strings.Contains(s, `"sawConfigCapability":true`):
			gotInit = true
		case strings.Contains(s, "test/config"):
			gotConfig = strings.Contains(s, "nixosConfigurations") && strings.Contains(s, "homeConfigurations") && strings.Contains(s, e.svc.Repo.Dir)
			if !gotConfig {
				t.Fatalf("config reply: %s", s)
			}
		case strings.Contains(s, `"id":1`):
			t.Fatalf("initialize did not advertise workspace/configuration: %s", s)
		}
	}
}

func TestLSPProcessEndsWithTheSocket(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("FAKE_NIXD", "1")
	t.Setenv("FAKE_NIXD_PIDFILE", pidFile)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, _ := http.NewRequest("GET", e.srv.URL, nil)
	hdr := http.Header{"Cookie": {cookieHeader(e.client.Jar.Cookies(u.URL))}}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/lsp", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatal(err)
	}
	c.Write(ctx, websocket.MessageText, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`))
	for { // drain until the fake has nothing more to say, like an idle nixd
		_, msg, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(msg), "test/config") {
			break
		}
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	c.Close(websocket.StatusNormalClosure, "")
	// Reaped, not just killed: signal 0 also succeeds for a zombie.
	for i := 0; ; i++ {
		if syscall.Kill(pid, 0) != nil {
			break
		}
		if i == 100 {
			t.Fatal("nixd still running after the socket closed")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLSPAcceptsForwardedHost(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	t.Setenv("FAKE_NIXD", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, _ := http.NewRequest("GET", e.srv.URL, nil)
	hdr := http.Header{"Cookie": {cookieHeader(e.client.Jar.Cookies(u.URL))}, "Origin": {"https://box.tailnet.ts.net"}, "X-Forwarded-Host": {"box.tailnet.ts.net"}}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/lsp", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("proxied origin refused: %v", err)
	}
	c.CloseNow()
	hdr.Del("X-Forwarded-Host")
	if _, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/lsp", &websocket.DialOptions{HTTPHeader: hdr}); err == nil {
		t.Fatal("foreign origin accepted")
	}
}

func TestLSPConnectionsAreCapped(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	t.Setenv("FAKE_NIXD", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, _ := http.NewRequest("GET", e.srv.URL, nil)
	hdr := http.Header{"Cookie": {cookieHeader(e.client.Jar.Cookies(u.URL))}}
	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/api/lsp"
	for i := 0; i < maxNixd; i++ {
		c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: hdr})
		if err != nil {
			t.Fatalf("connection %d: %v", i+1, err)
		}
		defer c.CloseNow()
	}
	if _, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: hdr}); err == nil || resp == nil || resp.StatusCode != 503 {
		t.Fatalf("connection over the cap: %v %v", err, resp)
	}
}

// fleetEnv serves the fixture fleet through scripts/dev/fake-nix.
func fleetEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	nix, _ := filepath.Abs("../../scripts/dev/fake-nix")
	fixtures, _ := filepath.Abs("../core/testdata/fleet")
	t.Setenv("NIXMGR_FAKE_FIXTURES", fixtures)
	e.svc.Jobs.Nix = nix
	lock, _ := os.ReadFile("../core/testdata/fleet/flake.lock")
	if _, err := e.svc.Write("flake.lock", string(lock), ""); err != nil {
		t.Fatal(err)
	}
	return e
}

// ready polls an endpoint that answers 202 while Nix works, until it answers 200.
func (e *env) ready(t *testing.T, path string, out any) {
	t.Helper()
	for i := 0; i < 200; i++ {
		var pending struct {
			State string
			Job   string
		}
		switch code := e.do(t, "GET", path, nil, &pending); code {
		case 200:
			e.do(t, "GET", path, nil, out)
			return
		case 202:
			if pending.State == "" {
				t.Fatalf("%s: 202 without a state", path)
			}
			time.Sleep(20 * time.Millisecond)
		default:
			t.Fatalf("%s: %d", path, code)
		}
	}
	t.Fatalf("%s never became ready", path)
}

func TestModelAndDiffOverHTTP(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	var first struct{ State, Job string }
	if code := e.do(t, "GET", "/api/model?at=working", nil, &first); code != 202 || first.State != "computing" || first.Job == "" {
		t.Fatalf("first model request: %d %+v", code, first)
	}
	var res struct {
		State string
		Model core.Model
	}
	e.ready(t, "/api/model?at=working", &res)
	if res.State != "ready" || len(res.Model.Hosts) != 4 {
		t.Fatalf("model %+v", res)
	}
	var diff struct {
		State string
		Hosts []core.HostDiff
	}
	e.ready(t, "/api/model/diff", &diff)
	if len(diff.Hosts) != 4 || diff.Hosts[3].Host != "workstation" || diff.Hosts[3].Status != "added" || len(diff.Hosts[3].PackagesAdded) == 0 {
		t.Fatalf("diff against an empty history %+v", diff)
	}
	var refresh struct{ Job string }
	if code := e.do(t, "POST", "/api/model/refresh", struct{}{}, &refresh); code != 200 || refresh.Job == "" {
		t.Fatalf("refresh: %d %+v", code, refresh)
	}
	if code := e.do(t, "GET", "/api/model?at=tomorrow", nil, nil); code != 400 {
		t.Fatalf("bad at: %d", code)
	}
}

func TestManagedEditsOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	var v core.ManagedView
	if code := e.do(t, "POST", "/api/hosts/web/packages", map[string]any{"add": []string{"git", "htop"}}, &v); code != 200 || len(v.Packages) != 2 {
		t.Fatalf("add packages: %d %+v", code, v)
	}
	if code := e.do(t, "POST", "/api/hosts/web/packages", map[string]any{"remove": []string{"htop"}}, &v); code != 200 || len(v.Packages) != 1 {
		t.Fatalf("remove packages: %d %+v", code, v)
	}
	if code := e.do(t, "POST", "/api/hosts/web/settings", map[string]any{"path": []string{"services", "openssh", "enable"}, "value": true}, &v); code != 200 {
		t.Fatalf("set option: %d", code)
	}
	var after core.ManagedView
	if code := e.do(t, "POST", "/api/hosts/web/settings", map[string]any{"path": []string{"services", "openssh", "enable"}, "unset": true}, &after); code != 200 || len(after.Settings) != 0 {
		t.Fatalf("unset option: %d %+v", code, after)
	}
	if code := e.do(t, "POST", "/api/hosts/web/settings", map[string]any{"path": []string{"time", "timeZone"}}, nil); code != 400 {
		t.Fatalf("set without a value: %d", code)
	}
	if code := e.do(t, "POST", "/api/hosts/web/packages", map[string]any{"add": []string{"rm -rf"}}, nil); code != 400 {
		t.Fatalf("bad package name: %d", code)
	}
	if code := e.do(t, "POST", "/api/hosts/web/settings", map[string]any{"path": []string{"time", "timeZone"}, "value": "UTC", "base_hash": "stale"}, nil); code != 409 {
		t.Fatalf("a form based on an old version: %d", code)
	}
	var got core.ManagedView
	e.do(t, "GET", "/api/hosts/web/managed", nil, &got)
	if got.Path != "hosts/web/managed.json" || !got.Exists || got.Packages[0] != "git" {
		t.Fatalf("managed view %+v", got)
	}
	var st core.Status
	e.do(t, "GET", "/api/status", nil, &st)
	if len(st.Files) != 1 || st.Files[0].Path != "hosts/web/managed.json" {
		t.Fatalf("an edit by clicking is not a normal change: %+v", st.Files)
	}
}

func TestSearchOverHTTP(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	var pk struct {
		State   string
		Results []core.PackageResult
	}
	e.ready(t, "/api/search/packages?q=fire&limit=5", &pk)
	if pk.State != "ready" || len(pk.Results) == 0 || pk.Results[0].Attr != "firefox" {
		t.Fatalf("package search %+v", pk)
	}
	var op struct {
		State   string
		Results []core.OptionDoc
	}
	e.ready(t, "/api/search/options?host=workstation&q=services.openssh.", &op)
	if len(op.Results) == 0 || op.Results[0].Name != "services.openssh.enable" {
		t.Fatalf("option search %+v", op)
	}
	e.ready(t, "/api/search/options?host=workstation&exact=1&q=time.timeZone", &op)
	if len(op.Results) != 1 || op.Results[0].Name != "time.timeZone" {
		t.Fatalf("exact option %+v", op)
	}
	if code := e.do(t, "GET", "/api/search/options?host=workstation&exact=1&q=no.such", nil, nil); code != 404 {
		t.Fatalf("missing exact option: %d", code)
	}
	var job struct{ Job string }
	if code := e.do(t, "POST", "/api/search/refresh", map[string]string{"kind": "packages"}, &job); code != 200 || job.Job == "" {
		t.Fatalf("refresh: %d %+v", code, job)
	}
	if code := e.do(t, "POST", "/api/search/refresh", map[string]string{"kind": "everything"}, nil); code != 400 {
		t.Fatalf("bad refresh kind: %d", code)
	}
}

// A last commit that does not evaluate must not make every machine look new.
func TestDiffReportsABrokenHead(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	ctx := context.Background()
	if _, err := e.svc.Write("BROKEN", "x\n", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Stage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Commit(ctx, "chore: broken", true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete("BROKEN"); err != nil {
		t.Fatal(err)
	}
	var diff struct {
		HeadError string
		Hosts     []core.HostDiff
	}
	e.ready(t, "/api/model/diff", &diff)
	if !strings.Contains(diff.HeadError, "syntax error") {
		t.Fatalf("diff against a broken commit: %+v", diff)
	}
}

func cookieHeader(cs []*http.Cookie) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}
