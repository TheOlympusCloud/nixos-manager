package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fleetService serves the fixture fleet through scripts/dev/fake-nix.
func fleetService(t *testing.T, fixtures string) *Service {
	t.Helper()
	s := NewService(newRepo(t))
	nix, _ := filepath.Abs("../../scripts/dev/fake-nix")
	dir, _ := filepath.Abs(fixtures)
	s.Jobs.Nix = nix
	t.Setenv("NIXMGR_FAKE_FIXTURES", dir)
	lock, err := os.ReadFile("testdata/fleet/flake.lock")
	if err != nil {
		t.Fatal(err)
	}
	write(t, s.Repo, "flake.nix", "{ }\n")
	write(t, s.Repo, "flake.lock", string(lock))
	return s
}

func readyModel(t *testing.T, s *Service, at string) *Model {
	t.Helper()
	m, j, err := s.Jobs.Model(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		wait(t, j)
		if m, _, err = s.Jobs.Model(context.Background(), at); err != nil || m == nil {
			t.Fatalf("model not ready after its job: %v", err)
		}
	}
	return m
}

func TestModelOfAFleet(t *testing.T) {
	s := fleetService(t, "testdata/fleet")
	m := readyModel(t, s, "working")

	var got []string
	for _, h := range m.Hosts {
		got = append(got, h.Name)
	}
	if strings.Join(got, ",") != "homeserver,oldbox,pi,workstation" {
		t.Fatalf("hosts %v", got)
	}
	old := m.Hosts[1]
	if !strings.Contains(old.Error, "infinite recursion") || old.Packages == nil || old.Features == nil {
		t.Fatalf("a failing machine must carry its error and empty lists: %+v", old)
	}
	ws := m.Hosts[3]
	if ws.ManagedFile == nil || *ws.ManagedFile != "hosts/workstation/managed.json" || len(ws.Packages) < 20 {
		t.Fatalf("workstation %+v", ws)
	}
	for i := 1; i < len(ws.Packages); i++ {
		if ws.Packages[i-1].Name > ws.Packages[i].Name {
			t.Fatalf("packages not sorted: %s before %s", ws.Packages[i-1].Name, ws.Packages[i].Name)
		}
	}
	if u := ws.Users[0]; u.Name != "pablo" || u.Home == nil || len(u.Home.Programs) == 0 {
		t.Fatalf("home-manager user %+v", u)
	}
	if m.Hosts[2].ManagedFile != nil {
		t.Fatal("pi does not load a managed file")
	}
	if ws.ManagedPackages["vscode"] != "vscode" || m.Hosts[2].ManagedPackages == nil {
		t.Fatalf("managed package names %+v / %+v", ws.ManagedPackages, m.Hosts[2].ManagedPackages)
	}
	if len(m.Homes) != 1 || m.Homes[0].Name != "pablo@laptop" {
		t.Fatalf("homes %+v", m.Homes)
	}
	if len(m.Inputs) != 3 || m.Inputs[2].Name != "nixpkgs" || m.Inputs[2].URL != "github:NixOS/nixpkgs/nixos-26.05" {
		t.Fatalf("inputs %+v", m.Inputs)
	}
	tree := must[string](t)(s.Repo.WorkingTree(context.Background()))
	if m.Tree != tree {
		t.Fatalf("model tree %s, working tree %s", m.Tree, tree)
	}
}

func TestModelIsCachedPerTree(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	m1 := readyModel(t, s, "working")
	m2, j, err := s.Jobs.Model(ctx, "working")
	if err != nil || m2 != m1 || j != nil {
		t.Fatalf("second call: %p %v %v, want the cached %p", m2, j, err, m1)
	}
	staged := must[Status](t)(s.Status(ctx)).StagedTree
	write(t, s.Repo, "hosts/new.nix", "{ }\n") // untracked files count for the working copy
	m3, j, _ := s.Jobs.Model(ctx, "working")
	if m3 != nil || j == nil {
		t.Fatal("an edit did not start a new evaluation")
	}
	// The same job, or its model when the fake Nix already finished it; never a second job.
	if again, j2, _ := s.Jobs.Model(ctx, "working"); j2 != j && (j2 != nil || again == nil) {
		t.Fatal("a second request started a duplicate evaluation")
	}
	wait(t, j)
	if tree := readyModel(t, s, "working").Tree; tree == m1.Tree || tree == staged {
		t.Fatal("working tree ignores the untracked file")
	}
	head := readyModel(t, s, "head") // no commits yet: an empty fleet, no Nix needed
	if head.Tree != "" || len(head.Hosts) != 0 {
		t.Fatalf("head model of an empty repo %+v", head)
	}
	isErr(t, func() error { _, _, err := s.Jobs.Model(ctx, "yesterday"); return err }(), ErrInvalid)
}

