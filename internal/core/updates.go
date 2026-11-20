package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Scheduled updates: on the schedule set in Inputs, or with Update now, every
// flake input is updated on a branch on top of the fleet's branch as it is on
// the remote, never in the working copy. The commit is pushed to flake-update
// and checked like any pushed commit, the result is notified, and Merge
// fast-forwards the fleet's branch to it. Nothing reaches that branch, so no
// machine gets the update, until someone merges it. updates.json keeps the
// schedule and the latest update.

const (
	updateBranch = "flake-update"
	updateRef    = "refs/nixos-manager/flake-update" // keeps the latest update's commit from git gc
	updateEvery  = time.Minute
)

// UpdateSchedule is when the inputs are updated by themselves.
type UpdateSchedule struct {
	Enabled bool `json:"enabled"`
	Day     int  `json:"day"`  // -1 every day, else 0 (Sunday) to 6 (Saturday)
	Hour    int  `json:"hour"` // 0 to 23, in the server's time zone
}

// Update is the latest update of the inputs on the branch.
type Update struct {
	// State is updating, checking, ready (passed its check), failed (its
	// check), current (no input moved), error (it could not run), merged or
	// discarded.
	State     string        `json:"state"`
	Scheduled bool          `json:"scheduled"` // by the schedule, not Update now
	Started   time.Time     `json:"started"`
	Finished  time.Time     `json:"finished,omitzero"`
	Branch    string        `json:"branch,omitempty"` // the fleet's branch it goes on top of
	Base      string        `json:"base,omitempty"`   // that branch's commit on the remote
	Commit    string        `json:"commit,omitempty"` // the update, pushed to flake-update
	Inputs    []InputChange `json:"inputs,omitempty"`
	Job       string        `json:"job,omitempty"`     // nix flake update
	Check     string        `json:"check,omitempty"`   // the check of Commit
	Summary   *CheckSummary `json:"summary,omitempty"` // what the check built
	Changed   []string      `json:"changed,omitempty"` // machines whose system the update changes
	Message   string        `json:"message,omitempty"` // what it changes, for notifications
	Error     string        `json:"error,omitempty"`
	Merged    time.Time     `json:"merged,omitzero"`
}

// UpdateStatus is what Inputs shows.
type UpdateStatus struct {
	Schedule UpdateSchedule `json:"schedule"`
	Next     time.Time      `json:"next,omitzero"` // the next scheduled run, while the schedule is on
	Zone     string         `json:"zone"`          // the server's time zone the hour is in, e.g. CET
	Update   *Update        `json:"update,omitempty"`
	// Outdated: the fleet's branch moved on since the update was made, as
	// last fetched, so it can no longer be merged.
	Outdated bool `json:"outdated,omitempty"`
}

type updatesFile struct {
	Schedule UpdateSchedule `json:"schedule"`
	// Since is the last scheduled run, or when the schedule was saved: the next
	// run is the first slot after it.
	Since  time.Time `json:"since"`
	Pushed string    `json:"pushed,omitempty"` // the commit this console last pushed to flake-update
	Update *Update   `json:"update,omitempty"`
}

// Updates runs the updates and keeps their state.
type Updates struct {
	Svc *Service

	path string
	now  func() time.Time
	loc  *time.Location
	wg   sync.WaitGroup // the run or check being waited for

	mu   sync.Mutex
	f    updatesFile
	busy bool // a run, a merge or a check of the update is going on
}

// OpenUpdates loads dataDir/updates.json. An update a stop interrupted while
// Nix updated the inputs becomes an error; one being checked is checked
// again by Run. The jobs of before the stop are gone.
func OpenUpdates(dataDir string, svc *Service) (*Updates, error) {
	u := &Updates{Svc: svc, path: filepath.Join(dataDir, "updates.json"), now: time.Now, loc: time.Local,
		f: updatesFile{Schedule: UpdateSchedule{Day: 1, Hour: 4}}} // off; Monday at 04:00 once turned on
	b, err := os.ReadFile(u.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &u.f); err != nil {
			return nil, errorf(ErrInvalid, "reading %s: %v", u.path, err)
		}
	}
	if up := u.f.Update; up != nil {
		up.Job, up.Check = "", "" // jobs do not survive a restart, and their ids start again
		if up.State == "updating" {
			up.State, up.Error, up.Finished = "error", "The server stopped while it updated the inputs: update again.", u.now()
		}
		u.save()
	}
	return u, nil
}

