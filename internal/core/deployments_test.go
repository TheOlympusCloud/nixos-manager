package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func sha(c byte) string { return strings.Repeat(string(c), 40) }

func TestDeploymentReportsAreValidatedAndKept(t *testing.T) {
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	now := time.Now()
	good := ReportIn{Host: "pi", Commit: sha('a'), Ref: "origin/main", Status: "done", Generation: "5f1c-77"}
	for name, in := range map[string]ReportIn{
		"host":       {Host: "../pi", Commit: sha('a'), Status: "done"},
		"long host":  {Host: strings.Repeat("h", 64), Commit: sha('a'), Status: "done"},
		"short sha":  {Host: "pi", Commit: "abc123", Status: "done"},
		"upper sha":  {Host: "pi", Commit: strings.ToUpper(sha('a')), Status: "done"},
		"status":     {Host: "pi", Commit: sha('a'), Status: "running"},
		"phase":      {Host: "pi", Commit: sha('a'), Phase: "eval", Status: "failed"},
		"build done": {Host: "pi", Commit: sha('a'), Phase: "build", Status: "done"},
		"ref":        {Host: "pi", Commit: sha('a'), Status: "done", Ref: "origin/main\n"},
		"generation": {Host: "pi", Commit: sha('a'), Status: "done", Generation: strings.Repeat("g", 65)},
		"error":      {Host: "pi", Commit: sha('a'), Status: "failed", Error: strings.Repeat("e", 8193)},
		"utf8":       {Host: "pi", Commit: sha('a'), Status: "failed", Error: "\xff"},
	} {
		if err := d.Record(in, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
	ok(t, d.Record(good, now))
	ok(t, d.Record(ReportIn{Host: "pi", Commit: sha('b'), Phase: "build", Status: "already built"}, now))
	for i := 0; i < keepReports+5; i++ {
		ok(t, d.Record(ReportIn{Host: "oldbox", Commit: sha('c'), Status: "failed", Error: "exit status 4"}, now))
	}
	fi, err := os.Stat(filepath.Join(dir, "deployments.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("deployments.json: %v %v", fi, err)
	}

	again := must[*Deployments](t)(OpenDeployments(dir, d.repo))
	if got := again.reports["pi"]; len(got) != 2 || got[0].Status != "already built" || got[1].Ref != "origin/main" || got[1].Phase != "deploy" {
		t.Fatalf("pi after reopening: %+v", got)
	}
	if n := len(again.reports["oldbox"]); n != keepReports {
		t.Fatalf("oldbox keeps %d reports, want %d", n, keepReports)
	}
}

func TestDeploymentTokenAndMachineCap(t *testing.T) {
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	tok := d.Token()
	if !strings.HasPrefix(tok, "nxd_") || !d.CheckToken(tok) || d.CheckToken("") || d.CheckToken(tok+"x") || d.CheckToken(" "+tok) {
		t.Fatalf("token %q checks wrong", tok)
	}
	if fi, err := os.Stat(filepath.Join(dir, "deploy-token")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file: %v %v", fi, err)
	}
	if again := must[*Deployments](t)(OpenDeployments(dir, d.repo)); again.Token() != tok {
		t.Fatal("the token changed on reopening")
	}
	fresh := must[string](t)(d.RegenerateToken())
	if fresh == tok || d.CheckToken(tok) || !d.CheckToken(fresh) {
		t.Fatal("regenerating did not replace the token")
	}

	var nilD *Deployments
	if nilD.CheckToken(fresh) || nilD.Record(ReportIn{}, time.Now()) == nil {
		t.Fatal("a missing store accepted a report")
	}

	for i := range maxReporters {
		ok(t, d.Record(ReportIn{Host: "m" + string(rune('a'+i%26)) + strings.Repeat("x", i/26), Commit: sha('a'), Status: "done"}, time.Now()))
	}
	isErr(t, d.Record(ReportIn{Host: "onemore", Commit: sha('a'), Status: "done"}, time.Now()), ErrInvalid)
	ok(t, d.Record(ReportIn{Host: "ma", Commit: sha('b'), Status: "done"}, time.Now())) // known machines still report
}

func TestDeploymentOverview(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "flake.nix", "1\n")
	c1 := commitAll(t, r, "first")
	write(t, r, "flake.nix", "2\n")
	c2 := commitAll(t, r, "second")
	write(t, r, "flake.nix", "3\n")
	c3 := commitAll(t, r, "third")
	// pi tries a change on its testing branch, on top of main.
	gitOut(t, r.Dir, "branch", "testing-pi")
	gitOut(t, r.Dir, "switch", "-q", "testing-pi")
	write(t, r, "pi.nix", "test\n")
	ct := commitAll(t, r, "try on pi")
	gitOut(t, r.Dir, "switch", "-q", "main")
	// A testing branch behind main is ignored by comin.
	gitOut(t, r.Dir, "branch", "testing-stale", c1)

	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), r))
	d.started = time.Time{}
	now := time.Now()
	at := func(ago time.Duration, host, commit, phase, status, msg string) {
		t.Helper()
		ok(t, d.Record(ReportIn{Host: host, Commit: commit, Phase: phase, Status: status, Error: msg}, now.Add(-ago)))
	}
	at(time.Minute, "homeserver", c3, "", "done", "")
	at(3*time.Hour, "workstation", c1, "", "done", "")
	at(10*24*time.Hour, "oldbox", c1, "", "done", "")
	at(2*time.Hour, "laptop", c2, "", "done", "")
	at(time.Hour, "laptop", c3, "build", "failed", "error: attribute 'foo' missing")
	at(time.Minute, "nas", c2, "", "done", "")
	at(30*time.Second, "nas", c3, "build", "already built", "") // nothing changed for it: no deployment follows
	at(time.Minute, "pi", ct, "", "done", "")
	at(time.Minute, "stale", c3, "", "done", "")
	at(time.Minute, "ghost", sha('e'), "", "done", "")
	at(2*time.Minute, "pc", c2, "", "done", "")
	at(time.Minute, "pc", c3, "build", "built", "") // built, but comin has not switched to it yet

	ov := must[DeployOverview](t)(d.Overview(ctx, fleet("homeserver", "newbox", "pi"), now))
	if !ov.Reporting || ov.Branch != "main" || len(ov.Commits) != 3 || ov.Commits[0].SHA != c3 {
		t.Fatalf("overview: %+v", ov)
	}
	got := map[string]MachineDeployment{}
	for _, m := range ov.Machines {
		got[m.Host] = m
	}
	if len(got) != 10 {
		t.Fatalf("machines: %v", ov.Machines)
	}
	want := map[string]string{"homeserver": "current", "workstation": "behind", "oldbox": "behind", "laptop": "failed",
		"nas": "current", "pi": "current", "stale": "current", "ghost": "unknown", "newbox": "never", "pc": "behind"}
	for h, st := range want {
		if got[h].State != st {
			t.Errorf("%s: state %q, want %q (%+v)", h, got[h].State, st, got[h])
		}
	}
	for h, m := range got {
		if m.Silent != (h == "oldbox") {
			t.Errorf("%s: silent %v", h, m.Silent)
		}
	}
	if w := got["workstation"]; w.Behind != 2 || w.Commit != c1 || w.Subject != "first" || w.BehindSince.IsZero() || now.Sub(w.BehindSince) > time.Minute || w.Target != c3 {
		t.Errorf("workstation: %+v", w)
	}
	if l := got["laptop"]; l.Failure == nil || l.Failure.Commit != c3 || l.Failure.Phase != "build" || l.Commit != c2 || l.Behind != 1 {
		t.Errorf("laptop: %+v", l)
	}
	if p := got["pi"]; p.Branch != "testing-pi" || p.Target != ct || p.Subject != "try on pi" {
		t.Errorf("pi follows %q at %q", p.Branch, p.Target)
	}
	if s := got["stale"]; s.Branch != "main" {
		t.Errorf("a testing branch behind main was followed: %+v", s)
	}
	if n := got["newbox"]; n.Commit != "" || len(n.Reports) != 0 || !n.LastSeen.IsZero() {
		t.Errorf("newbox: %+v", n)
	}
	if p := got["pc"]; p.Commit != c2 || p.Behind != 1 {
		t.Errorf("a build that is not deployed yet is not what pc runs: %+v", p)
	}
	at(0, "pc", c3, "deploy", "done", "")
	ov = must[DeployOverview](t)(d.Overview(ctx, fleet("pc"), now))
	if i := slices.IndexFunc(ov.Machines, func(m MachineDeployment) bool { return m.Host == "pc" }); i < 0 || ov.Machines[i].State != "current" || ov.Machines[i].Commit != c3 {
		t.Errorf("pc after deploying c3: %+v", ov.Machines)
	}

	var nilD *Deployments
	if ov := must[DeployOverview](t)(nilD.Overview(ctx, fleet("pi"), now)); ov.Reporting || len(ov.Machines) != 0 {
		t.Fatalf("no store: %+v", ov)
	}
}