func TestModelOfABrokenFlake(t *testing.T) {
	s := fleetService(t, t.TempDir()) // no fixtures: discovery fails like a flake with a syntax error
	m := readyModel(t, s, "working")
	if m.Error == "" || len(m.Hosts) != 0 {
		t.Fatalf("broken flake model %+v", m)
	}
	if again, j, _ := s.Jobs.Model(context.Background(), "working"); again != m || j != nil {
		t.Fatal("a broken flake is evaluated again on every request")
	}
	// After a transient failure (no network, say), the user can ask again.
	t.Setenv("NIXMGR_FAKE_FIXTURES", must[string](t)(filepath.Abs("testdata/fleet")))
	j := must[*Job](t)(s.Jobs.RefreshModel(context.Background()))
	wait(t, j)
	if fixed := readyModel(t, s, "working"); fixed.Error != "" || len(fixed.Hosts) != 4 {
		t.Fatalf("model after a refresh %+v", fixed)
	}
}

// Quick edits must not pile up full fleet evaluations: a newer request skips
// older ones still waiting in the queue.
func TestNewerModelSkipsQueuedOlderOnes(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	t.Setenv("NIXMGR_FAKE_DELAY", "0.2")
	_, running, _ := s.Jobs.Model(ctx, "working")
	for running.Info(false).State == "queued" {
		time.Sleep(5 * time.Millisecond)
	}
	write(t, s.Repo, "a.nix", "1\n")
	_, older, _ := s.Jobs.Model(ctx, "working")
	write(t, s.Repo, "a.nix", "2\n")
	_, newer, _ := s.Jobs.Model(ctx, "working")
	if older == newer {
		t.Fatal("different trees share a job")
	}
	if info := wait(t, older); info.State != "failed" || !strings.Contains(info.Output, "changed again") {
		t.Fatalf("the outdated evaluation ran: %+v", info)
	}
	if info := wait(t, newer); info.State != "passed" {
		t.Fatalf("latest evaluation: %+v", info)
	}
	if m, j, _ := s.Jobs.Model(ctx, "working"); m == nil || j != nil {
		t.Fatal("latest model not cached")
	}
}

// History asking for the commit the working copy matches shares its queued job;
// a later edit must not skip that job, or History shows a failure for the commit.
func TestEditsDoNotSkipAnEvaluationHistoryWaitsOn(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	t.Setenv("NIXMGR_FAKE_DELAY", "0.2")
	_, running, _ := s.Jobs.Model(ctx, "working")
	for running.Info(false).State == "queued" {
		time.Sleep(5 * time.Millisecond)
	}
	write(t, s.Repo, "a.nix", "1\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "one", true))
	_, shared, _ := s.Jobs.Model(ctx, "working")
	if _, j, _ := s.Jobs.Model(ctx, "head"); j != shared {
		t.Fatal("the commit's evaluation should share the queued job for the same tree")
	}
	write(t, s.Repo, "a.nix", "2\n")
	s.Jobs.Model(ctx, "working")
	if info := wait(t, shared); info.State != "passed" {
		t.Fatalf("the evaluation History waits on was skipped: %+v", info)
	}
}

// History can show what one commit changed: the model at any revision.
func TestModelAtRevision(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	ok(t, s.Stage(ctx, nil))
	c1 := must[string](t)(s.Commit(ctx, "first", true))
	write(t, s.Repo, "hosts/a.nix", "{ }\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "second", true))

	m := readyModel(t, s, "rev:"+c1)
	tree := strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", c1+"^{tree}"))
	if m.Tree != tree || len(m.Hosts) != 4 {
		t.Fatalf("model at %s: tree %s, want %s", c1, m.Tree, tree)
	}
	if p, ok := s.Repo.Parent(ctx, c1); ok || p != "" {
		t.Fatalf("root commit has a parent: %q", p)
	}
	if p, ok := s.Repo.Parent(ctx, "HEAD"); !ok || p != c1 {
		t.Fatalf("parent of HEAD: %q %v", p, ok)
	}
	_, _, err := s.Jobs.Model(ctx, "rev:nope")
	isErr(t, err, ErrNotFound)
}

func TestCappedOutput(t *testing.T) {
	c := capped{max: 4}
	if _, err := c.Write([]byte("abc")); err != nil || c.over {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("de")); err == nil || !c.over || c.buf.String() != "abc" {
		t.Fatalf("over the cap: %v %q", err, c.buf.String())
	}
}

// A standalone home-manager configuration with real Nix. Its programs include removed
// options, which throw when read (programs.octant): they must not hide the rest.
func TestHomeModelWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	write(t, s.Repo, "flake.nix", `{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  inputs.home-manager = {
    url = "github:nix-community/home-manager/release-26.05";
    inputs.nixpkgs.follows = "nixpkgs";
  };
  outputs =
    { nixpkgs, home-manager, ... }:
    let
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in
    {
      homeConfigurations.pablo = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [
          {
            home.username = "pablo";
            home.homeDirectory = "/home/pablo";
            home.stateVersion = "26.05";
            home.packages = [ pkgs.hello ];
            programs.git.enable = true;
          }
        ];
      };
    };
}
`)
	_, j, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	select {
	case <-j.Done():
	case <-time.After(15 * time.Minute):
		t.Fatal("the model did not finish")
	}
	m, _, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	if m == nil || m.Error != "" || len(m.Homes) != 1 {
		t.Fatalf("model: %+v\n%s", m, j.Info(true).Output)
	}
	h := m.Homes[0]
	if h.Error != "" || !slices.Contains(h.Packages, "hello") || !slices.Contains(h.Programs, "git") {
		t.Fatalf("home: %+v", h)
	}
}

// settle waits until the files are older than the racy window, so the next
// WorkingTree can be cached.
func settle(t *testing.T) {
	t.Helper()
	old := racyWindow
	racyWindow = 30 * time.Millisecond
	t.Cleanup(func() { racyWindow = old })
	time.Sleep(2 * racyWindow)
}

// cachedTree is WorkingTree while the repository lock is held by someone else:
// only a cached answer can come back.
func cachedTree(t *testing.T, r *Repo) (string, bool) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	got := make(chan string, 1)
	go func() {
		tree, _ := r.WorkingTree(context.Background())
		got <- tree
	}()
	select {
	case tree := <-got:
		return tree, true
	case <-time.After(500 * time.Millisecond):
		r.mu.Unlock()
		<-got // let it finish with the lock
		r.mu.Lock()
		return "", false
	}
}

