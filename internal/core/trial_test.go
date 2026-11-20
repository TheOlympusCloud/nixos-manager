package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trialService is buildService with what is pushed checked, and the working
// copy's model evaluated, as the UI has it by the time a change is checked.
func trialService(t *testing.T) *Service {
	t.Helper()
	s := buildService(t, true)
	if info := runCheck(t, s); info.State != "passed" {
		t.Fatalf("check of what is pushed:\n%s", info.Output)
	}
	return s
}

// stageChecked writes files, stages everything and checks it.
func stageChecked(t *testing.T, s *Service, files ...string) {
	t.Helper()
	for i := 0; i+1 < len(files); i += 2 {
		write(t, s.Repo, files[i], files[i+1])
	}
	if info := runCheck(t, s); info.State != "passed" {
		t.Fatalf("check:\n%s", info.Output)
	}
	readyModel(t, s, "working")
}

func remoteRef(t *testing.T, s *Service, ref string) string {
	t.Helper()
	out, err := s.Repo.git(context.Background(), "ls-remote", "origin", "refs/heads/"+ref)
	ok(t, err)
	sha, _, _ := strings.Cut(strings.TrimSpace(out), "\t")
	return sha
}

func branchOf(t *testing.T, s *Service) string {
	t.Helper()
	return must[Branches](t)(s.Branches(context.Background())).Current
}

// The staged changes go in a commit on the machine's testing branch, on top of
// main, which is pushed; the console then works on that branch.
func TestTryOnPushesTheStagedChangesOnTopOfMain(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	main := remoteRef(t, s, "main")
	stageChecked(t, s, "hosts/web", "web kernel=2\n")

	tr, err := s.TryOn(WithCaller(ctx, Caller{Via: "web session abcd1234", Person: true}), "web", "web: try kernel 2", false)
	ok(t, err)
	if tr.Host != "web" || tr.Branch != "testing-web" || tr.Main != "main" || tr.Commit == "" {
		t.Fatalf("trial %+v", tr)
	}
	if got := remoteRef(t, s, "testing-web"); got != tr.Commit {
		t.Fatalf("remote testing-web is %q, want %q", got, tr.Commit)
	}
	if parent := gitOut(t, s.Repo.Dir, "rev-parse", tr.Commit+"^"); parent != main {
		t.Fatalf("the trial's parent is %s, want main %s", parent, main)
	}
	if msg := gitOut(t, s.Repo.Dir, "log", "-1", "--format=%s|%an|%(trailers:key=Via,valueonly)", tr.Commit); msg != "web: try kernel 2|"+testName+"|web session abcd1234" {
		t.Fatalf("commit %q", msg)
	}
	if b := branchOf(t, s); b != "testing-web" {
		t.Fatalf("the console is on %q", b)
	}
	if st := must[Status](t)(s.Status(ctx)); len(st.Files) != 0 {
		t.Fatalf("changes left: %+v", st.Files)
	}
	if got, local := remoteRef(t, s, "main"), gitOut(t, s.Repo.Dir, "rev-parse", "main"); got != main || local != main {
		t.Fatalf("main moved: remote %s, local %s, want %s", got, local, main)
	}
	if up := gitOut(t, s.Repo.Dir, "rev-parse", "--abbrev-ref", "testing-web@{upstream}"); up != "origin/testing-web" {
		t.Fatalf("upstream %q", up)
	}
}

// Commits made on main but not pushed can be tried as they are.
func TestTryOnUnpushedCommits(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	stageChecked(t, s, "hosts/web", "web kernel=2\n")
	sha := must[string](t)(s.Commit(ctx, "web: kernel 2", false))

	tr, err := s.TryOn(ctx, "web", "", false)
	ok(t, err)
	if tr.Commit != sha || remoteRef(t, s, "testing-web") != sha || branchOf(t, s) != "testing-web" {
		t.Fatalf("trial %+v, remote %s, want %s", tr, remoteRef(t, s, "testing-web"), sha)
	}
}

