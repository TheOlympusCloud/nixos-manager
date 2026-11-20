package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// lockAt is a flake.lock with nixpkgs at rev, plus extra JSON fields.
func lockAt(rev string, modified int64, extra string) string {
	return fmt.Sprintf(`{"nodes":{"nixpkgs":{"locked":{"lastModified":%d,"owner":"NixOS","repo":"nixpkgs","rev":"%s","type":"github"%s},"original":{"owner":"NixOS","ref":"nixos-26.05","repo":"nixpkgs","type":"github"}},"root":{"inputs":{"nixpkgs":"nixpkgs"}}},"root":"root","version":7}`+"\n", modified, rev, extra)
}

const (
	oldRev = "aaaaaaa1111111111111111111111111111111111"
	newRev = "bbbbbbb2222222222222222222222222222222222"
)

// updateNix is buildNix where every machine's system follows flake.lock too,
// a build fails when flake.lock says "broken", and nix flake update writes the
// file the returned path names (when it exists) as the new flake.lock.
func updateNix(t *testing.T) (string, string) {
	t.Helper()
	next := filepath.Join(t.TempDir(), "next.lock")
	inner := strings.NewReplacer(
		`h=$(sha256sum < "$f" | cut -c1-12)`, `h=$(cat "$f" "$flake/flake.lock" 2>/dev/null | sha256sum | cut -c1-12)`,
		`grep -q broken "$src/hosts/$name"`, `grep -qs broken "$src/hosts/$name" "$src/flake.lock"`,
	).Replace(buildNix)
	return script(t, "nix", "#!/bin/sh\ncase \"$*\" in *\"flake update\"*)\n  echo \"nix $*\"; d=${*##*path:}\n  [ -f '"+next+"' ] && cp '"+next+"' \"$d/flake.lock\"\n  exit 0 ;;\nesac\nexec '"+script(t, "inner-nix", inner)+"' \"$@\"\n"), next
}

// updateService is buildService with nixpkgs locked at oldRev, pushed and
// checked, and Updates on it; next names the lock nix flake update writes.
func updateService(t *testing.T) (*Service, *Updates, string) {
	t.Helper()
	ctx := context.Background()
	s := buildService(t, true)
	nix, next := updateNix(t)
	s.Jobs.Nix = nix
	write(t, s.Repo, "flake.lock", lockAt(oldRev, 1793000000, ""))
	if info := runCheck(t, s); info.State != "passed" {
		t.Fatalf("check:\n%s", info.Output)
	}
	must[string](t)(s.Commit(ctx, "lock nixpkgs", false))
	must[string](t)(s.Push(ctx, false))
	u := must[*Updates](t)(OpenUpdates(t.TempDir(), s))
	s.Updates = u
	return s, u, next
}

// runUpdate starts an update and waits for it.
func runUpdate(t *testing.T, ctx context.Context, u *Updates) *Update {
	t.Helper()
	if _, err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	u.wait()
	return u.Status(ctx).Update
}

