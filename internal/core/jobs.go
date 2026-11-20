package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	jobTimeout   = 30 * time.Minute
	checkTimeout = 3 * time.Hour // a check builds the machines a change touches; a new kernel can take a while
	maxOutput    = 8 << 20       // per job; nix -L can be chatty
	keepJobs     = 50
	queueLength  = 256  // per lane; skipped model jobs keep their slot until the worker drains them
	keepChecks   = 1000 // passed trees remembered; the oldest are forgotten first
)

// Jobs runs Nix commands in three lanes, each one job at a time in submission
// order: checks, which build machines and can take long; the systems deployed
// machines should run, for Deployments; and everything else (the fleet model,
// search indexes, option values, lock and update), so the fleet views never
// wait for either. It keeps the jobs' output for the UI and
// MCP, and remembers which staged trees passed a check, in checks.json once
// PersistChecks has named it.
type Jobs struct {
	Repo   *Repo
	Nix    string // nix binary
	Nixfmt string // nixfmt binary
	// EvalJobs is how many machines the fleet model evaluates at once. Each is a
	// whole NixOS evaluation, which can take a gigabyte of memory.
	EvalJobs int
	// GCRoots is the folder where a check keeps each machine's last built system
	// from Nix's garbage collector, so the binary cache can still serve it; ""
	// keeps none.
	GCRoots string

	mu          sync.Mutex
	jobs        []*Job // oldest first
	nextID      int
	passed      map[string]CheckPass // by tree
	passedOrder []string             // the passed trees, oldest first
	checksPath  string               // "" keeps them in memory only
	queue       chan *Job            // every kind but check and system
	checkQueue  chan *Job
	systemQueue chan *Job
	systems     map[string]SystemEval // by commit/host
	systemOrder []string
	models      map[string]*Model // by git tree
	retry       *Model            // replaced by Evaluate again: the next evaluation of its tree keeps what evaluated
	modelOrder  []string
	indexes     map[string]any   // search indexes by key
	indexErrs   map[string]error // why the last build of an index failed
	valuesOrder []string         // option value results, oldest first
}

func newJobs(repo *Repo) *Jobs {
	j := &Jobs{Repo: repo, Nix: "nix", Nixfmt: "nixfmt", EvalJobs: min(2, runtime.NumCPU()), passed: map[string]CheckPass{}, models: map[string]*Model{}, indexes: map[string]any{}, indexErrs: map[string]error{},
		queue: make(chan *Job, queueLength), checkQueue: make(chan *Job, queueLength), systemQueue: make(chan *Job, queueLength), systems: map[string]SystemEval{}}
	go j.worker(j.queue)
	go j.worker(j.checkQueue)
	go j.worker(j.systemQueue)
	return j
}

// JobRequest asks for one of:
//   - "check": nix flake check on the staged changes, a build of every machine whose
//     system they change compared with what is pushed, then a dry build of each target attribute;
//   - "lock": nix flake lock of the working copy;
//   - "update": nix flake update of Inputs (all when empty) of the working copy.
type JobRequest struct {
	Kind    string   `json:"kind"`
	Targets []string `json:"targets,omitempty"`
	Inputs  []string `json:"inputs,omitempty"`
	Host    string   `json:"host,omitempty"`  // options index jobs
	Names   []string `json:"names,omitempty"` // option value jobs
	Hosts   []string `json:"hosts,omitempty"`
	// Base is the tree a push lands on, recorded by Submit for a check: the
	// machines are compared with it. Never from a client.
	Base string `json:"-"`
}

type Job struct {
	ID  string
	Req JobRequest

	mu       sync.Mutex
	tree     string
	key      string      // what a model or index job computes
	skip     atomic.Bool // set while queued when the result would be outdated
	keep     atomic.Bool // a request for a commit waits on it too: never skipped
	state    string
	started  time.Time
	finished time.Time
	summary  *CheckSummary // a check's, once it ran
	out      []byte
	changed  chan struct{} // closed and replaced on every write
	done     chan struct{}
}