func TestTryOnRefuses(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	ok(t, s.Stage(ctx, nil))
	readyModel(t, s, "working")
	_, err := s.TryOn(ctx, "web", "try", false)
	isErr(t, err, ErrNotChecked)

	stageChecked(t, s, "hosts/web", "web kernel=2\n", "hosts/off", "off nocomin\n", "hosts/notest", "notest testing=\n")
	for _, c := range []struct {
		host, msg string
		kind      error
		why       string
	}{
		{"db", "try", ErrInvalid, "does not change db"},
		{"off", "try", ErrInvalid, "comin"},
		{"notest", "try", ErrInvalid, "testing branch"},
		{"nope", "try", ErrNotFound, "nope"},
		{"web", " ", ErrInvalid, "message"},
	} {
		_, err := s.TryOn(ctx, c.host, c.msg, false)
		isErr(t, err, c.kind)
		if !strings.Contains(err.Error(), c.why) {
			t.Fatalf("%s: %v, want it to say %q", c.host, err, c.why)
		}
	}

	must[string](t)(s.Commit(ctx, "web: kernel 2", false))
	ok(t, s.Repo.CreateBranch(ctx, "elsewhere", ""))
	_, err = s.TryOn(ctx, "web", "", false)
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "main") {
		t.Fatalf("not on main: %v", err)
	}
	ok(t, s.Switch(ctx, "main"))
	gitOut(t, s.Repo.Dir, "reset", "-q", "--hard", "origin/main")
	_, err = s.TryOn(ctx, "web", "", false)
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "nothing to try") || remoteRef(t, s, "testing-web") != "" {
		t.Fatalf("nothing to try: %v", err)
	}
}

// comin only deploys a testing branch on top of main as it is on the remote:
// a main that moved since the last fetch is a pull first, and nothing is pushed.
func TestTryOnNeedsTheLatestMain(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	otherCommit(t, laptop(t, s), "notes", "from the laptop\n")
	stageChecked(t, s, "hosts/web", "web kernel=2\n")

	_, err := s.TryOn(ctx, "web", "try", false)
	isErr(t, err, ErrConflict)
	if !strings.Contains(err.Error(), "pull") || remoteRef(t, s, "testing-web") != "" || branchOf(t, s) != "main" {
		t.Fatalf("got %v, remote testing-web %q, on %s", err, remoteRef(t, s, "testing-web"), branchOf(t, s))
	}
}

// A testing branch with changes main does not have is only replaced when asked;
// one whose changes main has (promoted) is replaced without a question.
func TestTryOnAsksBeforeDroppingATrial(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	clone := laptop(t, s)
	gitOut(t, clone, "checkout", "-q", "-b", "testing-web")
	ok(t, os.WriteFile(filepath.Join(clone, "hosts/web"), []byte("web kernel=9\n"), 0o644))
	gitOut(t, clone, "commit", "-qam", "laptop: kernel 9")
	gitOut(t, clone, "push", "-q", "origin", "testing-web")
	old := remoteRef(t, s, "testing-web")
	stageChecked(t, s, "hosts/web", "web kernel=2\n")

	_, err := s.TryOn(ctx, "web", "try", false)
	isErr(t, err, ErrConflict)
	if !strings.Contains(err.Error(), "replace") || !strings.Contains(err.Error(), old[:7]) || remoteRef(t, s, "testing-web") != old {
		t.Fatalf("got %v", err)
	}
	tr, err := s.TryOn(ctx, "web", "try", true)
	ok(t, err)
	if remoteRef(t, s, "testing-web") != tr.Commit {
		t.Fatal("not replaced")
	}

	// Promote it: main gets the change, so the next trial replaces it quietly.
	ok(t, s.Switch(ctx, "main"))
	msg := must[string](t)(s.Merge(ctx, "testing-web"))
	must[string](t)(s.Commit(ctx, msg, false))
	must[string](t)(s.Push(ctx, false))
	stageChecked(t, s, "hosts/web", "web kernel=3\n")
	_, err = s.TryOn(ctx, "web", "kernel 3", false)
	ok(t, err)

	// A local commit on the testing branch that was never pushed counts too.
	ok(t, s.Switch(ctx, "main"))
	gitOut(t, s.Repo.Dir, "branch", "-f", "testing-db", "main")
	gitOut(t, s.Repo.Dir, "switch", "-q", "testing-db")
	write(t, s.Repo, "notes", "local only\n")
	commitAll(t, s.Repo, "local only")
	gitOut(t, s.Repo.Dir, "switch", "-q", "main")
	stageChecked(t, s, "hosts/db", "db v2\n")
	_, err = s.TryOn(ctx, "db", "db v2", false)
	isErr(t, err, ErrConflict)
}

