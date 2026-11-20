package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	keepSystems = 512       // machine and commit pairs remembered; the oldest are forgotten first
	systemRetry = time.Hour // a failed evaluation (no network, say) is tried again after this
)

// SystemEval is the system one machine builds at one commit
// (system.build.toplevel's output path), or why it could not be evaluated.
type SystemEval struct {
	Path  string    `json:"path,omitempty"`
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at"`
}

// ExpectedSystems returns the systems of the hosts that are evaluated at
// commit, and queues one job that evaluates the others in the systems lane,
// one machine at a time, so the fleet views never wait for it. They are
// evaluated as comin evaluates them: the flake at
// git+file://<working copy>?rev=<commit>, so self.rev and its friends are
// the ones the machine sees. A commit never changes, so a result is kept;
// a failure only for systemRetry.
// shortcut: results are kept in memory only; after a restart each deployed commit is evaluated again.
func (js *Jobs) ExpectedSystems(commit string, hosts []string) map[string]SystemEval {
	out := map[string]SystemEval{}
	if !reportSHA.MatchString(commit) {
		return out
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	var missing []string
	for _, h := range hosts {
		e, ok := js.systems[systemKey(commit, h)]
		switch {
		case ok && (e.Error == "" || time.Since(e.At) < systemRetry):
			out[h] = e
		case !reportHost.MatchString(h) || slices.Contains(missing, h) || js.evaluating(commit, h):
		default:
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		js.enqueue(JobRequest{Kind: "system", Hosts: missing}, commit, "system:"+commit) // a full queue: the next request asks again
	}
	return out
}

// evaluating says whether a queued or running job evaluates host at commit;
// js.mu must be held.
func (js *Jobs) evaluating(commit, host string) bool {
	return slices.ContainsFunc(js.jobs, func(j *Job) bool {
		return j.Req.Kind == "system" && j.tree == commit && !j.finishedState() && slices.Contains(j.Req.Hosts, host)
	})
}

// runSystems evaluates the system of each host of the job at its commit. As
// in the fleet model, a machine that does not evaluate is a result, not a
// failed job.
func (js *Jobs) runSystems(ctx context.Context, job *Job) error {
	ref := (&url.URL{Scheme: "git+file", Path: js.Repo.Dir, RawQuery: "rev=" + job.tree}).String()
	for _, h := range job.Req.Hosts {
		fmt.Fprintf(job, "Evaluating the system of %s at %.7s…\n", h, job.tree)
		e := SystemEval{At: time.Now()}
		e.Path, e.Error = js.evalSystem(ctx, ref+"#nixosConfigurations."+attrName(h)+".config.system.build.toplevel.outPath")
		if e.Error != "" {
			fmt.Fprintf(job, "%s: %s\n", h, e.Error)
		}
		js.mu.Lock()
		key := systemKey(job.tree, h)
		if _, ok := js.systems[key]; !ok {
			js.systemOrder = append(js.systemOrder, key)
		}
		js.systems[key] = e
		for len(js.systemOrder) > keepSystems {
			delete(js.systems, js.systemOrder[0])
			js.systemOrder = js.systemOrder[1:]
		}
		js.mu.Unlock()
	}
	return ctx.Err() // out of time: the machines not reached have the error
}

func systemKey(commit, host string) string { return commit + "/" + host }

// evalSystem prints one store path with nix eval --raw, or says why it cannot.
func (js *Jobs) evalSystem(ctx context.Context, installable string) (path, failure string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := js.nixCmd(ctx, js.nixArgs("eval", "--raw", installable)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return "", fmt.Sprintf("cannot run nix (%v); is Nix installed?", err)
		}
		return "", tail(stderr.String(), 25)
	}
	if p := strings.TrimSpace(stdout.String()); p != "" && reportPath.MatchString(p) {
		return p, ""
	}
	return "", fmt.Sprintf("nix printed %.200q, not a store path", stdout.String())
}
