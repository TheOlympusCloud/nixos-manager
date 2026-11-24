package core

import (
	"context"
	"strings"
)

// Service is the one entry point for the web API and the MCP server, so both
// get the same rules: the commit gate, path checks and the commit identity.
type Service struct {
	*Repo
	Jobs    *Jobs
	Deploys *Deployments // what the machines report after comin deploys; nil turns reports off
	Secrets *Secrets     // the fleet's sops-nix secrets; nil turns them off
	Updates *Updates     // scheduled flake input updates; nil turns them off
	ISOs    *ISOs        // installer ISOs; nil turns them off
}

func NewService(repo *Repo) *Service {
	return &Service{Repo: repo, Jobs: newJobs(repo)}
}

// Caller is who asks for a change. The web API and the MCP server set it on
// every request; the server's own work has none.
type Caller struct {
	Via    string // recorded in each commit's Via trailer, e.g. "token ci (K3J9QW2M4X1A)"
	Person bool   // the person logged in to the web UI; only they may skip a check
}

type callerKey struct{}

func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

func callerOf(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

// mayForce refuses to skip a check for anyone but the person in the web UI.
func mayForce(ctx context.Context, what string) error {
	if c, ok := callerOf(ctx); ok && !c.Person {
		return errorf(ErrForbidden, "only the person logged in to the web UI can %s without a check", what)
	}
	return nil
}

// Status adds whether the staged changes passed a check, and what it built.
func (s *Service) Status(ctx context.Context) (Status, error) {
	st, err := s.Repo.Status(ctx)
	if p, ok := s.Jobs.PassOf(st.StagedTree); err == nil && ok {
		st.StagedChecked, st.StagedCheck = true, &p
	}
	return st, err
}

// Commit commits the staged changes. Unless force is set they must have passed
// a check first, so nothing unvalidated reaches the machines by accident.
func (s *Service) Commit(ctx context.Context, message string, force bool) (string, error) {
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	tree, err := s.Repo.stagedTree(ctx)
	if err != nil {
		return "", err
	}
	if force {
		if err := mayForce(ctx, "commit"); err != nil {
			return "", err
		}
	} else if !s.Jobs.Passed(tree) {
		return "", errorf(ErrNotChecked, "the staged changes have not passed a check yet: run the checks first, or commit without checks")
	}
	return s.Repo.commit(ctx, message)
}

// Push sends the current branch to origin. Unless force is set, the commit
// being pushed must have a checked tree: a pull that rebased local commits
// creates a combination nobody checked, and comin would deploy it. The check
// must also have built every machine the push changes: those whose system
// differs from what the push lands on, the comparison it made.
func (s *Service) Push(ctx context.Context, force bool) (string, error) {
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	if _, err := s.Repo.currentBranch(ctx); err != nil { // no remote: nothing to push, checked or not
		return "", err
	}
	if force {
		if err := mayForce(ctx, "push"); err != nil {
			return "", err
		}
	} else {
		if n, err := s.Repo.git(ctx, "rev-list", "--count", "@{upstream}..HEAD"); err == nil && strings.TrimSpace(n) == "0" {
			return s.Repo.push(ctx) // nothing new to send
		}
		tree, err := s.Repo.git(ctx, "rev-parse", "--verify", "-q", "HEAD^{tree}")
		if err != nil {
			return "", errorf(ErrInvalid, "there is nothing to push yet")
		}
		pass, ok := s.Jobs.PassOf(strings.TrimSpace(tree))
		if !ok {
			return "", errorf(ErrNotChecked, "the commit to push has not passed a check (it was committed without one, or a pull rebased it onto new remote commits): run the checks with nothing staged or changed, then push, or push without checks")
		}
		base, err := s.Jobs.pushedBase(ctx)
		if err != nil {
			return "", err
		}
		if pass.Base != "" && pass.Base != base { // "": the check built every machine
			return "", errorf(ErrNotChecked, "this commit was checked against an earlier state of the branch on the remote (it moved since, or a check of it finished), so the push may change machines the check did not build: run the checks again with nothing staged or changed, then push, or push without checks")
		}
	}
	return s.Repo.push(ctx)
}
