package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeNix stands in for nix: it echoes its arguments, prints a.nix from any
// path: flake it is given, fails for targets named "broken", and answers the
// target discovery eval.
const fakeNix = `#!/bin/sh
case "$*" in *"eval --json"*) echo "{\"nixos\":[\"web\"],\"home\":[\"pablo@web\"],\"darwin\":[]}"; exit 0 ;; esac
echo "nix $*"
for a in "$@"; do
  case "$a" in
    path:*) d="${a#path:}"; d="${d%%#*}"; [ -f "$d/a.nix" ] && cat "$d/a.nix" ;;
  esac
done
case "$*" in *slow*) sleep 0.3 ;; esac
case "$*" in
  *broken*) echo "error: broken target" >&2; exit 1 ;;
esac
`

const fakeNixfmt = `#!/bin/sh
in=$(cat)
case "$in" in *"syntax-error"*) echo "<stdin>:1:3: unexpected end of input" >&2; exit 1 ;; esac
printf '%s\n' "formatted: $in"
`

func script(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	ok(t, os.WriteFile(p, []byte(body), 0o755))
	return p
}

func newService(t *testing.T) *Service {
	t.Helper()
	s := NewService(newRepo(t))
	s.Jobs.Nix = script(t, "nix", fakeNix)
	s.Jobs.Nixfmt = script(t, "nixfmt", fakeNixfmt)
	return s
}

func wait(t *testing.T, j *Job) JobInfo {
	t.Helper()
	select {
	case <-j.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("job did not finish")
	}
	return j.Info(true)
}

func TestCheckRunsOnStagedSnapshotAndGatesCommit(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	write(t, s.Repo, "a.nix", "staged content\n")
	ok(t, s.Stage(ctx, nil))
	write(t, s.Repo, "a.nix", "unstaged edit\n")

	_, err := s.Commit(ctx, "feat: a", false)
	isErr(t, err, ErrNotChecked)
	if st := must[Status](t)(s.Status(ctx)); st.StagedChecked {
		t.Fatal("staged tree reported as checked before any check")
	}

	j := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check", Targets: []string{"nixosConfigurations.web.config.system.build.toplevel"}}))
	info := wait(t, j)
	if info.State != "passed" {
		t.Fatalf("check failed:\n%s", info.Output)
	}
	for _, want := range []string{"flake check", "--no-update-lock-file", "build --dry-run", "#nixosConfigurations.web.config.system.build.toplevel", "staged content"} {
		if !strings.Contains(info.Output, want) {
			t.Fatalf("output lacks %q:\n%s", want, info.Output)
		}
	}
	if strings.Contains(info.Output, "unstaged edit") {
		t.Fatalf("check saw unstaged content:\n%s", info.Output)
	}
	if st := must[Status](t)(s.Status(ctx)); !st.StagedChecked {
		t.Fatal("staged tree not marked checked after a passing check")
	}
	must[string](t)(s.Commit(ctx, "feat: a", false))

	// New staged content needs a new check; force overrides.
	ok(t, s.Stage(ctx, nil))
	_, err = s.Commit(ctx, "feat: b", false)
	isErr(t, err, ErrNotChecked)
	must[string](t)(s.Commit(ctx, "feat: b", true))
}

// A restart (comin redeploying the console, say) must not forget which trees
// passed: commit and push would be refused until the checks ran again.
func TestPassedChecksSurviveARestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "checks.json")
	s := newService(t)
	ok(t, s.Jobs.PersistChecks(path))
	write(t, s.Repo, "a.nix", "x\n")
	ok(t, s.Stage(ctx, nil))
	check(t, s)
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("checks.json: %v %v", fi, err)
	}

	again := NewService(s.Repo)
	ok(t, again.Jobs.PersistChecks(path))
	if st := must[Status](t)(again.Status(ctx)); !st.StagedChecked {
		t.Fatal("the passed check was forgotten after a restart")
	}
	must[string](t)(again.Commit(ctx, "feat: a", false))

	ok(t, os.WriteFile(path, []byte(`{"passed":["`), 0o600)) // torn by hand: the writes themselves are atomic
	broken := NewService(s.Repo)
	broken.Jobs.Nix = s.Jobs.Nix
	ok(t, broken.Jobs.PersistChecks(path))
	write(t, s.Repo, "b.nix", "y\n")
	ok(t, s.Stage(ctx, nil))
	if st := must[Status](t)(broken.Status(ctx)); st.StagedChecked {
		t.Fatal("an unreadable file passed a tree")
	}
	check(t, broken)
	tree := must[Status](t)(broken.Status(ctx)).StagedTree
	if b := must[[]byte](t)(os.ReadFile(path)); !strings.Contains(string(b), tree) {
		t.Fatalf("the unreadable file was not replaced: %s", b)
	}
}

