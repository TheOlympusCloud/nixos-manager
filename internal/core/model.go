package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Model is what the flake configures, machine by machine, as Nix evaluates it.
// The JSON shape is shared with the web UI and the MCP tools.
type Model struct {
	Tree   string   `json:"tree"`
	Hosts  []Host   `json:"hosts"`
	Homes  []Home   `json:"homes"`
	Inputs []Input  `json:"inputs"`
	Darwin []string `json:"darwin,omitempty"` // nix-darwin machines, not evaluated; comin can deploy them too
	Error  string   `json:"error,omitempty"`  // the flake itself does not evaluate
}

type Host struct {
	Name         string  `json:"name"`
	HostName     string  `json:"hostName"`
	System       string  `json:"system"`
	Release      string  `json:"release"`
	StateVersion string  `json:"stateVersion"`
	TimeZone     *string `json:"timeZone"`
	Kernel       *string `json:"kernel"`
	// Toplevel is the derivation of the whole system (system.build.toplevel.drvPath):
	// when it differs, something on the machine changes.
	Toplevel string `json:"toplevel"`
	// Boot is the kernel, initrd and kernel parameters, which take effect only
	// after a reboot; "" when the machine boots no kernel of its own (a container).
	Boot        string  `json:"boot"`
	ManagedFile *string `json:"managedFile"`
	// ManagedPackages maps each attribute in managed.json to its package name
	// (nodejs_22 -> nodejs), so a package seen in the model can be removed by attribute.
	ManagedPackages map[string]string `json:"managedPackages"`
	Packages        []Package         `json:"packages"`
	Features        []Feature         `json:"features"`
	Users           []User            `json:"users"`
	Ports           Ports             `json:"ports"`
	FileSystems     []FileSystem      `json:"fileSystems"`
	Files           []string          `json:"files"`
	Checks          Checks            `json:"checks"`
	Error           string            `json:"error,omitempty"`
}

// Checks are the security and maintenance facts the fleet insights look at.
type Checks struct {
	SSH                       bool            `json:"ssh"`
	SSHPasswordAuthentication *bool           `json:"sshPasswordAuthentication"` // null: the NixOS default (allowed)
	SSHPermitRootLogin        *string         `json:"sshPermitRootLogin"`
	Fail2ban                  bool            `json:"fail2ban"`
	AutoGC                    bool            `json:"autoGC"`
	AutoOptimise              bool            `json:"autoOptimise"`
	Zram                      bool            `json:"zram"`
	SSHUserKeys               *int            `json:"sshUserKeys"`  // authorized keys of normal users; null: unknown
	Comin                     bool            `json:"comin"`        // deployed by comin from the repository
	CominRemotes              []string        `json:"cominRemotes"` // the repositories comin pulls from
	CominBranches             []CominBranches `json:"cominBranches"`
}

// CominBranches are the branches comin follows on one of its remotes
// (services.comin.remotes.*.branches).
type CominBranches struct {
	URL     string `json:"url"`
	Main    string `json:"main"`
	Testing string `json:"testing"` // deployed with switch-to-configuration test when on top of Main
}

type Package struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Files   []string `json:"files"`
}

// Feature is an enabled services.*, programs.*, virtualisation.* or hardware.* module.
type Feature struct {
	Group       string   `json:"group"`
	Name        string   `json:"name"`
	Files       []string `json:"files"`
	Description string   `json:"description"`
}

type User struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Groups      []string  `json:"groups"`
	Home        *HomeInfo `json:"home"`
}

// HomeInfo is a user's home-manager configuration inside a NixOS machine.
type HomeInfo struct {
	Packages []string `json:"packages"`
	Programs []string `json:"programs"`
}

type Ports struct {
	TCP      []int `json:"tcp"`
	UDP      []int `json:"udp"`
	Firewall bool  `json:"firewall"`
}

type FileSystem struct {
	MountPoint string `json:"mountPoint"`
	FSType     string `json:"fsType"`
	Device     string `json:"device"`
}

// Home is a standalone homeConfigurations entry.
type Home struct {
	Name     string   `json:"name"`
	Packages []string `json:"packages"`
	Programs []string `json:"programs"`
	Error    string   `json:"error,omitempty"`
}

// Input is a direct flake input, as locked in flake.lock.
type Input struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	URL          string `json:"url"`
	Rev          string `json:"rev"`
	LastModified int64  `json:"lastModified"`
}

