package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
	"github.com/TheOlympusCloud/nixos-manager/internal/fakesops"
)

// Settings → Secrets over HTTP: a machine's key, a secret in and back out,
// never cached, and the report tokens stored as the machines' secrets.
func TestSecretsOverHTTP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	var st core.SecretsStatus
	if code := e.do(t, "GET", "/api/secrets", nil, &st); code != 200 || !strings.HasPrefix(st.Recipient, "age1") || st.KeyFile == "" {
		t.Fatalf("status: %d %+v", code, st)
	}
	pub, _ := fakesops.HostKey(t)
	var m core.SecretMachine
	if code := e.do(t, "PUT", "/api/secrets/machines/web", map[string]string{"key": pub}, &m); code != 200 || m.Name != "web" || m.Recipient == "" {
		t.Fatalf("machine key: %d %+v", code, m)
	}
	if code := e.do(t, "PUT", "/api/secrets/machines/web", map[string]string{"key": "ssh-rsa AAAA"}, nil); code != 400 {
		t.Fatalf("an RSA key: %d", code)
	}
	if code := e.do(t, "PUT", "/api/secrets", map[string]string{"host": "web", "name": "api-key", "value": "s3cret value"}, nil); code != 204 {
		t.Fatalf("set: %d", code)
	}
	if code := e.do(t, "PUT", "/api/secrets", map[string]string{"host": "db", "name": "x", "value": "y"}, nil); code != 400 {
		t.Fatalf("set for a machine without a key: %d", code)
	}

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/secrets/reveal", strings.NewReader(`{"host":"web","name":"api-key"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Value string }
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 || out.Value != "s3cret value" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("reveal: %d %q %q", resp.StatusCode, out.Value, resp.Header.Get("Cache-Control"))
	}
	if code := e.do(t, "POST", "/api/secrets/reveal", map[string]string{"host": "web", "name": "nope"}, nil); code != 404 {
		t.Fatalf("reveal a missing secret: %d", code)
	}

	// Deployment reports are off on this server.
	if code := e.do(t, "POST", "/api/secrets/report-tokens", struct{}{}, nil); code != 400 {
		t.Fatalf("report tokens without reports: %d", code)
	}
	if e.svc.Deploys, err = core.OpenDeployments(t.TempDir(), e.svc.Repo); err != nil {
		t.Fatal(err)
	}
	var hosts struct{ Hosts []string }
	if code := e.do(t, "POST", "/api/secrets/report-tokens", struct{}{}, &hosts); code != 200 || len(hosts.Hosts) != 1 {
		t.Fatalf("report tokens: %d %+v", code, hosts)
	}

	if code := e.do(t, "POST", "/api/secrets/delete", map[string]string{"host": "web", "name": "api-key"}, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := e.do(t, "POST", "/api/secrets/reencrypt", struct{}{}, nil); code != 204 {
		t.Fatalf("re-encrypt: %d", code)
	}
	if code := e.do(t, "DELETE", "/api/secrets/machines/web", struct{}{}, nil); code != 204 {
		t.Fatalf("remove key: %d", code)
	}
	if code := e.do(t, "GET", "/api/secrets", nil, &st); code != 200 || len(st.Machines) != 0 {
		t.Fatalf("after removing: %d %+v", code, st)
	}
}

