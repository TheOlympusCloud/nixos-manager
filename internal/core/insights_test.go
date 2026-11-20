package core

import (
	"strings"
	"testing"
	"time"
)

func managed(name string) *string { p := "hosts/" + name + "/managed.json"; return &p }
func bp(b bool) *bool             { return &b }
func sp(s string) *string         { return &s }
func ip(n int) *int               { return &n }

func insightByID(in []Insight, id string) Insight {
	for _, x := range in {
		if x.ID == id {
			return x
		}
	}
	return Insight{}
}

func TestInsights(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	day := int64(24 * 3600)
	m := &Model{
		Hosts: []Host{
			{Name: "broken", Error: "error: infinite recursion encountered"},
			{Name: "pi", ManagedFile: managed("pi"), Ports: Ports{Firewall: false},
				Packages: []Package{{Name: "git", Files: []string{"modules/common.nix"}}},
				Checks:   Checks{SSH: true, SSHPasswordAuthentication: bp(true), SSHPermitRootLogin: sp("yes")}},
			{Name: "server", ManagedFile: managed("server"), Ports: Ports{Firewall: true},
				Packages: []Package{{Name: "git", Files: []string{"modules/common.nix"}}, {Name: "htop", Files: []string{"modules/common.nix"}}},
				Checks:   Checks{SSH: true, SSHPasswordAuthentication: bp(false), SSHPermitRootLogin: sp("no"), Fail2ban: true, AutoGC: true, AutoOptimise: true}},
			{Name: "laptop", Ports: Ports{Firewall: true},
				Packages: []Package{{Name: "git", Files: []string{"modules/common.nix"}}, {Name: "htop", Files: []string{"modules/common.nix"}}, {Name: "nano", Files: []string{"nixos"}}},
				Checks:   Checks{AutoGC: true, AutoOptimise: true}},
		},
		Inputs: []Input{
			{Name: "nixpkgs", LastModified: now.Unix() - 75*day},
			{Name: "home-manager", LastModified: now.Unix() - 40*day},
			{Name: "comin", LastModified: now.Unix() - 3*day},
		},
	}
	got := Insights(m, nil, "", nil, now)
	byID := map[string]Insight{}
	for _, in := range got {
		byID[in.ID] = in
	}
	for id, sev := range map[string]string{
		"eval-error":             "error",
		"firewall-off":           "warning",
		"ssh-password":           "warning",
		"ssh-root-login":         "warning",
		"ssh-no-fail2ban":        "info",
		"no-auto-gc":             "info",
		"no-store-optimise":      "info",
		"input-old:nixpkgs":      "warning",
		"input-old:home-manager": "info",
		"not-managed":            "info",
		"missing-package:htop":   "info",
	} {
		in, ok := byID[id]
		if !ok {
			t.Errorf("missing insight %s (got %v)", id, ids(got))
			continue
		}
		if in.Severity != sev {
			t.Errorf("%s: severity %s, want %s", id, in.Severity, sev)
		}
	}
	if _, ok := byID["input-old:comin"]; ok {
		t.Error("a fresh input reported as old")
	}
	if got[0].Severity != "error" || got[len(got)-1].Severity != "info" {
		t.Errorf("insights not ordered by severity: %v", ids(got))
	}
	if in := byID["ssh-password"]; len(in.Hosts) != 1 || in.Hosts[0] != "pi" || in.Fix == nil || strings.Join(in.Fix.Settings[0].Path, ".") != "services.openssh.settings.PasswordAuthentication" || in.Fix.Settings[0].Value != false {
		t.Errorf("ssh-password insight %+v", in)
	}
	if in := byID["ssh-password"]; len(in.NoSSHKey) != 1 || in.NoSSHKey[0] != "pi" {
		t.Errorf("pi's key count is unknown, so the fix must warn about it: %v", in.NoSSHKey)
	}
	m.Hosts[1].Checks.SSHUserKeys = ip(2)
	if in := insightByID(Insights(m, nil, "", nil, now), "ssh-password"); in.NoSSHKey != nil {
		t.Errorf("pi has keys now: %v", in.NoSSHKey)
	}
	if in := byID["no-auto-gc"]; len(in.Hosts) != 1 || in.Hosts[0] != "pi" || len(in.Fix.Settings) != 3 {
		t.Errorf("gc insight should only list pi (server and laptop collect garbage) and set gc fully: %+v", in)
	}
	if in := byID["input-old:nixpkgs"]; in.Fix == nil || in.Fix.UpdateInput != "nixpkgs" {
		t.Errorf("an old input should offer an update: %+v", in)
	}
	if in := byID["missing-package:htop"]; len(in.Hosts) != 1 || in.Hosts[0] != "pi" {
		t.Errorf("htop is on every machine but pi: %+v", in)
	}
	if _, ok := byID["missing-package:nano"]; ok {
		t.Error("NixOS default packages are not a consistency issue")
	}
	if len(Insights(&Model{}, nil, "", nil, now)) != 0 {
		t.Error("an empty fleet has nothing to report")
	}
}