// The update is made on top of the fleet's branch on the remote, pushed to
// flake-update and checked; the working copy, its index and main stay as they
// were until Merge, which fast-forwards main on the remote.
func TestUpdateOnABranchThenMerge(t *testing.T) {
	ctx := context.Background()
	s, u, next := updateService(t)
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, "")), 0o644))
	main := remoteRef(t, s, "main")
	write(t, s.Repo, "notes", "staged\n")
	ok(t, s.Stage(ctx, []string{"notes"}))
	write(t, s.Repo, "hosts/web", "web kernel=9\n") // not staged
	before := gitOut(t, s.Repo.Dir, "status", "--porcelain")

	up := runUpdate(t, WithCaller(ctx, Caller{Via: "web session abc"}), u)
	if up.State != "ready" || up.Scheduled || up.Branch != "main" || up.Base != main || up.Error != "" {
		t.Fatalf("update %+v", up)
	}
	if gitOut(t, s.Repo.Dir, "rev-parse", up.Commit+"^") != main {
		t.Fatal("the update is not on top of main")
	}
	if got := gitOut(t, s.Repo.Dir, "show", up.Commit+":flake.lock"); got != strings.TrimSpace(lockAt(newRev, 1794000000, "")) {
		t.Fatalf("flake.lock of the update: %s", got)
	}
	if got := gitOut(t, s.Repo.Dir, "diff", "--name-only", main, up.Commit); got != "flake.lock" {
		t.Fatalf("the update changes %q", got)
	}
	msg := gitOut(t, s.Repo.Dir, "log", "-1", "--format=%B", up.Commit)
	for _, want := range []string{"flake.lock: update nixpkgs\n", "nixpkgs: aaaaaaa → bbbbbbb (2026-10-26 → 2026-11-06)", "Via: web session abc"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if a := gitOut(t, s.Repo.Dir, "log", "-1", "--format=%an <%ae> %cn <%ce>", up.Commit); a != "Test User <test@example.invalid> Test User <test@example.invalid>" {
		t.Errorf("author and committer: %s", a)
	}
	if remoteRef(t, s, "flake-update") != up.Commit || remoteRef(t, s, "main") != main {
		t.Fatal("flake-update is not the update, or main moved")
	}
	if gitOut(t, s.Repo.Dir, "status", "--porcelain") != before || fileText(t, s.Repo, "flake.lock") != lockAt(oldRev, 1793000000, "") {
		t.Fatal("the update touched the working copy")
	}
	if b := must[Branches](t)(s.Branches(ctx)); !slices.Equal(b.Branches, []string{"main"}) {
		t.Fatalf("branches %v", b.Branches)
	}
	if len(up.Inputs) != 1 || up.Inputs[0].Name != "nixpkgs" || up.Inputs[0].FromRev != oldRev || up.Inputs[0].ToRev != newRev {
		t.Fatalf("inputs %+v", up.Inputs)
	}
	if !slices.Equal(up.Changed, []string{"db", "web"}) || up.Summary == nil || !slices.Equal(up.Summary.Built, []string{"db", "web"}) {
		t.Fatalf("changed %v, summary %+v", up.Changed, up.Summary)
	}
	if !strings.HasPrefix(up.Message, "Inputs: nixpkgs aaaaaaa → bbbbbbb.\ndb: other configuration\nweb: other configuration") {
		t.Fatalf("message %q", up.Message)
	}
	if job, _ := s.Jobs.Get(up.Job); job == nil || job.Info(false).Kind != "update-branch" || job.Info(false).State != "passed" {
		t.Fatal("no update-branch job")
	}
	ev := u.events()
	if len(ev) != 1 || ev[0].Kind != "update-ready" || !strings.HasPrefix(ev[0].Key, "update-ready:") || ev[0].Title != "Flake update ready to merge into main" ||
		!slices.Equal(ev[0].Hosts, []string{"db", "web"}) || ev[0].Link != "#/inputs" || ev[0].Message != up.Message {
		t.Fatalf("events %+v", ev)
	}

	_, err := u.Merge(ctx, main)
	isErr(t, err, ErrConflict)
	msg, err = u.Merge(ctx, up.Commit)
	ok(t, err)
	if !strings.Contains(msg, "Merged the update into main") || !strings.Contains(msg, "Pull brings it") {
		t.Fatalf("merge: %s", msg)
	}
	if remoteRef(t, s, "main") != up.Commit {
		t.Fatal("main is not the update")
	}
	if st := u.Status(ctx); st.Update.State != "merged" || st.Update.Merged.IsZero() || st.Outdated {
		t.Fatalf("status %+v", st)
	}
	if ev := u.events(); len(ev) != 0 {
		t.Fatalf("events after the merge: %+v", ev)
	}
	if gitOut(t, s.Repo.Dir, "status", "--porcelain") != before {
		t.Fatal("the merge touched a working copy with changes")
	}
	_, err = u.Merge(ctx, up.Commit)
	isErr(t, err, ErrConflict)
}

// A working copy on the branch with nothing of its own gets the merged update.
func TestUpdateMergeFastForwardsACleanWorkingCopy(t *testing.T) {
	ctx := context.Background()
	s, u, next := updateService(t)
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, "")), 0o644))
	up := runUpdate(t, ctx, u)
	msg, err := u.Merge(ctx, up.Commit)
	ok(t, err)
	if !strings.Contains(msg, "The working copy has it too.") || gitOut(t, s.Repo.Dir, "rev-parse", "HEAD") != up.Commit ||
		fileText(t, s.Repo, "flake.lock") != lockAt(newRev, 1794000000, "") {
		t.Fatalf("merge: %s", msg)
	}
}

