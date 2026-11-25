package server

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

func withDeploys(t *testing.T, e *env) *core.Deployments {
	t.Helper()
	d, err := core.OpenDeployments(t.TempDir(), e.svc.Repo)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Deploys = d
	return d
}

// report posts like a machine does: no cookie, the token as a bearer.
func report(t *testing.T, e *env, token, contentType, body string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/deployments/report", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestDeploymentReportsOverHTTP(t *testing.T) {
	e := newEnv(t)
	if code := report(t, e, "nxh_x", "application/json", `{}`); code != 401 {
		t.Fatalf("report while reports are off: %d", code)
	}
	d := withDeploys(t, e)
	tok, _ := d.HostToken("pi")
	other, _ := d.HostToken("nas")
	sha := strings.Repeat("a", 40)
	good := `{"host":"pi","commit":"` + sha + `","ref":"origin/main","status":"done","generation":"g-1"}`
	for _, c := range []struct {
		token, ct, body string
		want            int
	}{
		{"", "application/json", good, 401},
		{"nxh_wrong", "application/json", good, 401},
		{other, "application/json", good, 401}, // another machine's token
		{tok, "text/plain", good, 415},
		{tok, "application/json", `{"host":"pi","commit":"` + sha + `","status":"failed","error":"` + strings.Repeat("e", 20000) + `"}`, 413},
		{tok, "application/json", `{"host":"pi","commit":"HEAD","status":"done"}`, 400},
		{tok, "application/json", `not json`, 400},
		{tok, "application/json", good, 204},
		// The server has no closure of that system: the reply asks for it.
		{tok, "application/json", `{"host":"pi","phase":"heartbeat","current":"/nix/store/` + strings.Repeat("a", 32) + `-nixos-system-pi","silentAfter":2}`, 200},
	} {
		if code := report(t, e, c.token, c.ct, c.body); code != c.want {
			t.Errorf("token %q, %s, %.40s: %d, want %d", c.token, c.ct, c.body, code, c.want)
		}
	}

	if code := e.do(t, "GET", "/api/deployments", nil, nil); code != 401 {
		t.Fatalf("overview without a session: %d", code)
	}
	e.login(t)
	var ov core.DeployOverview
	if code := e.do(t, "GET", "/api/deployments", nil, &ov); code != 200 || !ov.Reporting || len(ov.Machines) != 1 || ov.Machines[0].Commit != sha ||
		!ov.Machines[0].Heartbeats || ov.Machines[0].System == nil || ov.Machines[0].SilentAfter != 2 || ov.SharedToken || ov.Machines[0].SharedToken {
		t.Fatalf("overview: %d %+v", code, ov)
	}
	var got struct{ Token string }
	if e.do(t, "GET", "/api/deployments/token?host=pi", nil, &got); got.Token != tok {
		t.Fatalf("token: %q", got.Token)
	}
	if code := e.do(t, "GET", "/api/deployments/token?host=../pi", nil, &got); code != 400 {
		t.Fatalf("token for an invalid name: %d", code)
	}
	var rotated struct{ Stored []string }
	if code := e.do(t, "POST", "/api/deployments/token", struct{}{}, &rotated); code != 200 || rotated.Stored == nil || len(rotated.Stored) != 0 {
		t.Fatalf("rotate: %d %+v", code, rotated) // no machine keeps its token in the secrets
	}
	if code := report(t, e, tok, "application/json", good); code != 401 {
		t.Fatalf("the old token still reports: %d", code)
	}
	e.do(t, "GET", "/api/deployments/token?host=pi", nil, &got)
	if code := report(t, e, got.Token, "application/json", good); code != 204 || got.Token == tok {
		t.Fatalf("the new token: %d", code)
	}
	if code := e.do(t, "DELETE", "/api/deployments/shared-token", struct{}{}, nil); code != 404 {
		t.Fatalf("dropping a shared token there is none of: %d", code)
	}
}