// freshTree is the working tree computed without the cache.
func freshTree(t *testing.T, r *Repo) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	return must[string](t)(r.indexTree(context.Background(), []string{"-A", "--", "."}))
}

// Polls reuse the working tree without the repository lock while no file
// changed, and see every change: content of the same size with the
// modification time put back, renames, new and deleted files, the executable
// bit, ignore rules and the index.
func TestWorkingTreeIsCachedUntilAnythingChanges(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "aaaa\n")
	write(t, r, "ignored.nix", "tracked though ignored\n")
	gitOut(t, r.Dir, "add", "-A")
	write(t, r, ".gitignore", "ignored.nix\n")
	commitAll(t, r, "init")
	abs := func(p string) string { return filepath.Join(r.Dir, p) }

	for _, c := range []struct {
		name   string
		change func()
	}{
		{"same size, same mtime", func() {
			fi, _ := os.Stat(abs("a.nix"))
			ok(t, os.WriteFile(abs("a.nix"), []byte("bbbb\n"), 0o644))
			ok(t, os.Chtimes(abs("a.nix"), fi.ModTime(), fi.ModTime()))
		}},
		{"rename", func() { ok(t, os.Rename(abs("a.nix"), abs("b.nix"))) }},
		{"new file", func() { ok(t, os.WriteFile(abs("c.nix"), []byte("c\n"), 0o644)) }},
		{"executable", func() { ok(t, os.Chmod(abs("c.nix"), 0o755)) }},
		{"delete", func() { ok(t, os.Remove(abs("c.nix"))) }},
		{"info/exclude", func() { ok(t, os.WriteFile(abs(".git/info/exclude"), []byte("b.nix\n"), 0o644)) }},
		{"index", func() { gitOut(t, r.Dir, "rm", "-q", "--cached", "ignored.nix") }},
	} {
		settle(t)
		before := must[string](t)(r.WorkingTree(ctx))
		if tree, hit := cachedTree(t, r); !hit || tree != before {
			t.Fatalf("%s: an unchanged working copy was not served from the cache (%v)", c.name, hit)
		}
		c.change()
		settle(t) // past the racy window, only the fingerprint can tell
		want := freshTree(t, r)
		if want == before {
			t.Fatalf("%s: the change does not change the tree; the test is wrong", c.name)
		}
		if got := must[string](t)(r.WorkingTree(ctx)); got != want {
			t.Fatalf("%s: stale working tree %s, want %s", c.name, got, want)
		}
	}
}

// The index and info/exclude are found in a repository whose path has spaces.
func TestWorkingTreeCacheWatchesTheIndexAnywhere(t *testing.T) {
	r := must[*Repo](t)(OpenRepo(filepath.Join(t.TempDir(), "my fleet"), "", testName, testEmail))
	if len(r.gitFiles) != 2 || filepath.Base(r.gitFiles[0]) != "index" || filepath.Base(r.gitFiles[1]) != "exclude" || !strings.Contains(r.gitFiles[0], "my fleet") {
		t.Fatalf("git files %q", r.gitFiles)
	}
}

// A file changed moments ago may change again within the same timestamp, so
// the tree is not cached until it is older than the racy window.
func TestRecentChangesAreNotCached(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	settle(t)
	racyWindow = time.Hour
	write(t, r, "a.nix", "a\n")
	must[string](t)(r.WorkingTree(ctx))
	if _, hit := cachedTree(t, r); hit {
		t.Fatal("a tree with a file changed just now was cached")
	}
}
