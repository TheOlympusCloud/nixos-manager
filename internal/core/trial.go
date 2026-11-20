package core

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
)

// Trial is a change tried on one machine: a commit on the testing branch its
// comin follows, which comin deploys there alone, with switch-to-configuration
// test, while the branch is on top of the main one.
type Trial struct {
	Host   string `json:"host"`
	Branch string `json:"branch"` // the testing branch
	Main   string `json:"main"`
	Commit string `json:"commit"`
}

// trialBranches are the branches host's comin follows, as the working copy's
// model has them (the Deployments view reads the same).
func (s *Service) trialBranches(ctx context.Context, host string) (CominBranches, error) {
	m, _, err := s.Jobs.Model(ctx, "working")
	if err != nil {
		return CominBranches{}, err
	}
	if m == nil {
		return CominBranches{}, errorf(ErrConflict, "the machines are still being evaluated: try again in a moment")
	}
	i := slices.IndexFunc(m.Hosts, func(h Host) bool { return h.Name == host })
	if i < 0 {
		return CominBranches{}, errorf(ErrNotFound, "the flake has no machine named %q", host)
	}
	b, ok := cominFollows(ctx, s.Repo, m)[host]
	switch {
	case !ok || !m.Hosts[i].Checks.Comin:
		return b, errorf(ErrInvalid, "%s is not deployed by comin, so nothing tries a branch on it", host)
	case b.Testing == "":
		return b, errorf(ErrInvalid, "comin on %s follows no testing branch (its branches.testing.name is empty)", host)
	}
	b.Main = cmp.Or(b.Main, defaultBranch)
	return b, nil
}

// TryOn deploys the staged changes, or the commits not pushed yet when nothing
// is staged, to one machine only: a commit on its testing branch on top of the
// main branch its comin follows, pushed there. The working copy is then on the
// testing branch, where further commits and pushes go to that machine; Merge
// promotes them to the main branch. What is tried must have passed a check that
// built the machine. An earlier trial on the branch with changes the main
// branch does not have is only replaced with replace. When the push fails,
// everything is left as it was.
func (s *Service) TryOn(ctx context.Context, host, message string, replace bool) (Trial, error) {
	b, err := s.trialBranches(ctx, host)
	if err != nil {
		return Trial{}, err
	}
	t := Trial{Host: host, Branch: b.Testing, Main: b.Main}
	r := s.Repo
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, err := r.currentBranch(ctx)
	if err != nil {
		return t, err
	}
	if cur != b.Main {
		return t, errorf(ErrInvalid, "a trial goes on top of %s, the branch comin on %s follows: switch to %s first", b.Main, host, b.Main)
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil { // comin compares with the remote as it is now
		return t, err
	}
	main, err := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+b.Main)
	if err != nil {
		return t, errorf(ErrConflict, "%s is not on the remote yet: push it first, as a trial goes on top of it", b.Main)
	}
	if !r.isAncestor(ctx, strings.TrimSpace(main), "HEAD") {
		return t, errorf(ErrConflict, "%s on the remote has commits this working copy does not have: pull first, as comin ignores a testing branch that is not on top of them", b.Main)
	}
	tree, err := r.stagedTree(ctx)
	if err != nil {
		return t, err
	}
	head, err := r.git(ctx, "rev-parse", "HEAD", "HEAD^{tree}")
	if err != nil {
		return t, err
	}
	headSHA, headTree, _ := strings.Cut(strings.TrimSpace(head), "\n")
	staged := tree != headTree
	if !staged && headSHA == strings.TrimSpace(main) {
		return t, errorf(ErrInvalid, "nothing to try: stage the changes, or commit them without pushing")
	}
	if staged && strings.TrimSpace(message) == "" {
		return t, errorf(ErrInvalid, "the commit message is empty")
	}
	pass, ok := s.Jobs.PassOf(tree)
	if !ok {
		return t, errorf(ErrNotChecked, "what you try has not passed a check: run the checks first")
	}
	if !slices.Contains(pass.Built, host) {
		return t, errorf(ErrInvalid, "this does not change %s's system: there is nothing to try on it", host)
	}
	for _, ref := range []string{"refs/remotes/origin/" + b.Testing, "refs/heads/" + b.Testing} {
		if c := r.notMerged(ctx, ref, headTree); c != "" && !replace {
			return t, errorf(ErrConflict, "the branch %s already holds a trial, %s, that %s does not have: replace it?", b.Testing, c, b.Main)
		}
	}

	// From here on a cancelled request (a closed tab) must not stop halfway:
	// the remote may take a push its client was killed in, and git refuses to
	// run the undo at all. The push keeps its own time limit.
	ctx = context.WithoutCancel(ctx)
	// Move to the testing branch without touching the files: HEAD and the
	// index stay, so going back is just pointing HEAD at the main branch again.
	oldLocal, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/heads/"+b.Testing)
	oldRemote, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+b.Testing)
	undo := func() {
		r.git(ctx, "symbolic-ref", "HEAD", "refs/heads/"+b.Main)
		if old := strings.TrimSpace(oldLocal); old != "" {
			r.git(ctx, "update-ref", "refs/heads/"+b.Testing, old)
		} else {
			r.git(ctx, "update-ref", "-d", "refs/heads/"+b.Testing)
		}
	}
	if _, err := r.git(ctx, "update-ref", "refs/heads/"+b.Testing, headSHA); err != nil {
		return t, err
	}
	if _, err := r.git(ctx, "symbolic-ref", "HEAD", "refs/heads/"+b.Testing); err != nil {
		undo()
		return t, err
	}
	t.Commit = headSHA
	if staged {
		if t.Commit, err = r.commit(ctx, message); err != nil {
			undo()
			return t, err
		}
	}
	pctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	lease := fmt.Sprintf("--force-with-lease=refs/heads/%s:%s", b.Testing, strings.TrimSpace(oldRemote))
	if _, errOut, err := r.gitRun(pctx, nil, nil, "push", lease, "-u", "origin", "refs/heads/"+b.Testing); err != nil {
		undo()
		if strings.Contains(errOut, "stale info") || strings.Contains(errOut, "[rejected]") {
			return t, errorf(ErrConflict, "%s changed on the remote meanwhile: try again.\n%s", b.Testing, strings.TrimSpace(errOut))
		}
		return t, err
	}
	return t, nil
}