// An install from before host tokens keeps its shared token until it is dropped.
func TestSharedTokenOverHTTP(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "deploy-token"), []byte("nxd_OLD\n"), 0o600)
	d, err := core.OpenDeployments(dir, e.svc.Repo)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Deploys = d
	body := `{"host":"pi","commit":"` + strings.Repeat("a", 40) + `","status":"done"}`
	if code := report(t, e, "nxd_OLD", "application/json", body); code != 204 {
		t.Fatalf("report with the shared token: %d", code)
	}
	e.login(t)
	var ov core.DeployOverview
	if e.do(t, "GET", "/api/deployments", nil, &ov); !ov.SharedToken || len(ov.Machines) != 1 || !ov.Machines[0].SharedToken {
		t.Fatalf("overview: %+v", ov)
	}
	tok := e.createToken(t, "full", "full", nil)
	if code := e.bearer(t, tok.Token, "DELETE", "/api/deployments/shared-token", struct{}{}, nil); code != 403 {
		t.Fatalf("an API token dropped the shared token: %d", code)
	}
	if code := e.do(t, "DELETE", "/api/deployments/shared-token", struct{}{}, nil); code != 204 {
		t.Fatalf("drop: %d", code)
	}
	if code := report(t, e, "nxd_OLD", "application/json", body); code != 401 {
		t.Fatalf("the dropped token still reports: %d", code)
	}
}

// Forgetting a machine is a web UI action; API tokens cannot do it.
func TestForgetAMachineOverHTTP(t *testing.T) {
	e := newEnv(t)
	d := withDeploys(t, e)
	body := `{"host":"oldpi","commit":"` + strings.Repeat("a", 40) + `","status":"done"}`
	tok, _ := d.HostToken("oldpi")
	if code := report(t, e, tok, "application/json", body); code != 204 {
		t.Fatalf("report: %d", code)
	}
	e.login(t)
	api := e.createToken(t, "full", "full", nil)
	if code := e.bearer(t, api.Token, "DELETE", "/api/deployments/machines/oldpi", struct{}{}, nil); code != 403 {
		t.Fatalf("a token forgot a machine: %d", code)
	}
	if code := e.do(t, "DELETE", "/api/deployments/machines/oldpi", struct{}{}, nil); code != 204 {
		t.Fatalf("forget: %d", code)
	}
	if code := e.do(t, "DELETE", "/api/deployments/machines/oldpi", struct{}{}, nil); code != 404 {
		t.Fatalf("forget again: %d", code)
	}
	var ov core.DeployOverview
	if code := e.do(t, "GET", "/api/deployments", nil, &ov); code != 200 || ov.Reporting || len(ov.Machines) != 0 {
		t.Fatalf("after forgetting: %d %+v", code, ov)
	}
}

func TestDeploymentFailureIsAnInsight(t *testing.T) {
	e := fleetEnv(t)
	d := withDeploys(t, e)
	body := `{"host":"pi","commit":"` + strings.Repeat("b", 40) + `","phase":"build","status":"failed","error":"error: attribute 'foo' missing"}`
	tok, _ := d.HostToken("pi")
	if code := report(t, e, tok, "application/json", body); code != 204 {
		t.Fatalf("report: %d", code)
	}
	e.login(t)
	var res struct{ Insights []core.Insight }
	e.ready(t, "/api/insights", &res)
	for _, in := range res.Insights {
		if in.ID == "deploy-failed:pi" && in.Severity == "error" && strings.Contains(in.Detail, "attribute 'foo' missing") {
			return
		}
	}
	t.Fatalf("no deploy-failed insight for pi: %+v", res.Insights)
}