func ids(in []Insight) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = x.ID
	}
	return out
}

func TestReleaseInsights(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	fleet := func(releases ...string) *Model {
		m := &Model{}
		for i, r := range releases {
			m.Hosts = append(m.Hosts, Host{Name: string(rune('a' + i)), Release: r, Ports: Ports{Firewall: true},
				Checks: Checks{AutoGC: true, AutoOptimise: true, Comin: true}, ManagedFile: managed("x")})
		}
		return m
	}
	byID := func(m *Model, at time.Time) map[string]Insight {
		out := map[string]Insight{}
		for _, in := range Insights(m, nil, "", nil, at) {
			out[in.ID] = in
		}
		return out
	}

	got := byID(fleet("26.05", "26.05", "25.05", "26.11pre-git"), now)
	if in, ok := got["release-eol"]; !ok || in.Severity != "warning" || strings.Join(in.Hosts, ",") != "c" {
		t.Errorf("25.05 ended on 2025-12-31, so only c is unsupported: %+v", in)
	}
	if in := byID(fleet("25.11", "25.05"), now)["release-eol"]; in.Title != "NixOS 25.11 and 25.05 no longer get security updates" {
		t.Errorf("title %q", in.Title)
	}
	if in, ok := got["release-mixed"]; !ok || in.Severity != "info" || strings.Join(in.Hosts, ",") != "a,b,c" {
		t.Errorf("all but the newest release should be listed: %+v", in)
	}
	// 26.05 is supported until the end of 2026: a heads-up in December, nothing before.
	if in := byID(fleet("26.05"), time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC))["release-eol-soon"]; len(in.Hosts) != 1 || in.Severity != "info" {
		t.Errorf("26.05 ends this month: %+v", in)
	}
	if got := byID(fleet("26.05", "26.05", ""), now); len(got) != 0 {
		t.Errorf("one supported release (and an unknown one) is healthy: %v", got)
	}

	// Inputs that follow a release branch whose support ended, unless the
	// machines' own release already says it.
	m := fleet("26.05", "25.05")
	m.Inputs = []Input{
		{Name: "nixpkgs", URL: "github:NixOS/nixpkgs/nixos-25.05", LastModified: now.Unix()},
		{Name: "home-manager", URL: "github:nix-community/home-manager/release-24.11", LastModified: now.Unix()},
		{Name: "stable", URL: "git+https://git.example/nixpkgs?ref=nixos-26.05-small", LastModified: now.Unix()}, // supported
		{Name: "darwin", URL: "github:NixOS/nixpkgs/nixpkgs-24.05-darwin", LastModified: now.Unix()},
		{Name: "unstable", URL: "github:NixOS/nixpkgs/nixos-unstable", LastModified: now.Unix()},
		{Name: "pinned", URL: "github:owner/repo/release-29.05", LastModified: now.Unix()},
	}
	got = byID(m, now)
	if in := got["input-eol:home-manager"]; in.Severity != "warning" || in.Title != "home-manager follows release-24.11, which no longer gets updates" || len(in.Hosts) != 0 {
		t.Errorf("home-manager: %+v", in)
	}
	if in := got["input-eol:darwin"]; in.Title != "darwin follows nixpkgs-24.05-darwin, which no longer gets updates" {
		t.Errorf("darwin: %+v", in)
	}
	for _, id := range []string{"input-eol:nixpkgs", "input-eol:stable", "input-eol:unstable", "input-eol:pinned"} {
		if in, ok := got[id]; ok {
			t.Errorf("%s: %+v", id, in)
		}
	}
}