// When no input moved there is nothing to commit, push or tell.
func TestUpdateWithNothingNew(t *testing.T) {
	ctx := context.Background()
	s, u, _ := updateService(t) // nix flake update writes the same lock
	up := runUpdate(t, ctx, u)
	if up.State != "current" || up.Commit != "" || up.Finished.IsZero() || remoteRef(t, s, "flake-update") != "" || len(u.events()) != 0 {
		t.Fatalf("update %+v", up)
	}
	_, err := u.Discard()
	isErr(t, err, ErrConflict)
}

// A later update replaces the last one on flake-update; a branch of that name
// this console did not push is never overwritten.
func TestUpdateReplacesOnlyItsOwnBranch(t *testing.T) {
	ctx := context.Background()
	s, u, next := updateService(t)
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, "")), 0o644))
	first := runUpdate(t, ctx, u)
	ok(t, os.WriteFile(next, []byte(lockAt("ccccccc3333333333333333333333333333333333", 1795000000, "")), 0o644))
	second := runUpdate(t, ctx, u)
	if second.State != "ready" || second.Commit == first.Commit || remoteRef(t, s, "flake-update") != second.Commit {
		t.Fatalf("second update %+v", second)
	}
	ev := u.events()
	if len(ev) != 1 || ev[0].Key == "update-ready:"+first.Started.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("events %+v", ev)
	}

	clone := laptop(t, s)
	gitOut(t, clone, "switch", "-q", "--no-track", "-c", "flake-update", "origin/main")
	ok(t, os.WriteFile(filepath.Join(clone, "mine"), []byte("someone's own branch\n"), 0o644))
	gitOut(t, clone, "add", "-A")
	gitOut(t, clone, "commit", "-q", "-m", "mine")
	gitOut(t, clone, "push", "-q", "-f", "origin", "flake-update")
	theirs := gitOut(t, clone, "rev-parse", "HEAD")
	third := runUpdate(t, ctx, u)
	if third.State != "error" || !strings.Contains(third.Error, "did not push there") || remoteRef(t, s, "flake-update") != theirs {
		t.Fatalf("update over someone's branch %+v", third)
	}
	if ev := u.events(); len(ev) != 1 || ev[0].Kind != "update-failed" || ev[0].Title != "The flake update could not run" || ev[0].Message != third.Error {
		t.Fatalf("events %+v", ev)
	}
}

// An update made before main moved is not merged over the new commits.
func TestUpdateMergeRefusesWhenMainMoved(t *testing.T) {
	ctx := context.Background()
	s, u, next := updateService(t)
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, "")), 0o644))
	up := runUpdate(t, ctx, u)
	moved := pushFrom(t, s, laptop(t, s), "notes", "laptop\n")
	if !u.Status(ctx).Outdated {
		t.Fatal("not outdated after main moved")
	}
	_, err := u.Merge(ctx, up.Commit)
	isErr(t, err, ErrConflict)
	if !strings.Contains(err.Error(), "moved on") || remoteRef(t, s, "main") != moved {
		t.Fatalf("merge: %v", err)
	}
	if st := u.Status(ctx); st.Update.State != "ready" {
		t.Fatalf("state %+v", st.Update)
	}
}

// An update whose check fails is told, never merged, and can be discarded.
func TestUpdateThatFailsItsCheck(t *testing.T) {
	ctx := context.Background()
	_, u, next := updateService(t)
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, `,"note":"broken"`)), 0o644))
	up := runUpdate(t, ctx, u)
	if up.State != "failed" || up.Summary == nil || !slices.Equal(up.Summary.Failed, []string{"db", "web"}) ||
		!strings.Contains(up.Message, "Its check failed: db and web do not build.") {
		t.Fatalf("update %+v", up)
	}
	ev := u.events()
	if len(ev) != 1 || ev[0].Kind != "update-failed" || ev[0].Severity != "warning" || !slices.Equal(ev[0].Hosts, []string{"db", "web"}) {
		t.Fatalf("events %+v", ev)
	}
	_, err := u.Merge(ctx, up.Commit)
	isErr(t, err, ErrNotChecked)
	st := must[UpdateStatus](t)(u.Discard())
	if st.Update.State != "discarded" || len(u.events()) != 0 {
		t.Fatalf("after discard %+v", st.Update)
	}
	_, err = u.Discard()
	isErr(t, err, ErrConflict)
	_, err = u.Merge(ctx, up.Commit)
	isErr(t, err, ErrConflict)
}