// save writes updates.json; u.mu held, or before anything else runs. Failing
// only loses the state at the next restart, so it is logged.
func (u *Updates) save() {
	b, _ := json.MarshalIndent(u.f, "", "  ") // plain data always marshals
	if err := writeAtomic(u.path, b); err != nil {
		slog.Warn("cannot save the update state; a restart forgets it", "err", err)
	}
}

// set changes the update being made and saves it.
func (u *Updates) set(up *Update, change func(*Update)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	change(up)
	u.save()
}

// nextRun is the first slot of s after after, in loc.
func nextRun(s UpdateSchedule, after time.Time, loc *time.Location) time.Time {
	after = after.In(loc)
	y, m, d := after.Date()
	for i := range 8 {
		t := time.Date(y, m, d+i, s.Hour, 0, 0, 0, loc) // an hour a clock change skips becomes the next one
		if t.After(after) && (s.Day < 0 || int(t.Weekday()) == s.Day) {
			return t
		}
	}
	return time.Time{} // unreachable for a valid schedule
}

// Status is the schedule, the next run and the latest update.
func (u *Updates) Status(ctx context.Context) UpdateStatus {
	u.mu.Lock()
	st := UpdateStatus{Schedule: u.f.Schedule}
	st.Zone, _ = u.now().In(u.loc).Zone()
	if u.f.Schedule.Enabled {
		st.Next = nextRun(u.f.Schedule, u.f.Since, u.loc)
	}
	if up := u.f.Update; up != nil {
		c := *up
		st.Update = &c
	}
	u.mu.Unlock()
	if up := st.Update; up != nil && (up.State == "ready" || up.State == "failed" || up.State == "checking") {
		r := u.Svc.Repo
		tip, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+up.Branch)
		tip = strings.TrimSpace(tip)
		if st.Outdated = tip != up.Base; st.Outdated && tip != "" && r.isAncestor(ctx, up.Commit, tip) {
			up.State, st.Outdated = "merged", false // in Forgejo, say, or before a restart recorded it
		}
	}
	return st
}

// SetSchedule saves the schedule. The next run is the first slot from now on,
// so saving never starts one for a slot already past.
func (u *Updates) SetSchedule(s UpdateSchedule) (UpdateStatus, error) {
	if s.Day < -1 || s.Day > 6 {
		return UpdateStatus{}, errorf(ErrInvalid, "the day is -1 (every day) or 0 (Sunday) to 6 (Saturday), not %d", s.Day)
	}
	if s.Hour < 0 || s.Hour > 23 {
		return UpdateStatus{}, errorf(ErrInvalid, "the hour is 0 to 23, not %d", s.Hour)
	}
	u.mu.Lock()
	u.f.Schedule, u.f.Since = s, u.now()
	u.save()
	u.mu.Unlock()
	return u.Status(context.Background()), nil
}

// Run checks again an update a stop interrupted during its check, then starts
// the scheduled updates, until ctx ends.
func (u *Updates) Run(ctx context.Context) {
	u.mu.Lock()
	up := u.f.Update
	resume := up != nil && up.State == "checking" && !u.busy
	if resume {
		u.busy = true
	}
	u.mu.Unlock()
	if resume {
		u.wg.Go(func() { u.check(up, false) })
	}
	t := time.NewTicker(updateEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			u.Tick(now)
		}
	}
}

