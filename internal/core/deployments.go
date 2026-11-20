package core

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// What machines report: comin runs modules/report-deployments.nix after each
// deployment (postDeploymentCommand, COMIN_STATUS done or failed) and, in comin
// versions that have it, after each build (postBuildCommand, COMIN_PHASE=build,
// built, already built or failed). A build that leaves the machine unchanged is
// never deployed, so without the build report it would look behind. A timer
// also sends a heartbeat every 15 minutes, and every report says which system
// the machine runs and boots.

const (
	MaxReportBody  = 16 << 10
	keepReports    = 20     // per machine
	maxReporters   = 200    // machines; with the shared token, one machine could invent others
	legacySilence  = 7 * 24 // hours; machines without heartbeats: behind and no report for this long
	maxSilentAfter = 24 * 366
	// silenceWarmup is how long after a start no machine is called silent: the
	// server may have been down, and the machines report again within a
	// heartbeat (15 minutes).
	silenceWarmup  = 20 * time.Minute
	behindAfter    = time.Hour
	rolloutCommits = 12
	defaultBranch  = "main" // comin's own default for a remote's main branch
)

var (
	reportHost = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}$`)
	reportSHA  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	reportRef  = regexp.MustCompile(`^[\x21-\x7e]{0,200}$`)
	reportGen  = regexp.MustCompile(`^[A-Za-z0-9-]{0,64}$`)
	reportOK   = map[string][]string{"deploy": {"done", "failed"}, "build": {"built", "already built", "failed"}, "heartbeat": {""}}
	// A store path as readlink -f gives it: Nix's base-32 hash, then the name.
	reportPath = regexp.MustCompile(`^(/nix/store/[0-9a-df-np-sv-z]{32}-[A-Za-z0-9+._?=-]{1,180})?$`)
)

// ReportIn is the body a machine posts.
type ReportIn struct {
	Host       string `json:"host"`
	Commit     string `json:"commit"`
	Ref        string `json:"ref"`
	Phase      string `json:"phase"` // deploy (or empty, from older comin) or build
	Status     string `json:"status"`
	Error      string `json:"error"`
	Generation string `json:"generation"`
	// What the machine runs (/run/current-system), booted (/run/booted-system)
	// and boots next (the system profile); older report scripts send none.
	Current string `json:"current"`
	Booted  string `json:"booted"`
	Profile string `json:"profile"`
	Reboot  bool   `json:"reboot"` // the running kernel, initrd or kernel modules are not the booted ones
	// SilentAfter is, in a heartbeat, how many hours the machine may go without
	// reporting before it is flagged (fleet.deployReport.silentAfter); 0 never.
	SilentAfter int `json:"silentAfter"`
	// SharedToken is set by the server, never by the machine: it reported
	// with the shared fleet token instead of its own.
	SharedToken bool `json:"-"`
}

// SystemState is what a machine last said it runs.
type SystemState struct {
	At      time.Time `json:"at"`
	Current string    `json:"current"`
	Booted  string    `json:"booted,omitempty"`
	Profile string    `json:"profile,omitempty"`
	Reboot  bool      `json:"reboot"`
}

// liveState is what a machine's reports of any kind say about it now.
type liveState struct {
	Seen        time.Time    `json:"seen"`          // its last report
	Beat        time.Time    `json:"beat,omitzero"` // its last heartbeat
	SilentAfter int          `json:"silentAfter"`   // hours, from its last heartbeat
	SharedToken bool         `json:"sharedToken,omitempty"`
	System      *SystemState `json:"system,omitempty"`
}

// DeployReport is one report as stored.
type DeployReport struct {
	At         time.Time `json:"at"`
	Commit     string    `json:"commit"`
	Ref        string    `json:"ref,omitempty"`
	Phase      string    `json:"phase"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	Generation string    `json:"generation,omitempty"`
}

func (r DeployReport) failed() bool { return r.Status == "failed" }

