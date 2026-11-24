package core

import (
	"fmt"
	"regexp"
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
	// NoSSHKey: affected machines where no user has an SSH key, so a keys-only fix
	// would lock everyone out; add a key before applying the fix there.
	NoSSHKey []string `json:"noSshKey,omitempty"`
	Link     string   `json:"link,omitempty"` // a console route with more about it, e.g. #/deployments
}

// InsightFix sets options in each affected machine's managed.json, updates a
// flake input, or checks a commit.
type InsightFix struct {
	Label       string    `json:"label"`
	Settings    []Setting `json:"settings,omitempty"`
	UpdateInput string    `json:"updateInput,omitempty"`
	Check       string    `json:"check,omitempty"` // a commit to check
}

type Setting struct {
	Path  []string `json:"path"`
	Value any      `json:"value"`
}

var severityRank = map[string]int{"error": 0, "warning": 1, "info": 2}

// Insights checks the evaluated fleet and what its machines deployed; origin is
// the remote this console pushes to ("" for none), tip the last commit of the
// branch comin deploys (nil when unknown), and now dates the flake inputs and
// the deployments.
func Insights(m *Model, deploys []MachineDeployment, origin string, tip *CommitCheck, now time.Time) []Insight {
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
	// Keys-only SSH locks a machine whose users have no key: tell every client, the MCP ones too.
	keysFirst := func(in Insight) Insight {
		for _, h := range healthy {
			if slices.Contains(in.Hosts, h.Name) && (h.Checks.SSHUserKeys == nil || *h.Checks.SSHUserKeys == 0) {
				in.NoSSHKey = append(in.NoSSHKey, h.Name)
			}
		}
		return in
	}
	add(keysFirst(Insight{ID: "ssh-password", Severity: "warning",
		Hosts: where(func(h Host) bool {
			return h.Checks.SSH && (h.Checks.SSHPasswordAuthentication == nil || *h.Checks.SSHPasswordAuthentication)
		}),
		Title:  "SSH accepts passwords",
		Detail: "Password logins invite brute-force attempts. Make sure your SSH key is authorized first, then allow keys only.",
		Fix:    &InsightFix{Label: "Allow SSH keys only", Settings: []Setting{{Path: []string{"services", "openssh", "settings", "PasswordAuthentication"}, Value: false}}}}))
	add(keysFirst(Insight{ID: "ssh-root-login", Severity: "warning",
		Hosts: where(func(h Host) bool {
			return h.Checks.SSH && h.Checks.SSHPermitRootLogin != nil && *h.Checks.SSHPermitRootLogin == "yes"
		}),
		Title:  "SSH allows root logins",
		Detail: "Log in as a user and use sudo instead.",
		Fix:    &InsightFix{Label: "Forbid root logins", Settings: []Setting{{Path: []string{"services", "openssh", "settings", "PermitRootLogin"}, Value: "no"}}}}))
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
	// 1 is the first managed.nix, 2 lacks modules/managed.json, 3 packages from another
	// flake input; 0 (not reported) is left alone.
	if old := where(func(h Host) bool { return h.ManagedFile != nil && h.ManagedVersion > 0 && h.ManagedVersion < 4 }); len(old) > 0 {
		out = append(out, Insight{ID: "managed-old", Severity: "warning", Hosts: old, Link: "#/machines/" + old[0] + "/packages",
			Title:  "modules/managed.nix is older than this console",
			Detail: "These machines load an older managed.nix: packages and options set on each machine still apply, but a package taken from another nixpkgs does not, nor, before version 3, what is set for every machine (modules/managed.json), nor, with the first managed.nix, removed or customised packages and home-manager users. Update it (here, or on a machine's Packages tab), then check, commit and push it."})
	}
	out = append(out, missingPackages(healthy)...)
	out = append(out, releaseInsights(healthy, now)...)
	out = append(out, inputEOL(m.Inputs, healthy, now)...)
	out = append(out, deployInsights(m, deploys, now)...)
	out = append(out, cominInsights(healthy, origin)...)
	out = append(out, tipInsights(tip)...)
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

// releaseEnd is when a NixOS release ("26.05", "26.11pre-git") stops getting
// updates: seven months after it comes out, at the end of a month.
func releaseEnd(release string) (time.Time, bool) {
	var yy, mm int
	if len(release) < 5 {
		return time.Time{}, false
	}
	if _, err := fmt.Sscanf(release[:5], "%2d.%2d", &yy, &mm); err != nil || (mm != 5 && mm != 11) {
		return time.Time{}, false
	}
	return time.Date(2000+yy, time.Month(mm+8), 1, 0, 0, 0, 0, time.UTC), true
}

// releaseInsights flags unsupported or soon unsupported NixOS releases, and a fleet on several.
func releaseInsights(healthy []Host, now time.Time) []Insight {
	var eol, soon, olds []string
	var eolRel, soonRel []string
	newest := ""
	for _, h := range healthy {
		if _, ok := releaseEnd(h.Release); ok && h.Release[:5] > newest {
			newest = h.Release[:5]
		}
	}
	for _, h := range healthy {
		end, ok := releaseEnd(h.Release)
		if !ok {
			continue
		}
		rel := h.Release[:5]
		switch {
		case !now.Before(end):
			eol = append(eol, h.Name)
			if !slices.Contains(eolRel, rel) {
				eolRel = append(eolRel, rel)
			}
		case now.After(end.AddDate(0, 0, -31)):
			soon = append(soon, h.Name)
			if !slices.Contains(soonRel, rel) {
				soonRel = append(soonRel, rel)
			}
		}
		if rel != newest {
			olds = append(olds, h.Name)
		}
	}
	var out []Insight
	if len(eol) > 0 {
		out = append(out, Insight{ID: "release-eol", Severity: "warning", Hosts: eol,
			Title:  fmt.Sprintf("NixOS %s no longer %s security updates", list(eolRel), map[bool]string{true: "gets", false: "get"}[len(eolRel) == 1]),
			Detail: "Point the nixpkgs input in flake.nix at a supported release branch, read its release notes, then check the machines before deploying."})
	}
	if len(soon) > 0 {
		end, _ := releaseEnd(soonRel[0])
		out = append(out, Insight{ID: "release-eol-soon", Severity: "info", Hosts: soon,
			Title:  fmt.Sprintf("NixOS %s support ends on %s", list(soonRel), end.AddDate(0, 0, -1).Format("2 Jan 2006")),
			Detail: "Plan the move to the next release: switch the nixpkgs input to its branch and check the machines."})
	}
	if len(olds) > 0 {
		out = append(out, Insight{ID: "release-mixed", Severity: "info", Hosts: olds,
			Title:  "Machines run different NixOS releases",
			Detail: fmt.Sprintf("The newest is %s. Mixed releases mean different package versions and options across the fleet.", newest)})
	}
	return out
}

// releaseBranch finds a NixOS release in an input's branch: nixos-26.05,
// nixos-26.05-small, nixpkgs-26.05-darwin, release-26.05.
var releaseBranch = regexp.MustCompile(`(?:^|[/=])((?:nixos|nixpkgs|release)-(\d\d\.\d\d)(?:-[a-z]+)?)(?:$|[&#])`)

// inputEOL flags inputs that follow a release branch whose support ended,
// except a release the machines already run (release-eol says it for them).
func inputEOL(inputs []Input, healthy []Host, now time.Time) []Insight {
	var out []Insight
	for _, in := range inputs {
		m := releaseBranch.FindStringSubmatch(in.URL)
		if m == nil {
			continue
		}
		end, ok := releaseEnd(m[2])
		if !ok || now.Before(end) || slices.ContainsFunc(healthy, func(h Host) bool { return strings.HasPrefix(h.Release, m[2]) }) {
			continue
		}
		out = append(out, Insight{ID: "input-eol:" + in.Name, Severity: "warning", Hosts: []string{},
			Title:  fmt.Sprintf("%s follows %s, which no longer gets updates", in.Name, m[1]),
			Detail: fmt.Sprintf("NixOS %s reached its end of life on %s. Point the input in flake.nix at a supported release branch (the one nixpkgs follows), update it, then check the machines.", m[2], end.AddDate(0, 0, -1).Format("2 Jan 2006"))})
	}
	return out
}

// cominInsights: a push only deploys machines whose comin pulls the repository this console pushes to.
func cominInsights(healthy []Host, origin string) []Insight {
	off, other := []string{}, []string{}
	var elsewhere string
	o, err := ParseRemote(origin)
	known := err == nil && o.Kind != "local"
	for _, h := range healthy {
		if !h.Checks.Comin {
			off = append(off, h.Name)
			continue
		}
		if !known || len(h.Checks.CominRemotes) == 0 {
			continue
		}
		if !slices.ContainsFunc(h.Checks.CominRemotes, func(r string) bool { return sameRepo(r, o) }) {
			other = append(other, h.Name)
			if elsewhere == "" {
				elsewhere = h.Checks.CominRemotes[0]
			}
		}
	}
	var out []Insight
	if len(off) > 0 {
		out = append(out, Insight{ID: "no-comin", Severity: "warning", Hosts: off,
			Title:  "Pushes don't reach these machines",
			Detail: "comin is off on them, so they change only when someone runs nixos-rebuild there. Turn on services.comin with the Forgejo repository as its remote (the starter fleet does it in modules/common.nix)."})
	}
	if len(other) > 0 {
		out = append(out, Insight{ID: "comin-other-repo", Severity: "warning", Hosts: other,
			Title:  "comin pulls another repository",
			Detail: fmt.Sprintf("These machines deploy from %s, not from the repository this console pushes to, so changes made here never reach them. Point services.comin.remotes at the Forgejo repository.", elsewhere)})
	}
	return out
}

// tipInsights flag the last commit of the branch comin deploys until it
// passed a check: pushed from elsewhere, or with Push without checks.
func tipInsights(tip *CommitCheck) []Insight {
	if tip == nil || tip.State == "passed" {
		return nil
	}
	in := Insight{ID: "unchecked-commit", Hosts: []string{}, Link: "#/changes?check=" + tip.Commit}
	switch tip.State {
	case "queued", "running":
		in.Severity, in.Title = "info", fmt.Sprintf("%.7s on %s is being checked", tip.Commit, tip.Branch)
		in.Detail = fmt.Sprintf("“%s” was pushed without the console's check, and comin deploys it anyway. The check builds every machine it changes; its log is in Changes → Checks.", tip.Subject)
	case "failed":
		in.Severity, in.Title = "error", fmt.Sprintf("%.7s on %s failed its check", tip.Commit, tip.Branch)
		in.Detail = fmt.Sprintf("“%s” is what comin deploys. A machine that does not build keeps its old system and reports a failed deployment. The check's log says what failed: fix it in a new commit, or revert this one in History.", tip.Subject)
		in.Fix = &InsightFix{Label: "Check again", Check: tip.Commit}
	default:
		in.Severity, in.Title = "warning", fmt.Sprintf("%.7s on %s has not passed a check", tip.Commit, tip.Branch)
		in.Detail = fmt.Sprintf("“%s” was pushed without the console's check (from another clone, Forgejo's web editor, or with Push without checks), and comin deploys it anyway. Checking it builds every machine it changes. With the push webhook (Settings → Forgejo), every push is checked by itself.", tip.Subject)
		in.Fix = &InsightFix{Label: fmt.Sprintf("Check %.7s", tip.Commit), Check: tip.Commit}
		in.Link = ""
	}
	return []Insight{in}
}

// sameRepo says whether a comin remote is the repository o.
func sameRepo(remote string, o RemoteURL) bool {
	c, err := ParseRemote(remote)
	return err == nil && strings.EqualFold(c.Host, o.Host) && strings.EqualFold(c.Path, o.Path)
}

func list(xs []string) string {
	if len(xs) <= 2 {
		return strings.Join(xs, " and ")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

// deployInsights flags fleet machines whose last deployment failed, that lag
// behind their branch, that stopped reporting, or that report without
// heartbeats.
func deployInsights(m *Model, deploys []MachineDeployment, now time.Time) []Insight {
	var out []Insight
	var behind, silent, never, legacy, shared, manual, unfinished, reboot []string
	reporting := false
	for _, d := range deploys {
		reporting = reporting || len(d.Reports) > 0
		if !slices.ContainsFunc(m.Hosts, func(h Host) bool { return h.Name == d.Host }) {
			continue // reported by a machine that is not in this fleet (any more)
		}
		if d.Silent {
			silent = append(silent, d.Host)
		}
		if len(d.Reports) > 0 && !d.Heartbeats {
			legacy = append(legacy, d.Host)
		}
		if d.SharedToken {
			shared = append(shared, d.Host)
		}
		switch d.Drift {
		case "manual":
			manual = append(manual, d.Host)
		case "unfinished":
			unfinished = append(unfinished, d.Host)
		}
		if d.System != nil && d.System.Reboot {
			reboot = append(reboot, d.Host)
		}
		switch {
		case d.State == "failed" && d.Failure != nil:
			f := d.Failure
			what := "deploy"
			if f.Phase == "build" {
				what = "build"
			}
			runs := "It has not deployed anything successfully yet."
			if d.Commit != "" {
				runs = fmt.Sprintf("It still runs %.7s.", d.Commit)
			}
			msg, _, more := strings.Cut(strings.TrimSpace(f.Error), "\n") // the rest is on Deployments
			if len(msg) > 240 {
				msg, more = strings.ToValidUTF8(msg[:240], ""), true
			}
			if more {
				msg += "…"
			}
			if msg == "" {
				msg = "comin gave no error message."
			}
			out = append(out, Insight{ID: "deploy-failed:" + d.Host, Severity: "error", Hosts: []string{d.Host}, Link: "#/deployments",
				Title:  fmt.Sprintf("%s failed to %s %.7s", d.Host, what, f.Commit),
				Detail: fmt.Sprintf("%s %s Deployments shows the error; journalctl -u comin on the machine has the whole log.", msg, runs)})
		case d.State == "behind" && !d.Silent && now.Sub(d.BehindSince) > behindAfter: // silent says why it is behind
			behind = append(behind, d.Host)
		case d.State == "never" && d.LastSeen.IsZero():
			never = append(never, d.Host)
		}
	}
	are := func(hs []string) string { return map[bool]string{true: "has", false: "have"}[len(hs) == 1] }
	if len(behind) > 0 {
		it, runs, them := "They", "run", "them"
		if len(behind) == 1 {
			it, runs, them = "It", "runs", "it"
		}
		out = append(out, Insight{ID: "deploy-behind", Severity: "warning", Hosts: behind, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s not caught up for over an hour", list(behind), are(behind)),
			Detail: fmt.Sprintf("%s still %s an older commit of the branch comin follows. comin may fail to evaluate or build the new one (older comin versions do not report that), or cannot reach Forgejo: run journalctl -u comin on %s.", it, runs, them)})
	}
	if len(silent) > 0 {
		out = append(out, Insight{ID: "deploy-silent", Severity: "warning", Hosts: silent, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s stopped reporting", list(silent), are(silent)),
			Detail: "No report or heartbeat for longer than fleet.deployReport.silentAfter allows (7 days and a commit behind for a machine without heartbeats). Check that it is on, that comin and its report timer run, and that it still reaches this console. A machine that is often off can set silentAfter = null."})
	}
	runs := func(hs []string) string { return map[bool]string{true: "runs", false: "run"}[len(hs) == 1] }
	if len(manual) > 0 {
		out = append(out, Insight{ID: "drift-manual", Severity: "warning", Hosts: manual, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s a system no deployment made", list(manual), runs(manual)),
			Detail: "What runs is neither what its deployed commit builds nor what it boots into from comin: someone ran nixos-rebuild there, or rolled back. comin replaces it with the next commit that changes the machine; to keep the change, make it in the repository."})
	}
	if len(unfinished) > 0 {
		out = append(out, Insight{ID: "drift-unfinished", Severity: "warning", Hosts: unfinished, Link: "#/deployments",
			Title:  fmt.Sprintf("%s did not finish switching", list(unfinished)),
			Detail: "What runs is not what its deployed commit builds, but comin got it there: a switch stopped or failed partway (a service that did not start, say), or the new system is installed and waits for a reboot. journalctl -u comin on the machine says why."})
	}
	if len(reboot) > 0 {
		out = append(out, Insight{ID: "reboot-pending", Severity: "info", Hosts: reboot, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s a reboot", list(reboot), map[bool]string{true: "needs", false: "need"}[len(reboot) == 1]),
			Detail: "A deployment changed the kernel, its modules or the initrd, which only a reboot loads: until then the machine runs the old kernel."})
	}
	if len(shared) > 0 {
		out = append(out, Insight{ID: "deploy-shared-token", Severity: "warning", Hosts: shared, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s with the shared token", list(shared), map[bool]string{true: "reports", false: "report"}[len(shared) == 1]),
			Detail: "Whoever holds the shared fleet token can report as any machine, for instance make a broken one look up to date. Put each machine's own token in its token file (Deployments → Setup), then stop accepting the shared token there. The next release no longer accepts it."})
	}
	if len(legacy) > 0 {
		out = append(out, Insight{ID: "deploy-no-heartbeat", Severity: "info", Hosts: legacy, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s no heartbeats", list(legacy), map[bool]string{true: "sends", false: "send"}[len(legacy) == 1]),
			Detail: "Its report module only reports deployments, so a machine that stops while up to date shows only once a commit is 7 days late, and what it runs is not compared with its commit. Copy report-deployment.sh and report-deployments.nix from the starter fleet's modules into the repository's modules (README, Deployments)."})
	}
	if reporting && len(never) > 0 {
		out = append(out, Insight{ID: "deploy-never", Severity: "info", Hosts: never, Link: "#/deployments",
			Title:  fmt.Sprintf("%s %s never reported a deployment", list(never), are(never)),
			Detail: "Other machines report what they deploy. Put the report token on these (Deployments shows how); the next deployment then reports."})
	}
	return out
}