// Tick starts the scheduled update when its slot has come. A slot that comes
// while another update runs is skipped; one missed while the server was down
// runs once.
func (u *Updates) Tick(now time.Time) {
	u.mu.Lock()
	s := u.f.Schedule
	if !s.Enabled || now.Before(nextRun(s, u.f.Since, u.loc)) {
		u.mu.Unlock()
		return
	}
	u.f.Since = now
	u.save()
	busy := u.busy
	u.mu.Unlock()
	if busy {
		slog.Info("skipping the scheduled update: an update is already running")
		return
	}
	if _, err := u.start(context.Background(), true); err != nil {
		slog.Warn("cannot start the scheduled update", "err", err)
	}
}

// Start updates the inputs on the branch now, in the background.
func (u *Updates) Start(ctx context.Context) (UpdateStatus, error) {
	if _, err := u.start(ctx, false); err != nil {
		return UpdateStatus{}, err
	}
	return u.Status(ctx), nil
}

func (u *Updates) start(ctx context.Context, scheduled bool) (*Update, error) {
	via := "scheduled update"
	if c, ok := callerOf(ctx); ok && c.Via != "" {
		via = c.Via
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy {
		return nil, errorf(ErrConflict, "an update is already running")
	}
	u.busy = true
	up := &Update{State: "updating", Scheduled: scheduled, Started: u.now()}
	u.f.Update = up
	u.save()
	u.wg.Go(func() { u.run(up, via) })
	return up, nil
}

// fail ends the update with an error and frees the next one.
func (u *Updates) fail(up *Update, err error) {
	slog.Warn("the flake update failed", "err", err)
	u.mu.Lock()
	defer u.mu.Unlock()
	up.State, up.Error, up.Finished = "error", err.Error(), u.now()
	u.busy = false
	u.save()
}

// run makes the update: nix flake update on an export of the fleet branch's
// last commit on the remote, a commit of the new flake.lock on top of it,
// pushed to flake-update, then its check.
func (u *Updates) run(up *Update, via string) {
	ctx := context.Background()
	branch, err := u.Svc.fleetBranch(ctx)
	if err != nil {
		u.fail(up, err)
		return
	}
	r, js := u.Svc.Repo, u.Svc.Jobs
	r.mu.Lock()
	base, baseTree, pushed, err := u.fetchBase(ctx, branch)
	r.mu.Unlock()
	if err != nil {
		u.fail(up, err)
		return
	}
	u.set(up, func(up *Update) { up.Branch, up.Base = branch, base })

	js.mu.Lock()
	job, err := js.enqueue(JobRequest{Kind: "update-branch"}, baseTree, "")
	js.mu.Unlock()
	if err != nil {
		u.fail(up, err)
		return
	}
	u.set(up, func(up *Update) { up.Job = job.ID })
	<-job.Done()
	info := job.Info(true)
	if info.State != "passed" {
		u.fail(up, fmt.Errorf("nix flake update failed:\n%s", tail(info.Output, 8)))
		return
	}
	lock := job.Result()
	old, _ := r.git(ctx, "cat-file", "blob", base+":flake.lock")
	if old == string(lock) {
		u.mu.Lock()
		up.State, up.Finished = "current", u.now()
		u.busy = false
		u.save()
		u.mu.Unlock()
		return
	}
	before, _ := parseInputs([]byte(old)) // a lock Nix wrote and one the check accepted: unreadable only means no list
	after, _ := parseInputs(lock)
	inputs := DiffInputs(before, after)

	r.mu.Lock()
	made, _ := r.git(ctx, "rev-parse", "--verify", "-q", updateRef) // the last update made, pushed or not
	sha, err := r.commitLock(ctx, base, updateMessage(inputs, via), lock)
	if err == nil {
		err = u.pushUpdate(ctx, sha, pushed, strings.TrimSpace(made))
	}
	r.mu.Unlock()
	if err != nil {
		u.fail(up, err)
		return
	}
	u.mu.Lock()
	u.f.Pushed = sha
	up.Commit, up.Inputs, up.State = sha, inputs, "checking"
	u.save()
	u.mu.Unlock()
	u.check(up, false)
}

// fetchBase fetches origin and returns the last commit of branch there, its
// tree, and the remote's flake-update ("" when there is none). Needs r.mu.
func (u *Updates) fetchBase(ctx context.Context, branch string) (string, string, string, error) {
	r := u.Svc.Repo
	if _, err := r.git(ctx, "remote", "get-url", "origin"); err != nil {
		return "", "", "", errorf(ErrInvalid, "no remote is configured: the update goes on top of what the machines deploy from it")
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "", "", "", err
	}
	out, err := r.git(ctx, "log", "-1", "--format=%H %T", "refs/remotes/origin/"+branch, "--")
	base, tree, found := strings.Cut(strings.TrimSpace(out), " ")
	if err != nil || !found {
		return "", "", "", errorf(ErrConflict, "%s is not on the remote yet: push it first", branch)
	}
	pushed, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+updateBranch)
	return base, tree, strings.TrimSpace(pushed), nil
}

