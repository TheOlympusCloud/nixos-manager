package server

import (
	"net/http"
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
	if code := report(t, e, "nxd_x", "application/json", `{}`); code != 401 {
		t.Fatalf("report while reports are off: %d", code)
	}
	d := withDeploys(t, e)
	tok := d.Token()
	sha := strings.Repeat("a", 40)
	good := `{"host":"pi","commit":"` + sha + `","ref":"origin/main","status":"done","generation":"g-1"}`
	for _, c := range []struct {
		token, ct, body string
		want            int
	}{
		{"", "application/json", good, 401},
		{"nxd_wrong", "application/json", good, 401},
		{tok, "text/plain", good, 415},
		{tok, "application/json", `{"host":"pi","commit":"` + sha + `","status":"failed","error":"` + strings.Repeat("e", 20000) + `"}`, 413},
		{tok, "application/json", `{"host":"pi","commit":"HEAD","status":"done"}`, 400},
		{tok, "application/json", `not json`, 400},
		{tok, "application/json", good, 204},
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
	if code := e.do(t, "GET", "/api/deployments", nil, &ov); code != 200 || !ov.Reporting || len(ov.Machines) != 1 || ov.Machines[0].Commit != sha {
		t.Fatalf("overview: %d %+v", code, ov)
	}
	var got struct{ Token string }
	if e.do(t, "GET", "/api/deployments/token", nil, &got); got.Token != tok {
		t.Fatalf("token: %q", got.Token)
	}
	if code := e.do(t, "POST", "/api/deployments/token", struct{}{}, &got); code != 200 || got.Token == tok || got.Token == "" {
		t.Fatalf("regenerate: %d %q", code, got.Token)
	}
	if code := report(t, e, tok, "application/json", good); code != 401 {
		t.Fatalf("the old token still reports: %d", code)
	}
	if code := report(t, e, got.Token, "application/json", good); code != 204 {
		t.Fatalf("the new token: %d", code)
	}
}

// Forgetting a machine is a web UI action; API tokens cannot do it.
func TestForgetAMachineOverHTTP(t *testing.T) {
	e := newEnv(t)
	d := withDeploys(t, e)
	body := `{"host":"oldpi","commit":"` + strings.Repeat("a", 40) + `","status":"done"}`
	if code := report(t, e, d.Token(), "application/json", body); code != 204 {
		t.Fatalf("report: %d", code)
	}
	e.login(t)
	tok := e.createToken(t, "full", "full", nil)
	if code := e.bearer(t, tok.Token, "DELETE", "/api/deployments/machines/oldpi", struct{}{}, nil); code != 403 {
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
	if code := report(t, e, d.Token(), "application/json", body); code != 204 {
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
