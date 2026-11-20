package templates

import (
	"encoding/json"
	"fmt"
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
// strict mode, the settings, then the script from the starter fleet. root
// stands for / when the script looks at what the machine runs.
func reporter(t *testing.T, url, tokenFile, stateDir, root string) string {
	t.Helper()
	for _, tool := range []string{"bash", "curl", "jq", "flock"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("needs %s", tool)
		}
	}
	body, err := fs.ReadFile(Fleet, "modules/report-deployment.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/usr/bin/env bash\nset -o errexit\nset -o nounset\nset -o pipefail\n" +
		"url='" + url + "'\ntoken_file='" + tokenFile + "'\nhost='pi'\nstate_dir='" + stateDir + "'\nsilent_after=2\nroot='" + root + "'\n" + string(body)
	p := filepath.Join(t.TempDir(), "report-deployment")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

type seen struct {
	auth, contentType string
	body              map[string]any
}

// fakeManager answers reports with the status in code and passes each on.
func fakeManager(t *testing.T) (*httptest.Server, chan seen, *atomic.Int32) {
	got := make(chan seen, 10)
	var code atomic.Int32
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
	t.Cleanup(srv.Close)
	return srv, got, &code
}

// fakeSystem lays out what a NixOS machine has under root: the running and the
// booted system with a kernel each, and the system profile.
func fakeSystem(t *testing.T, root string, current, booted string) {
	t.Helper()
	store := filepath.Join(root, "nix", "store")
	for _, sys := range []string{current, booted} {
		os.MkdirAll(filepath.Join(store, sys), 0o755)
		os.MkdirAll(filepath.Join(store, "k-"+sys), 0o755)
		os.Remove(filepath.Join(store, sys, "kernel"))
		if err := os.Symlink(filepath.Join(store, "k-"+sys), filepath.Join(store, sys, "kernel")); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(root, "run"), 0o755)
	os.MkdirAll(filepath.Join(root, "nix", "var", "nix", "profiles"), 0o755)
	for link, to := range map[string]string{"run/current-system": current, "run/booted-system": booted, "nix/var/nix/profiles/system": current} {
		os.Remove(filepath.Join(root, link))
		if err := os.Symlink(filepath.Join(store, to), filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReportDeploymentScript(t *testing.T) {
	srv, got, code := fakeManager(t)
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("nxh_secret\n"), 0o600)
	root := filepath.Join(dir, "root")
	sys := strings.Repeat("a", 32) + "-nixos-system-pi-26.05"
	fakeSystem(t, root, sys, sys)
	script := reporter(t, srv.URL+"/", tokenFile, filepath.Join(dir, "state"), root)
	sha := strings.Repeat("a", 40)

	run := func(env ...string) string {
		t.Helper()
		cmd := exec.Command(script)
		if len(env) == 1 && env[0] == "heartbeat" { // what the timer runs
			cmd, env = exec.Command(script, "heartbeat"), nil
		}
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
	want := map[string]any{"host": "pi", "commit": sha, "phase": "deploy", "status": "failed", "ref": "origin/main",
		"generation": "5f1c", "error": "error: \"quoted\"\nsecond line",
		"current": "/nix/store/" + sys, "booted": "/nix/store/" + sys, "profile": "/nix/store/" + sys, "reboot": false}
	if s.auth != "Bearer nxh_secret" || s.contentType != "application/json" || len(s.body) != len(want) {
		t.Fatalf("request: %+v", s)
	}
	for k, v := range want {
		if s.body[k] != v {
			t.Errorf("%s = %#v, want %#v", k, s.body[k], v)
		}
	}

	run("COMIN_PHASE=build", "COMIN_STATUS=already built", "COMIN_GIT_SHA="+sha)
	if s := <-got; s.body["phase"] != "build" || s.body["status"] != "already built" || s.body["error"] != "" {
		t.Errorf("build report: %+v", s.body)
	}

	// The new system has another kernel than the booted one: a reboot is needed.
	next := strings.Repeat("b", 32) + "-nixos-system-pi-26.05"
	fakeSystem(t, root, next, sys)
	run("heartbeat")
	if s := <-got; s.body["phase"] != "heartbeat" || s.body["current"] != "/nix/store/"+next || s.body["booted"] != "/nix/store/"+sys ||
		s.body["reboot"] != true || s.body["silentAfter"] != float64(2) || s.body["commit"] != nil || len(s.body) != 7 {
		t.Errorf("heartbeat: %+v", s.body)
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
	run("heartbeat")
	none("no token file")
	srv.Close()
	os.WriteFile(tokenFile, []byte("nxh_secret"), 0o600)
	if out := run("COMIN_STATUS=done", "COMIN_GIT_SHA="+sha); !strings.Contains(out, "could not report") {
		t.Errorf("an unreachable manager says nothing: %q", out)
	}
	if out := run("heartbeat"); !strings.Contains(out, "could not report") {
		t.Errorf("an unreachable manager says nothing to a heartbeat: %q", out)
	}
}

// Reports that cannot be delivered wait, in order, and every later run sends
// them first: the next heartbeat, or the next comin report. A heartbeat sent
// while one still waits says nothing about the system, and a report the
// manager refuses is dropped.
func TestUndeliveredReportsWaitInOrder(t *testing.T) {
	srv, got, code := fakeManager(t)
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("nxh_secret"), 0o600)
	state, root := filepath.Join(dir, "state"), filepath.Join(dir, "root")
	sys := strings.Repeat("a", 32) + "-nixos-system-pi-26.05"
	fakeSystem(t, root, sys, sys)
	script := reporter(t, srv.URL, tokenFile, state, root)
	pending := filepath.Join(state, "pending.jsonl")
	run := func(env ...string) string {
		t.Helper()
		cmd := exec.Command(script)
		if len(env) == 1 && env[0] == "heartbeat" { // what the timer runs
			cmd, env = exec.Command(script, "heartbeat"), nil
		}
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v exited with %v: %s", env, err, out)
		}
		return string(out)
	}
	report := func(c byte, phase, status string) string {
		return run("COMIN_PHASE="+phase, "COMIN_STATUS="+status, "COMIN_GIT_SHA="+strings.Repeat(string(c), 40))
	}
	// received says what the manager got, in order: a commit's first letter, or "beat"
	// for a heartbeat, "beat-" for one without a system.
	received := func(n int) string {
		t.Helper()
		var out []string
		for range n {
			s := <-got
			switch c, _ := s.body["commit"].(string); {
			case s.body["phase"] == "heartbeat" && s.body["current"] == "":
				out = append(out, "beat-")
			case s.body["phase"] == "heartbeat":
				out = append(out, "beat")
			default:
				out = append(out, c[:1])
			}
		}
		select {
		case s := <-got:
			t.Fatalf("one more report: %+v", s.body)
		default:
		}
		return strings.Join(out, " ")
	}
	waiting := func() string {
		b, err := os.ReadFile(pending)
		if os.IsNotExist(err) {
			return ""
		}
		var out []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var r map[string]any
			if err := json.Unmarshal([]byte(l), &r); err != nil {
				t.Fatalf("pending line %q: %v", l, err)
			}
			out = append(out, r["commit"].(string)[:1])
		}
		return strings.Join(out, " ")
	}

	// Not delivered, and not a status curl retries by itself (5xx would be sent twice).
	code.Store(http.StatusNotFound)
	if out := report('a', "deploy", "done"); !strings.Contains(out, "the next heartbeat sends it") {
		t.Errorf("output: %q", out)
	}
	if fi, err := os.Stat(pending); err != nil || fi.Mode().Perm() != 0o600 || received(1) != "a" || waiting() != "a" {
		t.Fatalf("pending report: %v %v %q", fi, err, waiting())
	}
	if fi, err := os.Stat(state); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("state folder: %v %v", fi, err)
	}
	// The manager is still down when comin's next build fails: both wait, in order.
	report('b', "build", "failed")
	if r, w := received(1), waiting(); r != "a" || w != "a b" {
		t.Fatalf("after the failed build: received %q, waiting %q", r, w)
	}
	// While reports wait, a heartbeat tells that the machine is up, not what it runs.
	run("heartbeat")
	if r, w := received(2), waiting(); r != "a beat-" || w != "a b" {
		t.Fatalf("heartbeat while waiting: received %q, waiting %q", r, w)
	}
	code.Store(http.StatusNoContent)
	run("heartbeat")
	if r, w := received(3), waiting(); r != "a b beat" || w != "" {
		t.Fatalf("heartbeat once the manager is back: received %q, waiting %q", r, w)
	}

	// A comin report sends the waiting ones first, so a build report never
	// overtakes the deployment that tells what runs.
	code.Store(http.StatusUnauthorized)
	report('c', "deploy", "done")
	received(1)
	code.Store(http.StatusNoContent)
	report('d', "build", "failed")
	if r, w := received(2), waiting(); r != "c d" || w != "" {
		t.Fatalf("a later report: received %q, waiting %q", r, w)
	}

	// Refused: sending it again cannot help, so it is dropped.
	code.Store(http.StatusNotFound)
	report('e', "deploy", "done")
	received(1)
	code.Store(http.StatusBadRequest)
	run("heartbeat")
	if r, w := received(2), waiting(); r != "e beat" || w != "" {
		t.Fatalf("a refused report: received %q, waiting %q", r, w)
	}

	// At most the newest 20 wait.
	code.Store(http.StatusNotFound)
	for i := range 25 {
		run("COMIN_STATUS=done", fmt.Sprintf("COMIN_GIT_SHA=%040x", i))
		received(1)
	}
	b, _ := os.ReadFile(pending)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 20 || !strings.Contains(lines[0], fmt.Sprintf("%040x", 5)) || !strings.Contains(lines[19], fmt.Sprintf("%040x", 24)) {
		t.Fatalf("%d waiting, first %.80s", len(lines), lines[0])
	}
}

func TestReportDeploymentScriptPassesShellcheck(t *testing.T) {
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("needs shellcheck")
	}
	if out, err := exec.Command("shellcheck", reporter(t, "http://manager", "/run/token", "/var/lib/nixos-manager-report", "")).CombinedOutput(); err != nil {
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
	if got["deploy"] != true || got["off"] != true || (got["build"] != true && got["build"] != "absent") || got["heartbeat"] != true ||
		fmt.Sprint(got["silentAfter"]) != "[true true]" {
		t.Fatalf("wiring: %v", got)
	}
	if out, err := exec.Command("nix", append(nix, "build", "--impure", "--no-link", "--file", "testdata/report-module.nix", "script")...).CombinedOutput(); err != nil {
		t.Fatalf("building the script: %v\n%s", err, out)
	}
}

// The binary cache module adds the fleet's cache next to cache.nixos.org,
// refuses a cache without its key, and writes a nix.conf Nix accepts.
func TestBinaryCacheModule(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	nix := []string{"--extra-experimental-features", "nix-command flakes"}
	out, err := exec.Command("nix", append(nix, "eval", "--impure", "--json", "--file", "testdata/binary-cache-module.nix", "--apply", "x: removeAttrs x [\"conf\"]")...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("nix eval: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var got map[string]bool
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"substituters", "key", "timeout", "off", "noKey"} {
		if !got[k] {
			t.Errorf("%s: %v", k, got)
		}
	}
	if out, err := exec.Command("nix", append(nix, "build", "--impure", "--no-link", "--file", "testdata/binary-cache-module.nix", "conf")...).CombinedOutput(); err != nil {
		t.Fatalf("building nix.conf: %v\n%s", err, out)
	}
}