// When main's own commit passes a check after the update's check began, the
// update is checked again against it before it is merged, as Push would ask.
func TestUpdateMergeChecksAgainstTheNewestCheckedCommit(t *testing.T) {
	ctx := context.Background()
	s, u, next := updateService(t)
	tip := pushFrom(t, s, laptop(t, s), "notes", "laptop\n") // not checked
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, "")), 0o644))
	up := runUpdate(t, ctx, u)
	if up.State != "ready" || up.Summary.Base != treeOf(t, s, tip+"^") {
		t.Fatalf("update %+v", up)
	}
	job, _, err := s.Jobs.CheckCommit(ctx, tip, "main")
	ok(t, err)
	wait(t, job)

	_, err = u.Merge(ctx, up.Commit)
	isErr(t, err, ErrNotChecked)
	u.wait()
	up = u.Status(ctx).Update
	if up.State != "ready" || up.Summary.Base != treeOf(t, s, tip) {
		t.Fatalf("checked again %+v", up)
	}
	must[string](t)(u.Merge(ctx, up.Commit))
	if remoteRef(t, s, "main") != up.Commit {
		t.Fatal("not merged")
	}
}

// A stop during the check checks again when the server starts; a stop while
// Nix updates leaves an error.
func TestUpdateResumesItsCheckAfterARestart(t *testing.T) {
	ctx := context.Background()
	s, u, next := updateService(t)
	ok(t, os.WriteFile(next, []byte(lockAt(newRev, 1794000000, "")), 0o644))
	block := filepath.Join(t.TempDir(), "block")
	ok(t, os.WriteFile(block, nil, 0o644))
	inner := s.Jobs.Nix
	s.Jobs.Nix = script(t, "blocking-nix", "#!/bin/sh\ncase \"$*\" in *\"flake check\"*) while [ -f '"+block+"' ]; do sleep 0.05; done ;; esac\nexec '"+inner+"' \"$@\"\n")
	must[UpdateStatus](t)(u.Start(ctx))
	deadline := time.Now().Add(10 * time.Second)
	for u.Status(ctx).Update.State != "checking" || u.Status(ctx).Update.Check == "" {
		if time.Now().After(deadline) {
			t.Fatalf("never checking: %+v", u.Status(ctx).Update)
		}
		time.Sleep(20 * time.Millisecond)
	}

	again := must[*Updates](t)(OpenUpdates(filepath.Dir(u.path), s)) // the same file, as after a restart
	if st := again.Status(ctx).Update; st.State != "checking" || st.Commit == "" {
		t.Fatalf("reopened %+v", st)
	}
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go again.Run(rctx)
	ok(t, os.Remove(block))
	u.wait()
	deadline = time.Now().Add(10 * time.Second)
	for again.Status(ctx).Update.State != "ready" {
		if time.Now().After(deadline) {
			t.Fatalf("not ready after the restart: %+v", again.Status(ctx).Update)
		}
		time.Sleep(20 * time.Millisecond)
	}
	again.wait()

	// A stop while Nix updated the inputs leaves an error to update again from.
	var f updatesFile
	ok(t, json.Unmarshal([]byte(must[[]byte](t)(os.ReadFile(again.path))), &f))
	f.Update.State = "updating"
	b, _ := json.Marshal(f)
	ok(t, os.WriteFile(again.path, b, 0o600))
	third := must[*Updates](t)(OpenUpdates(filepath.Dir(again.path), s))
	if st := third.Status(ctx).Update; st.State != "error" || !strings.Contains(st.Error, "stopped") {
		t.Fatalf("after a stop while updating %+v", st)
	}
}