// updateMessage is the update's commit message: which inputs moved, and how
// it was made.
func updateMessage(inputs []InputChange, via string) string {
	names := make([]string, len(inputs))
	var body strings.Builder
	for i, c := range inputs {
		names[i] = c.Name
		switch c.Status {
		case "added":
			fmt.Fprintf(&body, "%s: added at %.7s\n", c.Name, c.ToRev)
		case "removed":
			fmt.Fprintf(&body, "%s: removed\n", c.Name)
		default:
			fmt.Fprintf(&body, "%s: %.7s → %.7s%s\n", c.Name, c.FromRev, c.ToRev, dates(c))
		}
	}
	subject := "flake.lock: update"
	switch {
	case len(names) > 3:
		subject += fmt.Sprintf(" %d inputs", len(names))
	case len(names) > 0:
		subject += " " + strings.Join(names, ", ")
	}
	msg := subject + "\n\n"
	if body.Len() > 0 {
		msg += body.String() + "\n"
	}
	return msg + "Via: " + via + "\n"
}

// dates is " (2 Nov → 18 Nov 2026)" for an input whose lock has both dates.
func dates(c InputChange) string {
	if c.FromDate == 0 || c.ToDate == 0 {
		return ""
	}
	return fmt.Sprintf(" (%s → %s)", time.Unix(c.FromDate, 0).UTC().Format("2006-01-02"), time.Unix(c.ToDate, 0).UTC().Format("2006-01-02"))
}

// commitLock records base's tree with flake.lock replaced by lock, as a
// commit on top of base, without touching the working copy or its index, and
// keeps it from git gc. Needs r.mu.
func (r *Repo) commitLock(ctx context.Context, base, message string, lock []byte) (string, error) {
	tmp, err := os.MkdirTemp("", "nixmgr-update-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if _, _, err := r.gitRun(ctx, env, nil, "read-tree", base); err != nil {
		return "", err
	}
	blob, _, err := r.gitRun(ctx, nil, lock, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	if _, _, err := r.gitRun(ctx, env, nil, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(blob)+",flake.lock"); err != nil {
		return "", err
	}
	tree, _, err := r.gitRun(ctx, env, nil, "write-tree")
	if err != nil {
		return "", err
	}
	sha, _, err := r.gitRun(ctx, nil, []byte(message), "commit-tree", strings.TrimSpace(tree), "-p", base, "-F", "-")
	if err != nil {
		return "", err
	}
	sha = strings.TrimSpace(sha)
	_, err = r.git(ctx, "update-ref", updateRef, sha)
	return sha, err
}

// pushUpdate pushes sha to flake-update on the remote, which holds remote
// ("" for no such branch): only when that is a commit this console pushed
// there, so a branch of that name someone else made is never lost. made is
// the last update made before sha: pushed by a server that stopped before it
// recorded the push. Needs r.mu.
func (u *Updates) pushUpdate(ctx context.Context, sha, remote, made string) error {
	u.mu.Lock()
	pushed := u.f.Pushed
	u.mu.Unlock()
	if remote != "" && remote != pushed && remote != made {
		return errorf(ErrConflict, "the remote has a branch %s that this console did not push there (%.7s): rename or delete it, as the update goes there", updateBranch, remote)
	}
	nctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), netTimeout)
	defer cancel()
	_, errOut, err := u.Svc.Repo.gitRun(nctx, nil, nil, "push", fmt.Sprintf("--force-with-lease=refs/heads/%s:%s", updateBranch, remote), "origin", sha+":refs/heads/"+updateBranch)
	if err != nil && (strings.Contains(errOut, "stale info") || strings.Contains(errOut, "[rejected]")) {
		return errorf(ErrConflict, "%s changed on the remote meanwhile: update again.\n%s", updateBranch, strings.TrimSpace(errOut))
	}
	return err
}

