package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type createdToken struct {
	ID, Name, Token, Scope string
	Expires                string
}

func (e *env) createToken(t *testing.T, name, scope string, expires *time.Time) createdToken {
	t.Helper()
	in := map[string]any{"name": name, "scope": scope}
	if expires != nil {
		in["expires"] = expires.Format(time.RFC3339)
	}
	var tok createdToken
	if code := e.do(t, "POST", "/api/tokens", in, &tok); code != 200 || tok.Token == "" || tok.Scope != scope {
		t.Fatalf("create %s token: %d %+v", scope, code, tok)
	}
	return tok
}

// bearer sends a request with an API token and no cookies.
func (e *env) bearer(t *testing.T, secret, method, path string, body any, out any) int {
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
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func lastVia(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%(trailers:key=Via,valueonly)").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func (e *env) passCheck(t *testing.T, secret string) {
	t.Helper()
	var job struct{ ID string }
	if code := e.bearer(t, secret, "POST", "/api/jobs", map[string]any{"kind": "check"}, &job); code != 200 {
		t.Fatalf("check: %d", code)
	}
	j, _ := e.svc.Jobs.Get(job.ID)
	select {
	case <-j.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("check did not finish")
	}
}

func TestTokenScopesOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	read := e.createToken(t, "status page", "read", nil)
	edit := e.createToken(t, "agent", "edit", nil)
	full := e.createToken(t, "deployer", "full", nil)

	for _, c := range []struct {
		tok          createdToken
		method, path string
		body         any
		want         int
	}{
		{read, "GET", "/api/tree", nil, 200},
		{read, "GET", "/api/status", nil, 200},
		{read, "GET", "/api/nope", nil, 404},
		{read, "DELETE", "/api/file", map[string]any{}, 404}, // a known path with another method: as for the web UI
		{read, "PUT", "/api/file", map[string]string{"path": "r.nix", "content": "1"}, 403},
		{read, "POST", "/api/stage", map[string]any{}, 403},
		{edit, "POST", "/api/push", map[string]any{}, 403},
		{edit, "POST", "/api/try", map[string]string{"host": "pi"}, 403}, // a trial pushes
		{edit, "POST", "/api/try/end", map[string]string{"host": "pi"}, 403},
		{full, "POST", "/api/push", map[string]any{}, 400},                         // allowed; there is no remote
		{edit, "POST", "/api/hostkey", map[string]string{"address": "nas;x"}, 403}, // connects out from the server
		{full, "POST", "/api/hostkey", map[string]string{"address": "nas;x"}, 400},
		// Only the web UI login reaches these, whatever the token's scope.
		{full, "GET", "/api/tokens", nil, 403},
		{full, "POST", "/api/tokens", map[string]string{"name": "more", "scope": "full"}, 403},
		{full, "DELETE", "/api/tokens/" + read.ID, map[string]any{}, 403},
		{full, "GET", "/api/deployments/token", nil, 403},
		{full, "GET", "/api/lsp", nil, 403},
		{createdToken{Token: "nxm_wrong", Scope: "wrong"}, "GET", "/api/tree", nil, 401},
	} {
		if code := e.bearer(t, c.tok.Token, c.method, c.path, c.body, nil); code != c.want {
			t.Errorf("%s token: %s %s = %d, want %d", c.tok.Scope, c.method, c.path, code, c.want)
		}
	}

	// An edit token changes, checks and commits, never without a check.
	if code := e.bearer(t, edit.Token, "PUT", "/api/file", map[string]string{"path": "a.nix", "content": "1\n"}, nil); code != 200 {
		t.Fatalf("edit token write: %d", code)
	}
	if code := e.bearer(t, edit.Token, "POST", "/api/stage", map[string]any{}, nil); code != 204 {
		t.Fatalf("edit token stage: %d", code)
	}
	var er struct{ Error string }
	if code := e.bearer(t, edit.Token, "POST", "/api/commit", map[string]any{"message": "x", "force": true}, &er); code != 403 || !strings.Contains(er.Error, "web UI") {
		t.Fatalf("token skipped the check: %d %q", code, er.Error)
	}
	e.passCheck(t, edit.Token)
	if code := e.bearer(t, edit.Token, "POST", "/api/commit", map[string]any{"message": "feat: a"}, nil); code != 200 {
		t.Fatalf("edit token commit: %d", code)
	}
	if via := lastVia(t, e.svc.Repo.Dir); via != "token agent ("+edit.ID+")" {
		t.Fatalf("Via %q", via)
	}

	// The web UI's commits name the session, never its secret id.
	e.svc.Write("b.nix", "2\n", "")
	e.do(t, "POST", "/api/stage", map[string]any{}, nil)
	if code := e.do(t, "POST", "/api/commit", map[string]any{"message": "feat: b", "force": true}, nil); code != 200 {
		t.Fatalf("UI commit: %d", code)
	}
	if via := lastVia(t, e.svc.Repo.Dir); !regexp.MustCompile(`^web session [0-9a-f]{8}$`).MatchString(via) {
		t.Fatalf("UI Via %q", via)
	}

	// A token header wins over a session cookie: a wrong one is refused.
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/tree", nil)
	req.Header.Set("Authorization", "Bearer nxm_wrong")
	if resp, err := e.client.Do(req); err != nil || resp.StatusCode != 401 {
		t.Fatalf("wrong token with a session: %v %v", resp.StatusCode, err)
	}
}

func TestTokenExpiryOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	soon := time.Now().Add(2 * time.Second)
	tok := e.createToken(t, "short", "read", &soon)
	if tok.Expires == "" {
		t.Fatal("no expiry in the answer")
	}
	if code := e.bearer(t, tok.Token, "GET", "/api/tree", nil, nil); code != 200 {
		t.Fatalf("before expiry: %d", code)
	}
	past := time.Now().Add(-time.Hour)
	for _, in := range []map[string]any{
		{"name": "late", "scope": "read", "expires": past.Format(time.RFC3339)},
		{"name": "x", "scope": "admin"},
		{"name": "x"},
	} {
		if code := e.do(t, "POST", "/api/tokens", in, nil); code != 400 {
			t.Errorf("created %v: %d", in, code)
		}
	}
	var list struct {
		Tokens []struct{ ID, Scope, Expires string }
	}
	e.do(t, "GET", "/api/tokens", nil, &list)
	if len(list.Tokens) != 1 || list.Tokens[0].Scope != "read" || list.Tokens[0].Expires == "" {
		t.Fatalf("list %+v", list)
	}
	time.Sleep(time.Until(soon) + 100*time.Millisecond)
	if code := e.bearer(t, tok.Token, "GET", "/api/tree", nil, nil); code != 401 {
		t.Fatalf("after expiry: %d", code)
	}
}

// Every route the token table names exists, so a typo cannot silently lock a route.
func TestTokenRoutesAreRegistered(t *testing.T) {
	e := newEnv(t)
	api := e.s.apiMux()
	for pattern := range tokenRoutes {
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			method, path = "GET", pattern+"anything"
		}
		path = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "x")
		req, _ := http.NewRequest(method, path, nil)
		if _, got := api.Handler(req); got != pattern {
			t.Errorf("%s matches %q", pattern, got)
		}
	}
}

func TestMCPToolsFollowTheTokenScope(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	connect := func(secret string) *mcp.ClientSession {
		hc := &http.Client{Transport: bearer{secret}}
		tr := &mcp.StreamableClientTransport{Endpoint: e.srv.URL + "/mcp", HTTPClient: hc}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(context.Background(), tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	tools := func(cs *mcp.ClientSession) map[string]bool {
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, tl := range res.Tools {
			m[tl.Name] = true
		}
		return m
	}
	read := tools(connect(e.createToken(t, "ro", "read", nil).Token))
	edit := tools(connect(e.createToken(t, "ed", "edit", nil).Token))
	full := tools(connect(e.createToken(t, "fu", "full", nil).Token))
	if !read["read_file"] || !read["deployments"] || read["write_file"] || read["run_check"] || read["commit"] || read["push"] {
		t.Errorf("read-only tools %v", read)
	}
	if !edit["write_file"] || !edit["commit"] || !edit["pull"] || edit["push"] {
		t.Errorf("edit tools %v", edit)
	}
	if !full["push"] || len(full) <= len(edit) || len(edit) <= len(read) {
		t.Errorf("full tools %v", full)
	}

	// A call to a tool outside the scope fails like an unknown tool.
	cs := connect(e.createToken(t, "ro2", "read", nil).Token)
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "write_file", Arguments: map[string]any{"path": "a.nix", "content": "1"}}); err == nil && !res.IsError {
		t.Fatal("a read-only token wrote a file")
	}
	if _, _, err := e.svc.Read("a.nix"); err == nil {
		t.Fatal("the file exists")
	}
}

// The SDK binds an MCP session to the token that opened it; another token
// cannot continue it, whatever its scope.
func TestMCPSessionStaysWithItsToken(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	a := e.createToken(t, "a", "read", nil)
	b := e.createToken(t, "b", "full", nil)
	post := func(secret, session, body string) *http.Response {
		req, _ := http.NewRequest("POST", e.srv.URL+"/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+secret)
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp
	}
	init := post(a.Token, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	session := init.Header.Get("Mcp-Session-Id")
	if init.StatusCode != 200 || session == "" {
		t.Fatalf("initialize: %d, session %q", init.StatusCode, session)
	}
	list := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	if code := post(b.Token, session, list).StatusCode; code != 403 {
		t.Fatalf("another token continued the session: %d", code)
	}
	if code := post(a.Token, session, list).StatusCode; code == 403 || code == 401 {
		t.Fatalf("the session's own token was refused: %d", code)
	}
}