func TestDeployInsights(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	m := &Model{Hosts: []Host{{Name: "web"}, {Name: "pi"}, {Name: "nas"}, {Name: "new"}, {Name: "off"}, {Name: "fresh"}, {Name: "quiet"}, {Name: "old"}, {Name: "box"}}}
	rep := []DeployReport{{At: now}}
	deploys := []MachineDeployment{
		{Host: "web", State: "current", Reports: rep, Heartbeats: true},
		{Host: "pi", State: "failed", Commit: strings.Repeat("a", 40), Reports: rep, Heartbeats: true,
			Failure: &DeployReport{Commit: strings.Repeat("b", 40), Phase: "build", Status: "failed", Error: "error: attribute 'foo' missing"}},
		{Host: "nas", State: "behind", Behind: 2, BehindSince: now.Add(-2 * time.Hour), Reports: rep, Heartbeats: true},
		{Host: "fresh", State: "behind", Behind: 1, BehindSince: now.Add(-10 * time.Minute), Reports: rep, Heartbeats: true},
		{Host: "new", State: "never", Reports: []DeployReport{}},
		// Behind because it is off: silent says it, behind would only repeat it.
		{Host: "off", State: "behind", Behind: 3, BehindSince: now.Add(-9 * time.Hour), Silent: true, Reports: rep, Heartbeats: true},
		{Host: "quiet", State: "current", Silent: true, Reports: rep, Heartbeats: true},                          // up to date, but gone quiet
		{Host: "old", State: "current", Reports: rep, SharedToken: true},                                         // an older report module: no heartbeats, the shared token
		{Host: "box", State: "never", Reports: []DeployReport{}, LastSeen: now, Heartbeats: true},                // set up, nothing deployed since
		{Host: "gone", State: "failed", Failure: &DeployReport{Status: "failed"}, Reports: rep, Drift: "manual"}, // not in the fleet
		{Host: "hand", State: "current", Reports: rep, Heartbeats: true, Commit: strings.Repeat("c", 40), Drift: "manual", System: &SystemState{Reboot: true}},
		{Host: "half", State: "current", Reports: rep, Heartbeats: true, Commit: strings.Repeat("c", 40), Drift: "unfinished"},
	}
	m.Hosts = append(m.Hosts, Host{Name: "hand"}, Host{Name: "half"})
	got := Insights(m, deploys, "", nil, now)
	f := insightByID(got, "deploy-failed:pi")
	if f.Severity != "error" || f.Link != "#/deployments" || f.Title != "pi failed to build bbbbbbb" ||
		!strings.Contains(f.Detail, "attribute 'foo' missing") || !strings.Contains(f.Detail, "still runs aaaaaaa") {
		t.Errorf("deploy-failed: %+v", f)
	}
	if b := insightByID(got, "deploy-behind"); b.Severity != "warning" || strings.Join(b.Hosts, ",") != "nas" {
		t.Errorf("deploy-behind: %+v", b)
	}
	if s := insightByID(got, "deploy-silent"); s.Severity != "warning" || strings.Join(s.Hosts, ",") != "off,quiet" || s.Title != "off and quiet have stopped reporting" {
		t.Errorf("deploy-silent: %+v", s)
	}
	if h := insightByID(got, "drift-manual"); h.Severity != "warning" || strings.Join(h.Hosts, ",") != "hand" || h.Title != "hand runs a system no deployment made" {
		t.Errorf("drift-manual: %+v", h)
	}
	if h := insightByID(got, "drift-unfinished"); h.Severity != "warning" || strings.Join(h.Hosts, ",") != "half" || h.Title != "half did not finish switching" {
		t.Errorf("drift-unfinished: %+v", h)
	}
	if h := insightByID(got, "reboot-pending"); h.Severity != "info" || strings.Join(h.Hosts, ",") != "hand" || h.Title != "hand needs a reboot" {
		t.Errorf("reboot-pending: %+v", h)
	}
	if h := insightByID(got, "deploy-shared-token"); h.Severity != "warning" || strings.Join(h.Hosts, ",") != "old" || h.Title != "old reports with the shared token" {
		t.Errorf("deploy-shared-token: %+v", h)
	}
	if h := insightByID(got, "deploy-no-heartbeat"); h.Severity != "info" || strings.Join(h.Hosts, ",") != "old" || h.Title != "old sends no heartbeats" {
		t.Errorf("deploy-no-heartbeat: %+v", h)
	}
	if n := insightByID(got, "deploy-never"); n.Severity != "info" || strings.Join(n.Hosts, ",") != "new" || n.Title != "new has never reported a deployment" {
		t.Errorf("deploy-never: %+v", n)
	}
	if g := insightByID(got, "deploy-failed:gone"); g.ID != "" {
		t.Errorf("a machine outside the fleet got a finding: %+v", g)
	}
	// Nobody reports yet: machines that never reported are not news.
	quiet := []MachineDeployment{{Host: "new", State: "never", Reports: []DeployReport{}}}
	if s := insightByID(Insights(m, quiet, "", nil, now), "deploy-never"); s.ID != "" {
		t.Errorf("deploy-never without reporting: %+v", s)
	}
}