// check runs the update's check, or joins it, then records the result and
// what the update changes on each machine, and frees the next update. With
// again it checks even a tree that passed, against the newest checked commit
// of the branch now.
func (u *Updates) check(up *Update, again bool) {
	ctx := context.Background()
	job, pass, err := u.Svc.Jobs.checkCommit(ctx, up.Commit, updateBranch, again)
	if err != nil {
		u.fail(up, err)
		return
	}
	state, sum := "ready", (*CheckSummary)(nil)
	if job != nil {
		u.set(up, func(up *Update) { up.State, up.Check = "checking", job.ID })
		<-job.Done()
		info := job.Info(false)
		if sum = info.Summary; info.State != "passed" {
			state = "failed"
		}
	} else {
		sum = &CheckSummary{Base: pass.Base, Changed: pass.Built, Built: pass.Built}
	}
	diffs := u.changes(ctx, up)
	u.mu.Lock()
	defer u.mu.Unlock()
	up.State, up.Summary, up.Finished = state, sum, u.now()
	up.Changed = []string{}
	for _, d := range diffs {
		if d.Status == "added" || d.Status == "changed" {
			up.Changed = append(up.Changed, d.Host)
		}
	}
	up.Message = updateNotice(up, diffs)
	u.busy = false
	u.save()
}

// changes compares every machine before and after the update: nil when either
// side does not evaluate.
func (u *Updates) changes(ctx context.Context, up *Update) []HostDiff {
	js := u.Svc.Jobs
	out, err := js.Repo.git(ctx, "rev-parse", up.Base+"^{tree}", up.Commit+"^{tree}")
	trees := strings.Fields(out)
	if err != nil || len(trees) != 2 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	before, err := js.waitModel(ctx, trees[0])
	if err != nil || before.Error != "" {
		return nil
	}
	after, err := js.waitModel(ctx, trees[1])
	if err != nil || after.Error != "" {
		return nil
	}
	return DiffModels(before, after)
}

// updateNotice says what an update changes, for a notification: the inputs,
// the check's verdict, and per machine the kernel and the first versions that
// change.
func updateNotice(up *Update, diffs []HostDiff) string {
	var b strings.Builder
	moved := make([]string, len(up.Inputs))
	for i, c := range up.Inputs {
		switch c.Status {
		case "added":
			moved[i] = c.Name + " (added)"
		case "removed":
			moved[i] = c.Name + " (removed)"
		default:
			moved[i] = fmt.Sprintf("%s %.7s → %.7s", c.Name, c.FromRev, c.ToRev)
		}
	}
	fmt.Fprintf(&b, "Inputs: %s.", strings.Join(moved, ", "))
	if sum := up.Summary; up.State == "failed" && sum != nil && len(sum.Failed) > 0 {
		fmt.Fprintf(&b, " Its check failed: %s %s not build.", list(sum.Failed), map[bool]string{true: "does", false: "do"}[len(sum.Failed) == 1])
	} else if up.State == "failed" {
		b.WriteString(" Its check failed; the log says why.")
	}
	if diffs == nil {
		return truncate(b.String(), 1500)
	}
	const machines, versions = 6, 3
	n := 0
	for _, d := range diffs {
		if d.Status != "added" && d.Status != "changed" {
			continue
		}
		if n++; n > machines {
			continue
		}
		var vs []string
		if k := d.Kernel; k != nil {
			vs = append(vs, fmt.Sprintf("kernel %s → %s", k.From, k.To))
		}
		for _, v := range d.VersionsChanged {
			vs = append(vs, fmt.Sprintf("%s %s → %s", v.Name, v.From, v.To))
		}
		line := "other configuration"
		if len(vs) > versions {
			line = strings.Join(vs[:versions], ", ") + fmt.Sprintf(" and %d more", len(vs)-versions)
		} else if len(vs) > 0 {
			line = strings.Join(vs, ", ")
		}
		if d.Reboot {
			line += "; reboot needed"
		}
		fmt.Fprintf(&b, "\n%s: %s", d.Host, line)
	}
	switch {
	case n == 0:
		b.WriteString("\nNo machine's system changes.")
	case n > machines:
		fmt.Fprintf(&b, "\nAnd %d more machines.", n-machines)
	}
	return truncate(b.String(), 1500)
}