// Deployments keeps the last reports of each machine in deployments.json,
// what its latest report of any kind (heartbeats too) said in heartbeats.json,
// and the key each machine's report token is derived from.
type Deployments struct {
	// Systems evaluates what each machine builds at the commit it deployed, to
	// compare with what it runs; nil compares nothing.
	Systems SystemSource

	repo      *Repo
	path      string
	livePath  string
	keyPath   string
	tokenPath string    // the shared fleet token of installs from before host tokens
	started   time.Time // when this process opened them, for the silence warm-up

	mu      sync.Mutex
	key     []byte
	token   string                    // the shared token while it is accepted, else ""
	reports map[string][]DeployReport // newest first
	live    map[string]liveState
}

// OpenDeployments loads dataDir/deployments.json, heartbeats.json and the
// report key, creating the key on first use, and the shared token of an
// older install if there is one.
func OpenDeployments(dataDir string, repo *Repo) (*Deployments, error) {
	d := &Deployments{repo: repo, path: filepath.Join(dataDir, "deployments.json"), livePath: filepath.Join(dataDir, "heartbeats.json"),
		keyPath: filepath.Join(dataDir, "report-key"), tokenPath: filepath.Join(dataDir, "deploy-token"), started: time.Now(),
		reports: map[string][]DeployReport{}, live: map[string]liveState{}}
	for path, into := range map[string]any{d.path: &d.reports, d.livePath: &d.live} {
		b, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			if err := json.Unmarshal(b, into); err != nil {
				return nil, errorf(ErrInvalid, "reading %s: %v", path, err)
			}
		}
	}
	t, err := os.ReadFile(d.tokenPath)
	switch {
	case err == nil:
		d.token = strings.TrimSpace(string(t))
		slog.Warn("machines may still report with the shared fleet token; give each its own (Deployments → Setup), then stop accepting it there")
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	k, err := os.ReadFile(d.keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return d, d.RotateKey()
	}
	if err != nil {
		return nil, err
	}
	if d.key, err = hex.DecodeString(strings.TrimSpace(string(k))); err != nil || len(d.key) < 32 {
		return nil, errorf(ErrInvalid, "reading %s: not a report key", d.keyPath)
	}
	return d, nil
}

// HostToken is the report token of one machine: a keyed hash of its name, so
// it is good for that machine only, and the server keeps no list of tokens.
func (d *Deployments) HostToken(host string) (string, error) {
	if d == nil {
		return "", errorf(ErrNotFound, "deployment reports are not set up")
	}
	if !reportHost.MatchString(host) {
		return "", errorf(ErrInvalid, "host must be a machine name")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return hostToken(d.key, host), nil
}

func hostToken(key []byte, host string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("nixos-manager report token\x00" + host))
	return "nxh_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(m.Sum(nil))
}

// RotateKey makes a new report key: every machine's token changes, and
// machines with an old one stop reporting until they get the new one.
func (d *Deployments) RotateKey() error {
	if d == nil {
		return errorf(ErrNotFound, "deployment reports are not set up")
	}
	key := make([]byte, 32)
	rand.Read(key)
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := writeAtomic(d.keyPath, []byte(hex.EncodeToString(key)+"\n")); err != nil {
		return err
	}
	d.key = key
	return nil
}

// Authorize checks, in constant time, that a presented token is host's own,
// or the shared fleet token while it is accepted.
// shortcut: the shared token is accepted for one release after host tokens; drop it then.
func (d *Deployments) Authorize(secret, host string) (shared, ok bool) {
	if d == nil || secret == "" || !reportHost.MatchString(host) {
		return false, false
	}
	d.mu.Lock()
	own, fleet := hostToken(d.key, host), d.token
	d.mu.Unlock()
	same := func(a, b string) bool {
		x, y := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
		return subtle.ConstantTimeCompare(x[:], y[:]) == 1
	}
	if same(secret, own) {
		return false, true
	}
	if fleet != "" && same(secret, fleet) {
		return true, true
	}
	return false, false
}

// SharedToken says whether the shared fleet token of an older install is
// still accepted.
func (d *Deployments) SharedToken() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.token != ""
}

