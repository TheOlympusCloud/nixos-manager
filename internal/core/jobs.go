package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	jobTimeout  = 30 * time.Minute
	maxOutput   = 8 << 20 // per job; nix -L can be chatty
	keepJobs    = 50
	queueLength = 256 // skipped model jobs keep their slot until the worker drains them
)

// Jobs runs Nix commands one at a time, in submission order, and keeps their
// output for the UI and MCP. It remembers which staged trees passed a check.
type Jobs struct {
	Repo   *Repo
	Nix    string // nix binary
	Nixfmt string // nixfmt binary

	mu         sync.Mutex
	jobs       []*Job // oldest first
	nextID     int
	passed     map[string]bool
	queue      chan *Job
	models     map[string]*Model // by git tree
	modelOrder []string
	indexes    map[string]any   // search indexes by key
	indexErrs  map[string]error // why the last build of an index failed
}

func newJobs(repo *Repo) *Jobs {
	j := &Jobs{Repo: repo, Nix: "nix", Nixfmt: "nixfmt", passed: map[string]bool{}, models: map[string]*Model{}, indexes: map[string]any{}, indexErrs: map[string]error{}, queue: make(chan *Job, queueLength)}
	go j.worker()
	return j
}

// JobRequest asks for one of:
//   - "check": nix flake check on the staged changes, then a dry build of each target attribute;
//   - "lock": nix flake lock in the working copy;
//   - "update": nix flake update of Inputs (all when empty) in the working copy.
type JobRequest struct {
	Kind    string   `json:"kind"`
	Targets []string `json:"targets,omitempty"`
	Inputs  []string `json:"inputs,omitempty"`
	Host    string   `json:"host,omitempty"` // options index jobs
}

type Job struct {
	ID  string
	Req JobRequest

	mu       sync.Mutex
	tree     string
	key      string      // what a model or index job computes
	skip     atomic.Bool // set while queued when the result would be outdated
	state    string
	started  time.Time
	finished time.Time
	out      []byte
	changed  chan struct{} // closed and replaced on every write
	done     chan struct{}
}

type JobInfo struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	State    string    `json:"state"` // queued, running, passed, failed
	Targets  []string  `json:"targets"`
	Inputs   []string  `json:"inputs,omitempty"`
	Tree     string    `json:"tree,omitempty"`
	Started  time.Time `json:"started,omitzero"`
	Finished time.Time `json:"finished,omitzero"`
	Output   string    `json:"output,omitempty"`
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
	info := JobInfo{ID: j.ID, Kind: j.Req.Kind, State: j.state, Targets: j.Req.Targets, Inputs: j.Req.Inputs, Tree: j.tree, Started: j.started, Finished: j.finished}
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

// Submit validates and queues a job. A check records the staged tree now, so
// later staging does not change what it validates.
func (js *Jobs) Submit(ctx context.Context, req JobRequest) (*Job, error) {
	var tree string
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

// enqueue queues a job for the worker; js.mu must be held.
func (js *Jobs) enqueue(req JobRequest, tree, key string) (*Job, error) {
	js.nextID++
	job := newJob(fmt.Sprintf("j%d", js.nextID), req)
	job.tree, job.key = tree, key
	select {
	case js.queue <- job:
	default:
		return nil, errorf(ErrConflict, "too many jobs are waiting; try again when they finish")
	}
	js.jobs = append(js.jobs, job)
	for len(js.jobs) > keepJobs && js.jobs[0].finishedState() {
		js.jobs = js.jobs[1:]
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
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.passed[tree]
}

func (js *Jobs) worker() {
	for job := range js.queue {
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
			js.passed[job.tree] = true
			js.mu.Unlock()
		}
		job.finish(err == nil)
	}
}

func (js *Jobs) nixArgs(args ...string) []string {
	return append([]string{"--extra-experimental-features", "nix-command flakes"}, args...)
}

func (js *Jobs) run(job *Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
	defer cancel()
	switch job.Req.Kind {
	case "model":
		return js.runModel(ctx, job)
	case "packages", "options":
		return js.runIndex(ctx, job)
	}
	dir := js.Repo.Dir
	var steps [][]string
	switch job.Req.Kind {
	case "check":
		tmp, err := os.MkdirTemp("", "nixmgr-check-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		src := filepath.Join(tmp, "src")
		if err := js.Repo.export(ctx, job.tree, src); err != nil {
			return err
		}
		fmt.Fprintf(job, "Checking the staged changes (tree %.12s).\n", job.tree)
		dir = src
		steps = append(steps, js.nixArgs("flake", "check", "-L", "--keep-going", "--no-update-lock-file", "path:"+src))
		for _, t := range job.Req.Targets {
			steps = append(steps, js.nixArgs("build", "--dry-run", "--no-link", "-L", "--no-update-lock-file", "path:"+src+"#"+t))
		}
	case "lock":
		// lock and update write flake.lock without Repo.mu, which would block every edit for
		// the whole download. Safe: a concurrent save of flake.lock is hash-checked and atomic.
		// shortcut: path: copies the whole folder (with .git) to the store; fine for a config repo, revisit if .git grows large.
		steps = append(steps, js.nixArgs("flake", "lock", "path:"+dir))
	case "update":
		steps = append(steps, js.nixArgs(append(append([]string{"flake", "update"}, job.Req.Inputs...), "--flake", "path:"+dir)...))
	}
	for _, args := range steps {
		fmt.Fprintf(job, "$ nix %s\n", strings.Join(args[2:], " "))
		cmd := exec.CommandContext(ctx, js.Nix, args...)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = job, job
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("stopped after %s", jobTimeout)
			}
			return fmt.Errorf("failed: %v", err)
		}
	}
	fmt.Fprintln(job, "\nAll steps passed.")
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