// fleet is an evaluated fleet of these machines, without comin settings.
func fleet(hosts ...string) *Model {
	m := &Model{Hosts: []Host{}}
	for _, h := range hosts {
		m.Hosts = append(m.Hosts, Host{Name: h})
	}
	return m
}

// Each machine is compared with the branch its own comin follows: the one set
// for the remote this console pushes to, else for its first remote. Machines
// without comin settings follow the branch most of the fleet follows.
func TestDeploymentOverviewFollowsCominBranches(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "flake.nix", "1\n")
	c1 := commitAll(t, r, "first")
	write(t, r, "flake.nix", "2\n")
	c2 := commitAll(t, r, "second")
	gitOut(t, r.Dir, "switch", "-q", "-c", "try-a", c1)
	write(t, r, "a.nix", "try\n")
	ct := commitAll(t, r, "try on a")
	gitOut(t, r.Dir, "switch", "-q", "main")
	gitOut(t, r.Dir, "remote", "add", "origin", "ssh://git@forgejo.lan:2222/pablo/fleet.git")
	for ref, c := range map[string]string{"main": c2, "stable": c1, "try-a": ct} {
		gitOut(t, r.Dir, "update-ref", "refs/remotes/origin/"+ref, c)
	}

	m := fleet("a", "b", "c", "d")
	m.Hosts[0].Checks.CominBranches = []CominBranches{
		{URL: "https://elsewhere.example/x.git", Main: "main", Testing: "testing-a"},
		{URL: "https://forgejo.lan/pablo/fleet", Main: "stable", Testing: "try-a"}, // origin, over HTTPS
	}
	m.Hosts[1].Checks.CominBranches = []CominBranches{{URL: "https://elsewhere.example/x.git", Main: "stable", Testing: "testing-b"}}
	m.Hosts[3].Checks.CominBranches = []CominBranches{{URL: "https://forgejo.lan/pablo/fleet.git", Main: "main", Testing: "testing-d"}}
	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), r))
	now := time.Now()
	for _, h := range []string{"a", "b", "c", "d", "gone"} {
		ok(t, d.Record(ReportIn{Host: h, Commit: c1, Status: "done"}, now))
	}

	ov := must[DeployOverview](t)(d.Overview(ctx, m, now))
	if ov.Branch != "stable" || len(ov.Commits) != 1 || ov.Commits[0].SHA != c1 {
		t.Fatalf("the fleet follows %q: %+v", ov.Branch, ov.Commits)
	}
	got := map[string]MachineDeployment{}
	for _, md := range ov.Machines {
		got[md.Host] = md
	}
	for h, want := range map[string][3]string{
		"a": {"try-a", ct, "behind"}, "b": {"stable", c1, "current"}, "c": {"stable", c1, "current"},
		"d": {"main", c2, "behind"}, "gone": {"stable", c1, "current"},
	} {
		if g := got[h]; g.Branch != want[0] || g.Target != want[1] || g.State != want[2] {
			t.Errorf("%s follows %q at %.7s (%s), want %v", h, g.Branch, g.Target, g.State, want)
		}
	}
}