func TestUpdateSchedule(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	ok(t, err)
	at := func(s string) time.Time {
		t.Helper()
		v, err := time.ParseInLocation("2006-01-02 15:04", s, madrid)
		ok(t, err)
		return v
	}
	for _, c := range []struct {
		s           UpdateSchedule
		after, want string
	}{
		{UpdateSchedule{Day: -1, Hour: 4}, "2026-11-20 03:59", "2026-11-20 04:00"},
		{UpdateSchedule{Day: -1, Hour: 4}, "2026-11-20 04:00", "2026-11-21 04:00"},
		{UpdateSchedule{Day: 1, Hour: 4}, "2026-11-20 10:00", "2026-11-23 04:00"}, // Friday → Monday
		{UpdateSchedule{Day: 1, Hour: 4}, "2026-11-23 05:00", "2026-11-30 04:00"}, // Monday, after the slot
		{UpdateSchedule{Day: 5, Hour: 23}, "2026-11-20 22:00", "2026-11-20 23:00"},
		{UpdateSchedule{Day: 0, Hour: 2}, "2026-03-28 12:00", "2026-03-29 03:00"},  // 02:00 does not exist that night
		{UpdateSchedule{Day: -1, Hour: 2}, "2026-10-25 01:00", "2026-10-25 02:00"}, // the first of two 02:00
	} {
		if got := nextRun(c.s, at(c.after), madrid); !got.Equal(at(c.want)) {
			t.Errorf("nextRun(%+v, %s) = %v, want %s", c.s, c.after, got, c.want)
		}
	}
	if got := nextRun(UpdateSchedule{Day: -1, Hour: 4}, at("2026-11-20 03:00").UTC(), madrid); !got.Equal(at("2026-11-20 04:00")) {
		t.Errorf("a time in UTC: %v", got)
	}
}

// The schedule starts an update at its slot, once, never for a slot that was
// already past when it was saved, and skips a slot while another update runs.
func TestUpdateTick(t *testing.T) {
	ctx := context.Background()
	s, u, _ := updateService(t)
	madrid, _ := time.LoadLocation("Europe/Madrid")
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, madrid)
	u.now, u.loc = func() time.Time { return now }, madrid
	for _, bad := range []UpdateSchedule{{Day: 7}, {Day: -2}, {Hour: 24}, {Hour: -1}} {
		_, err := u.SetSchedule(bad)
		isErr(t, err, ErrInvalid)
	}
	st := must[UpdateStatus](t)(u.SetSchedule(UpdateSchedule{Enabled: true, Day: -1, Hour: 4}))
	if !st.Next.Equal(time.Date(2026, 11, 21, 4, 0, 0, 0, madrid)) || st.Update != nil {
		t.Fatalf("status %+v", st)
	}
	u.Tick(now.Add(time.Hour))
	u.wait()
	if u.Status(ctx).Update != nil {
		t.Fatal("ran before its slot")
	}
	now = time.Date(2026, 11, 23, 9, 0, 0, 0, madrid) // three slots missed while down
	u.Tick(now)
	u.wait()
	up := u.Status(ctx).Update
	if up == nil || !up.Scheduled || up.State != "current" {
		t.Fatalf("scheduled update %+v", up)
	}
	started := up.Started
	u.Tick(now.Add(time.Minute))
	u.wait()
	if u.Status(ctx).Update.Started != started {
		t.Fatal("ran twice for the missed slots")
	}
	if got := u.Status(ctx).Next; !got.Equal(time.Date(2026, 11, 24, 4, 0, 0, 0, madrid)) {
		t.Fatalf("next %v", got)
	}

	u.mu.Lock()
	u.busy = true
	u.mu.Unlock()
	now = time.Date(2026, 11, 24, 4, 0, 0, 0, madrid)
	u.Tick(now)
	_, err := u.Start(ctx)
	isErr(t, err, ErrConflict)
	u.mu.Lock()
	u.busy = false
	u.mu.Unlock()
	if u.Status(ctx).Update.Started != started || !u.Status(ctx).Next.Equal(time.Date(2026, 11, 25, 4, 0, 0, 0, madrid)) {
		t.Fatal("the slot during another update was not skipped")
	}
	must[UpdateStatus](t)(u.SetSchedule(UpdateSchedule{Day: -1, Hour: 4}))
	u.Tick(now.Add(48 * time.Hour))
	if u.Status(ctx).Update.Started != started || !u.Status(ctx).Next.IsZero() {
		t.Fatal("ran while off")
	}
	_ = s
}

