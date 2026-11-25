package templates

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type closurePost struct {
	host, system, auth, contentType string
	body                            map[string]any
}

// closureManager answers reports with 200 {"closure":"wanted"} while want is
// set (else 204), and passes on every closure posted to it, answered with
// closureCode.
func closureManager(t *testing.T) (srv *httptest.Server, reports chan map[string]any, closures chan closurePost, want *atomic.Bool, closureCode *atomic.Int32) {
	reports, closures = make(chan map[string]any, 30), make(chan closurePost, 30)
	want, closureCode = &atomic.Bool{}, &atomic.Int32{}
	closureCode.Store(http.StatusNoContent)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Errorf("%s: body %s: %v", r.URL.Path, b, err)
		}
		switch r.URL.Path {
		case "/api/deployments/report":
			reports <- body
			if want.Load() {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"closure":"wanted"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/api/deployments/closure":
			q := r.URL.Query()
			closures <- closurePost{q.Get("host"), q.Get("system"), r.Header.Get("Authorization"), r.Header.Get("Content-Type"), body}
			w.WriteHeader(int(closureCode.Load()))
		default:
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return
}

// fakeNix puts a nix on PATH that records how it was run in args and prints
// the file named by $FAKE_NIX, fails like a missing daemon when it is "fail",
// and hangs like a stuck daemon when it is "hang".
func fakeNix(t *testing.T, dir string) (path, args string) {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	args = filepath.Join(dir, "nix-args")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"NIX_REMOTE=$NIX_REMOTE\" \"$@\" >'" + args + "'\n" +
		"if [[ $FAKE_NIX == fail ]]; then echo \"error: cannot connect to socket at '/nix/var/nix/daemon-socket/socket'\" >&2; exit 1; fi\n" +
		"if [[ $FAKE_NIX == hang ]]; then exec sleep 30; fi\n" +
		"cat \"$FAKE_NIX\"\n"
	if err := os.WriteFile(filepath.Join(bin, "nix"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin + string(os.PathListSeparator) + os.Getenv("PATH"), args
}

// closureSetup is a machine with a token, a running system and a fake nix.
type closureSetup struct {
	script, state, path, args, sys string
}

func newClosureSetup(t *testing.T, url string) closureSetup {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("nxh_secret\n"), 0o600)
	root, state := filepath.Join(dir, "root"), filepath.Join(dir, "state")
	sys := strings.Repeat("a", 32) + "-nixos-system-pi-26.05+x"
	fakeSystem(t, root, sys, sys)
	path, args := fakeNix(t, dir)
	return closureSetup{script: reporter(t, url, tokenFile, state, root), state: state, path: path, args: args, sys: "/nix/store/" + sys}
}

// run runs the script as the timer (heartbeat) or comin does, with $FAKE_NIX set to fake.
func (c closureSetup) run(t *testing.T, fake string, args ...string) string {
	t.Helper()
	if len(args) == 0 {
		return c.comin(t, fake, "COMIN_STATUS=done")
	}
	return c.exec(t, fake, args, nil)
}

// comin runs the script as comin does, with these COMIN_* variables besides the commit.
func (c closureSetup) comin(t *testing.T, fake string, env ...string) string {
	t.Helper()
	return c.exec(t, fake, nil, append(env, "COMIN_GIT_SHA="+strings.Repeat("c", 40)))
}

func (c closureSetup) exec(t *testing.T, fake string, args, env []string) string {
	t.Helper()
	cmd := exec.Command(c.script, args...)
	cmd.Env = append(append(os.Environ(), "PATH="+c.path, "FAKE_NIX="+fake), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v exited with %v: %s", args, err, out)
	}
	return string(out)
}

// recv waits a while for what a channel gets.
func recv[T any](t *testing.T, what string, ch chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: nothing arrived", what)
	}
	panic("unreachable")
}

func noMore[T any](t *testing.T, what string, ch chan T) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("%s: one more: %+v", what, v)
	default:
	}
}