// Reports of machines that left the flake are dropped, but only when the
// working copy is exactly what comin deploys: on another branch, or with an
// uncommitted deletion, the machine may still be in service.
func TestReportsOfRemovedMachinesArePruned(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "flake.nix", "1\n")
	c1 := commitAll(t, r, "first")
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, r))
	now := time.Now()
	for _, h := range []string{"kept", "gone"} {
		ok(t, d.Record(ReportIn{Host: h, Commit: c1, Status: "done"}, now))
	}
	hosts := func(ov DeployOverview) (out []string) {
		for _, m := range ov.Machines {
			out = append(out, m.Host)
		}
		return out
	}
	tip := must[string](t)(r.WorkingTree(ctx))
	m := fleet("kept")
	for name, model := range map[string]*Model{
		"no model":       nil,
		"broken flake":   {Tree: tip, Error: "error: syntax error"},
		"another tree":   {Tree: strings.Repeat("0", 40), Hosts: m.Hosts},
		"unknown tree":   {Hosts: m.Hosts},
		"no main branch": {Tree: tip, Hosts: m.Hosts},
	} {
		if name == "no main branch" {
			gitOut(t, r.Dir, "switch", "-q", "-c", "other")
			write(t, r, "flake.nix", "2\n")
			commitAll(t, r, "elsewhere")
			gitOut(t, r.Dir, "branch", "-q", "-D", "main") // main is gone: no tip to compare with
		}
		if got := hosts(must[DeployOverview](t)(d.Overview(ctx, model, now))); !slices.Equal(got, []string{"gone", "kept"}) {
			t.Fatalf("%s: machines %v", name, got)
		}
	}
	gitOut(t, r.Dir, "switch", "-q", "-c", "main", c1)

	// Machines that may still be in service from another branch keep their reports:
	// one that runs a commit main does not have (nas, defined only on stable), and one
	// that comin deploys from another branch pinned at an older commit of main (old).
	gitOut(t, r.Dir, "switch", "-q", "-c", "stable")
	write(t, r, "nas.nix", "{ }\n")
	cs := commitAll(t, r, "add nas on stable")
	gitOut(t, r.Dir, "switch", "-q", "main")
	ok(t, d.Record(ReportIn{Host: "nas", Commit: cs, Ref: "origin/stable", Status: "done"}, now))
	ok(t, d.Record(ReportIn{Host: "old", Commit: c1, Ref: "origin/stable", Status: "done"}, now))
	ok(t, d.Record(ReportIn{Host: "gone", Commit: c1, Ref: "origin/main", Status: "done"}, now))
	ok(t, d.Record(ReportIn{Host: "mac", Commit: c1, Status: "done"}, now)) // nix-darwin: in the flake, not in the model's hosts
	if got := hosts(must[DeployOverview](t)(d.Overview(ctx, &Model{Tree: tip, Hosts: m.Hosts, Darwin: []string{"mac"}}, now))); !slices.Equal(got, []string{"kept", "mac", "nas", "old"}) {
		t.Fatalf("at the tip: machines %v", got)
	}
	if again := must[*Deployments](t)(OpenDeployments(dir, r)); again.reports["gone"] != nil || again.reports["kept"] == nil {
		t.Fatalf("after reopening: %v", again.reports)
	}
}

