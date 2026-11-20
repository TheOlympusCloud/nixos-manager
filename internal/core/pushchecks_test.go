package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// statusPost is a commit status as Forgejo received it.
type statusPost struct {
	SHA, Auth   string
	State       string `json:"state"`
	Target      string `json:"target_url"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

// fakeForgejo answers the API calls nixos-manager makes: the repository (for
// the token test) and commit statuses, which it records.
type fakeForgejo struct {
	*httptest.Server
	mu       sync.Mutex
	posts    []statusPost
	calls    int // status posts received
	fail     int // answer 500 to this many status posts first
	repoCode int // the answer to GET of the repository; 0 is 200
}

func newFakeForgejo(t *testing.T) *fakeForgejo {
	f := &fakeForgejo{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/repos/pablo/fleet":
			if r.Header.Get("Authorization") != "token good-token" {
				w.WriteHeader(401)
				w.Write([]byte(`{"message":"token is required"}`))
				return
			}
			if f.repoCode != 0 {
				w.WriteHeader(f.repoCode)
				return
			}
			w.Write([]byte(`{"full_name":"pablo/fleet"}`))
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/v1/repos/pablo/fleet/statuses/"):
			f.calls++
			if r.Header.Get("Authorization") != "token good-token" {
				w.WriteHeader(401)
				w.Write([]byte(`{"message":"user does not have permission"}`))
				return
			}
			if f.fail > 0 {
				f.fail--
				w.WriteHeader(500)
				return
			}
			var p statusPost
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				w.WriteHeader(400)
				return
			}
			p.SHA, p.Auth = strings.TrimPrefix(r.URL.Path, "/api/v1/repos/pablo/fleet/statuses/"), r.Header.Get("Authorization")
			f.posts = append(f.posts, p)
			w.WriteHeader(201)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// states lists the statuses posted for sha, in order.
func (f *fakeForgejo) states(sha string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, p := range f.posts {
		if p.SHA == sha {
			out = append(out, p.State)
		}
	}
	return out
}

func (f *fakeForgejo) last(sha string) statusPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.posts) - 1; i >= 0; i-- {
		if f.posts[i].SHA == sha {
			return f.posts[i]
		}
	}
	return statusPost{}
}

// waitFor polls until cond holds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for range 1000 {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// final waits for a final status of sha and returns every state posted for it.
func (f *fakeForgejo) final(t *testing.T, sha string) []string {
	t.Helper()
	waitFor(t, "the result of "+sha[:7], func() bool {
		s := f.states(sha)
		return len(s) > 0 && s[len(s)-1] != "pending"
	})
	return f.states(sha)
}

// forgejoFleet is buildService on a remote that looks like Forgejo's
// (https://forgejo.test/pablo/fleet.git, which git maps to the local bare
// repository), with what is pushed checked, the push webhook and a status
// token for the fake Forgejo.
func forgejoFleet(t *testing.T) (*Service, *Forgejo, *fakeForgejo) {
	t.Helper()
	old := hookDelay
	hookDelay = 10 * time.Millisecond
	t.Cleanup(func() { hookDelay = old })
	ctx := context.Background()
	s := buildService(t, true)
	bare := s.Repo.originURL(ctx)
	const remote = "https://forgejo.test/pablo/fleet.git"
	gitOut(t, s.Repo.Dir, "config", "url."+bare+".insteadOf", remote)
	ok(t, s.Repo.SetRemote(ctx, remote))
	if info := runCheck(t, s); info.State != "passed" {
		t.Fatalf("check:\n%s", info.Output)
	}
	fake := newFakeForgejo(t)
	ok(t, s.Repo.SetWebBase(ctx, fake.URL))
	f := &Forgejo{Repo: s.Repo, DataDir: t.TempDir(), Jobs: s.Jobs}
	ok(t, f.Resume())
	ok(t, f.SetStatusToken(ctx, "good-token", "http://console.lan:8080/"))
	return s, f, fake
}

// deliver sends a signed push event for branch, as Forgejo does.
func deliver(t *testing.T, f *Forgejo, branch, before, after string) int {
	t.Helper()
	secret := must[string](t)(f.HookSecret())
	body := `{"ref":"refs/heads/` + branch + `","before":"` + before + `","after":"` + after + `"}`
	code, _ := f.Hook("push", sign(secret, body), []byte(body))
	return code
}

func pushedFile(t *testing.T, f *Forgejo) []pushedCommit {
	t.Helper()
	var out []pushedCommit
	if b, err := os.ReadFile(filepath.Join(f.DataDir, "pushed-checks.json")); err == nil {
		ok(t, json.Unmarshal(b, &out))
	}
	return out
}

// A commit pushed from elsewhere is checked, and Forgejo hears pending, then
// the result, with a link to the check in the console.
func TestPushedCommitIsCheckedAndReported(t *testing.T) {
	s, f, fake := forgejoFleet(t)
	clone := laptop(t, s)
	before := gitOut(t, clone, "rev-parse", "HEAD")
	otherCommit(t, clone, "hosts/web", "web kernel=2\n")
	sha := gitOut(t, clone, "rev-parse", "HEAD")

	if code := deliver(t, f, "main", before, sha); code != 202 {
		t.Fatalf("hook: %d", code)
	}
	if got := fake.final(t, sha); !slices.Equal(got, []string{"pending", "success"}) {
		t.Fatalf("statuses %v", got)
	}
	p := fake.last(sha)
	if p.Context != "nixos-manager/check" || p.Auth != "token good-token" || p.Target != "http://console.lan:8080/#/changes?check="+sha ||
		!strings.Contains(p.Description, "web") {
		t.Fatalf("status %+v", p)
	}
	i := slices.IndexFunc(s.Jobs.List(), func(j JobInfo) bool { return j.Commit == sha })
	if i < 0 || s.Jobs.List()[i].Branch != "main" || !slices.Equal(s.Jobs.List()[i].Summary.Built, []string{"web"}) {
		t.Fatalf("jobs %+v", s.Jobs.List())
	}
	if c, _, _ := s.Repo.Read("hosts/web"); c != "web kernel=2\n" {
		t.Fatal("the console's branch was not fast-forwarded")
	}
	waitFor(t, "the pushed commit to leave the list", func() bool { return len(pushedFile(t, f)) == 0 })
	if d := f.LastStatus(); d == nil || !d.OK || d.Commit != sha || d.State != "success" {
		t.Fatalf("last status %+v", d)
	}
}

// A push to another branch is checked too, compared with the last checked
// commit before it; a machine that does not build fails it.
func TestPushedCommitThatFailsIsReportedAsFailure(t *testing.T) {
	s, f, fake := forgejoFleet(t)
	clone := laptop(t, s)
	gitOut(t, clone, "checkout", "-q", "-b", "testing-db")
	ok(t, os.WriteFile(filepath.Join(clone, "hosts/db"), []byte("db broken\n"), 0o644))
	gitOut(t, clone, "commit", "-q", "-am", "db: break it")
	gitOut(t, clone, "push", "-q", "origin", "testing-db")
	sha := gitOut(t, clone, "rev-parse", "HEAD")

	if code := deliver(t, f, "testing-db", strings.Repeat("0", 40), sha); code != 202 {
		t.Fatalf("hook: %d", code)
	}
	if got := fake.final(t, sha); !slices.Equal(got, []string{"pending", "failure"}) {
		t.Fatalf("statuses %v", got)
	}
	if p := fake.last(sha); !strings.Contains(p.Description, "db") {
		t.Fatalf("status %+v", p)
	}
	if st := must[Status](t)(s.Repo.Status(context.Background())); st.Branch != "main" {
		t.Fatalf("the console moved to %s", st.Branch)
	}
}

// A push from the console was checked before it left: Forgejo hears success
// at once, and nothing is built again.
func TestPushFromTheConsoleIsReportedWithoutACheck(t *testing.T) {
	ctx := context.Background()
	s, f, fake := forgejoFleet(t)
	before := gitOut(t, s.Repo.Dir, "rev-parse", "HEAD")
	write(t, s.Repo, "hosts/web", "web kernel=3\n")
	runCheck(t, s)
	must[string](t)(s.Commit(ctx, "web: kernel 3", false))
	must[string](t)(s.Push(ctx, false))
	sha := gitOut(t, s.Repo.Dir, "rev-parse", "HEAD")
	checks := func() int {
		n := 0
		for _, j := range s.Jobs.List() {
			if j.Kind == "check" {
				n++
			}
		}
		return n
	}
	jobs := checks()

	deliver(t, f, "main", before, sha)
	if got := fake.final(t, sha); !slices.Equal(got, []string{"success"}) {
		t.Fatalf("statuses %v", got)
	}
	if p := fake.last(sha); !strings.Contains(p.Description, "web") {
		t.Fatalf("status %+v", p)
	}
	if n := checks(); n != jobs {
		t.Fatalf("%d jobs, want %d", n, jobs)
	}
}

// The same commit delivered twice, and pushed to a second branch, is checked
// once; tags and deleted branches are not checked.
func TestPushedCommitIsCheckedOnce(t *testing.T) {
	s, f, fake := forgejoFleet(t)
	nix, block := blockingNix(t)
	s.Jobs.Nix = nix
	clone := laptop(t, s)
	otherCommit(t, clone, "hosts/web", "web kernel=2\n")
	sha := gitOut(t, clone, "rev-parse", "HEAD")
	gitOut(t, clone, "push", "-q", "origin", "HEAD:refs/heads/testing-web")

	zero := strings.Repeat("0", 40)
	for _, b := range []string{"main", "main", "testing-web"} {
		if code := deliver(t, f, b, zero, sha); code != 202 {
			t.Fatalf("hook %s: %d", b, code)
		}
	}
	waitFor(t, "the pending status", func() bool { return len(fake.states(sha)) > 0 })
	secret := must[string](t)(f.HookSecret())
	for body, want := range map[string]int{
		`{"ref":"refs/tags/v1","after":"` + sha + `"}`:           200,
		`{"ref":"refs/heads/old","after":"` + zero + `"}`:        200,
		`{"ref":"refs/heads/main","after":"` + sha[:12] + `"}`:   400,
		`{"ref":"refs/heads/main","after":"--upload-pack=evil"}`: 400,
	} {
		if code, _ := f.Hook("push", sign(secret, body), []byte(body)); code != want {
			t.Errorf("%s: %d, want %d", body, code, want)
		}
	}
	ok(t, os.Remove(block))
	fake.final(t, sha)
	n := 0
	for _, j := range s.Jobs.List() {
		if j.Commit != "" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d commit checks, want 1: %+v", n, s.Jobs.List())
	}
}

// Commits waiting for a check when the server stops are checked after it
// starts again, so Forgejo is not left with a pending status.
func TestPushedCommitsSurviveARestart(t *testing.T) {
	s, f, fake := forgejoFleet(t)
	clone := laptop(t, s)
	otherCommit(t, clone, "hosts/web", "web kernel=2\n")
	sha := gitOut(t, clone, "rev-parse", "HEAD")
	hookDelay = time.Hour // the server "stops" before it fetches
	deliver(t, f, "main", strings.Repeat("0", 40), sha)
	if got := pushedFile(t, f); len(got) != 1 || got[0].Commit != sha || got[0].Branch != "main" {
		t.Fatalf("pushed-checks.json %+v", got)
	}

	again := &Forgejo{Repo: s.Repo, DataDir: f.DataDir, Jobs: s.Jobs}
	ok(t, again.Resume())
	if got := fake.final(t, sha); got[len(got)-1] != "success" {
		t.Fatalf("statuses %v", got)
	}
	waitFor(t, "the list to empty", func() bool { return len(pushedFile(t, again)) == 0 })
}

// A status Forgejo fails to take is sent again; one it refuses is not.
func TestStatusIsRetried(t *testing.T) {
	old := statusRetry
	statusRetry = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { statusRetry = old })
	s, f, fake := forgejoFleet(t)
	sha := gitOut(t, s.Repo.Dir, "rev-parse", "HEAD") // checked: success at once
	fake.mu.Lock()
	fake.fail = 2
	fake.mu.Unlock()
	deliver(t, f, "main", strings.Repeat("0", 40), sha)
	if got := fake.final(t, sha); !slices.Equal(got, []string{"success"}) {
		t.Fatalf("statuses %v", got)
	}

	calls := func() int {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return fake.calls
	}
	fake.mu.Lock()
	fake.fail = 3
	fake.mu.Unlock()
	n := calls()
	f.postStatus(context.Background(), sha, "success", "again")
	if d := f.LastStatus(); d == nil || d.OK || !strings.Contains(d.Result, "500") || calls()-n != 3 {
		t.Fatalf("last status after %d failures %+v", calls()-n, d)
	}
	fake.mu.Lock()
	fake.fail = 0
	fake.mu.Unlock()

	ok(t, os.WriteFile(filepath.Join(f.DataDir, "forgejo-status.json"), []byte(`{"token":"revoked"}`), 0o600))
	n = calls()
	f.postStatus(context.Background(), sha, "success", "again")
	if d := f.LastStatus(); d == nil || d.OK || !strings.Contains(d.Result, "refused the token") || !strings.Contains(d.Result, "permission") || calls()-n != 1 {
		t.Fatalf("last status with a refused token, after %d tries: %+v", calls()-n, d)
	}
}

// Without a status token, pushed commits are still checked; nothing is sent.
func TestPushedCommitIsCheckedWithoutAToken(t *testing.T) {
	ctx := context.Background()
	s, f, fake := forgejoFleet(t)
	ok(t, f.SetStatusToken(ctx, "", ""))
	clone := laptop(t, s)
	otherCommit(t, clone, "hosts/web", "web kernel=2\n")
	sha := gitOut(t, clone, "rev-parse", "HEAD")
	deliver(t, f, "main", strings.Repeat("0", 40), sha)
	waitFor(t, "the check", func() bool { state, _ := s.Jobs.CommitState(ctx, sha); return state == "passed" })
	waitFor(t, "the list to empty", func() bool { return len(pushedFile(t, f)) == 0 })
	if got := fake.states(sha); len(got) != 0 || f.LastStatus() != nil {
		t.Fatalf("statuses %v, last %+v", got, f.LastStatus())
	}
}

// The token is tried on the repository before it is kept, kept 0600 in the
// data dir, and never shown back.
func TestSetStatusToken(t *testing.T) {
	ctx := context.Background()
	s, f, fake := forgejoFleet(t)
	st := must[ForgejoState](t)(f.State(ctx))
	if !st.StatusToken || st.StatusConsole != "http://console.lan:8080" || st.StatusAPI != fake.URL+"/api/v1/repos/pablo/fleet" {
		t.Fatalf("state %+v", st)
	}
	b := must[[]byte](t)(json.Marshal(st))
	if strings.Contains(string(b), "good-token") {
		t.Fatal("the state shows the token")
	}
	file := filepath.Join(f.DataDir, "forgejo-status.json")
	if fi := must[os.FileInfo](t)(os.Stat(file)); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}

	err := f.SetStatusToken(ctx, "wrong-token", "")
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "refused the token") {
		t.Fatalf("message %q", err)
	}
	fake.mu.Lock()
	fake.repoCode = 404
	fake.mu.Unlock()
	err = f.SetStatusToken(ctx, "good-token", "")
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "pablo/fleet") {
		t.Fatalf("message %q", err)
	}
	for _, bad := range [][2]string{{"to ken", ""}, {"good-token", "javascript:alert(1)"}, {"good-token", "http://user:pw@console.lan"}} {
		isErr(t, f.SetStatusToken(ctx, bad[0], bad[1]), ErrInvalid)
	}
	if st := must[ForgejoState](t)(f.State(ctx)); !st.StatusToken || st.StatusConsole != "http://console.lan:8080" {
		t.Fatalf("a refused token replaced the saved one: %+v", st)
	}

	ok(t, f.SetStatusToken(ctx, "", ""))
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the token file stayed: %v", err)
	}
	ok(t, s.Repo.SetRemote(ctx, "/srv/git/fleet.git"))
	err = f.SetStatusToken(ctx, "good-token", "")
	isErr(t, err, ErrInvalid)
}
