package templates

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// reporter writes the command report-deployments.nix builds: writeShellApplication's
// strict mode, the three settings, then the script from the starter fleet.
func reporter(t *testing.T, url, tokenFile string) string {
	t.Helper()
	for _, tool := range []string{"bash", "curl", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("needs %s", tool)
		}
	}
	body, err := fs.ReadFile(Fleet, "modules/report-deployment.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/usr/bin/env bash\nset -o errexit\nset -o nounset\nset -o pipefail\n" +
		"url='" + url + "'\ntoken_file='" + tokenFile + "'\nhost='pi'\n" + string(body)
	p := filepath.Join(t.TempDir(), "report-deployment")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

type seen struct {
	auth, contentType string
	body              map[string]string
}

func TestReportDeploymentScript(t *testing.T) {
	got := make(chan seen, 10)
	var code atomic.Int32 // the status the fake manager answers with
	code.Store(http.StatusNoContent)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/deployments/report" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var s seen
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &s.body); err != nil {
			t.Errorf("body %s: %v", b, err)
		}
		s.auth, s.contentType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		got <- s
		w.WriteHeader(int(code.Load()))
	}))
	defer srv.Close()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("nxd_secret\n"), 0o600)
	script := reporter(t, srv.URL+"/", tokenFile)
	sha := strings.Repeat("a", 40)

	run := func(env ...string) string {
		t.Helper()
		cmd := exec.Command(script)
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v exited with %v: %s", env, err, out)
		}
		return string(out)
	}
	none := func(what string) {
		t.Helper()
		select {
		case s := <-got:
			t.Errorf("%s: reported %+v", what, s)
		default:
		}
	}

	run("COMIN_GIT_SHA="+sha, "COMIN_STATUS=failed", "COMIN_GIT_REF=origin/main", "COMIN_GENERATION=5f1c",
		"COMIN_ERROR_MSG=error: \"quoted\"\nsecond line")
	s := <-got
	want := map[string]string{"host": "pi", "commit": sha, "phase": "deploy", "status": "failed", "ref": "origin/main",
		"generation": "5f1c", "error": "error: \"quoted\"\nsecond line"}
	if s.auth != "Bearer nxd_secret" || s.contentType != "application/json" || len(s.body) != len(want) {
		t.Fatalf("request: %+v", s)
	}
	for k, v := range want {
		if s.body[k] != v {
			t.Errorf("%s = %q, want %q", k, s.body[k], v)
		}
	}

	run("COMIN_PHASE=build", "COMIN_STATUS=already built", "COMIN_GIT_SHA="+sha)
	if s := <-got; s.body["phase"] != "build" || s.body["status"] != "already built" || s.body["error"] != "" {
		t.Errorf("build report: %+v", s.body)
	}

	run("COMIN_STATUS=running", "COMIN_GIT_SHA="+sha)
	none("a running deployment")
	run("COMIN_PHASE=build", "COMIN_STATUS=done", "COMIN_GIT_SHA="+sha)
	none("an unknown build status")
	run("COMIN_STATUS=done", "COMIN_GIT_SHA=HEAD")
	none("a commit that is not a SHA")
	run() // comin without any variable
	none("no variables")

	code.Store(http.StatusUnauthorized)
	if out := run("COMIN_STATUS=done", "COMIN_GIT_SHA="+sha); !strings.Contains(out, "could not report") {
		t.Errorf("a refused report says nothing: %q", out)
	}
	<-got

	os.Remove(tokenFile)
	run("COMIN_STATUS=done", "COMIN_GIT_SHA="+sha)
	none("no token file")
	srv.Close()
	os.WriteFile(tokenFile, []byte("nxd_secret"), 0o600)
	if out := run("COMIN_STATUS=done", "COMIN_GIT_SHA="+sha); !strings.Contains(out, "could not report") {
		t.Errorf("an unreachable manager says nothing: %q", out)
	}
}

func TestReportDeploymentScriptPassesShellcheck(t *testing.T) {
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("needs shellcheck")
	}
	if out, err := exec.Command("shellcheck", reporter(t, "http://manager", "/run/token")).CombinedOutput(); err != nil {
		t.Fatalf("shellcheck: %v\n%s", err, out)
	}
}

// The module turns reporting on with a URL and hooks both of comin's commands.
func TestReportModuleWiresComin(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	nix := []string{"--extra-experimental-features", "nix-command flakes"}
	out, err := exec.Command("nix", append(nix, "eval", "--impure", "--json", "--file", "testdata/report-module.nix", "--apply", "x: removeAttrs x [\"script\"]")...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nix eval: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["deploy"] != true || got["off"] != true || (got["build"] != true && got["build"] != "absent") {
		t.Fatalf("wiring: %v", got)
	}
	if out, err := exec.Command("nix", append(nix, "build", "--impure", "--no-link", "--file", "testdata/report-module.nix", "script")...).CombinedOutput(); err != nil {
		t.Fatalf("building the script: %v\n%s", err, out)
	}
}