// Forget drops a machine's reports on request; one still in the fleet then
// shows as never reported.
func TestForgetAMachine(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	ok(t, d.Record(ReportIn{Host: "pi", Commit: sha('a'), Status: "done"}, time.Now()))
	ok(t, d.Record(ReportIn{Host: "nas", Commit: sha('a'), Status: "done"}, time.Now()))
	ok(t, d.Forget("pi"))
	isErr(t, d.Forget("pi"), ErrNotFound)
	ov := must[DeployOverview](t)(d.Overview(ctx, fleet("pi"), time.Now()))
	if len(ov.Machines) != 2 || ov.Machines[1].Host != "pi" || ov.Machines[1].State != "never" {
		t.Fatalf("after forgetting pi: %+v", ov.Machines)
	}
	if again := must[*Deployments](t)(OpenDeployments(dir, d.repo)); again.reports["pi"] != nil || again.reports["nas"] == nil {
		t.Fatalf("after reopening: %v", again.reports)
	}
	var nilD *Deployments
	isErr(t, nilD.Forget("pi"), ErrNotFound)
}

func TestCommitsBetween(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a", "1\n")
	c1 := commitAll(t, r, "one")
	write(t, r, "a", "2\n")
	commitAll(t, r, "two")
	write(t, r, "a", "3\n")
	c3 := commitAll(t, r, "three")
	n, since, err := r.CommitsBetween(ctx, c1, c3)
	if err != nil || n != 2 || since.IsZero() {
		t.Fatalf("c1..c3: %d %v %v", n, since, err)
	}
	if n, _, err := r.CommitsBetween(ctx, c3, c1); err != nil || n != 0 {
		t.Fatalf("c3..c1: %d %v", n, err)
	}
	if _, _, err := r.CommitsBetween(ctx, sha('e'), c3); err == nil {
		t.Fatal("an unknown commit was counted")
	}
}