// When the reply to a report asks for it, the script reads the closure of
// the running system through the daemon and posts it with indices for
// references, whichever JSON shape this Nix prints.
func TestReportSendsClosureWhenAsked(t *testing.T) {
	srv, reports, closures, want, _ := closureManager(t)
	c := newClosureSetup(t, srv.URL)
	bash := "/nix/store/" + strings.Repeat("b", 32) + "-bash-5.2p37"
	glibc := "/nix/store/" + strings.Repeat("d", 32) + "-glibc-2.40-66"
	base := func(p string) string { return strings.TrimPrefix(p, "/nix/store/") }
	dir := t.TempDir()
	shapes := map[string]string{
		// Nix before 2.19, and Lix.
		"array": `[{"path":"` + c.sys + `","narSize":10,"references":["` + glibc + `","` + bash + `","` + c.sys + `"]},` +
			`{"path":"` + bash + `","narSize":200,"references":["` + glibc + `"]},{"path":"` + glibc + `","narSize":4000,"references":["` + glibc + `"]}]`,
		// Nix 2.19 and later, with an invalid path as null and references as base names.
		"object": `{"` + c.sys + `":{"narSize":10,"references":["` + base(glibc) + `","` + base(bash) + `"]},` +
			`"` + bash + `":{"narSize":200,"references":["` + glibc + `"]},"` + glibc + `":{"narSize":4000,"references":[]},"/nix/store/` + strings.Repeat("z", 32) + `-gone":null}`,
	}
	wantBody := map[string]any{"paths": []any{
		map[string]any{"path": c.sys, "narSize": 10.0, "refs": []any{2.0, 1.0}},
		map[string]any{"path": bash, "narSize": 200.0, "refs": []any{2.0}},
		map[string]any{"path": glibc, "narSize": 4000.0, "refs": []any{}},
	}}

	c.run(t, "unused", "heartbeat")
	if r := recv(t, "heartbeat", reports); r["sendsClosure"] != true || r["current"] != c.sys {
		t.Fatalf("heartbeat: %+v", r)
	}
	noMore(t, "not asked", closures)

	want.Store(true)
	for name, out := range shapes {
		f := filepath.Join(dir, name+".json")
		os.WriteFile(f, []byte(out), 0o600)
		for _, args := range [][]string{{"heartbeat"}, nil} {
			c.run(t, f, args...)
			if r := recv(t, name, reports); r["sendsClosure"] != true {
				t.Fatalf("%s %v: report %+v", name, args, r)
			}
			p := recv(t, name, closures)
			if p.host != "pi" || p.system != c.sys || p.auth != "Bearer nxh_secret" || p.contentType != "application/json" || !reflect.DeepEqual(p.body, wantBody) {
				t.Fatalf("%s %v: posted %+v", name, args, p)
			}
			noMore(t, name, closures)
		}
	}
	b, _ := os.ReadFile(c.args)
	if got := strings.Fields(string(b)); !reflect.DeepEqual(got, []string{"NIX_REMOTE=daemon", "--extra-experimental-features", "nix-command", "path-info", "--recursive", "--json", c.sys}) {
		t.Fatalf("nix ran as %q", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(c.state, ".closure*"))
	if len(leftovers) > 0 {
		t.Fatalf("left behind: %v", leftovers)
	}
}

// A Nix that fails posts its error instead, so the console shows it.
func TestReportNixFailurePostsError(t *testing.T) {
	srv, reports, closures, want, _ := closureManager(t)
	c := newClosureSetup(t, srv.URL)
	want.Store(true)
	c.run(t, "fail", "heartbeat")
	recv(t, "report", reports)
	p := recv(t, "closure", closures)
	if e, _ := p.body["error"].(string); !strings.Contains(e, "cannot connect to socket") || p.body["paths"] != nil || p.system != c.sys {
		t.Fatalf("posted %+v", p)
	}
}

// An upload that fails is logged, never retried by the script (the next
// reply asks again), and never fails the run.
func TestReportClosureUploadFailureExitsZero(t *testing.T) {
	srv, reports, closures, want, closureCode := closureManager(t)
	c := newClosureSetup(t, srv.URL)
	f := filepath.Join(t.TempDir(), "closure.json")
	os.WriteFile(f, []byte(`[{"path":"`+c.sys+`","narSize":1,"references":[]}]`), 0o600)
	want.Store(true)
	closureCode.Store(http.StatusRequestEntityTooLarge)
	if out := c.run(t, f, "heartbeat"); !strings.Contains(out, "did not take") {
		t.Fatalf("output: %q", out)
	}
	recv(t, "report", reports)
	recv(t, "closure", closures)
	noMore(t, "retried", closures)
	if _, err := os.Stat(filepath.Join(c.state, "pending.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("a closure is never spooled: %v", err)
	}
}

// Waiting reports are flushed first; only the reply to the run's own report
// may ask for the closure, so one run posts it once.
func TestFlushedReportDoesNotSendClosure(t *testing.T) {
	srv, reports, closures, want, _ := closureManager(t)
	c := newClosureSetup(t, srv.URL)
	f := filepath.Join(t.TempDir(), "closure.json")
	os.WriteFile(f, []byte(`[{"path":"`+c.sys+`","narSize":1,"references":[]}]`), 0o600)
	os.MkdirAll(c.state, 0o700)
	waiting := `{"host":"pi","commit":"` + strings.Repeat("b", 40) + `","phase":"deploy","status":"done","current":"` + c.sys + `"}`
	os.WriteFile(filepath.Join(c.state, "pending.jsonl"), []byte(waiting+"\n"+waiting+"\n"), 0o600)
	want.Store(true)
	c.run(t, f, "heartbeat")
	for range 3 {
		recv(t, "report", reports)
	}
	recv(t, "closure", closures)
	noMore(t, "flushed reports", closures)
}

// A Nix daemon that does not answer must not hold up the run: the read gives
// up after closure_timeout seconds and posts that as the machine's error.
func TestReportClosureReadTimesOut(t *testing.T) {
	srv, reports, closures, want, _ := closureManager(t)
	c := newClosureSetup(t, srv.URL)
	b, err := os.ReadFile(c.script)
	if err != nil || !strings.Contains(string(b), "\nclosure_timeout=30 ") {
		t.Fatalf("the script sets no closure_timeout=30: %v", err)
	}
	os.WriteFile(c.script, []byte(strings.Replace(string(b), "\nclosure_timeout=30 ", "\nclosure_timeout=1 ", 1)), 0o755)
	want.Store(true)
	start := time.Now()
	c.run(t, "hang", "heartbeat")
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("the run took %v", d)
	}
	recv(t, "report", reports)
	if p := recv(t, "closure", closures); !strings.Contains(fmt.Sprint(p.body["error"]), "did not finish within 1 seconds") {
		t.Fatalf("posted %+v", p)
	}
}

// Between comin's build and its deployment nothing is read or sent: the
// deployment's report, or the next heartbeat, sends the closure.
func TestBuildReportSendsNoClosure(t *testing.T) {
	srv, reports, closures, want, _ := closureManager(t)
	c := newClosureSetup(t, srv.URL)
	f := filepath.Join(t.TempDir(), "closure.json")
	os.WriteFile(f, []byte(`[{"path":"`+c.sys+`","narSize":1,"references":[]}]`), 0o600)
	want.Store(true)
	c.comin(t, f, "COMIN_PHASE=build", "COMIN_STATUS=built")
	if r := recv(t, "build report", reports); r["phase"] != "build" {
		t.Fatalf("report: %+v", r)
	}
	time.Sleep(200 * time.Millisecond)
	noMore(t, "after a build report", closures)
	c.comin(t, f, "COMIN_PHASE=deploy", "COMIN_STATUS=done")
	recv(t, "deploy report", reports)
	recv(t, "closure after the deployment", closures)
}