// Merge fast-forwards the fleet's branch on the remote to the update, which
// deploys it: only the update waiting, which passed a check against the
// branch as it is now, while the branch is still where the update was made.
// A working copy on that branch with nothing of its own follows.
func (u *Updates) Merge(ctx context.Context, commit string) (string, error) {
	u.mu.Lock()
	up := u.f.Update
	var err error
	switch {
	case up == nil || up.Commit == "" || up.Commit != commit:
		err = errorf(ErrConflict, "%.7s is not the update waiting to be merged: reload", commit)
	case up.State == "checking":
		err = errorf(ErrConflict, "the update is still being checked")
	case u.busy:
		err = errorf(ErrConflict, "the update is being merged already")
	case up.State == "failed":
		err = errorf(ErrNotChecked, "the update failed its check, so it is not merged")
	case up.State != "ready":
		err = errorf(ErrConflict, "the update is %s, so there is nothing to merge", up.State)
	}
	if err != nil {
		u.mu.Unlock()
		return "", err
	}
	u.busy = true
	branch, base := up.Branch, up.Base
	u.mu.Unlock()

	r := u.Svc.Repo
	r.mu.Lock()
	msg, recheck, err := u.merge(ctx, commit, branch, base)
	r.mu.Unlock()
	if recheck {
		u.set(up, func(up *Update) { up.State, up.Check = "checking", "" })
		u.wg.Go(func() { u.check(up, true) }) // it frees the next update
		return "", errorf(ErrNotChecked, "a commit of %s passed a check after this update's check began, so the update is checked again against it: merge it when that passes", branch)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.busy = false
	if err != nil {
		return "", err
	}
	up.State, up.Merged = "merged", u.now()
	u.save()
	return msg, nil
}

// merge pushes the update to branch; recheck asks for a new check first.
// Needs r.mu.
func (u *Updates) merge(ctx context.Context, commit, branch, base string) (msg string, recheck bool, err error) {
	r, js := u.Svc.Repo, u.Svc.Jobs
	if _, err := r.git(ctx, "remote", "get-url", "origin"); err != nil {
		return "", false, errorf(ErrInvalid, "no remote is configured")
	}
	nctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	if _, err := r.git(nctx, "fetch", "-q", "origin"); err != nil {
		return "", false, err
	}
	tip, _ := r.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+branch)
	if tip = strings.TrimSpace(tip); tip != base {
		if tip != "" && r.isAncestor(ctx, commit, tip) {
			return fmt.Sprintf("%s already has the update.", branch), false, nil
		}
		return "", false, errorf(ErrConflict, "%s moved on since the update was made (it is at %.7s now): update again, on top of it", branch, cmpOr(tip, "nothing"))
	}
	tree, err := r.git(ctx, "rev-parse", commit+"^{tree}")
	if err != nil {
		return "", false, err
	}
	pass, ok := js.PassOf(strings.TrimSpace(tree))
	if !ok {
		return "", true, nil // forgotten among a thousand newer checks
	}
	out, err := r.git(ctx, "log", "--first-parent", "--format=%T", fmt.Sprintf("--max-count=%d", checkedAncestors+1), "--end-of-options", tip, "--")
	if err != nil {
		return "", false, err
	}
	js.mu.Lock()
	newest := js.newestPassed(strings.Fields(out))
	js.mu.Unlock()
	if pass.Base != "" && pass.Base != newest { // as Push: the check built what differs from the newest checked commit
		return "", true, nil
	}

	// A cancelled request must not stop between the push and recording it.
	ctx = context.WithoutCancel(ctx)
	pctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	lease := fmt.Sprintf("--force-with-lease=refs/heads/%s:%s", branch, base)
	if _, errOut, err := r.gitRun(pctx, nil, nil, "push", lease, "origin", commit+":refs/heads/"+branch); err != nil {
		if strings.Contains(errOut, "stale info") || strings.Contains(errOut, "[rejected]") {
			return "", false, errorf(ErrConflict, "%s changed on the remote meanwhile: update again.\n%s", branch, strings.TrimSpace(errOut))
		}
		return "", false, err
	}
	msg = fmt.Sprintf("Merged the update into %s: comin deploys it to the machines it changes.", branch)
	if cur, _ := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD"); strings.TrimSpace(cur) == branch {
		if r.clean(ctx) == nil && r.isAncestor(ctx, "HEAD", commit) {
			if _, err := r.git(ctx, "merge", "-q", "--ff-only", commit); err == nil {
				return msg + " The working copy has it too.", false, nil
			}
		}
		msg += " Pull brings it into the working copy."
	}
	return msg, false, nil
}

