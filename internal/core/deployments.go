package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
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
// never deployed, so without the build report it would look behind.

const (
	MaxReportBody  = 16 << 10
	keepReports    = 20  // per machine
	maxReporters   = 200 // machines; the token is shared, so one machine could invent others
	silentAfter    = 7 * 24 * time.Hour
	behindAfter    = time.Hour
	rolloutCommits = 12
	// shortcut: comin's main branch is assumed to be "main" (the starter fleet's); read it from comin's settings if a fleet uses another.
	mainBranch = "main"
)

var (
	reportHost = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}$`)
	reportSHA  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	reportRef  = regexp.MustCompile(`^[\x21-\x7e]{0,200}$`)
	reportGen  = regexp.MustCompile(`^[A-Za-z0-9-]{0,64}$`)
	reportOK   = map[string][]string{"deploy": {"done", "failed"}, "build": {"built", "already built", "failed"}}
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

// Deployments keeps the last reports of each machine in deployments.json and
// the fleet report token machines send them with.
type Deployments struct {
	repo      *Repo
	path      string
	tokenPath string

	mu      sync.Mutex
	token   string
	reports map[string][]DeployReport // newest first
}

// OpenDeployments loads dataDir/deployments.json and the report token,
// creating the token on first use.
func OpenDeployments(dataDir string, repo *Repo) (*Deployments, error) {
	d := &Deployments{repo: repo, path: filepath.Join(dataDir, "deployments.json"), tokenPath: filepath.Join(dataDir, "deploy-token"), reports: map[string][]DeployReport{}}
	b, err := os.ReadFile(d.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &d.reports); err != nil {
			return nil, errorf(ErrInvalid, "reading %s: %v", d.path, err)
		}
	}
	t, err := os.ReadFile(d.tokenPath)
	if errors.Is(err, os.ErrNotExist) {
		_, err = d.RegenerateToken()
		return d, err
	}
	d.token = strings.TrimSpace(string(t))
	return d, err
}

// Token is the fleet report token, for the setup panel.
func (d *Deployments) Token() string {
	if d == nil {
		return ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.token
}

// RegenerateToken replaces the token; machines with the old one stop reporting.
func (d *Deployments) RegenerateToken() (string, error) {
	if d == nil {
		return "", errorf(ErrNotFound, "deployment reports are not set up")
	}
	t := "nxd_" + rand.Text()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := writeAtomic(d.tokenPath, []byte(t+"\n")); err != nil {
		return "", err
	}
	d.token = t
	return t, nil
}

// CheckToken compares a presented token with the fleet's in constant time.
func (d *Deployments) CheckToken(secret string) bool {
	if d == nil || secret == "" {
		return false
	}
	a, b := sha256.Sum256([]byte(secret)), sha256.Sum256([]byte(d.Token()))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// Record validates a machine's report and stores it.
func (d *Deployments) Record(in ReportIn, now time.Time) error {
	if d == nil {
		return errorf(ErrNotFound, "deployment reports are not set up")
	}
	if in.Phase == "" {
		in.Phase = "deploy"
	}
	switch {
	case !reportHost.MatchString(in.Host):
		return errorf(ErrInvalid, "host must be a machine name")
	case !reportSHA.MatchString(in.Commit):
		return errorf(ErrInvalid, "commit must be a 40-character hex SHA")
	case !slices.Contains(reportOK[in.Phase], in.Status):
		return errorf(ErrInvalid, "unknown phase/status %q/%q", in.Phase, in.Status)
	case !reportRef.MatchString(in.Ref) || !reportGen.MatchString(in.Generation):
		return errorf(ErrInvalid, "invalid ref or generation")
	case len(in.Error) > 8192 || !utf8.ValidString(in.Error):
		return errorf(ErrInvalid, "error must be UTF-8 text of at most 8 KiB")
	}
	rep := DeployReport{At: now.UTC().Truncate(time.Second), Commit: in.Commit, Ref: in.Ref, Phase: in.Phase, Status: in.Status, Error: in.Error, Generation: in.Generation}
	d.mu.Lock()
	defer d.mu.Unlock()
	old, known := d.reports[in.Host]
	if !known && len(d.reports) >= maxReporters {
		return errorf(ErrInvalid, "already keeping reports of %d machines", maxReporters)
	}
	d.reports[in.Host] = append([]DeployReport{rep}, old[:min(len(old), keepReports-1)]...)
	if err := d.save(); err != nil {
		if known {
			d.reports[in.Host] = old
		} else {
			delete(d.reports, in.Host)
		}
		return err
	}
	return nil
}

func (d *Deployments) save() error {
	b, err := json.MarshalIndent(d.reports, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(d.path, b)
}

// writeAtomic replaces path with data, readable by the service user only.
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
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// MachineDeployment is what one machine reported and what that means against
// the branch comin follows for it.
type MachineDeployment struct {
	Host string `json:"host"`
	// State: never (no report), failed (the last report is a failure), unknown
	// (its commit is not in this working copy or not on the branch), current,
	// behind, or silent (behind and no report for 7 days: probably off).
	State  string `json:"state"`
	Branch string `json:"branch"`           // main, or testing-<host> when that is on top of main
	Target string `json:"target,omitempty"` // the branch's last commit
	// Commit is the last commit it deployed, or built and found unchanged.
	Commit      string         `json:"commit,omitempty"`
	Subject     string         `json:"subject,omitempty"`
	DeployedAt  time.Time      `json:"deployedAt,omitzero"`
	LastSeen    time.Time      `json:"lastSeen,omitzero"`
	Behind      int            `json:"behind"`
	BehindSince time.Time      `json:"behindSince,omitzero"` // when the oldest commit it lacks was made
	Failure     *DeployReport  `json:"failure,omitempty"`
	Reports     []DeployReport `json:"reports"`
}

type DeployOverview struct {
	Reporting bool                `json:"reporting"` // some machine has reported
	Branch    string              `json:"branch"`
	Commits   []Commit            `json:"commits"` // the newest on the branch, for the rollout picture
	Machines  []MachineDeployment `json:"machines"`
}

// HostNames lists the machines of an evaluated fleet.
func HostNames(m *Model) []string {
	var out []string
	if m != nil {
		for _, h := range m.Hosts {
			out = append(out, h.Name)
		}
	}
	return out
}

// Overview says, for the given machines and every machine that reported, what
// it runs and how that compares with the branch comin follows.
func (d *Deployments) Overview(ctx context.Context, hosts []string, now time.Time) (DeployOverview, error) {
	ov := DeployOverview{Branch: mainBranch, Commits: []Commit{}, Machines: []MachineDeployment{}}
	if d == nil {
		return ov, nil
	}
	hosts = slices.Clone(hosts) // sorted and extended below
	d.mu.Lock()
	reports := make(map[string][]DeployReport, len(d.reports))
	for h, rs := range d.reports {
		reports[h] = slices.Clone(rs)
		hosts = append(hosts, h)
	}
	d.mu.Unlock()
	slices.Sort(hosts)
	hosts = slices.Compact(hosts)
	ov.Reporting = len(reports) > 0

	tips, err := d.branchTips(ctx)
	if err != nil {
		return ov, err
	}
	mainTip := tips[mainBranch]
	if mainTip != "" {
		if ov.Commits, err = d.repo.Log(ctx, "", mainTip, rolloutCommits); err != nil {
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
		m := MachineDeployment{Host: h, Branch: mainBranch, Target: mainTip, Reports: reports[h]}
		if m.Reports == nil {
			m.Reports = []DeployReport{}
		}
		if t := tips["testing-"+h]; t != "" && mainTip != "" && t != mainTip && d.isAncestor(ctx, mainTip, t) {
			m.Branch, m.Target = "testing-"+h, t // comin deploys a testing branch that is on top of main
		}
		if len(m.Reports) > 0 {
			m.LastSeen = m.Reports[0].At
			if m.Reports[0].failed() {
				m.Failure = &m.Reports[0]
			}
		}
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
		case now.Sub(m.LastSeen) > silentAfter:
			m.State = "silent"
		default:
			m.State = "behind"
		}
		ov.Machines = append(ov.Machines, m)
	}
	return ov, nil
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

func (d *Deployments) isAncestor(ctx context.Context, a, b string) bool {
	_, err := d.repo.git(ctx, "merge-base", "--is-ancestor", "--end-of-options", a, b)
	return err == nil
}