// Any token sees the names; values go in with edit and come out with full
// access only; storing the report tokens needs the web login.
func TestSecretsFollowTheTokenScope(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	pub, _ := fakesops.HostKey(t)
	if code := e.do(t, "PUT", "/api/secrets/machines/web", map[string]string{"key": pub}, nil); code != 200 {
		t.Fatalf("machine key: %d", code)
	}
	read, edit, full := e.createToken(t, "ro", "read", nil).Token, e.createToken(t, "ed", "edit", nil).Token, e.createToken(t, "fu", "full", nil).Token
	set := map[string]string{"name": "wifi", "value": "pw"}
	reveal := map[string]string{"name": "wifi"}
	for _, c := range []struct {
		secret, method, path string
		body                 any
		want                 int
	}{
		{read, "GET", "/api/secrets", nil, 200},
		{read, "PUT", "/api/secrets", set, 403},
		{read, "PUT", "/api/secrets/machines/pi", map[string]string{"key": pub}, 403},
		{edit, "PUT", "/api/secrets/machines/pi", map[string]string{"key": pub}, 403},
		{edit, "DELETE", "/api/secrets/machines/web", struct{}{}, 403},
		{edit, "PUT", "/api/secrets", set, 204},
		{read, "POST", "/api/secrets/reveal", reveal, 403},
		{edit, "POST", "/api/secrets/reveal", reveal, 403},
		{full, "POST", "/api/secrets/reveal", reveal, 200},
		{full, "POST", "/api/secrets/report-tokens", struct{}{}, 403},
		{read, "POST", "/api/secrets/delete", reveal, 403},
		{edit, "POST", "/api/secrets/delete", reveal, 204},
		{full, "PUT", "/api/secrets/machines/pi", map[string]string{"key": pub}, 200},
	} {
		if code := e.bearer(t, c.secret, c.method, c.path, c.body, nil); code != c.want {
			t.Errorf("%s %s: %d, want %d", c.method, c.path, code, c.want)
		}
	}

	// An edit token that writes its own key into the working copy and asks for
	// a re-encryption gets nothing encrypted for it.
	if code := e.bearer(t, edit, "PUT", "/api/secrets", set, nil); code != 204 {
		t.Fatalf("set: %d", code)
	}
	evil, _ := fakesops.HostKey(t)
	for _, p := range []string{"secrets/hosts/evil.pub", "secrets/hosts/web.pub"} {
		var f struct{ Hash string }
		e.bearer(t, edit, "GET", "/api/file?path="+p, nil, &f)
		if code := e.bearer(t, edit, "PUT", "/api/file", map[string]string{"path": p, "content": evil + "\n", "base_hash": f.Hash}, nil); code != 200 {
			t.Fatalf("plant %s: %d", p, code)
		}
	}
	if code := e.bearer(t, edit, "POST", "/api/secrets/reencrypt", struct{}{}, nil); code != 204 {
		t.Fatalf("re-encrypt: %d", code)
	}
	var f struct{ Content string }
	e.bearer(t, edit, "GET", "/api/file?path=secrets/common.json", nil, &f)
	cmd := exec.Command(e.svc.Secrets.SSHToAge)
	cmd.Stdin = strings.NewReader(evil + "\n")
	evilAge, err := cmd.Output()
	if err != nil || !strings.HasPrefix(string(evilAge), "age1") {
		t.Fatalf("ssh-to-age: %v %q", err, evilAge)
	}
	if !strings.Contains(f.Content, `"recipient"`) || strings.Contains(f.Content, strings.TrimSpace(string(evilAge))) {
		t.Fatalf("common.json is encrypted for a key the edit token wrote:\n%s", f.Content)
	}
}

// Over MCP a read-only token lists the secrets, an edit token sets them, and
// no tool shows a value.
func TestSecretsOverMCP(t *testing.T) {
	e := newEnv(t)
	e.login(t)
	connect := func(secret string) *mcp.ClientSession {
		tr := &mcp.StreamableClientTransport{Endpoint: e.srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer{secret}}}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(context.Background(), tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	call := func(cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
		return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	}
	ro := connect(e.createToken(t, "ro", "read", nil).Token)
	if res, err := call(ro, "list_secrets", map[string]any{}); err != nil || res.IsError {
		t.Fatalf("list_secrets: %v %+v", err, res)
	}
	if res, err := call(ro, "set_secret", map[string]any{"name": "wifi", "value": "pw"}); err == nil && !res.IsError {
		t.Fatal("a read-only token set a secret")
	}
	ed := connect(e.createToken(t, "ed", "edit", nil).Token)
	if res, err := call(ed, "set_secret", map[string]any{"name": "wifi", "value": "pw"}); err != nil || res.IsError {
		t.Fatalf("set_secret: %v %+v", err, res)
	}
	if v, err := e.svc.Secrets.Reveal(context.Background(), "", "wifi"); err != nil || v != "pw" {
		t.Fatalf("stored %q %v", v, err)
	}
	full := connect(e.createToken(t, "fu", "full", nil).Token)
	res, err := full.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range res.Tools {
		if strings.Contains(tl.Name, "reveal") || tl.Name == "get_secret" {
			t.Errorf("%s shows values over MCP", tl.Name)
		}
	}
	if res, err := call(ed, "delete_secret", map[string]any{"name": "wifi"}); err != nil || res.IsError {
		t.Fatalf("delete_secret: %v %+v", err, res)
	}
}