// When the push fails, everything is as it was: on main, the same staged
// changes, the local testing branch where it was.
func TestTryOnLeavesEverythingWhenThePushFails(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	gitOut(t, s.Repo.Dir, "branch", "testing-web", "main")
	before := gitOut(t, s.Repo.Dir, "rev-parse", "testing-web")
	bare := s.Repo.Remote(ctx)
	hook := filepath.Join(bare, "hooks", "pre-receive")
	ok(t, os.WriteFile(hook, []byte("#!/bin/sh\necho refused by the test >&2\nexit 1\n"), 0o755))
	stageChecked(t, s, "hosts/web", "web kernel=2\n")
	staged := must[Status](t)(s.Status(ctx)).StagedTree

	if _, err := s.TryOn(ctx, "web", "try", false); err == nil || !strings.Contains(err.Error(), "refused by the test") {
		t.Fatalf("got %v", err)
	}
	st := must[Status](t)(s.Status(ctx))
	if branchOf(t, s) != "main" || st.StagedTree != staged || !st.StagedChecked {
		t.Fatalf("on %s, staged %s (want %s), checked %v", branchOf(t, s), st.StagedTree, staged, st.StagedChecked)
	}
	if got := gitOut(t, s.Repo.Dir, "rev-parse", "testing-web"); got != before {
		t.Fatalf("local testing-web moved to %s", got)
	}
	gitOut(t, s.Repo.Dir, "branch", "-D", "testing-web")
	if _, err := s.TryOn(ctx, "web", "try", false); err == nil {
		t.Fatal("the push should fail again")
	}
	if out := gitOut(t, s.Repo.Dir, "branch", "--list", "testing-web"); out != "" {
		t.Fatalf("a failed trial left the branch %q", out)
	}
	ok(t, os.Remove(hook))
	_, err := s.TryOn(ctx, "web", "try", false)
	ok(t, err)
}

// Promote to main is the existing merge: after one trial, the merge stages the
// tree that passed, so commit and push need no new check.
func TestPromoteATrial(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	stageChecked(t, s, "hosts/web", "web kernel=2\n")
	tr := must[Trial](t)(s.TryOn(ctx, "web", "try", false))

	ok(t, s.Switch(ctx, "main"))
	msg := must[string](t)(s.Merge(ctx, tr.Branch))
	if st := must[Status](t)(s.Status(ctx)); st.StagedTree != treeOf(t, s, tr.Commit) || !st.StagedChecked {
		t.Fatalf("staged %+v", st)
	}
	must[string](t)(s.Commit(ctx, msg, false))
	must[string](t)(s.Push(ctx, false))
	if treeOf(t, s, "origin/main") != treeOf(t, s, tr.Commit) {
		t.Fatal("main does not have the trial")
	}
}

// Ending a trial puts the testing branch back on main's last commit, which
// comin deploys; deleting it would not do, as comin's clone keeps it.
func TestEndTrial(t *testing.T) {
	ctx := context.Background()
	s := trialService(t)
	readyModel(t, s, "working")
	if msg, err := s.EndTrial(ctx, "web"); err != nil || !strings.Contains(msg, "follows main") || remoteRef(t, s, "testing-web") != "" {
		t.Fatalf("no trial: %q, %v", msg, err)
	}
	stageChecked(t, s, "hosts/web", "web kernel=2\n")
	must[Trial](t)(s.TryOn(ctx, "web", "try", false))

	msg, err := s.EndTrial(ctx, "web")
	ok(t, err)
	if main := remoteRef(t, s, "main"); remoteRef(t, s, "testing-web") != main || !strings.Contains(msg, "main") {
		t.Fatalf("%q: testing-web %s, main %s", msg, remoteRef(t, s, "testing-web"), main)
	}
	if branchOf(t, s) != "testing-web" {
		t.Fatal("ending a trial does not move the working copy")
	}
	if msg, err := s.EndTrial(ctx, "web"); err != nil || !strings.Contains(msg, "follows main") {
		t.Fatalf("second end: %q, %v", msg, err)
	}
	_, err = s.EndTrial(ctx, "nope")
	isErr(t, err, ErrNotFound)
}
