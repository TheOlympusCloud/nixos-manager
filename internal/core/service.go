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
}

func NewService(repo *Repo) *Service {
	return &Service{Repo: repo, Jobs: newJobs(repo)}
}

// Status adds whether the staged changes passed a check.
func (s *Service) Status(ctx context.Context) (Status, error) {
	st, err := s.Repo.Status(ctx)
	st.StagedChecked = err == nil && s.Jobs.Passed(st.StagedTree)
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
	if !force && !s.Jobs.Passed(tree) {
		return "", errorf(ErrNotChecked, "the staged changes have not passed a check yet: run the checks first, or commit without checks")
	}
	return s.Repo.commit(ctx, message)
}

// Push sends the current branch to origin. Unless force is set, the commit
// being pushed must have a checked tree: a pull that rebased local commits
// creates a combination nobody checked, and comin would deploy it.
func (s *Service) Push(ctx context.Context, force bool) (string, error) {
	s.Repo.mu.Lock()
	defer s.Repo.mu.Unlock()
	if _, err := s.Repo.currentBranch(ctx); err != nil { // no remote: nothing to push, checked or not
		return "", err
	}
	if !force {
		if n, err := s.Repo.git(ctx, "rev-list", "--count", "@{upstream}..HEAD"); err == nil && strings.TrimSpace(n) == "0" {
			return s.Repo.push(ctx) // nothing new to send
		}
		tree, err := s.Repo.git(ctx, "rev-parse", "--verify", "-q", "HEAD^{tree}")
		if err != nil {
			return "", errorf(ErrInvalid, "there is nothing to push yet")
		}
		if !s.Jobs.Passed(strings.TrimSpace(tree)) {
			return "", errorf(ErrNotChecked, "the commit to push has not passed a check (it was committed without one, or a pull rebased it onto new remote commits): run the checks with nothing staged or changed, then push, or push without checks")
		}
	}
	return s.Repo.push(ctx)
}
