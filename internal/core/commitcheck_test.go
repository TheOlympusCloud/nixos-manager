package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// laptop is a clone of s's remote, where commits are made and pushed as from
// another computer. The service has fetched none of them yet.
func laptop(t *testing.T, s *Service) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "laptop")
	gitOut(t, t.TempDir(), "clone", "-q", s.Repo.Remote(context.Background()), dir) // after insteadOf
	gitOut(t, dir, "config", "user.name", "Laptop")
	gitOut(t, dir, "config", "user.email", "laptop@example.invalid")
	return dir
}

// pushFrom commits content to file in a clone, pushes it, lets the service
// fetch it, and returns the commit.
func pushFrom(t *testing.T, s *Service, clone, file, content string) string {
	t.Helper()
	otherCommit(t, clone, file, content)
	gitOut(t, s.Repo.Dir, "fetch", "-q", "origin")
	return gitOut(t, clone, "rev-parse", "HEAD")
}

func treeOf(t *testing.T, s *Service, rev string) string {
	t.Helper()
	return gitOut(t, s.Repo.Dir, "rev-parse", rev+"^{tree}")
}

// blockingNix runs buildNix, but holds nix flake check while the file it
// returns exists.
func blockingNix(t *testing.T) (string, string) {
	t.Helper()
	block := filepath.Join(t.TempDir(), "block")
	ok(t, os.WriteFile(block, nil, 0o644))
	inner := script(t, "inner-nix", buildNix)
	return script(t, "nix", "#!/bin/sh\ncase \"$*\" in *\"flake check\"*) while [ -f '"+block+"' ]; do sleep 0.05; done ;; esac\nexec '"+inner+"' \"$@\"\n"), block
}

// A pushed commit is compared with the newest commit before it that passed a
// check, so every machine changed since then is built, by however many commits.
func TestCheckCommitComparesWithTheLastCheckedAncestor(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	if info := runCheck(t, s); info.State != "passed" { // what is pushed now passes
		t.Fatalf("check:\n%s", info.Output)
	}
	checked := treeOf(t, s, "origin/main")
	clone := laptop(t, s)
	pushFrom(t, s, clone, "hosts/web", "web kernel=2\n")
	sha := pushFrom(t, s, clone, "notes", "only notes\n")

	job, pass, err := s.Jobs.CheckCommit(ctx, sha, "main")
	if err != nil || pass != nil || job == nil {
		t.Fatalf("CheckCommit: %v %+v %v", job, pass, err)
	}
	info := wait(t, job)
	if info.State != "passed" || info.Commit != sha || info.Branch != "main" || info.Tree != treeOf(t, s, sha) {
		t.Fatalf("check %+v:\n%s", info, info.Output)
	}
	if sum := info.Summary; sum.Base != checked || !slices.Equal(sum.Built, []string{"web"}) {
		t.Fatalf("summary %+v", sum)
	}
	for _, want := range []string{"Checking commit " + sha[:12], "pushed to main", "the newest commit before it that passed a check"} {
		if !strings.Contains(info.Output, want) {
			t.Errorf("output lacks %q:\n%s", want, info.Output)
		}
	}
	if p, ok := s.Jobs.PassOf(info.Tree); !ok || p.Base != checked {
		t.Fatalf("pass %+v %v", p, ok)
	}
}