// A check that passed stays passed when the file cannot be written, and says so.
func TestPassedCheckThatCannotBeSaved(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	ok(t, s.Jobs.PersistChecks(filepath.Join(t.TempDir(), "gone", "checks.json")))
	write(t, s.Repo, "a.nix", "x\n")
	ok(t, s.Stage(ctx, nil))
	info := wait(t, must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check"})))
	if info.State != "passed" || !strings.Contains(info.Output, "could not be saved") {
		t.Fatalf("job %s:\n%s", info.State, info.Output)
	}
	if st := must[Status](t)(s.Status(ctx)); !st.StagedChecked {
		t.Fatal("the pass was lost with the file")
	}
}

// The list is capped; a tree that passes again counts as new.
func TestPassedChecksKeepTheNewest(t *testing.T) {
	s := newService(t)
	js := s.Jobs
	for i := range keepChecks + 1 {
		js.pass(fmt.Sprintf("%040x", i))
	}
	js.pass(fmt.Sprintf("%040x", 1))
	js.pass(fmt.Sprintf("%040x", keepChecks+1))
	if js.Passed(fmt.Sprintf("%040x", 0)) || js.Passed(fmt.Sprintf("%040x", 2)) {
		t.Fatal("the oldest passed trees were kept past the cap")
	}
	if !js.Passed(fmt.Sprintf("%040x", 1)) || !js.Passed(fmt.Sprintf("%040x", keepChecks+1)) || len(js.passedOrder) != keepChecks {
		t.Fatalf("kept %d trees; the newest are missing", len(js.passedOrder))
	}
}

func TestFailingCheckDoesNotPass(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	write(t, s.Repo, "a.nix", "x\n")
	ok(t, s.Stage(ctx, nil))
	j := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check", Targets: []string{"nixosConfigurations.broken.config.system.build.toplevel"}}))
	info := wait(t, j)
	if info.State != "failed" || !strings.Contains(info.Output, "error: broken target") {
		t.Fatalf("job %+v", info)
	}
	if st := must[Status](t)(s.Status(ctx)); st.StagedChecked {
		t.Fatal("failed check marked the tree as checked")
	}
}

func TestJobsRunOneAtATimeInOrder(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	a := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check", Targets: []string{"slow"}}))
	b := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if b.Info(false).State != "queued" {
		t.Fatalf("second job state %s, want queued", b.Info(false).State)
	}
	ai, bi := wait(t, a), wait(t, b)
	if bi.Started.Before(ai.Finished) {
		t.Fatalf("jobs overlapped: a finished %s, b started %s", ai.Finished, bi.Started)
	}
	list := s.Jobs.List()
	if len(list) != 2 || list[0].ID != b.ID || list[0].Output != "" {
		t.Fatalf("list %+v", list)
	}
	if got, ok := s.Jobs.Get(a.ID); !ok || got != a {
		t.Fatal("Get did not find the job")
	}
}

func TestLockAndUpdateCommands(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	info := wait(t, must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "update", Inputs: []string{"nixpkgs", "home-manager"}})))
	if !strings.Contains(info.Output, "flake update nixpkgs home-manager --flake path:"+s.Repo.Dir) {
		t.Fatalf("update output:\n%s", info.Output)
	}
	info = wait(t, must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"})))
	if !strings.Contains(info.Output, "flake lock path:"+s.Repo.Dir) {
		t.Fatalf("lock output:\n%s", info.Output)
	}
	for _, bad := range []JobRequest{{Kind: "update", Inputs: []string{"--override-input"}}, {Kind: "rm"}, {Kind: "check", Targets: []string{"a\nb"}}} {
		_, err := s.Jobs.Submit(ctx, bad)
		isErr(t, err, ErrInvalid)
	}
}

