package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// What machines have installed: after a report, the server's reply asks a
// machine for the closure of the system it runs when the server does not hold
// it, and the report script posts it (every store path, its NAR size and what
// it references). The server keeps the latest one of each machine in
// closures/<host>.json and shows it as a tree per machine and as a table
// across the fleet. It runs no Nix for this.

const maxClosureError = 4096

// The largest closure taken, in bytes and in store paths (a desktop's has a few
// thousand, well under a MiB); variables for tests.
var (
	MaxClosureBody  int64 = 16 << 20
	maxClosurePaths       = 50_000
)

// closurePath is a store path as Nix allows its name (reportPath caps it lower).
var closurePath = regexp.MustCompile(`^/nix/store/[0-9a-df-np-sv-z]{32}-[A-Za-z0-9+._?=-]{1,211}$`)

// ClosurePath is one store path of a closure; Refs index the closure's paths.
type ClosurePath struct {
	Path    string `json:"path"`
	NarSize int64  `json:"narSize"`
	Refs    []int  `json:"refs,omitempty"`
}

// ClosureIn is what a machine posts for the system named in the query: its
// paths, or why it could not read them.
type ClosureIn struct {
	Paths []ClosurePath `json:"paths"`
	Error string        `json:"error"`
}

// closureRecord is what is kept of a machine's closure: the paths, or why the
// server refused them, or why the machine could not read them.
type closureRecord struct {
	System  string        `json:"system"`
	At      time.Time     `json:"at"`
	Paths   []ClosurePath `json:"paths,omitempty"`
	Refused string        `json:"refused,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// checkClosure says what is wrong with a closure of system, "" when nothing.
func checkClosure(system string, in ClosureIn) string {
	if in.Error != "" {
		if len(in.Error) > maxClosureError || !utf8.ValidString(in.Error) {
			return fmt.Sprintf("its error is not UTF-8 text of at most %d bytes", maxClosureError)
		}
		if len(in.Paths) > 0 {
			return "it has both paths and an error"
		}
		return ""
	}
	switch {
	case len(in.Paths) == 0:
		return "it has no paths"
	case len(in.Paths) > maxClosurePaths:
		return fmt.Sprintf("it has more than %d paths", maxClosurePaths)
	}
	seen := make(map[string]bool, len(in.Paths))
	// Each path references every other one at most once, which bounds what a
	// closure can hold whatever its size: mark[r] == i+1 when path i references r.
	mark := make([]int, len(in.Paths))
	for i, p := range in.Paths {
		switch {
		case !closurePath.MatchString(p.Path):
			return fmt.Sprintf("%.200q is not a store path", p.Path)
		case seen[p.Path]:
			return fmt.Sprintf("%s is listed twice", p.Path)
		case p.NarSize < 0:
			return fmt.Sprintf("%s has a negative size", p.Path)
		case len(p.Refs) >= len(in.Paths):
			return fmt.Sprintf("%s references more paths than the closure has", p.Path)
		}
		seen[p.Path] = true
		for _, r := range p.Refs {
			switch {
			case r < 0 || r >= len(in.Paths):
				return fmt.Sprintf("%s references entry %d of %d", p.Path, r, len(in.Paths))
			case r == i:
				return fmt.Sprintf("%s references itself", p.Path)
			case mark[r] == i+1:
				return fmt.Sprintf("%s references entry %d twice", p.Path, r)
			}
			mark[r] = i + 1
		}
	}
	if !seen[system] {
		return "the system is not among its paths"
	}
	return ""
}

// RecordClosure keeps what a machine posted for system: its closure, or the
// error that kept it from reading one. A closure with anything wrong is
// refused whole, and the refusal is kept, so the machine is not asked again
// until it runs another system. Only a machine that has reported may post.
func (d *Deployments) RecordClosure(host, system string, in ClosureIn, now time.Time) error {
	if err := d.closureFrom(host, system); err != nil {
		return err
	}
	r := &closureRecord{System: system, At: now.UTC().Truncate(time.Second)}
	reason := checkClosure(system, in)
	switch {
	case reason != "":
		r.Refused = reason
	case in.Error != "":
		r.Error = in.Error
	default:
		r.Paths = in.Paths
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.keepClosure(host, r); err != nil {
		return err
	}
	if reason != "" {
		return errorf(ErrInvalid, "closure refused: %s", reason)
	}
	return nil
}

// RefuseClosure keeps that the server refused the closure of system for
// reason (one too large to read, say).
func (d *Deployments) RefuseClosure(host, system, reason string, now time.Time) error {
	if err := d.closureFrom(host, system); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.keepClosure(host, &closureRecord{System: system, At: now.UTC().Truncate(time.Second), Refused: reason})
}

// closureFrom checks who a closure is from and for which system.
func (d *Deployments) closureFrom(host, system string) error {
	if d == nil {
		return errorf(ErrNotFound, "deployment reports are not set up")
	}
	if !reportHost.MatchString(host) {
		return errorf(ErrInvalid, "host must be a machine name")
	}
	if !closurePath.MatchString(system) {
		return errorf(ErrInvalid, "system must be a store path")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.live[host]; !ok {
		return errorf(ErrInvalid, "%s has not reported: a closure follows a report", host)
	}
	return nil
}

// keepClosure writes a machine's closure record; d.mu must be held.
func (d *Deployments) keepClosure(host string, r *closureRecord) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.closuresDir, 0o700); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(d.closuresDir, host+".json"), b); err != nil {
		return err
	}
	d.closures[host] = r
	return nil
}

// loadClosures reads closures/; a file that does not read is skipped, as the
// machine sends its closure again when asked.
func (d *Deployments) loadClosures() error {
	d.closures = map[string]*closureRecord{}
	entries, err := os.ReadDir(d.closuresDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		host, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() || !reportHost.MatchString(host) {
			continue
		}
		var r closureRecord
		b, err := os.ReadFile(filepath.Join(d.closuresDir, e.Name()))
		if err == nil {
			err = json.Unmarshal(b, &r)
		}
		if err == nil && r.Paths != nil {
			if reason := checkClosure(r.System, ClosureIn{Paths: r.Paths}); reason != "" {
				err = errors.New(reason)
			}
		}
		if err != nil {
			slog.Warn("skipping a machine's closure; the machine sends it again", "host", host, "err", err)
			continue
		}
		d.closures[host] = &r
	}
	return nil
}

// WantsClosure says whether the server should ask host for the closure of the
// system it runs: it holds none of that system, or only the machine's error.
func (d *Deployments) WantsClosure(host, current string) bool {
	if d == nil || current == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.live[host]; !ok {
		return false
	}
	r := d.closures[host]
	return r == nil || r.System != current || r.Error != ""
}

// DepNode is one store path of a machine's closure.
type DepNode struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Output  string `json:"output,omitempty"` // bin, lib, dev... when the name ends in one
	NarSize int64  `json:"narSize"`
	Refs    []int  `json:"refs"`
	// Via is the entry before this one on a shortest chain from the system,
	// which says why it is there; -1 for the system itself.
	Via int `json:"via"`
}

// MachineDeps is a machine's closure as a graph, or why there is none.
type MachineDeps struct {
	Host string `json:"host"`
	// State: ready (Nodes holds it), none (the machine does not report),
	// shared (it reports with the shared fleet token), old (its report module
	// sends no closures), waiting (asked; it comes with the next report),
	// refused or error (Message says why).
	State   string    `json:"state"`
	Message string    `json:"message,omitempty"`
	System  string    `json:"system,omitempty"`  // the system the closure (or refusal, or error) is of
	Current string    `json:"current,omitempty"` // what the machine last said it runs
	At      time.Time `json:"at,omitzero"`       // when it arrived
	Size    int64     `json:"size"`              // bytes, every path's NAR size added up
	Root    int       `json:"root"`              // the system's index in Nodes
	Nodes   []DepNode `json:"nodes"`
}

// closureView is what is known of a host's closure, read under d.mu.
type closureView struct {
	live   liveState
	alive  bool
	shared bool
	rec    *closureRecord
}

func (d *Deployments) closureView(host string) closureView {
	l, alive := d.live[host]
	return closureView{live: l, alive: alive, shared: l.SharedToken && d.token != "", rec: d.closures[host]}
}

func (v closureView) state() (state, message string) {
	r := v.rec
	if r != nil && r.Paths == nil && v.live.System != nil && v.live.System.Current != "" && r.System != v.live.System.Current {
		r = nil // a refusal or an error of a system it no longer runs: it is asked again
	}
	switch {
	case !v.alive:
		return "none", ""
	case v.shared:
		return "shared", ""
	case r != nil && r.Paths != nil:
		return "ready", ""
	case r != nil && r.Refused != "":
		return "refused", r.Refused
	case r != nil && r.Error != "":
		return "error", r.Error
	case !v.live.SendsClosure:
		return "old", ""
	}
	return "waiting", ""
}

// MachineDependencies returns host's closure as a graph, or why it has none.
func (d *Deployments) MachineDependencies(host string) (MachineDeps, error) {
	md := MachineDeps{Host: host, Nodes: []DepNode{}}
	if d == nil {
		return md, errorf(ErrNotFound, "deployment reports are not set up")
	}
	if !reportHost.MatchString(host) {
		return md, errorf(ErrInvalid, "host must be a machine name")
	}
	d.mu.Lock()
	v := d.closureView(host)
	d.mu.Unlock()
	md.State, md.Message = v.state()
	if v.live.System != nil {
		md.Current = v.live.System.Current
	}
	if v.rec != nil {
		md.System, md.At = v.rec.System, v.rec.At
	}
	if md.State != "ready" {
		return md, nil
	}
	// A record is replaced, never changed, so its paths are read without the lock.
	ps := v.rec.Paths
	md.Nodes = make([]DepNode, len(ps))
	for i, p := range ps {
		n := DepNode{Path: p.Path, NarSize: p.NarSize, Refs: p.Refs, Via: -1}
		if n.Refs == nil {
			n.Refs = []int{}
		}
		n.Name, n.Version, n.Output = splitStoreName(p.Path)
		md.Nodes[i] = n
		md.Size += p.NarSize
		if p.Path == v.rec.System {
			md.Root = i
		}
	}
	// Breadth first from the system: each entry's Via is on a shortest chain to it.
	seen := make([]bool, len(ps))
	seen[md.Root] = true
	for queue := []int{md.Root}; len(queue) > 0; queue = queue[1:] {
		for _, r := range ps[queue[0]].Refs {
			if !seen[r] {
				seen[r] = true
				md.Nodes[r].Via = queue[0]
				queue = append(queue, r)
			}
		}
	}
	return md, nil
}

// storeOutputs are the output names a store path's name can end in.
var storeOutputs = map[string]bool{"bin": true, "lib": true, "dev": true, "man": true, "doc": true, "devdoc": true,
	"info": true, "debug": true, "static": true, "modules": true, "out": true}

// splitStoreName splits a store path's name as builtins.parseDrvName does
// (the version starts at the first dash followed by a non-letter), with a
// trailing output name kept apart.
func splitStoreName(path string) (name, version, output string) {
	name = storeLabel(path)
	for i := 0; i+1 < len(name); i++ {
		if c := name[i+1]; name[i] == '-' && !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z') {
			name, version = name[:i], name[i+1:]
			if j := strings.LastIndexByte(version, '-'); j >= 0 && storeOutputs[version[j+1:]] {
				version, output = version[:j], version[j+1:]
			}
			return name, version, output
		}
	}
	return name, "", ""
}

// FleetDepsHost is one machine of the fleet table, and whether it has a closure.
type FleetDepsHost struct {
	Host    string    `json:"host"`
	State   string    `json:"state"` // as MachineDeps.State
	Message string    `json:"message,omitempty"`
	At      time.Time `json:"at,omitzero"`
}

// DepRow is one dependency name across the fleet: the versions each machine
// has (empty when it has none), and whether the machines that have it differ.
type DepRow struct {
	Name     string     `json:"name"`
	Versions [][]string `json:"versions"` // per host, in FleetDeps.Hosts order
	Differs  bool       `json:"differs"`
}

// FleetDeps is every versioned dependency across the fleet's machines.
type FleetDeps struct {
	Hosts []FleetDepsHost `json:"hosts"`
	Rows  []DepRow        `json:"rows"`
}

// FleetDependencies tabulates the closures of the machines of the evaluated
// fleet (nil while it is evaluated) and of every machine that reports. Store
// paths without a version (unit files, /etc entries...) and each machine's
// own system are left out.
func (d *Deployments) FleetDependencies(model *Model) FleetDeps {
	f := FleetDeps{Hosts: []FleetDepsHost{}, Rows: []DepRow{}}
	if d == nil {
		return f
	}
	hosts := hostNames(model)
	d.mu.Lock()
	for h := range d.live {
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	hosts = slices.Compact(hosts)
	views := make([]closureView, len(hosts))
	for i, h := range hosts {
		views[i] = d.closureView(h)
	}
	d.mu.Unlock()

	rows := map[string]*DepRow{}
	for i, h := range hosts {
		v := views[i]
		fh := FleetDepsHost{Host: h}
		fh.State, fh.Message = v.state()
		if v.rec != nil {
			fh.At = v.rec.At
		}
		f.Hosts = append(f.Hosts, fh)
		if fh.State != "ready" {
			continue
		}
		for _, p := range v.rec.Paths {
			name, version, _ := splitStoreName(p.Path)
			if version == "" || p.Path == v.rec.System { // the system is the machine's own
				continue
			}
			r := rows[name]
			if r == nil {
				r = &DepRow{Name: name, Versions: make([][]string, len(hosts))}
				for j := range r.Versions {
					r.Versions[j] = []string{}
				}
				rows[name] = r
			}
			if !slices.Contains(r.Versions[i], version) {
				r.Versions[i] = append(r.Versions[i], version)
			}
		}
	}
	for _, r := range rows {
		first := ""
		for _, vs := range r.Versions {
			if len(vs) == 0 {
				continue
			}
			slices.Sort(vs)
			s := strings.Join(vs, " ")
			if first == "" {
				first = s
			} else if s != first {
				r.Differs = true
			}
		}
		f.Rows = append(f.Rows, *r)
	}
	slices.SortFunc(f.Rows, func(a, b DepRow) int { return strings.Compare(a.Name, b.Name) })
	return f
}

// DepEntry is one entry of a machine's closure with what is around it.
type DepEntry struct {
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	Output  string   `json:"output,omitempty"`
	Size    int64    `json:"size"`   // its own NAR size in bytes
	Uses    []string `json:"uses"`   // what it references
	UsedBy  []string `json:"usedBy"` // what references it
	Why     []string `json:"why"`    // a shortest chain from the system to it
}

// maxNeighbours caps Uses and UsedBy: glibc is used by most of a system.
const maxNeighbours = 100

// Lookup returns the entries whose name contains name, in any case (all of
// them for ""), exact names first and then by name, at most limit of them,
// and how many more matched.
func (md MachineDeps) Lookup(name string, limit int) ([]DepEntry, int) {
	q := strings.ToLower(name)
	var hits []int
	for i, n := range md.Nodes {
		if strings.Contains(strings.ToLower(n.Name), q) {
			hits = append(hits, i)
		}
	}
	slices.SortFunc(hits, func(a, b int) int {
		x, y := md.Nodes[a], md.Nodes[b]
		if ex, ey := strings.EqualFold(x.Name, name), strings.EqualFold(y.Name, name); ex != ey {
			if ex {
				return -1
			}
			return 1
		}
		return strings.Compare(storeLabel(x.Path), storeLabel(y.Path))
	})
	more := max(0, len(hits)-limit)
	hits = hits[:len(hits)-more]
	usedBy := map[int][]int{}
	for i, n := range md.Nodes {
		for _, r := range n.Refs {
			usedBy[r] = append(usedBy[r], i)
		}
	}
	labels := func(is []int) []string {
		out := []string{}
		for _, i := range is[:min(len(is), maxNeighbours)] {
			out = append(out, storeLabel(md.Nodes[i].Path))
		}
		slices.Sort(out)
		return out
	}
	out := []DepEntry{}
	for _, i := range hits {
		n := md.Nodes[i]
		e := DepEntry{Path: n.Path, Name: n.Name, Version: n.Version, Output: n.Output, Size: n.NarSize, Uses: labels(n.Refs), UsedBy: labels(usedBy[i])}
		for j := i; j >= 0; j = md.Nodes[j].Via {
			e.Why = append(e.Why, storeLabel(md.Nodes[j].Path))
		}
		slices.Reverse(e.Why)
		out = append(out, e)
	}
	return out, more
}

// storeLabel is a store path's name without the store and the hash.
func storeLabel(path string) string {
	name := path[strings.LastIndexByte(path, '/')+1:]
	if len(name) > 33 && name[32] == '-' {
		return name[33:]
	}
	return name
}