type JobInfo struct {
	ID       string        `json:"id"`
	Kind     string        `json:"kind"`
	State    string        `json:"state"` // queued, running, passed, failed
	Targets  []string      `json:"targets"`
	Inputs   []string      `json:"inputs,omitempty"`
	Tree     string        `json:"tree,omitempty"`
	Started  time.Time     `json:"started,omitzero"`
	Finished time.Time     `json:"finished,omitzero"`
	Summary  *CheckSummary `json:"summary,omitempty"`
	Output   string        `json:"output,omitempty"`
}

func newJob(id string, req JobRequest) *Job {
	return &Job{ID: id, Req: req, state: "queued", changed: make(chan struct{}), done: make(chan struct{})}
}

func (j *Job) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if room := maxOutput - len(j.out); room <= 0 {
		return len(p), nil
	} else if len(p) > room {
		j.out = append(j.out, p[:room]...)
		j.out = append(j.out, "\n[output truncated]\n"...)
	} else {
		j.out = append(j.out, p...)
	}
	close(j.changed)
	j.changed = make(chan struct{})
	return len(p), nil
}

// Read returns output from offset on, whether the job has finished, and a
// channel that is closed when more output arrives.
func (j *Job) Read(offset int) ([]byte, bool, <-chan struct{}) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var chunk []byte
	if offset < len(j.out) {
		chunk = bytes.Clone(j.out[offset:])
	}
	return chunk, j.state == "passed" || j.state == "failed", j.changed
}

// Done is closed when the job has finished.
func (j *Job) Done() <-chan struct{} { return j.done }

func (j *Job) Info(withOutput bool) JobInfo {
	j.mu.Lock()
	defer j.mu.Unlock()
	info := JobInfo{ID: j.ID, Kind: j.Req.Kind, State: j.state, Targets: j.Req.Targets, Inputs: j.Req.Inputs, Tree: j.tree, Started: j.started, Finished: j.finished, Summary: j.summary}
	if info.Targets == nil {
		info.Targets = []string{}
	}
	if withOutput {
		info.Output = string(j.out)
	}
	return info
}

func (j *Job) finish(passed bool) {
	j.mu.Lock()
	j.state = "failed"
	if passed {
		j.state = "passed"
	}
	j.finished = time.Now()
	close(j.changed)
	j.changed = make(chan struct{})
	j.mu.Unlock()
	close(j.done)
}

var inputName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_'-]*$`)