// DropSharedToken stops accepting the shared fleet token: machines that still
// use it stop reporting until they get their own.
func (d *Deployments) DropSharedToken() error {
	if d == nil {
		return errorf(ErrNotFound, "deployment reports are not set up")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.token == "" {
		return errorf(ErrNotFound, "the shared token is not accepted anyway")
	}
	if err := os.Remove(d.tokenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	d.token = ""
	return nil
}

// Record validates a machine's report and stores it: a deployment or build in
// its history, and what any report (a heartbeat too) says it runs now.
func (d *Deployments) Record(in ReportIn, now time.Time) error {
	if d == nil {
		return errorf(ErrNotFound, "deployment reports are not set up")
	}
	if in.Phase == "" {
		in.Phase = "deploy"
	}
	beat := in.Phase == "heartbeat"
	switch {
	case !reportHost.MatchString(in.Host):
		return errorf(ErrInvalid, "host must be a machine name")
	case beat && in.Commit != "":
		return errorf(ErrInvalid, "a heartbeat names no commit")
	case !beat && !reportSHA.MatchString(in.Commit):
		return errorf(ErrInvalid, "commit must be a 40-character hex SHA")
	case !slices.Contains(reportOK[in.Phase], in.Status):
		return errorf(ErrInvalid, "unknown phase/status %q/%q", in.Phase, in.Status)
	case !reportRef.MatchString(in.Ref) || !reportGen.MatchString(in.Generation):
		return errorf(ErrInvalid, "invalid ref or generation")
	case len(in.Error) > 8192 || !utf8.ValidString(in.Error):
		return errorf(ErrInvalid, "error must be UTF-8 text of at most 8 KiB")
	case !reportPath.MatchString(in.Current) || !reportPath.MatchString(in.Booted) || !reportPath.MatchString(in.Profile):
		return errorf(ErrInvalid, "current, booted and profile must be store paths")
	case in.SilentAfter < 0 || in.SilentAfter > maxSilentAfter:
		return errorf(ErrInvalid, "silentAfter must be 0 to %d hours", maxSilentAfter)
	}
	at := now.UTC().Truncate(time.Second)
	d.mu.Lock()
	defer d.mu.Unlock()
	old, known := d.reports[in.Host]
	oldLive, alive := d.live[in.Host]
	if !known && !alive && d.machines() >= maxReporters {
		return errorf(ErrInvalid, "already keeping reports of %d machines", maxReporters)
	}
	l := oldLive
	l.Seen, l.SharedToken = at, in.SharedToken
	if beat {
		l.Beat, l.SilentAfter = at, in.SilentAfter
	}
	if in.Current != "" {
		l.System = &SystemState{At: at, Current: in.Current, Booted: in.Booted, Profile: in.Profile, Reboot: in.Reboot}
	}
	d.live[in.Host] = l
	if !beat {
		d.reports[in.Host] = append([]DeployReport{{At: at, Commit: in.Commit, Ref: in.Ref, Phase: in.Phase, Status: in.Status, Error: in.Error, Generation: in.Generation}}, old[:min(len(old), keepReports-1)]...)
	}
	undo := func() {
		if alive {
			d.live[in.Host] = oldLive
		} else {
			delete(d.live, in.Host)
		}
		if known {
			d.reports[in.Host] = old
		} else {
			delete(d.reports, in.Host)
		}
	}
	if err := d.saveLive(); err != nil {
		undo()
		return err
	}
	if !beat {
		if err := d.save(); err != nil {
			undo()
			d.saveLive() // best effort: back to what the files said before
			return err
		}
	}
	return nil
}

// machines counts the machines anything is kept about; d.mu must be held.
func (d *Deployments) machines() int {
	n := len(d.reports)
	for h := range d.live {
		if _, ok := d.reports[h]; !ok {
			n++
		}
	}
	return n
}

func (d *Deployments) save() error {
	b, err := json.MarshalIndent(d.reports, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(d.path, b)
}

func (d *Deployments) saveLive() error {
	b, err := json.MarshalIndent(d.live, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(d.livePath, b)
}

// writeAtomic replaces path with data, readable by the service user only:
// readers see the old file or the new one, never a part.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil { // a crash right after the rename must not leave an empty file
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// SystemSource says what machines build at a commit (Jobs evaluates it).
type SystemSource interface {
	ExpectedSystems(commit string, hosts []string) map[string]SystemEval
}

// MachineDeployment is what one machine reported and what that means against
// the branch comin follows for it.
type MachineDeployment struct {
	Host string `json:"host"`
	// State: never (no deployment reported), failed (the last report is a
	// failure), unknown (its commit is not in this working copy or not on the
	// branch), current or behind.
	State string `json:"state"`
	// Silent: no report for longer than SilentAfter hours (0: never flagged),
	// whatever the state. A machine without heartbeats (an older report module)
	// only reports deployments: it is silent when behind for 7 days.
	Silent      bool `json:"silent"`
	SilentAfter int  `json:"silentAfter"`
	Heartbeats  bool `json:"heartbeats"` // it sends them
	// SharedToken: its last report used the shared fleet token, not its own.
	SharedToken bool `json:"sharedToken,omitempty"`
	// System is what it last said it runs, boots and boots next.
	System *SystemState `json:"system,omitempty"`
	// Expected is the system Commit builds, as comin evaluates it; "" while
	// that is not known.
	Expected string `json:"expected,omitempty"`
	// Drift compares System with Expected: "" (the same, or no verdict yet),
	// manual (it runs a system no deployment made: nixos-rebuild by hand, or a
	// rollback) or unfinished (the deployed system is installed but does not
	// run: a switch that stopped or failed partway).
	Drift  string `json:"drift,omitempty"`
	inRepo bool   // Commit is in this working copy
	// Branch is the main branch its comin follows (services.comin.remotes), or
	// its testing branch when that is on top of the main one.
	Branch string `json:"branch"`
	// Main and Testing are those two branches; Testing is "" when its comin
	// settings are unknown or name none. A trial is a commit pushed to Testing.
	Main    string `json:"main"`
	Testing string `json:"testing,omitempty"`
	// Culprits, when its last deployment failed on its main branch, are the
	// commits after the last one it deployed up to the failed one, newest
	// first: reverting them brings it back to what it runs. Empty when that is
	// not known, or they include a merge, or are more than maxCulprits.
	Culprits []Commit `json:"culprits,omitempty"`
	Target   string   `json:"target,omitempty"` // the branch's last commit
	// Commit is the last commit it deployed, or built and found unchanged.
	Commit      string         `json:"commit,omitempty"`
	Subject     string         `json:"subject,omitempty"`
	DeployedAt  time.Time      `json:"deployedAt,omitzero"`
	LastSeen    time.Time      `json:"lastSeen,omitzero"` // its last report of any kind
	Behind      int            `json:"behind"`
	BehindSince time.Time      `json:"behindSince,omitzero"` // when the oldest commit it lacks was made
	Failure     *DeployReport  `json:"failure,omitempty"`
	Reports     []DeployReport `json:"reports"`
}

type DeployOverview struct {
	Reporting bool `json:"reporting"` // some machine has reported
	// SharedToken: the shared fleet token of an older install is still accepted.
	SharedToken bool                `json:"sharedToken"`
	Branch      string              `json:"branch"`  // the main branch most of the fleet follows
	Commits     []Commit            `json:"commits"` // the newest on the branch, for the rollout picture
	Machines    []MachineDeployment `json:"machines"`
}

// hostNames lists the machines of an evaluated fleet.
func hostNames(m *Model) []string {
	var out []string
	if m != nil {
		for _, h := range m.Hosts {
			out = append(out, h.Name)
		}
	}
	return out
}

// Overview says, for the machines of the evaluated fleet (nil while it is
// evaluated) and every machine that reported, what it runs and how that
// compares with the branch its comin follows.
func (d *Deployments) Overview(ctx context.Context, model *Model, now time.Time) (DeployOverview, error) {
	ov := DeployOverview{Branch: defaultBranch, Commits: []Commit{}, Machines: []MachineDeployment{}}
	if d == nil {
		return ov, nil
	}
	follows := cominFollows(ctx, d.repo, model)
	ov.Branch = fleetBranch(follows)
	ov.SharedToken = d.SharedToken()
	d.mu.Lock()
	reports := make(map[string][]DeployReport, len(d.reports))
	for h, rs := range d.reports {
		reports[h] = slices.Clone(rs)
	}
	live := maps.Clone(d.live)
	d.mu.Unlock()

	tips, err := d.branchTips(ctx)
	if err != nil {
		return ov, err
	}
	if gone := d.removed(ctx, model, reports, ov.Branch, tips[ov.Branch]); len(gone) > 0 {
		if err := d.Forget(gone...); err != nil && !errors.Is(err, ErrNotFound) { // not found: a request at the same time dropped them
			slog.Warn("cannot drop the reports of machines no longer in the flake", "hosts", gone, "err", err)
		} else {
			if err == nil {
				slog.Info("dropped the reports of machines no longer in the flake", "hosts", gone)
			}
			for _, h := range gone {
				delete(reports, h)
				delete(live, h)
			}
		}
	}
	hosts := hostNames(model)
	for h := range reports {
		hosts = append(hosts, h)
	}
	for h := range live {
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	hosts = slices.Compact(hosts)
	ov.Reporting = len(reports) > 0 || len(live) > 0

	if tip := tips[ov.Branch]; tip != "" {
		if ov.Commits, err = d.repo.Log(ctx, "", tip, rolloutCommits); err != nil {
			return ov, err
		}
	}
	subjects := map[string]string{}
	for _, c := range ov.Commits {
		subjects[c.SHA] = c.Subject
	}
	type gap struct {
		n     int
		since time.Time
		err   error
	}
	gaps := map[[2]string]gap{}
	for _, h := range hosts {
		b := follows[h]
		m := MachineDeployment{Host: h, Testing: b.Testing, Reports: reports[h]}
		b.Main, b.Testing = cmp.Or(b.Main, ov.Branch), cmp.Or(b.Testing, "testing-"+h)
		m.Branch, m.Main, m.Target = b.Main, b.Main, tips[b.Main]
		if m.Reports == nil {
			m.Reports = []DeployReport{}
		}
		if t := tips[b.Testing]; t != "" && m.Target != "" && t != m.Target && d.repo.isAncestor(ctx, m.Target, t) {
			m.Branch, m.Target = b.Testing, t // comin deploys a testing branch that is on top of the main one
		}
		if len(m.Reports) > 0 {
			m.LastSeen = m.Reports[0].At
			if m.Reports[0].failed() {
				m.Failure = &m.Reports[0]
			}
		}
		l := live[h]
		if l.Seen.After(m.LastSeen) { // reports from before heartbeats have no live state
			m.LastSeen = l.Seen
		}
		m.Heartbeats, m.SharedToken, m.System = !l.Beat.IsZero(), l.SharedToken && ov.SharedToken, l.System
		// What it runs: the last deployment, or a build that left its system unchanged ("already built").
		// "built" is not running yet: comin switches (or waits for confirmation) after it, then reports.
		if i := slices.IndexFunc(m.Reports, func(r DeployReport) bool { return !r.failed() && r.Status != "built" }); i >= 0 {
			m.Commit, m.DeployedAt = m.Reports[i].Commit, m.Reports[i].At
		}
		known := false // the commit is in this working copy
		if m.Commit != "" {
			s, ok := subjects[m.Commit]
			if !ok {
				if out, err := d.repo.git(ctx, "show", "-s", "--format=%s", "--end-of-options", m.Commit+"^{commit}", "--"); err == nil {
					s, ok = strings.TrimSpace(out), true
					subjects[m.Commit] = s
				}
			}
			m.Subject, known = s, ok
			m.inRepo = ok
		}
		if known && m.Target != "" && m.Commit != m.Target {
			k := [2]string{m.Commit, m.Target}
			g, ok := gaps[k]
			if !ok {
				g.n, g.since, g.err = d.repo.CommitsBetween(ctx, m.Commit, m.Target)
				gaps[k] = g
			}
			m.Behind, m.BehindSince = g.n, g.since
			known = g.err == nil && g.n > 0 // n == 0: it runs commits the branch does not have
		}
		switch {
		case len(m.Reports) == 0:
			m.State = "never"
		case m.Failure != nil || m.Commit == "":
			m.State = "failed"
		case !known || m.Target == "":
			m.State = "unknown"
		case m.Commit == m.Target:
			m.State = "current"
		default:
			m.State = "behind"
		}
		if m.Failure != nil && m.Commit != "" {
			m.Culprits = d.culprits(ctx, m.Commit, m.Failure.Commit, tips[b.Main])
		}
		m.SilentAfter = legacySilence
		if m.Heartbeats {
			m.SilentAfter = l.SilentAfter
		}
		m.Silent = m.SilentAfter > 0 && !m.LastSeen.IsZero() && (m.Heartbeats || m.State == "behind") &&
			now.Sub(m.LastSeen) > time.Duration(m.SilentAfter)*time.Hour && now.Sub(d.started) > silenceWarmup
		ov.Machines = append(ov.Machines, m)
	}
	d.drift(ctx, ov.Machines)
	return ov, nil
}

// maxCulprits is how many commits a failed deployment names at most.
// shortcut: a failure after more commits than this gets no revert to offer;
// revert the commits from History, or raise it if fleets push that much at once.
const maxCulprits = 10

// culprits are the commits after good up to failed, newest first, when failed
// is on the branch whose last commit is tip and comes after good, with no merge
// between them; nil otherwise.
func (d *Deployments) culprits(ctx context.Context, good, failed, tip string) []Commit {
	if tip == "" || good == failed || !d.repo.isAncestor(ctx, failed, tip) || !d.repo.isAncestor(ctx, good, failed) {
		return nil
	}
	if n, err := d.repo.git(ctx, "rev-list", "--merges", "--count", "--end-of-options", good+".."+failed, "--"); err != nil || strings.TrimSpace(n) != "0" {
		return nil
	}
	out, err := d.repo.git(ctx, "log", logFormat, fmt.Sprintf("-n%d", maxCulprits+1), "--end-of-options", good+".."+failed, "--")
	if cs := parseLog(out); err == nil && len(cs) <= maxCulprits {
		return cs
	}
	return nil
}

// drift compares what each machine last said it runs with what the commit it
// deployed builds, asking Systems for the systems it needs (those not known
// yet are evaluated in the background, and judged at a later call).
func (d *Deployments) drift(ctx context.Context, ms []MachineDeployment) {
	if d.Systems == nil {
		return
	}
	want := map[string][]string{} // commit -> machines
	var judge []*MachineDeployment
	for i := range ms {
		m := &ms[i]
		// No verdict from a snapshot older than the deployment, or while comin
		// switches to a build ("built" is followed by the deployment's report).
		if m.System == nil || !m.inRepo || m.System.At.Before(m.DeployedAt) || m.Reports[0].Status == "built" {
			continue
		}
		judge = append(judge, m)
		want[m.Commit] = append(want[m.Commit], m.Host)
		if f := m.Reports[0]; f.Phase == "deploy" && f.failed() && f.Commit != m.Commit {
			if _, err := d.repo.git(ctx, "cat-file", "-e", "--end-of-options", f.Commit+"^{commit}"); err == nil {
				want[f.Commit] = append(want[f.Commit], m.Host)
			}
		}
	}
	got := map[string]map[string]SystemEval{}
	for c, hosts := range want {
		got[c] = d.Systems.ExpectedSystems(c, hosts)
	}
	for _, m := range judge {
		e, ok := got[m.Commit][m.Host]
		if !ok || e.Error != "" {
			continue
		}
		m.Expected = e.Path
		run := m.System.Current
		if run == e.Path {
			continue
		}
		// A switch that failed partway leaves the failed commit's system running.
		if f := m.Reports[0]; f.Phase == "deploy" && f.failed() && f.Commit != m.Commit {
			fe, ok := got[f.Commit][m.Host]
			if !ok && slices.Contains(want[f.Commit], m.Host) {
				continue // not evaluated yet: no verdict
			}
			if ok && fe.Error == "" && run == fe.Path {
				m.Drift = "unfinished"
				continue
			}
		}
		if m.System.Profile == e.Path {
			m.Drift = "unfinished" // installed as the boot default, not switched to
		} else {
			m.Drift = "manual"
		}
	}
}

// removed lists the machines that reported but are no longer in the flake,
// when that is certain: the flake evaluated, the working copy is exactly the
// tip of the branch comin deploys, and the machine's last report is of a
// commit of that branch, deployed from it. On another branch, or with a
// machine deleted but not pushed, the machine may still be in service.
func (d *Deployments) removed(ctx context.Context, model *Model, reports map[string][]DeployReport, branch, tip string) []string {
	if model == nil || model.Error != "" || model.Tree == "" || tip == "" {
		return nil
	}
	in := append(hostNames(model), model.Darwin...)
	var gone []string
	for h, rs := range reports {
		if !slices.Contains(in, h) && len(rs) > 0 {
			gone = append(gone, h)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	if tree, err := d.repo.git(ctx, "rev-parse", "--verify", "-q", "--end-of-options", tip+"^{tree}"); err != nil || strings.TrimSpace(tree) != model.Tree {
		return nil
	}
	gone = slices.DeleteFunc(gone, func(h string) bool {
		last := reports[h][0]
		// comin's ref is <remote>/<branch>; older reports have none.
		fromBranch := last.Ref == "" || last.Ref == branch || strings.HasSuffix(last.Ref, "/"+branch)
		return !fromBranch || !d.repo.isAncestor(ctx, last.Commit, tip)
	})
	slices.Sort(gone)
	return gone
}

// Forget deletes the reports of these machines. One that is still in the flake
// shows as never reported until it reports again.
func (d *Deployments) Forget(hosts ...string) error {
	if d == nil {
		return errorf(ErrNotFound, "deployment reports are not set up")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	old, oldLive := map[string][]DeployReport{}, map[string]liveState{}
	for _, h := range hosts {
		if rs, ok := d.reports[h]; ok {
			old[h] = rs
			delete(d.reports, h)
		}
		if l, ok := d.live[h]; ok {
			oldLive[h] = l
			delete(d.live, h)
		}
	}
	if len(old) == 0 && len(oldLive) == 0 {
		return errorf(ErrNotFound, "no reports from %s", strings.Join(hosts, ", "))
	}
	err := d.save()
	if err == nil {
		err = d.saveLive()
	}
	if err != nil {
		maps.Copy(d.reports, old)
		maps.Copy(d.live, oldLive)
		d.save() // best effort: back to what the files said before
		return err
	}
	return nil
}

// cominFollows maps the fleet's machines to the branches their comin follows:
// those set for the remote this console pushes to, else for its first remote.
func cominFollows(ctx context.Context, r *Repo, model *Model) map[string]CominBranches {
	out := map[string]CominBranches{}
	if model == nil {
		return out
	}
	o, err := ParseRemote(r.Remote(ctx))
	known := err == nil && o.Kind != "local"
	for _, h := range model.Hosts {
		if bs := h.Checks.CominBranches; len(bs) > 0 {
			i := slices.IndexFunc(bs, func(b CominBranches) bool { return known && sameRepo(b.URL, o) })
			out[h.Name] = bs[max(i, 0)]
		}
	}
	return out
}

// fleetBranch is the main branch most machines follow (ties: the first by
// name), for the rollout picture and machines whose comin settings are unknown.
func fleetBranch(follows map[string]CominBranches) string {
	n := map[string]int{}
	for _, b := range follows {
		if b.Main != "" {
			n[b.Main]++
		}
	}
	best := defaultBranch
	for br, c := range n {
		if c > n[best] || c == n[best] && br < best {
			best = br
		}
	}
	return best
}

// branchTips maps branch names to their last commit as comin sees them: on
// origin when there is one, else (no remote yet) the local branches.
func (d *Deployments) branchTips(ctx context.Context) (map[string]string, error) {
	prefix := "refs/heads/"
	if _, err := d.repo.git(ctx, "remote", "get-url", "origin"); err == nil {
		prefix = "refs/remotes/origin/"
	}
	out, err := d.repo.git(ctx, "for-each-ref", "--format=%(refname) %(objectname)", prefix)
	if err != nil {
		return nil, err
	}
	tips := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if ref, sha, ok := strings.Cut(l, " "); ok {
			tips[strings.TrimPrefix(ref, prefix)] = sha
		}
	}
	return tips, nil
}
