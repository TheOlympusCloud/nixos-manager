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
	defer js.mu.Unlock()
	if p, ok := js.passed[trees[0]]; ok {
		return nil, &p, nil
	}
	if i := slices.IndexFunc(js.jobs, func(j *Job) bool { return j.Req.Commit == sha && !j.finishedState() }); i >= 0 {
		return js.jobs[i], nil, nil
	}
	base := ""
	if i := slices.IndexFunc(trees[1:], func(t string) bool { _, ok := js.passed[t]; return ok }); i >= 0 {
		base = trees[1+i]
	}
	job, err := js.enqueue(JobRequest{Kind: "check", Commit: sha, Branch: branch, Base: base}, trees[0], "")
	return job, nil, err
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
