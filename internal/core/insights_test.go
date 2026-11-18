package core

import (
	"strings"
	"testing"
	"time"
)

func managed(name string) *string { p := "hosts/" + name + "/managed.json"; return &p }
func bp(b bool) *bool             { return &b }
func sp(s string) *string         { return &s }

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
	got := Insights(m, now)
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
	if len(Insights(&Model{}, now)) != 0 {
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