// notMerged names the commit at ref ("abc1234 “subject”") when merging it into
// HEAD, whose tree is headTree, would bring changes HEAD lacks: a trial that
// was not promoted. "" when ref does not exist or brings nothing. Needs r.mu.
func (r *Repo) notMerged(ctx context.Context, ref, headTree string) string {
	if _, err := r.git(ctx, "rev-parse", "--verify", "-q", ref); err != nil {
		return ""
	}
	if out, err := r.git(ctx, "merge-tree", "--write-tree", "HEAD", ref); err == nil {
		if merged, _, _ := strings.Cut(out, "\n"); merged == headTree {
			return ""
		}
	}
	c, _ := r.git(ctx, "log", "-1", "--format=%h “%s”", ref, "--")
	return strings.TrimSpace(c)
}

// EndTrial sends a machine back to its main branch: its testing branch on the
// remote is set to the main branch's last commit, which comin then deploys
// there. Deleting the branch would not do: comin's clone keeps the branches it
// fetched. A working copy on the testing branch goes back to the main one, as
// a push from there would put the trial back; the trial's commits stay on the
// local testing branch.
func (s *Service) EndTrial(ctx context.Context, host string) (string, error) {
	b, err := s.trialBranches(ctx, host)
	if err != nil {
		return "", err
	}
	r := s.Repo
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.git(ctx, "remote", "get-url", "origin"); err != nil {
		return "", errorf(ErrInvalid, "no remote is configured")
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "", err
	}
	main, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+b.Main)
	tip, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+b.Testing)
	main, tip = strings.TrimSpace(main), strings.TrimSpace(tip)
	if main == "" || tip == "" || tip == main || !r.isAncestor(ctx, main, tip) {
		return fmt.Sprintf("%s already follows %s: %s is not ahead of it on the remote.", host, b.Main, b.Testing), nil
	}
	lease := fmt.Sprintf("--force-with-lease=refs/heads/%s:%s", b.Testing, tip)
	if _, errOut, err := r.gitRun(nctx, nil, nil, "push", lease, "origin", main+":refs/heads/"+b.Testing); err != nil {
		if strings.Contains(errOut, "stale info") || strings.Contains(errOut, "[rejected]") {
			return "", errorf(ErrConflict, "%s changed on the remote meanwhile: try again.\n%s", b.Testing, strings.TrimSpace(errOut))
		}
		return "", err
	}
	msg := fmt.Sprintf("%s goes back to %s: %s now points at its last commit, %.7s, so comin switches %s to it.", host, b.Main, b.Testing, main, host)
	if cur, _ := r.currentBranch(ctx); cur == b.Testing {
		if _, err := r.git(context.WithoutCancel(ctx), "switch", "-q", "--no-guess", b.Main); err != nil {
			return msg + fmt.Sprintf(" This console stays on %s, as its changes there conflict with %s: commit or discard them and switch to %s, or a push from %s sends the trial back to %s.", b.Testing, b.Main, b.Main, b.Testing, host), nil
		}
		msg += fmt.Sprintf(" This console is back on %s; the trial's commits stay on its %s branch.", b.Main, b.Testing)
	}
	return msg, nil
}