func TestTargets(t *testing.T) {
	s := newService(t)
	got := must[[]Target](t)(s.Jobs.Targets(context.Background()))
	want := []Target{
		{Kind: "nixos", Name: "web", Attr: "nixosConfigurations.web.config.system.build.toplevel"},
		{Kind: "home", Name: "pablo@web", Attr: `homeConfigurations."pablo@web".activationPackage`},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("targets %+v", got)
	}
}

func TestFormat(t *testing.T) {
	s := newService(t)
	out := must[string](t)(s.Jobs.Format(context.Background(), "{ }"))
	if out != "formatted: { }\n" {
		t.Fatalf("format = %q", out)
	}
	_, err := s.Jobs.Format(context.Background(), "{ syntax-error")
	isErr(t, err, ErrSyntax)
	if !strings.Contains(err.Error(), "unexpected end of input") {
		t.Fatalf("nixfmt message lost: %v", err)
	}
}

func TestJobReadStreams(t *testing.T) {
	j := newJob("j1", JobRequest{Kind: "lock"})
	j.Write([]byte("hello "))
	chunk, done, waitCh := j.Read(0)
	if string(chunk) != "hello " || done {
		t.Fatalf("first read %q %v", chunk, done)
	}
	go func() { j.Write([]byte("world")); j.finish(true) }()
	<-waitCh
	chunk, _, _ = j.Read(6)
	<-j.Done()
	rest, done, _ := j.Read(6 + len(chunk))
	if string(chunk)+string(rest) != "world" || !done {
		t.Fatalf("second read %q+%q %v", chunk, rest, done)
	}
}

func check(t *testing.T, s *Service) {
	t.Helper()
	if info := wait(t, must[*Job](t)(s.Jobs.Submit(context.Background(), JobRequest{Kind: "check"}))); info.State != "passed" {
		t.Fatalf("check failed:\n%s", info.Output)
	}
}

// A pull that rebases local commits produces a tree nobody checked; it must
// not reach the remote (and comin) until it is checked or the push is forced.
func TestPushNeedsACheckedHead(t *testing.T) {
	ctx := context.Background()
	r, other := remotePair(t)
	s := NewService(r)
	s.Jobs.Nix = script(t, "nix", fakeNix)

	write(t, r, "c.nix", "c\n")
	ok(t, s.Stage(ctx, nil))
	check(t, s)
	must[string](t)(s.Commit(ctx, "add c", false))
	must[string](t)(s.Push(ctx, false)) // checked commit: allowed

	gitOut(t, other, "pull", "-q")
	otherCommit(t, other, "d.nix", "d\n")
	must[string](t)(s.Pull(ctx)) // fast-forward: nothing of ours to push
	must[string](t)(s.Push(ctx, false))

	otherCommit(t, other, "e.nix", "e\n")
	write(t, r, "f.nix", "f\n")
	ok(t, s.Stage(ctx, nil))
	check(t, s)
	must[string](t)(s.Commit(ctx, "add f", false))
	must[string](t)(s.Pull(ctx)) // rebases "add f" onto e.nix: a new, unchecked tree
	_, err := s.Push(ctx, false)
	isErr(t, err, ErrNotChecked)
	if gitOut(t, other, "ls-remote", "origin", "main") == gitOut(t, r.Dir, "rev-parse", "HEAD") {
		t.Fatal("the unchecked rebase reached the remote")
	}
	check(t, s) // clean working copy: the staged tree is HEAD's tree
	must[string](t)(s.Push(ctx, false))

	write(t, r, "g.nix", "g\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "add g unchecked", true))
	_, err = s.Push(ctx, false)
	isErr(t, err, ErrNotChecked)
	must[string](t)(s.Push(ctx, true))
}

// Without a remote there is nothing to push, checked or not: say that first.
func TestPushWithoutRemoteSaysSoBeforeTheCheckGate(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	write(t, s.Repo, "a.nix", "1\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "unchecked", true))
	_, err := s.Push(ctx, false)
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "remote") {
		t.Fatalf("message %q", err)
	}
}