// With no checked commit before it, every machine is built.
func TestCheckCommitBuildsEverythingWithoutACheckedAncestor(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true) // pushed without a check
	sha := pushFrom(t, s, laptop(t, s), "hosts/web", "web kernel=2\n")
	job, _, err := s.Jobs.CheckCommit(ctx, sha, "main")
	ok(t, err)
	info := wait(t, job)
	if info.State != "passed" || info.Summary.Base != "" || !slices.Equal(info.Summary.Built, []string{"db", "web"}) ||
		!strings.Contains(info.Output, "No commit before it passed a check, so every machine is built.") {
		t.Fatalf("check %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
}

// A commit whose tree passed a check (a push from the console) is not checked
// again; the API refuses to.
func TestCheckCommitOfACheckedTreeIsNotRunAgain(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	info := runCheck(t, s)
	sha := gitOut(t, s.Repo.Dir, "rev-parse", "origin/main")
	job, pass, err := s.Jobs.CheckCommit(ctx, sha, "main")
	if err != nil || job != nil || pass == nil || pass.Tree != info.Tree {
		t.Fatalf("CheckCommit: %v %+v %v", job, pass, err)
	}
	_, err = s.Jobs.Submit(ctx, JobRequest{Kind: "check", Commit: sha})
	isErr(t, err, ErrConflict)
	if state, j := s.Jobs.CommitState(ctx, sha); state != "passed" || j != "" {
		t.Fatalf("state %q %q", state, j)
	}
}

// A second request for a commit under check (a webhook delivered twice, the
// commit pushed to two branches, the Check button) joins that check. A failed
// check is run again when asked.
func TestCheckCommitJoinsTheRunningCheck(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	nix, block := blockingNix(t)
	s.Jobs.Nix = nix
	sha := pushFrom(t, s, laptop(t, s), "hosts/db", "db broken\n")
	if state, j := s.Jobs.CommitState(ctx, sha); state != "unchecked" || j != "" {
		t.Fatalf("before: %q %q", state, j)
	}

	first := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check", Commit: sha}))
	second, _, err := s.Jobs.CheckCommit(ctx, sha, "testing-db")
	if err != nil || second != first {
		t.Fatalf("second request: %v %v, want %v", second, err, first)
	}
	if state, j := s.Jobs.CommitState(ctx, sha); (state != "queued" && state != "running") || j != first.ID {
		t.Fatalf("while checked: %q %q", state, j)
	}
	ok(t, os.Remove(block))
	if info := wait(t, first); info.State != "failed" || info.Commit != sha || info.Branch != "" {
		t.Fatalf("check %+v:\n%s", info, info.Output)
	}
	if state, j := s.Jobs.CommitState(ctx, sha); state != "failed" || j != first.ID {
		t.Fatalf("after: %q %q", state, j)
	}

	again, _, err := s.Jobs.CheckCommit(ctx, sha, "main")
	if err != nil || again == nil || again == first {
		t.Fatalf("again: %v %v", again, err)
	}
	wait(t, again)
}

func TestCheckCommitRefusesBadInput(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	sha := gitOut(t, s.Repo.Dir, "rev-parse", "origin/main")
	for _, bad := range []string{"", sha[:12], strings.ToUpper(sha), "-" + sha[1:], "origin/main", strings.Repeat("z", 40)} {
		_, _, err := s.Jobs.CheckCommit(ctx, bad, "main")
		isErr(t, err, ErrInvalid)
	}
	_, _, err := s.Jobs.CheckCommit(ctx, strings.Repeat("0", 40), "main")
	isErr(t, err, ErrNotFound)
	_, err = s.Jobs.Submit(ctx, JobRequest{Kind: "check", Commit: sha, Targets: []string{systemAttr("web")}})
	isErr(t, err, ErrInvalid)
	if state, _ := s.Jobs.CommitState(ctx, "nonsense"); state != "unchecked" {
		t.Fatalf("state of nonsense %q", state)
	}
}

// The tip is the branch as comin sees it: on origin, or without a remote, the
// local branch.
func TestTipCheck(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true) // pushed without a check
	tip := s.TipCheck(ctx, "main")
	if tip == nil || tip.State != "unchecked" || tip.Commit != gitOut(t, s.Repo.Dir, "rev-parse", "origin/main") || tip.Subject != "machines" || tip.Branch != "main" {
		t.Fatalf("tip %+v", tip)
	}
	runCheck(t, s)
	if tip := s.TipCheck(ctx, "main"); tip.State != "passed" {
		t.Fatalf("after the check: %+v", tip)
	}
	if tip := s.TipCheck(ctx, "elsewhere"); tip != nil {
		t.Fatalf("a missing branch: %+v", tip)
	}
	if tip := s.TipCheck(ctx, "--output=/tmp/x"); tip != nil {
		t.Fatalf("an option as branch: %+v", tip)
	}

	local := buildService(t, false)
	if tip := local.TipCheck(ctx, "main"); tip == nil || tip.Commit != gitOut(t, local.Repo.Dir, "rev-parse", "main") || tip.State != "unchecked" {
		t.Fatalf("without a remote: %+v", tip)
	}
	if tip := NewService(newRepo(t)).TipCheck(ctx, "main"); tip != nil {
		t.Fatalf("no commit yet: %+v", tip)
	}
}

// A check of the staged changes compares with the newest pushed commit that
// passed a check, not with what is pushed: on top of a pushed commit that broke
// db, an edit of web builds db too, and fails.
func TestStagedCheckOnTopOfAFailedCommitBuildsItsBrokenMachine(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	checked := runCheck(t, s).Tree // what is pushed passes
	sha := pushFrom(t, s, laptop(t, s), "hosts/db", "db broken\n")
	job, _, err := s.Jobs.CheckCommit(ctx, sha, "main")
	ok(t, err)
	if info := wait(t, job); info.State != "failed" {
		t.Fatalf("the pushed commit's check %s:\n%s", info.State, info.Output)
	}
	must[string](t)(s.Repo.Pull(ctx))

	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	info := runCheck(t, s)
	if info.State != "failed" || info.Summary.Base != checked || !slices.Equal(info.Summary.Changed, []string{"db", "web"}) ||
		!slices.Equal(info.Summary.Failed, []string{"db"}) || !strings.Contains(info.Output, "the newest pushed commit that passed a check") {
		t.Fatalf("check on top of the broken commit %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
	must[string](t)(s.Commit(ctx, "web: new kernel", true))
	_, err = s.Push(ctx, false)
	isErr(t, err, ErrNotChecked)
	if tip := s.TipCheck(ctx, "main"); tip.Commit != sha || tip.State != "failed" {
		t.Fatalf("tip %+v", tip)
	}

	// Once db is fixed, the check passes and the push goes through.
	write(t, s.Repo, "hosts/db", "db\n")
	if info := runCheck(t, s); info.State != "passed" || !slices.Equal(info.Summary.Built, []string{"web"}) {
		t.Fatalf("check with db fixed %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
	must[string](t)(s.Commit(ctx, "db: fix it", false))
	must[string](t)(s.Push(ctx, false))
	if tip := s.TipCheck(ctx, "main"); tip.State != "passed" {
		t.Fatalf("tip after the fix %+v", tip)
	}
}