// Without a remote there is nothing to update on top of.
func TestUpdateNeedsARemote(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, false)
	u := must[*Updates](t)(OpenUpdates(t.TempDir(), s))
	if up := runUpdate(t, ctx, u); up.State != "error" || !strings.Contains(up.Error, "no remote") {
		t.Fatalf("update %+v", up)
	}
}

func TestUpdateMessage(t *testing.T) {
	got := updateMessage([]InputChange{
		{Name: "home-manager", Status: "updated", FromRev: "1111111aaaa", ToRev: "2222222bbbb"},
		{Name: "nixpkgs", Status: "updated", FromRev: oldRev, ToRev: newRev, FromDate: 1793000000, ToDate: 1794000000},
	}, "scheduled update")
	want := "flake.lock: update home-manager, nixpkgs\n\nhome-manager: 1111111 → 2222222\nnixpkgs: aaaaaaa → bbbbbbb (2026-10-26 → 2026-11-06)\n\nVia: scheduled update\n"
	if got != want {
		t.Fatalf("message:\n%s\nwant:\n%s", got, want)
	}
	many := []InputChange{{Name: "a", Status: "added", ToRev: newRev}, {Name: "b", Status: "removed"}, {Name: "c"}, {Name: "d"}}
	if got := updateMessage(many, "x"); !strings.HasPrefix(got, "flake.lock: update 4 inputs\n\na: added at bbbbbbb\nb: removed\n") {
		t.Fatalf("message:\n%s", got)
	}
}

func TestUpdateNotice(t *testing.T) {
	up := &Update{State: "ready", Inputs: []InputChange{{Name: "nixpkgs", Status: "updated", FromRev: oldRev, ToRev: newRev}, {Name: "x", Status: "added"}}}
	diffs := []HostDiff{
		{Host: "db", Status: "unchanged"},
		{Host: "pi", Status: "changed", Reboot: true, Kernel: &VersionChange{Name: "linux", From: "6.6.50", To: "6.6.52"},
			VersionsChanged: []VersionChange{{Name: "firefox", From: "130.0", To: "131.0"}, {Name: "git", From: "2.50", To: "2.51"}, {Name: "vim", From: "9.0", To: "9.1"}}},
		{Host: "web", Status: "changed", VersionsChanged: []VersionChange{{Name: "nginx", From: "1.26", To: "1.27"}}},
		{Host: "box", Status: "changed"},
	}
	want := "Inputs: nixpkgs aaaaaaa → bbbbbbb, x (added).\npi: kernel 6.6.50 → 6.6.52, firefox 130.0 → 131.0, git 2.50 → 2.51 and 1 more; reboot needed\nweb: nginx 1.26 → 1.27\nbox: other configuration"
	if got := updateNotice(up, diffs); got != want {
		t.Fatalf("notice:\n%s\nwant:\n%s", got, want)
	}
	if got := updateNotice(up, []HostDiff{{Host: "db", Status: "unchanged"}}); !strings.HasSuffix(got, "\nNo machine's system changes.") {
		t.Fatalf("nothing changes: %q", got)
	}
	var lots []HostDiff
	for i := range 9 {
		lots = append(lots, HostDiff{Host: fmt.Sprintf("m%d", i), Status: "changed"})
	}
	if got := updateNotice(up, lots); !strings.HasSuffix(got, "\nm5: other configuration\nAnd 3 more machines.") {
		t.Fatalf("many machines: %q", got)
	}
	failed := &Update{State: "failed", Inputs: up.Inputs[:1], Summary: &CheckSummary{Failed: []string{"pi"}}}
	if got := updateNotice(failed, nil); got != "Inputs: nixpkgs aaaaaaa → bbbbbbb. Its check failed: pi does not build." {
		t.Fatalf("failed: %q", got)
	}
}