type lockRef struct {
	Type         string `json:"type"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Ref          string `json:"ref"`
	URL          string `json:"url"`
	Path         string `json:"path"`
	Rev          string `json:"rev"`
	LastModified int64  `json:"lastModified"`
}

type flakeLock struct {
	Nodes map[string]struct {
		Inputs   map[string]json.RawMessage `json:"inputs"`
		Locked   lockRef                    `json:"locked"`
		Original lockRef                    `json:"original"`
	} `json:"nodes"`
	Root string `json:"root"`
}

func (l lockRef) url() string {
	switch l.Type {
	case "github", "gitlab", "sourcehut":
		u := fmt.Sprintf("%s:%s/%s", l.Type, l.Owner, l.Repo)
		if l.Ref != "" {
			u += "/" + l.Ref
		}
		return u
	case "path":
		return l.Path
	}
	return l.URL
}

// parseInputs lists the root flake's own inputs from flake.lock. Inputs that
// follow another input (an array in the lock) are skipped.
func parseInputs(lock []byte) ([]Input, error) {
	inputs := []Input{}
	if len(lock) == 0 {
		return inputs, nil
	}
	var l flakeLock
	if err := json.Unmarshal(lock, &l); err != nil {
		return nil, fmt.Errorf("reading flake.lock: %w", err)
	}
	for name, ref := range l.Nodes[l.Root].Inputs {
		var node string
		if json.Unmarshal(ref, &node) != nil {
			continue
		}
		n := l.Nodes[node]
		url := n.Original.url()
		if url == "" {
			url = n.Locked.url()
		}
		inputs = append(inputs, Input{Name: name, Type: n.Locked.Type, URL: url, Rev: n.Locked.Rev, LastModified: n.Locked.LastModified})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	return inputs, nil
}

type HostDiff struct {
	Host            string   `json:"host"`
	Status          string   `json:"status"` // added, removed, changed, unchanged, error
	Error           string   `json:"error,omitempty"`
	PackagesAdded   []string `json:"packagesAdded,omitempty"`
	PackagesRemoved []string `json:"packagesRemoved,omitempty"`
	FeaturesAdded   []string `json:"featuresAdded,omitempty"`
	FeaturesRemoved []string `json:"featuresRemoved,omitempty"`
	UsersAdded      []string `json:"usersAdded,omitempty"`
	UsersRemoved    []string `json:"usersRemoved,omitempty"`
	PortsAdded      []string `json:"portsAdded,omitempty"`
	PortsRemoved    []string `json:"portsRemoved,omitempty"`
	// Packages present before and after with a different version (e.g. after a nixpkgs update).
	VersionsChanged []VersionChange `json:"versionsChanged,omitempty"`
	Kernel          *VersionChange  `json:"kernel,omitempty"`
	// Reboot: the kernel, initrd or kernel parameters change, which only a reboot applies.
	Reboot bool `json:"reboot,omitempty"`
}

type VersionChange struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

func setDiff(before, after []string) (added, removed []string) {
	for _, a := range after {
		if !slices.Contains(before, a) && !slices.Contains(added, a) {
			added = append(added, a)
		}
	}
	for _, b := range before {
		if !slices.Contains(after, b) && !slices.Contains(removed, b) {
			removed = append(removed, b)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed
}

func names[T any](xs []T, f func(T) string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

func portNames(p Ports) []string {
	out := []string{}
	for _, n := range p.TCP {
		out = append(out, fmt.Sprintf("tcp/%d", n))
	}
	for _, n := range p.UDP {
		out = append(out, fmt.Sprintf("udp/%d", n))
	}
	return out
}

// DiffModels says, machine by machine, what changes from head to working.
func DiffModels(head, working *Model) []HostDiff {
	before, after := map[string]Host{}, map[string]Host{}
	for _, h := range head.Hosts {
		before[h.Name] = h
	}
	for _, h := range working.Hosts {
		after[h.Name] = h
	}
	var all []string
	for n := range before {
		all = append(all, n)
	}
	for n := range after {
		if _, ok := before[n]; !ok {
			all = append(all, n)
		}
	}
	slices.Sort(all)
	out := make([]HostDiff, 0, len(all))
	for _, n := range all {
		b, inBefore := before[n]
		a, inAfter := after[n]
		d := HostDiff{Host: n}
		if inAfter && a.Error != "" {
			d.Status, d.Error = "error", a.Error
			out = append(out, d)
			continue
		}
		pkg := func(p Package) string { return p.Name }
		feat := func(f Feature) string { return f.Group + "." + f.Name }
		user := func(u User) string { return u.Name }
		d.PackagesAdded, d.PackagesRemoved = setDiff(names(b.Packages, pkg), names(a.Packages, pkg))
		d.FeaturesAdded, d.FeaturesRemoved = setDiff(names(b.Features, feat), names(a.Features, feat))
		d.UsersAdded, d.UsersRemoved = setDiff(names(b.Users, user), names(a.Users, user))
		d.PortsAdded, d.PortsRemoved = setDiff(portNames(b.Ports), portNames(a.Ports))
		d.VersionsChanged = versionChanges(b.Packages, a.Packages)
		if inBefore && b.Error == "" && a.Boot != "" { // a machine that boots its own kernel, known before and after
			d.Reboot = a.Boot != b.Boot
			if b.Kernel != nil && a.Kernel != nil && *b.Kernel != *a.Kernel {
				d.Kernel = &VersionChange{Name: "kernel", From: *b.Kernel, To: *a.Kernel}
			}
		}
		switch {
		case !inBefore:
			d.Status = "added"
		case !inAfter:
			d.Status = "removed"
		case d.PackagesAdded != nil || d.PackagesRemoved != nil || d.FeaturesAdded != nil || d.FeaturesRemoved != nil ||
			d.UsersAdded != nil || d.UsersRemoved != nil || d.PortsAdded != nil || d.PortsRemoved != nil || d.VersionsChanged != nil ||
			a.Toplevel != b.Toplevel:
			d.Status = "changed"
		default:
			d.Status = "unchanged"
		}
		out = append(out, d)
	}
	return out
}

func versionChanges(before, after []Package) []VersionChange {
	// A name can come in several versions (python3 3.12 and 3.13): compare the sets.
	versions := func(ps []Package) map[string]string {
		by := map[string][]string{}
		for _, p := range ps {
			if !slices.Contains(by[p.Name], p.Version) {
				by[p.Name] = append(by[p.Name], p.Version)
			}
		}
		out := map[string]string{}
		for n, vs := range by {
			slices.Sort(vs)
			out[n] = strings.Join(vs, ", ")
		}
		return out
	}
	was, now := versions(before), versions(after)
	var out []VersionChange
	for name, v := range now {
		if w, ok := was[name]; ok && w != v {
			out = append(out, VersionChange{Name: name, From: w, To: v})
		}
	}
	slices.SortFunc(out, func(a, b VersionChange) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// InputChange is a flake input that the working copy adds, removes or moves to another revision.
type InputChange struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // added, removed, updated
	FromRev    string `json:"fromRev,omitempty"`
	ToRev      string `json:"toRev,omitempty"`
	FromDate   int64  `json:"fromDate,omitempty"`
	ToDate     int64  `json:"toDate,omitempty"`
	CompareURL string `json:"compareUrl,omitempty"` // GitHub inputs: the commits between the two revisions
}

// DiffInputs compares the flake inputs of two models, sorted by name.
func DiffInputs(head, working []Input) []InputChange {
	before := map[string]Input{}
	for _, in := range head {
		before[in.Name] = in
	}
	out := []InputChange{}
	seen := map[string]bool{}
	for _, a := range working {
		seen[a.Name] = true
		b, ok := before[a.Name]
		switch {
		case !ok:
			out = append(out, InputChange{Name: a.Name, Status: "added", ToRev: a.Rev, ToDate: a.LastModified})
		case b.Rev != a.Rev:
			c := InputChange{Name: a.Name, Status: "updated", FromRev: b.Rev, ToRev: a.Rev, FromDate: b.LastModified, ToDate: a.LastModified}
			if owner, repo, ok := githubRepo(a.URL); ok && b.Rev != "" && a.Rev != "" {
				c.CompareURL = fmt.Sprintf("https://github.com/%s/%s/compare/%s...%s", owner, repo, b.Rev, a.Rev)
			}
			out = append(out, c)
		}
	}
	for _, b := range head {
		if !seen[b.Name] {
			out = append(out, InputChange{Name: b.Name, Status: "removed", FromRev: b.Rev, FromDate: b.LastModified})
		}
	}
	slices.SortFunc(out, func(x, y InputChange) int { return strings.Compare(x.Name, y.Name) })
	return out
}

// githubRepo reads owner and repo from a "github:owner/repo[/ref]" flake URL.
func githubRepo(url string) (string, string, bool) {
	rest, ok := strings.CutPrefix(url, "github:")
	if !ok {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