// Discard sets the update aside: it is no longer offered or notified. Its
// branch stays on the remote until the next update replaces it.
func (u *Updates) Discard() (UpdateStatus, error) {
	u.mu.Lock()
	up := u.f.Update
	switch {
	case up == nil:
		u.mu.Unlock()
		return UpdateStatus{}, errorf(ErrNotFound, "there is no update")
	case u.busy:
		u.mu.Unlock()
		return UpdateStatus{}, errorf(ErrConflict, "the update is still running")
	case up.State != "ready" && up.State != "failed" && up.State != "error":
		u.mu.Unlock()
		return UpdateStatus{}, errorf(ErrConflict, "the update is %s: there is nothing to discard", up.State)
	}
	up.State = "discarded"
	u.save()
	u.mu.Unlock()
	return u.Status(context.Background()), nil
}

// events are the notifications the latest update calls for while it waits:
// ready to merge, failed its check, or could not run.
func (u *Updates) events() []Event {
	u.mu.Lock()
	defer u.mu.Unlock()
	up := u.f.Update
	if up == nil {
		return nil
	}
	id := up.Started.UTC().Format(time.RFC3339Nano)
	e := Event{Kind: "update-failed", Severity: "warning", Hosts: []string{}, Link: "#/inputs", Message: up.Message}
	switch up.State {
	case "ready":
		e.Kind, e.Severity, e.Hosts = "update-ready", "info", slices.Clone(up.Changed)
		e.Title = fmt.Sprintf("Flake update ready to merge into %s", up.Branch)
	case "failed":
		e.Title = "The flake update failed its check"
		if up.Summary != nil && len(up.Summary.Failed) > 0 {
			e.Hosts = slices.Clone(up.Summary.Failed)
		}
	case "error":
		e.Title, e.Message = "The flake update could not run", up.Error
	default:
		return nil
	}
	e.Key = e.Kind + ":" + id
	if e.Hosts == nil {
		e.Hosts = []string{}
	}
	return []Event{e}
}

// fleetBranch is the branch most machines' comin deploys, as Deployments
// shows it, from the working copy's machines.
func (s *Service) fleetBranch(ctx context.Context) (string, error) {
	for range 3 { // the working copy can move on while it is evaluated
		m, j, err := s.Jobs.Model(ctx, "working")
		if err != nil {
			return "", err
		}
		if m != nil {
			return fleetBranch(cominFollows(ctx, s.Repo, m)), nil
		}
		<-j.Done()
	}
	return "", errorf(ErrConflict, "the machines were evaluated again and again while the update waited for them: update again in a moment")
}

// wait waits for the run or check going on; for tests.
func (u *Updates) wait() { u.wg.Wait() }