// storePath is a valid store path for a system.
func storePath(c byte, name string) string {
	return "/nix/store/" + strings.Repeat(string(c), 32) + "-nixos-system-" + name
}

// Heartbeats keep what a machine runs and when it last reported, without
// filling its deployment history; every report kind updates them.
func TestHeartbeatsAndSystemSnapshots(t *testing.T) {
	dir := t.TempDir()
	d := must[*Deployments](t)(OpenDeployments(dir, newRepo(t)))
	now := time.Now().UTC().Truncate(time.Second)
	beat := ReportIn{Host: "pi", Phase: "heartbeat", Current: storePath('a', "pi"), Booted: storePath('b', "pi"), Profile: storePath('a', "pi"), Reboot: true, SilentAfter: 2}
	for name, in := range map[string]ReportIn{
		"commit":       {Host: "pi", Phase: "heartbeat", Commit: sha('a')},
		"status":       {Host: "pi", Phase: "heartbeat", Status: "done"},
		"store path":   {Host: "pi", Phase: "heartbeat", Current: "/etc/passwd"},
		"short hash":   {Host: "pi", Phase: "heartbeat", Current: "/nix/store/abc-nixos-system-pi"},
		"booted":       {Host: "pi", Phase: "heartbeat", Current: storePath('a', "pi"), Booted: "/nix/store/" + strings.Repeat("e", 32) + "-x"},
		"profile":      {Host: "pi", Phase: "heartbeat", Profile: storePath('a', "pi") + "\n"},
		"negative":     {Host: "pi", Phase: "heartbeat", SilentAfter: -1},
		"a year":       {Host: "pi", Phase: "heartbeat", SilentAfter: 24*366 + 1},
		"deploy paths": {Host: "pi", Commit: sha('a'), Status: "done", Current: "relative"},
	} {
		if err := d.Record(in, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
	ok(t, d.Record(beat, now.Add(-time.Hour)))
	if len(d.reports) != 0 {
		t.Fatalf("a heartbeat became a deployment report: %v", d.reports)
	}
	l := d.live["pi"]
	if !l.Seen.Equal(now.Add(-time.Hour)) || !l.Beat.Equal(l.Seen) || l.SilentAfter != 2 || l.System == nil || l.System.Current != beat.Current || !l.System.Reboot || l.System.Booted != beat.Booted {
		t.Fatalf("after a heartbeat: %+v %+v", l, l.System)
	}
	// A deployment report moves the snapshot and the last report, not the heartbeat.
	ok(t, d.Record(ReportIn{Host: "pi", Commit: sha('c'), Status: "done", Current: storePath('c', "pi"), Profile: storePath('c', "pi")}, now))
	l = d.live["pi"]
	if !l.Seen.Equal(now) || !l.Beat.Equal(now.Add(-time.Hour)) || l.System.Current != storePath('c', "pi") || l.System.Reboot || !l.System.At.Equal(now) {
		t.Fatalf("after a deployment: %+v %+v", l, l.System)
	}
	// An older report script sends no system: the last snapshot stays.
	ok(t, d.Record(ReportIn{Host: "pi", Commit: sha('c'), Phase: "build", Status: "already built", SharedToken: true}, now.Add(time.Minute)))
	if l = d.live["pi"]; l.System.Current != storePath('c', "pi") || !l.SharedToken {
		t.Fatalf("after a report without a system: %+v", l)
	}
	if fi, err := os.Stat(filepath.Join(dir, "heartbeats.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("heartbeats.json: %v %v", fi, err)
	}
	again := must[*Deployments](t)(OpenDeployments(dir, d.repo))
	if g := again.live["pi"]; !g.Beat.Equal(l.Beat) || g.System == nil || g.System.Current != l.System.Current || len(again.reports["pi"]) != 2 {
		t.Fatalf("after reopening: %+v", g)
	}
	// Machines that only send heartbeats count towards the cap too.
	for i := range maxReporters - 1 {
		ok(t, d.Record(ReportIn{Host: fmt.Sprintf("m%d", i), Phase: "heartbeat"}, now))
	}
	isErr(t, d.Record(ReportIn{Host: "onemore", Phase: "heartbeat"}, now), ErrInvalid)
	ok(t, d.Forget("m0"))
	ok(t, d.Record(ReportIn{Host: "onemore", Phase: "heartbeat"}, now))
	isErr(t, d.Forget("nobody"), ErrNotFound)
}

// A machine that sends heartbeats is flagged as silent once it has not reported
// for longer than it allows, whatever its deployment state. One that only
// reports deployments keeps the old rule: behind and quiet for 7 days.
func TestSilentMachines(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "flake.nix", "1\n")
	c1 := commitAll(t, r, "first")
	write(t, r, "flake.nix", "2\n")
	c2 := commitAll(t, r, "second")
	d := must[*Deployments](t)(OpenDeployments(t.TempDir(), r))
	d.started = time.Time{} // up for long: no warm-up
	now := time.Now()
	rec := func(ago time.Duration, in ReportIn) {
		t.Helper()
		ok(t, d.Record(in, now.Add(-ago)))
	}
	deploy := func(host, commit string) ReportIn { return ReportIn{Host: host, Commit: commit, Status: "done"} }
	beat := func(host string, hours int) ReportIn {
		return ReportIn{Host: host, Phase: "heartbeat", SilentAfter: hours}
	}
	rec(3*time.Hour, deploy("server", c2))
	rec(3*time.Hour, beat("server", 2))
	rec(5*time.Hour, ReportIn{Host: "broken", Commit: c2, Status: "failed", Error: "boom"})
	rec(5*time.Hour, beat("broken", 2))
	rec(30*24*time.Hour, deploy("laptop", c1))
	rec(30*24*time.Hour, beat("laptop", 0)) // often off: never flagged
	rec(10*time.Minute, deploy("fresh", c2))
	rec(10*time.Minute, beat("fresh", 2))
	rec(10*24*time.Hour, deploy("legacy-current", c2))
	rec(10*24*time.Hour, deploy("legacy-behind", c1))
	rec(time.Minute, beat("newbox", 2)) // set up, nothing deployed since

	got := func() map[string]MachineDeployment {
		out := map[string]MachineDeployment{}
		for _, m := range must[DeployOverview](t)(d.Overview(ctx, nil, now)).Machines {
			out[m.Host] = m
		}
		return out
	}
	ms := got()
	for h, want := range map[string]struct {
		state      string
		silent     bool
		after      int
		heartbeats bool
	}{
		"server": {"current", true, 2, true}, "broken": {"failed", true, 2, true}, "laptop": {"behind", false, 0, true},
		"fresh": {"current", false, 2, true}, "legacy-current": {"current", false, 168, false},
		"legacy-behind": {"behind", true, 168, false}, "newbox": {"never", false, 2, true},
	} {
		m := ms[h]
		if m.State != want.state || m.Silent != want.silent || m.SilentAfter != want.after || m.Heartbeats != want.heartbeats {
			t.Errorf("%s: %s silent=%v after=%d heartbeats=%v, want %+v", h, m.State, m.Silent, m.SilentAfter, m.Heartbeats, want)
		}
	}
	if n := ms["newbox"]; n.LastSeen.IsZero() || n.Commit != "" {
		t.Errorf("newbox: %+v", n)
	}
	if s := ms["server"]; !s.LastSeen.Equal(now.Add(-3 * time.Hour).UTC().Truncate(time.Second)) {
		t.Errorf("server last seen %v", s.LastSeen)
	}

	// Just after a start, machines have not had the time to report again: none is silent.
	d.started = now.Add(-5 * time.Minute)
	for h, m := range got() {
		if m.Silent {
			t.Errorf("%s is silent during the warm-up", h)
		}
	}
}