// machinePost posts like a machine does and returns the status and the reply.
func machinePost(t *testing.T, e *env, token, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// A report's reply asks for the closure the server lacks; the machine posts it
// with its own token, and the web UI and API tokens read it.
func TestClosureOverHTTP(t *testing.T) {
	e := newEnv(t)
	d := withDeploys(t, e)
	tok, _ := d.HostToken("pi")
	other, _ := d.HostToken("nas")
	sys := "/nix/store/" + strings.Repeat("a", 32) + "-nixos-system-pi"
	next := "/nix/store/" + strings.Repeat("c", 32) + "-nixos-system-pi+x"
	glibc := "/nix/store/" + strings.Repeat("b", 32) + "-glibc-2.40-66"
	beat := func(current string) (int, string) {
		return machinePost(t, e, tok, "/api/deployments/report", `{"host":"pi","phase":"heartbeat","current":"`+current+`","sendsClosure":true}`)
	}
	closure := func(token, system, body string) int {
		code, _ := machinePost(t, e, token, "/api/deployments/closure?host=pi&system="+url.QueryEscape(system), body)
		return code
	}
	good := `{"paths":[{"path":"` + sys + `","narSize":10,"refs":[1]},{"path":"` + glibc + `","narSize":20}]}`

	if code := closure(tok, sys, good); code != 400 {
		t.Fatalf("a closure before any report: %d", code)
	}
	if code, reply := beat(sys); code != 200 || !strings.Contains(reply, `"closure":"wanted"`) {
		t.Fatalf("heartbeat: %d %s", code, reply)
	}
	for _, c := range []struct {
		token, system, body string
		want                int
	}{
		{"", sys, good, 401},
		{"nxh_wrong", sys, good, 401},
		{other, sys, good, 401}, // another machine's token
		{tok, "/etc", good, 400},
		{tok, sys, `not json`, 400},
	} {
		if code := closure(c.token, c.system, c.body); code != c.want {
			t.Errorf("token %q, system %s, %.30s: %d, want %d", c.token, c.system, c.body, code, c.want)
		}
	}
	if code := closure(tok, sys, good); code != 204 {
		t.Fatalf("closure: %d", code)
	}
	if code, reply := beat(sys); code != 204 {
		t.Fatalf("heartbeat after the closure: %d %s", code, reply)
	}
	// A new system: asked again. The + in its name survives the query.
	if code, _ := beat(next); code != 200 {
		t.Fatalf("heartbeat with a new system: %d", code)
	}
	if code := closure(tok, next, `{"paths":[{"path":"`+next+`","refs":[3]}]}`); code != 400 {
		t.Fatalf("a bad closure: %d", code)
	}
	if code, _ := beat(next); code != 204 {
		t.Fatalf("a refused closure is not asked again: %d", code)
	}
	limit := core.MaxClosureBody
	core.MaxClosureBody = 64
	if code := closure(tok, sys, good); code != 413 {
		t.Fatalf("too large: %d", code)
	}
	core.MaxClosureBody = limit
	if code, _ := beat(sys); code != 204 {
		t.Fatalf("a closure too large is not asked again: %d", code)
	}

	if code := e.do(t, "GET", "/api/hosts/pi/dependencies", nil, nil); code != 401 {
		t.Fatalf("dependencies without a session: %d", code)
	}
	e.login(t)
	var md core.MachineDeps
	if code := e.do(t, "GET", "/api/hosts/pi/dependencies", nil, &md); code != 200 || md.State != "refused" || !strings.Contains(md.Message, "64 bytes") {
		t.Fatalf("pi after the refusal: %d %+v", code, md)
	}
	beat(next)
	if code := closure(tok, next, `{"paths":[{"path":"`+next+`","narSize":5,"refs":[1]},{"path":"`+glibc+`","narSize":20}]}`); code != 204 {
		t.Fatalf("closure of the new system: %d", code)
	}
	read := e.createToken(t, "read", "read", nil)
	if code := e.bearer(t, read.Token, "GET", "/api/hosts/pi/dependencies", nil, &md); code != 200 || md.State != "ready" || len(md.Nodes) != 2 || md.Size != 25 {
		t.Fatalf("pi with a read token: %d %+v", code, md)
	}
	var f core.FleetDeps
	if code := e.bearer(t, read.Token, "GET", "/api/dependencies", nil, &f); code != 200 || len(f.Hosts) != 1 || len(f.Rows) != 1 || f.Rows[0].Name != "glibc" {
		t.Fatalf("fleet: %d %+v", code, f)
	}
	if code := e.do(t, "GET", "/api/hosts/..%2Fpi/dependencies", nil, nil); code != 400 && code != 404 {
		t.Fatalf("an invalid host: %d", code)
	}
}

// The shared fleet token cannot post closures: with it, any machine could
// write another's.
func TestClosureRefusesTheSharedToken(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "deploy-token"), []byte("nxd_OLD\n"), 0o600)
	d, err := core.OpenDeployments(dir, e.svc.Repo)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.Deploys = d
	sys := "/nix/store/" + strings.Repeat("a", 32) + "-nixos-system-pi"
	if code, reply := machinePost(t, e, "nxd_OLD", "/api/deployments/report", `{"host":"pi","phase":"heartbeat","current":"`+sys+`","sendsClosure":true}`); code != 204 {
		t.Fatalf("the shared token is never asked for a closure: %d %s", code, reply)
	}
	code, _ := machinePost(t, e, "nxd_OLD", "/api/deployments/closure?host=pi&system="+url.QueryEscape(sys), `{"paths":[{"path":"`+sys+`"}]}`)
	if code != 403 {
		t.Fatalf("closure with the shared token: %d", code)
	}
}
