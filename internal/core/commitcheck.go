package core

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// checkedAncestors is how many commits back a check of a commit looks for one
// that passed a check, to compare the machines with.
const checkedAncestors = 100

var fullSHA = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// CheckCommit queues a check of a commit (a full SHA), as pushed to branch
// ("" when unknown). It builds every machine whose system differs from the
// newest commit before it, on its first-parent line, whose tree passed a check;
// with none in the last 100, every machine. It returns the check of the same
// commit already queued or running instead of a second one, and no job but the
// pass when the commit's tree already passed (a push from the console).
func (js *Jobs) CheckCommit(ctx context.Context, sha, branch string) (*Job, *CheckPass, error) {
	if !fullSHA.MatchString(sha) {
		return nil, nil, errorf(ErrInvalid, "%q is not a full commit SHA", sha)
	}
	js.Repo.mu.Lock()
	_, err := js.Repo.resolve(ctx, sha)
	var out string
	if err == nil {
		out, err = js.Repo.git(ctx, "log", "--first-parent", "--format=%T", fmt.Sprintf("--max-count=%d", checkedAncestors+1), "--end-of-options", sha, "--")
	}
	js.Repo.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	trees := strings.Fields(out)
	js.mu.Lock()
	if p, ok := js.passed[trees[0]]; ok {
		js.mu.Unlock()
		return nil, &p, nil
	}
	if j := js.unfinished(sha); j != nil {
		js.mu.Unlock()
		return j, nil, nil
	}
	job, err := js.enqueue(JobRequest{Kind: "check", Commit: sha, Branch: branch, Base: js.newestPassed(trees[1:])}, trees[0], "")
	tell := js.onCommit
	js.mu.Unlock()
	if err == nil && tell != nil {
		tell(job)
	}
	return job, nil, err
}

// newestPassed is the first of trees that passed a check, or "" for none;
// js.mu held. A check builds every machine whose system differs from it, so
// a machine that did not build in a failed or unchecked commit is built again.
func (js *Jobs) newestPassed(trees []string) string {
	if i := slices.IndexFunc(trees, func(t string) bool { _, ok := js.passed[t]; return ok }); i >= 0 {
		return trees[i]
	}
	return ""
}

// pushedBase is the tree a check of the staged changes compares the machines
// with: the newest pushed commit that passed a check (see newestPassed). Needs
// Repo.mu.
func (js *Jobs) pushedBase(ctx context.Context) (string, error) {
	trees, err := js.Repo.pushedTrees(ctx)
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.newestPassed(trees), err
}

// WatchCommits has fn told of every check of a commit queued from now on.
func (js *Jobs) WatchCommits(fn func(*Job)) {
	js.mu.Lock()
	js.onCommit = fn
	js.mu.Unlock()
}

// unfinishedCheck is the queued or running check of a commit, or nil.
func (js *Jobs) unfinishedCheck(sha string) *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.unfinished(sha)
}

// unfinished is unfinishedCheck with js.mu held.
func (js *Jobs) unfinished(sha string) *Job {
	if i := slices.IndexFunc(js.jobs, func(j *Job) bool { return j.Req.Commit == sha && !j.finishedState() }); i >= 0 {
		return js.jobs[i]
	}
	return nil
}

// CommitState says where a commit stands: passed (its tree passed a check),
// queued, running or failed (its newest check, whose id is job), or unchecked.
func (js *Jobs) CommitState(ctx context.Context, sha string) (state, job string) {
	tree := ""
	if fullSHA.MatchString(sha) {
		out, _ := js.Repo.git(ctx, "rev-parse", "--verify", "-q", sha+"^{tree}")
		tree = strings.TrimSpace(out)
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	if _, ok := js.passed[tree]; ok && tree != "" {
		return "passed", ""
	}
	for i := len(js.jobs) - 1; i >= 0; i-- {
		if j := js.jobs[i]; j.Req.Commit == sha && sha != "" {
			if s := j.Info(false).State; s != "passed" { // passed, but forgotten since: unchecked
				return s, j.ID
			}
			break
		}
	}
	return "unchecked", ""
}

// CommitCheck is where the last commit of a branch stands with checks.
type CommitCheck struct {
	Branch  string `json:"branch"`
	Commit  string `json:"commit"`
	Subject string `json:"subject"`
	State   string `json:"state"`         // passed, unchecked, queued, running or failed
	Job     string `json:"job,omitempty"` // its newest check, unless it passed
}

// TipCheck is the state of branch's last commit as comin sees it: on origin,
// or without a remote, the local branch. nil when there is no such branch.
func (s *Service) TipCheck(ctx context.Context, branch string) *CommitCheck {
	ref := "refs/heads/" + branch
	if _, err := s.Repo.git(ctx, "remote", "get-url", "origin"); err == nil {
		ref = "refs/remotes/origin/" + branch
	}
	out, err := s.Repo.git(ctx, "log", "-1", "--format=%H%x00%s", "--end-of-options", ref, "--")
	sha, subject, ok := strings.Cut(strings.TrimSpace(out), "\x00")
	if err != nil || !ok {
		return nil
	}
	state, job := s.Jobs.CommitState(ctx, sha)
	return &CommitCheck{Branch: branch, Commit: sha, Subject: subject, State: state, Job: job}
}
