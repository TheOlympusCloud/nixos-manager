package core

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Insight is a health or consistency finding about the fleet, with a fix
// the console can apply by clicking when there is one.
type Insight struct {
	ID       string      `json:"id"`
	Severity string      `json:"severity"` // error, warning, info
	Title    string      `json:"title"`
	Detail   string      `json:"detail"`
	Hosts    []string    `json:"hosts"`
	Fix      *InsightFix `json:"fix,omitempty"`
}

// InsightFix either sets options in each affected machine's managed.json or updates a flake input.
type InsightFix struct {
	Label       string    `json:"label"`
	Settings    []Setting `json:"settings,omitempty"`
	UpdateInput string    `json:"updateInput,omitempty"`
}

type Setting struct {
	Path  []string `json:"path"`
	Value any      `json:"value"`
}

var severityRank = map[string]int{"error": 0, "warning": 1, "info": 2}

// Insights checks the evaluated fleet; now dates the flake inputs.
func Insights(m *Model, now time.Time) []Insight {
	out := []Insight{}
	add := func(in Insight) { // only when some machine is affected
		if len(in.Hosts) > 0 {
			out = append(out, in)
		}
	}
	var healthy []Host
	var broken []string
	for _, h := range m.Hosts {
		if h.Error != "" {
			broken = append(broken, h.Name)
		} else {
			healthy = append(healthy, h)
		}
	}
	where := func(f func(Host) bool) []string {
		hosts := []string{}
		for _, h := range healthy {
			if f(h) {
				hosts = append(hosts, h.Name)
			}
		}
		return hosts
	}
	if len(broken) > 0 {
		out = append(out, Insight{ID: "eval-error", Severity: "error", Hosts: broken,
			Title:  fmt.Sprintf("%s does not evaluate", list(broken)),
			Detail: "Nix cannot build the configuration, so nothing about it can be shown or deployed. Open the machine to see the error."})
	}
	add(Insight{ID: "firewall-off", Severity: "warning", Hosts: where(func(h Host) bool { return !h.Ports.Firewall }),
		Title:  "Firewall disabled",
		Detail: "Every port a service opens is reachable from the network.",
		Fix:    &InsightFix{Label: "Turn the firewall on", Settings: []Setting{{Path: []string{"networking", "firewall", "enable"}, Value: true}}}})
	add(Insight{ID: "ssh-password", Severity: "warning",
		Hosts: where(func(h Host) bool {
			return h.Checks.SSH && (h.Checks.SSHPasswordAuthentication == nil || *h.Checks.SSHPasswordAuthentication)
		}),
		Title:  "SSH accepts passwords",
		Detail: "Password logins invite brute-force attempts. Make sure your SSH key is authorized first, then allow keys only.",
		Fix:    &InsightFix{Label: "Allow SSH keys only", Settings: []Setting{{Path: []string{"services", "openssh", "settings", "PasswordAuthentication"}, Value: false}}}})
	add(Insight{ID: "ssh-root-login", Severity: "warning",
		Hosts: where(func(h Host) bool {
			return h.Checks.SSH && h.Checks.SSHPermitRootLogin != nil && *h.Checks.SSHPermitRootLogin == "yes"
		}),
		Title:  "SSH allows root logins",
		Detail: "Log in as a user and use sudo instead.",
		Fix:    &InsightFix{Label: "Forbid root logins", Settings: []Setting{{Path: []string{"services", "openssh", "settings", "PermitRootLogin"}, Value: "no"}}}})
	add(Insight{ID: "ssh-no-fail2ban", Severity: "info", Hosts: where(func(h Host) bool { return h.Checks.SSH && !h.Checks.Fail2ban }),
		Title:  "SSH without fail2ban",
		Detail: "fail2ban blocks addresses that keep failing to log in.",
		Fix:    &InsightFix{Label: "Enable fail2ban", Settings: []Setting{{Path: []string{"services", "fail2ban", "enable"}, Value: true}}}})
	add(Insight{ID: "no-auto-gc", Severity: "info", Hosts: where(func(h Host) bool { return !h.Checks.AutoGC }),
		Title:  "No automatic garbage collection",
		Detail: "Old generations pile up in /nix/store until the disk fills.",
		Fix: &InsightFix{Label: "Collect weekly, keep 14 days", Settings: []Setting{
			{Path: []string{"nix", "gc", "automatic"}, Value: true},
			{Path: []string{"nix", "gc", "dates"}, Value: "weekly"},
			{Path: []string{"nix", "gc", "options"}, Value: "--delete-older-than 14d"},
		}}})
	add(Insight{ID: "no-store-optimise", Severity: "info", Hosts: where(func(h Host) bool { return !h.Checks.AutoOptimise }),
		Title:  "Store not optimised",
		Detail: "Identical files in /nix/store are kept as separate copies; optimising hard-links them and saves space.",
		Fix:    &InsightFix{Label: "Optimise the store automatically", Settings: []Setting{{Path: []string{"nix", "optimise", "automatic"}, Value: true}}}})
	for _, in := range m.Inputs {
		if in.LastModified == 0 {
			continue
		}
		days := int(now.Sub(time.Unix(in.LastModified, 0)).Hours() / 24)
		sev := ""
		switch {
		case days > 60:
			sev = "warning"
		case days > 30:
			sev = "info"
		}
		if sev != "" {
			out = append(out, Insight{ID: "input-old:" + in.Name, Severity: sev, Hosts: []string{},
				Title:  fmt.Sprintf("%s is %d days old", in.Name, days),
				Detail: "Updating brings security fixes and new versions; check the result before committing.",
				Fix:    &InsightFix{Label: "Update " + in.Name, UpdateInput: in.Name}})
		}
	}
	add(Insight{ID: "not-managed", Severity: "info", Hosts: where(func(h Host) bool { return h.ManagedFile == nil }),
		Title:  "Visual editing off",
		Detail: "These machines do not load their managed.json, so packages and options cannot be changed by clicking."})
	out = append(out, missingPackages(healthy)...)
	slices.SortStableFunc(out, func(a, b Insight) int {
		if d := severityRank[a.Severity] - severityRank[b.Severity]; d != 0 {
			return d
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// missingPackages finds repository packages that every healthy machine has but one.
func missingPackages(healthy []Host) []Insight {
	if len(healthy) < 3 {
		return nil
	}
	count := map[string]int{}
	for _, h := range healthy {
		for _, p := range h.Packages {
			if slices.ContainsFunc(p.Files, func(f string) bool { return f != "nixos" }) {
				count[p.Name]++
			}
		}
	}
	var out []Insight
	for name, n := range count {
		if n != len(healthy)-1 {
			continue
		}
		for _, h := range healthy {
			if !slices.ContainsFunc(h.Packages, func(p Package) bool { return p.Name == name }) {
				out = append(out, Insight{ID: "missing-package:" + name, Severity: "info", Hosts: []string{h.Name},
					Title:  fmt.Sprintf("%s is on every machine except %s", name, h.Name),
					Detail: "Probably an oversight; add it if this machine should match the others."})
			}
		}
	}
	return out
}

func list(xs []string) string {
	if len(xs) <= 2 {
		return strings.Join(xs, " and ")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}