// Pushes only deploy machines whose comin pulls the repository this console pushes to.
func TestCominInsights(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	repo := "https://forgejo.home.lan/pablo/nixos-config.git"
	host := func(name string, comin bool, remotes ...string) Host {
		return Host{Name: name, ManagedFile: managed(name), Ports: Ports{Firewall: true},
			Checks: Checks{AutoGC: true, AutoOptimise: true, Comin: comin, CominRemotes: remotes}}
	}
	m := &Model{Hosts: []Host{
		host("same", true, repo),
		host("same-over-ssh", true, "ssh://git@Forgejo.home.lan:2222/pablo/nixos-config"),
		host("other", true, "https://github.com/someone/dotfiles.git"),
		host("off", false),
		{Name: "broken", Error: "boom"},
	}}
	got := map[string]Insight{}
	for _, in := range Insights(m, nil, "git@forgejo.home.lan:pablo/nixos-config.git", nil, now) {
		got[in.ID] = in
	}
	if in := got["no-comin"]; strings.Join(in.Hosts, ",") != "off" || in.Severity != "warning" {
		t.Errorf("no-comin: %+v", in)
	}
	if in := got["comin-other-repo"]; strings.Join(in.Hosts, ",") != "other" || !strings.Contains(in.Detail, "github.com/someone/dotfiles") {
		t.Errorf("comin-other-repo: %+v", in)
	}
	for _, in := range Insights(m, nil, "", nil, now) {
		if in.ID == "comin-other-repo" {
			t.Error("without a remote there is nothing to compare with")
		}
	}
}

// The last commit of the branch comin deploys shows until it passed a check.
func TestUncheckedCommitInsight(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	sha := strings.Repeat("ab", 20)
	at := func(state, job string) Insight {
		return insightByID(Insights(&Model{}, nil, "", &CommitCheck{Branch: "main", Commit: sha, Subject: "pi: add htop", State: state, Job: job}, now), "unchecked-commit")
	}
	in := at("unchecked", "")
	if in.Severity != "warning" || in.Title != "abababa on main has not passed a check" ||
		in.Fix == nil || in.Fix.Check != sha || in.Fix.Label != "Check abababa" || in.Link != "" || in.Hosts == nil || !strings.Contains(in.Detail, "“pi: add htop”") {
		t.Fatalf("unchecked: %+v", in)
	}
	for _, state := range []string{"queued", "running"} {
		if in := at(state, "j4"); in.Severity != "info" || in.Title != "abababa on main is being checked" || in.Fix != nil || in.Link != "#/changes?check="+sha {
			t.Fatalf("%s: %+v", state, in)
		}
	}
	if in := at("failed", "j4"); in.Severity != "error" || in.Title != "abababa on main failed its check" || in.Fix == nil || in.Fix.Check != sha ||
		in.Fix.Label != "Check again" || in.Link != "#/changes?check="+sha {
		t.Fatalf("failed: %+v", in)
	}
	if in := at("passed", ""); in.ID != "" {
		t.Fatalf("passed: %+v", in)
	}
	if in := insightByID(Insights(&Model{}, nil, "", nil, now), "unchecked-commit"); in.ID != "" {
		t.Fatalf("no tip: %+v", in)
	}
}
