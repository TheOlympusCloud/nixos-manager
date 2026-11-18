package core

import "context"

// Service is the one entry point for the web API and the MCP server, so both
// get the same rules: the commit gate, path checks and the commit identity.
type Service struct {
	*Repo
	Jobs *Jobs
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