// Submit validates and queues a job. A check records the staged tree and the
// tree a push lands on now, so later staging or fetching does not change what
// it validates.
func (js *Jobs) Submit(ctx context.Context, req JobRequest) (*Job, error) {
	var tree string
	req.Base = ""
	switch req.Kind {
	case "check":
		for _, t := range req.Targets {
			if t == "" || strings.ContainsAny(t, "\x00\r\n") {
				return nil, errorf(ErrInvalid, "invalid target %q", t)
			}
		}
		js.Repo.mu.Lock()
		var err error
		tree, err = js.Repo.stagedTree(ctx)
		if err == nil {
			req.Base, err = js.Repo.pushBase(ctx)
		}
		js.Repo.mu.Unlock()
		if err != nil {
			return nil, err
		}
	case "lock":
	case "update":
		for _, in := range req.Inputs {
			if !inputName.MatchString(in) {
				return nil, errorf(ErrInvalid, "invalid input name %q", in)
			}
		}
	default:
		return nil, errorf(ErrInvalid, "unknown job kind %q (want check, lock or update)", req.Kind)
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.enqueue(req, tree, "")
}

// enqueue queues a job in its lane; js.mu must be held.
func (js *Jobs) enqueue(req JobRequest, tree, key string) (*Job, error) {
	js.nextID++
	job := newJob(fmt.Sprintf("j%d", js.nextID), req)
	job.tree, job.key = tree, key
	lane := js.queue
	switch req.Kind {
	case "check":
		lane = js.checkQueue
	case "system":
		lane = js.systemQueue
	}
	select {
	case lane <- job:
	default:
		return nil, errorf(ErrConflict, "too many jobs are waiting; try again when they finish")
	}
	js.jobs = append(js.jobs, job)
	// Forget the oldest finished jobs; one lane may finish many while the other runs a long one.
	for len(js.jobs) > keepJobs {
		i := slices.IndexFunc(js.jobs, (*Job).finishedState)
		if i < 0 {
			break
		}
		js.jobs = slices.Delete(js.jobs, i, i+1)
	}
	return job, nil
}

// List returns the jobs newest first, without their output.
func (js *Jobs) List() []JobInfo {
	js.mu.Lock()
	defer js.mu.Unlock()
	out := make([]JobInfo, 0, len(js.jobs))
	for i := len(js.jobs) - 1; i >= 0; i-- {
		out = append(out, js.jobs[i].Info(false))
	}
	return out
}

func (js *Jobs) Get(id string) (*Job, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	for _, j := range js.jobs {
		if j.ID == id {
			return j, true
		}
	}
	return nil, false
}

// Passed reports whether a check passed for exactly this staged tree.
func (js *Jobs) Passed(tree string) bool {
	_, ok := js.PassOf(tree)
	return ok
}

// PassOf returns the check that passed for exactly this tree.
func (js *Jobs) PassOf(tree string) (CheckPass, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	p, ok := js.passed[tree]
	return p, ok
}

type checksFile struct {
	Checks []CheckPass `json:"checks"` // oldest first
	// Passed are trees that passed a check before checks built the machines:
	// they only evaluated, so they no longer count.
	Passed []string `json:"passed,omitempty"`
}

// PersistChecks loads the trees that passed a check from path and saves every
// new one there, so a restart does not ask for the checks again. An unreadable
// file only means running checks again, so it is reported and then replaced.
func (js *Jobs) PersistChecks(path string) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var f checksFile
	if err == nil {
		if err := json.Unmarshal(b, &f); err != nil {
			slog.Warn("cannot read the passed checks; they must run again", "file", path, "err", err)
			f = checksFile{}
		}
	}
	if len(f.Passed) > 0 {
		slog.Info("checks that passed before checks built the machines only evaluated; they must run again", "count", len(f.Passed))
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	js.checksPath = path
	for _, p := range f.Checks {
		js.pass(p)
	}
	return nil
}

// pass records a passed check as the newest and returns the list to save;
// js.mu must be held.
func (js *Jobs) pass(p CheckPass) []CheckPass {
	js.passedOrder = append(slices.DeleteFunc(js.passedOrder, func(t string) bool { return t == p.Tree }), p.Tree)
	js.passed[p.Tree] = p
	if n := len(js.passedOrder) - keepChecks; n > 0 {
		for _, t := range js.passedOrder[:n] {
			delete(js.passed, t)
		}
		js.passedOrder = slices.Delete(js.passedOrder, 0, n)
	}
	out := make([]CheckPass, len(js.passedOrder))
	for i, t := range js.passedOrder {
		out[i] = js.passed[t]
	}
	return out
}

func (js *Jobs) worker(lane <-chan *Job) {
	for job := range lane {
		if job.skip.Load() {
			fmt.Fprintln(job, "Skipped: the files changed again before this evaluation started.")
			job.finish(false)
			continue
		}
		job.mu.Lock()
		job.state, job.started = "running", time.Now()
		job.mu.Unlock()
		err := js.run(job)
		if err != nil {
			fmt.Fprintf(job, "\n%v\n", err)
		}
		if err == nil && job.Req.Kind == "check" {
			js.mu.Lock()
			passed, path := js.pass(CheckPass{Tree: job.tree, Base: job.Req.Base, Built: job.Info(false).Summary.Built}), js.checksPath
			js.mu.Unlock()
			if path != "" { // only the check lane saves, so the writes cannot overtake each other
				b, _ := json.Marshal(checksFile{Checks: passed}) // strings always marshal
				if err := writeAtomic(path, b); err != nil {
					fmt.Fprintf(job, "\nThe check passed, but saving that failed (%v): after a restart it must run again.\n", err)
				}
			}
		}
		job.finish(err == nil)
	}
}

func (js *Jobs) nixArgs(args ...string) []string {
	return append([]string{"--extra-experimental-features", "nix-command flakes"}, args...)
}

func (js *Jobs) run(job *Job) error {
	timeout := jobTimeout
	if job.Req.Kind == "check" {
		timeout = checkTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	switch job.Req.Kind {
	case "check":
		return js.runCheck(ctx, job)
	case "model":
		return js.runModel(ctx, job)
	case "packages", "options":
		return js.runIndex(ctx, job)
	case "values":
		return js.runValues(ctx, job)
	case "system":
		return js.runSystems(ctx, job)
	}
	var dir string
	var steps [][]string
	var after func() error // once every step passed
	switch job.Req.Kind {
	case "lock", "update":
		// Nix copies a path: flake whole into the world-readable store, so it gets an
		// export, never the working copy with .git and stray files. Repo.mu is held
		// only to export and to copy flake.lock back, not for the whole download.
		tree, base, err := js.Repo.lockSource(ctx)
		if err != nil {
			return err
		}
		src, cleanup, err := js.snapshot(ctx, tree)
		if err != nil {
			return err
		}
		defer cleanup()
		dir = src
		if job.Req.Kind == "lock" {
			steps = append(steps, js.nixArgs("flake", "lock", "path:"+src))
		} else {
			steps = append(steps, js.nixArgs(append(append([]string{"flake", "update"}, job.Req.Inputs...), "--flake", "path:"+src)...))
		}
		after = func() error {
			lock, err := os.ReadFile(filepath.Join(src, "flake.lock"))
			if err != nil {
				return fmt.Errorf("nix wrote no flake.lock: %v", err)
			}
			return js.Repo.putLock(base, lock)
		}
	}
	for _, args := range steps {
		if err := js.step(ctx, job, dir, args); err != nil {
			return err
		}
	}
	if after != nil {
		if err := after(); err != nil {
			return err
		}
	}
	fmt.Fprintln(job, "\nAll steps passed.")
	return nil
}

// step runs one nix command in dir, its output going to the job's.
func (js *Jobs) step(ctx context.Context, job *Job, dir string, args []string) error {
	fmt.Fprintf(job, "$ nix %s\n", strings.Join(args[2:], " "))
	cmd := exec.CommandContext(ctx, js.Nix, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = job, job
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("stopped: the job ran out of time")
		}
		return fmt.Errorf("failed: %v", err)
	}
	return nil
}

// Target is a buildable flake output that a check can dry-build.
type Target struct {
	Kind string `json:"kind"` // nixos, home or darwin
	Name string `json:"name"`
	Attr string `json:"attr"`
}

var plainAttr = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_'-]*$`)

func attrName(n string) string {
	if plainAttr.MatchString(n) {
		return n
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `${`, `\${`).Replace(n) + `"`
}

// Targets lists the NixOS, home-manager and nix-darwin configurations of the
// staged flake.
func (js *Jobs) Targets(ctx context.Context) ([]Target, error) {
	js.Repo.mu.Lock()
	tree, err := js.Repo.stagedTree(ctx)
	js.Repo.mu.Unlock()
	if err != nil {
		return nil, err
	}
	src, cleanup, err := js.snapshot(ctx, tree)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	names, err := js.discover(ctx, src)
	if err != nil {
		return nil, err
	}
	targets := []Target{}
	for _, k := range []struct{ kind, prefix, suffix string }{
		{"nixos", "nixosConfigurations.", ".config.system.build.toplevel"},
		{"home", "homeConfigurations.", ".activationPackage"},
		{"darwin", "darwinConfigurations.", ".system"},
	} {
		for _, n := range names[k.kind] {
			targets = append(targets, Target{Kind: k.kind, Name: n, Attr: k.prefix + attrName(n) + k.suffix})
		}
	}
	return targets, nil
}

func lastLine(b []byte) []byte {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	return lines[len(lines)-1]
}

// Format runs nixfmt on content.
func (js *Jobs) Format(ctx context.Context, content string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, js.Nixfmt)
	cmd.Stdin = strings.NewReader(content)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return "", fmt.Errorf("running nixfmt: %w", err)
		}
		return "", errorf(ErrSyntax, "nixfmt: %s", strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
