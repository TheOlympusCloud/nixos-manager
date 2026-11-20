package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// systemsNix answers the system evaluation like nix eval --raw would, logs
// every call, and fails for a machine named broken.
const systemsNix = `#!/bin/sh
echo "$*" >> "$NIXMGR_TEST_LOG"
for a in "$@"; do
  case "$a" in
    git+file://*'#nixosConfigurations.broken.'*) echo "error: attribute 'foo' missing" >&2; exit 1 ;;
    git+file://*'#nixosConfigurations.'*)
      host=${a#*#nixosConfigurations.}; host=${host%%.*}
      rev=${a#*rev=}; rev=${rev%%#*}
      printf '/nix/store/0123456789abcdfghijklmnpqrsvwxyz-nixos-system-%s-%s' "$host" "$rev"; exit 0 ;;
  esac
done
exit 1
`

// waitSystems asks until every host has a result, as the Deployments view polls.
func waitSystems(t *testing.T, js *Jobs, commit string, hosts []string, limit time.Duration) map[string]SystemEval {
	t.Helper()
	deadline := time.Now().Add(limit)
	for {
		got := js.ExpectedSystems(commit, hosts)
		if len(got) == len(hosts) {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("no system for %v after %v: %v\n%v", hosts, limit, got, js.List())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The system a machine builds at a commit is evaluated once, in its own lane,
// from the commit as comin sees it; a failure is kept, then tried again later.
func TestExpectedSystems(t *testing.T) {
	s := newService(t)
	log := filepath.Join(t.TempDir(), "log")
	t.Setenv("NIXMGR_TEST_LOG", log)
	s.Jobs.Nix = script(t, "nix", systemsNix)
	write(t, s.Repo, "flake.nix", "{ }\n")
	c := commitAll(t, s.Repo, "first")

	if got := s.Jobs.ExpectedSystems(c, []string{"web", "broken"}); len(got) != 0 {
		t.Fatalf("before the evaluation: %v", got)
	}
	s.Jobs.ExpectedSystems(c, []string{"web", "broken"}) // already queued: no second job
	got := waitSystems(t, s.Jobs, c, []string{"web", "broken"}, 10*time.Second)
	if w := got["web"]; w.Path != "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-nixos-system-web-"+c || w.Error != "" {
		t.Errorf("web: %+v", w)
	}
	if b := got["broken"]; b.Path != "" || !strings.Contains(b.Error, "attribute 'foo' missing") {
		t.Errorf("broken: %+v", b)
	}
	jobs := slices.DeleteFunc(s.Jobs.List(), func(j JobInfo) bool { return j.Kind != "system" })
	if len(jobs) != 1 || jobs[0].Tree != c {
		t.Fatalf("system jobs: %+v", jobs)
	}
	calls := strings.Split(strings.TrimSpace(fileOf(t, log)), "\n")
	ref := "git+file://" + s.Repo.Dir + "?rev=" + c + "#nixosConfigurations.web.config.system.build.toplevel.outPath"
	if len(calls) != 2 || !strings.Contains(calls[0]+calls[1], "eval --raw "+ref) {
		t.Fatalf("nix calls: %q", calls)
	}

	// Kept: asking again evaluates nothing, until a failure is an hour old.
	s.Jobs.ExpectedSystems(c, []string{"web", "broken"})
	s.Jobs.mu.Lock()
	b := s.Jobs.systems[systemKey(c, "broken")]
	b.At = b.At.Add(-2 * time.Hour)
	s.Jobs.systems[systemKey(c, "broken")] = b
	s.Jobs.mu.Unlock()
	if got := s.Jobs.ExpectedSystems(c, []string{"web", "broken"}); len(got) != 1 || got["web"].Path == "" {
		t.Fatalf("an old failure is not tried again: %v", got)
	}
	waitSystems(t, s.Jobs, c, []string{"web", "broken"}, 10*time.Second)
	if n := len(strings.Split(strings.TrimSpace(fileOf(t, log)), "\n")); n != 3 {
		t.Fatalf("%d nix calls, want 3 (web once, broken twice)", n)
	}
	for _, bad := range []string{"HEAD", "../x", strings.Repeat("g", 40)} {
		if got := s.Jobs.ExpectedSystems(bad, []string{"web"}); len(got) != 0 {
			t.Errorf("commit %q: %v", bad, got)
		}
	}
}

// The console evaluates a deployed commit to exactly the system comin builds
// from its own clone, even when the configuration records the commit
// (configurationRevision from self.rev), and for a commit it only has as a
// remote branch, pushed from elsewhere.
func TestExpectedSystemIsWhatCominBuildsWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	def := fileText(t, s.Repo, "hosts/example/default.nix")
	write(t, s.Repo, "hosts/example/default.nix", strings.Replace(strings.Replace(def, "{ ... }:", "{ inputs, ... }:", 1),
		"system.stateVersion", "system.configurationRevision = inputs.self.rev or \"dirty\";\n  system.stateVersion", 1))
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if info := wait40(t, lock); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "fleet", true))
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, s.Repo.Dir, "init", "-q", "--bare", "-b", "main", bare)
	gitOut(t, s.Repo.Dir, "remote", "add", "origin", bare)
	must[string](t)(s.Push(ctx, true))

	// A commit pushed from a laptop: the console only fetches it.
	laptop := filepath.Join(t.TempDir(), "laptop")
	gitOut(t, s.Repo.Dir, "clone", "-q", bare, laptop)
	ok(t, os.WriteFile(filepath.Join(laptop, "hosts", "example", "default.nix"), []byte(strings.Replace(fileOf(t, filepath.Join(laptop, "hosts", "example", "default.nix")),
		"system.stateVersion", "boot.kernel.sysctl.\"vm.swappiness\" = 10;\n  system.stateVersion", 1)), 0o644))
	gitOut(t, laptop, "-c", "user.name=Pablo", "-c", "user.email=p@example.invalid", "commit", "-q", "-am", "swappiness")
	gitOut(t, laptop, "push", "-q", "origin", "main")
	gitOut(t, s.Repo.Dir, "fetch", "-q", "origin")
	tip := strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", "origin/main"))

	got := waitSystems(t, s.Jobs, tip, []string{"example"}, 40*time.Minute)["example"]
	if got.Error != "" {
		t.Fatalf("evaluating %s: %s", tip, got.Error)
	}
	// What comin evaluates on the machine: its own clone of the remote, at that commit.
	comin := filepath.Join(t.TempDir(), "comin")
	gitOut(t, s.Repo.Dir, "clone", "-q", bare, comin)
	attr := "#nixosConfigurations.example.config.system.build.toplevel.outPath"
	want := nixRaw(t, "git+file://"+comin+"?rev="+tip+attr)
	if got.Path != want {
		t.Fatalf("the console expects %s, comin builds %s", got.Path, want)
	}
	// The same files as a path: flake have no revision, so they build another system:
	// that is why the console evaluates the commit, not a copy of its files.
	src := filepath.Join(t.TempDir(), "src")
	ok(t, s.Repo.export(ctx, strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", tip+"^{tree}")), src))
	if other := nixRaw(t, "path:"+src+attr); other == want {
		t.Fatalf("configurationRevision did not change the system: %s", other)
	}
}

func wait40(t *testing.T, j *Job) JobInfo {
	t.Helper()
	select {
	case <-j.Done():
	case <-time.After(40 * time.Minute):
		t.Fatal("the job did not finish")
	}
	return j.Info(true)
}

func nixRaw(t *testing.T, ref string) string {
	t.Helper()
	cmd := exec.Command("nix", "--extra-experimental-features", "nix-command flakes", "eval", "--raw", ref)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("nix eval %s: %v\n%s", ref, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}
