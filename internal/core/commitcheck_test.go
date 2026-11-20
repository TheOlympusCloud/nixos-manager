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
