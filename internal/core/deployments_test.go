package core

import (
	"context"
	"errors"
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

	ov := must[DeployOverview](t)(d.Overview(ctx, []string{"homeserver", "newbox", "pi"}, now))
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
	want := map[string]string{"homeserver": "current", "workstation": "behind", "oldbox": "silent", "laptop": "failed",
		"nas": "current", "pi": "current", "stale": "current", "ghost": "unknown", "newbox": "never", "pc": "behind"}
	for h, st := range want {
		if got[h].State != st {
			t.Errorf("%s: state %q, want %q (%+v)", h, got[h].State, st, got[h])
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
	ov = must[DeployOverview](t)(d.Overview(ctx, []string{"pc"}, now))
	if i := slices.IndexFunc(ov.Machines, func(m MachineDeployment) bool { return m.Host == "pc" }); i < 0 || ov.Machines[i].State != "current" || ov.Machines[i].Commit != c3 {
		t.Errorf("pc after deploying c3: %+v", ov.Machines)
	}

	var nilD *Deployments
	if ov := must[DeployOverview](t)(nilD.Overview(ctx, []string{"pi"}, now)); ov.Reporting || len(ov.Machines) != 0 {
		t.Fatalf("no store: %+v", ov)
	}
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
