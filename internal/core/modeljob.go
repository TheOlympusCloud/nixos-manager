package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

//go:embed nix/*.nix
var nixFiles embed.FS

func nixExpr(name string) string {
	b, err := nixFiles.ReadFile("nix/" + name)
	if err != nil {
		panic(err) // embedded at build time
	}
	return string(b)
}

const keepModels = 16

// WorkingTree is the tree object of everything in the working copy, untracked
// files included, built in a private index so the real one is untouched. Every
// UI poll asks for it, so it is kept until a file changes, and served without
// the repository lock until then.
func (r *Repo) WorkingTree(ctx context.Context) (string, error) {
	sum, cacheable := r.fingerprint()
	if cacheable {
		r.wt.mu.Lock()
		tree, same := r.wt.tree, r.wt.sum == sum
		r.wt.mu.Unlock()
		if same && tree != "" {
			return tree, nil
		}
	}
	r.mu.Lock()
	tree, err := r.indexTree(ctx, []string{"-A", "--", "."})
	r.mu.Unlock()
	if err == nil && cacheable { // the fingerprint was taken first: a change since then is a miss next time
		r.wt.mu.Lock()
		r.wt.sum, r.wt.tree = sum, tree
		r.wt.mu.Unlock()
	}
	return tree, err
}

// racyWindow is how old every timestamp must be before a fingerprint is
// trusted: a file changed within the same clock tick as the walk could change
// again without moving its times. Linux file times advance in jiffies, a few
// milliseconds; this leaves room for coarser file systems.
var racyWindow = 2 * time.Second

// fingerprint hashes what git add -A reads: the path, type, mode, size, times
// and inode of everything in the working copy, and of the index and
// info/exclude. cacheable is false when a timestamp is too recent to prove
// that a later change would move it, or the walk failed.
// shortcut: the global excludes file (core.excludesFile) is not watched; a change there shows once a repository file changes.
func (r *Repo) fingerprint() (sum [32]byte, cacheable bool) {
	start := time.Now()
	h := sha256.New()
	cacheable = true
	add := func(name string, fi fs.FileInfo) {
		ctime, ino, ok := fileID(fi)
		if !ok || start.Sub(ctime) < racyWindow || start.Sub(fi.ModTime()) < racyWindow {
			cacheable = false
		}
		fmt.Fprintf(h, "%q %v %d %d %d %d\x00", name, fi.Mode(), fi.Size(), fi.ModTime().UnixNano(), ctime.UnixNano(), ino)
	}
	err := filepath.WalkDir(r.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" && filepath.Dir(p) == r.Dir {
			return fs.SkipDir
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		add(p, fi)
		return nil
	})
	for _, p := range r.gitFiles {
		if fi, err := os.Lstat(p); err == nil {
			add(p, fi)
		} else {
			fmt.Fprintf(h, "%q absent\x00", p)
		}
	}
	h.Sum(sum[:0])
	return sum, cacheable && err == nil
}

// indexTree runs each git add in a private copy of the index, so the real one
// is untouched, and returns the tree it then holds. Needs r.mu.
func (r *Repo) indexTree(ctx context.Context, adds ...[]string) (string, error) {
	tmp, err := os.CreateTemp("", "nixmgr-index-")
	if err != nil {
		return "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	// Start from the real index so git can reuse its file stat cache.
	if p, err := r.git(ctx, "rev-parse", "--git-path", "index"); err == nil {
		if b, err := os.ReadFile(filepath.Join(r.Dir, strings.TrimSpace(p))); err == nil {
			err = os.WriteFile(tmp.Name(), b, 0o600)
		} else {
			os.Remove(tmp.Name()) // git builds a new index; an empty file is not one
		}
	}
	env := []string{"GIT_INDEX_FILE=" + tmp.Name()}
	for _, add := range adds {
		if _, _, err := r.gitRun(ctx, env, nil, append([]string{"add"}, add...)...); err != nil {
			return "", err
		}
	}
	out, _, err := r.gitRun(ctx, env, nil, "write-tree")
	return strings.TrimSpace(out), err
}

// Parent returns the first parent of a commit, or false for a root commit.
func (r *Repo) Parent(ctx context.Context, rev string) (string, bool) {
	sha, err := r.resolve(ctx, rev)
	if err != nil {
		return "", false
	}
	out, err := r.git(ctx, "rev-parse", "--verify", "-q", sha+"^1")
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(out), true
}

func (r *Repo) headTree(ctx context.Context) (string, error) {
	if !r.hasHead(ctx) {
		return "", nil
	}
	out, err := r.git(ctx, "rev-parse", "HEAD^{tree}")
	return strings.TrimSpace(out), err
}

// Model returns the evaluated fleet at "working" (the working copy), "head",
// "pushed" (what a push of the current branch lands on) or "rev:<commit>".
// When it is not computed yet it returns the job computing it instead.
func (js *Jobs) Model(ctx context.Context, at string) (*Model, *Job, error) {
	var tree string
	var err error
	switch at {
	case "working", "":
		tree, err = js.Repo.WorkingTree(ctx)
	case "head":
		tree, err = js.Repo.headTree(ctx)
	case "pushed":
		js.Repo.mu.Lock()
		tree, err = js.Repo.pushBase(ctx)
		js.Repo.mu.Unlock()
	default:
		rev, ok := strings.CutPrefix(at, "rev:")
		if !ok {
			return nil, nil, errorf(ErrInvalid, "unknown model %q (want working, head, pushed or rev:<commit>)", at)
		}
		var sha string
		if sha, err = js.Repo.resolve(ctx, rev); err == nil {
			var out string
			out, err = js.Repo.git(ctx, "rev-parse", sha+"^{tree}")
			tree = strings.TrimSpace(out)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if tree == "" {
		return emptyModel(), nil, nil
	}
	key := "model:" + at
	if key == "model:" {
		key = "model:working"
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.modelFor(tree, key)
}

func emptyModel() *Model { return &Model{Hosts: []Host{}, Homes: []Home{}, Inputs: []Input{}} }

// modelFor returns the model of tree, or the job computing it, which it queues
// if there is none. Only the working copy's requests may be skipped by newer
// ones. js.mu must be held.
func (js *Jobs) modelFor(tree, key string) (*Model, *Job, error) {
	if m := js.models[tree]; m != nil {
		return m, nil, nil
	}
	for _, j := range js.jobs {
		if j.Req.Kind == "model" && j.tree == tree && !j.finishedState() {
			if key != "model:working" {
				j.keep.Store(true)
			}
			return nil, j, nil
		}
	}
	if key == "model:working" { // the working copy moved on: older evaluations still waiting are pointless
		for _, j := range js.jobs {
			if j.key == key && !j.keep.Load() && j.Info(false).State == "queued" {
				j.skip.Store(true)
			}
		}
	}
	j, err := js.enqueue(JobRequest{Kind: "model"}, tree, key)
	return nil, j, err
}

// RefreshModel evaluates the working copy again, for when the last evaluation
// failed for a reason outside the files (no network, a fixed Nix setup). The
// machines that evaluated are kept: their inputs, the same files, did not change.
func (js *Jobs) RefreshModel(ctx context.Context) (*Job, error) {
	tree, err := js.Repo.WorkingTree(ctx)
	if err != nil {
		return nil, err
	}
	js.mu.Lock()
	js.retry = js.models[tree]
	delete(js.models, tree)
	js.mu.Unlock()
	_, j, err := js.Model(ctx, "working")
	if err == nil && j == nil {
		err = errorf(ErrConflict, "the model is already being refreshed")
	}
	return j, err
}

// prepareModel queues the evaluation of tree for a check, which never trusts a
// cached failure (no network, say): that model is evaluated again, keeping the
// machines that evaluated. Tree "" is a fleet with no machines.
func (js *Jobs) prepareModel(tree string) error {
	if tree == "" {
		return nil
	}
	js.mu.Lock()
	defer js.mu.Unlock()
	if m := js.models[tree]; m != nil && modelFailed(m) {
		js.retry = m
		delete(js.models, tree)
	}
	_, _, err := js.modelFor(tree, "model:check")
	return err
}

// waitModel waits for the model of tree, after prepareModel.
func (js *Jobs) waitModel(ctx context.Context, tree string) (*Model, error) {
	if tree == "" {
		return emptyModel(), nil
	}
	for range 3 { // the model can be dropped from the cache between the job's end and this read
		js.mu.Lock()
		m, j, err := js.modelFor(tree, "model:check")
		js.mu.Unlock()
		if m != nil || err != nil {
			return m, err
		}
		select {
		case <-j.Done():
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, errors.New("the fleet evaluation kept being dropped before the check could read it; run the check again")
}

// keepModel caches m for tree as the newest and forgets the oldest beyond
// keepModels. A tree evaluated again (its failed model dropped) moves to the
// end: forgetting its old place would drop the fresh model. Needs js.mu.
func (js *Jobs) keepModel(tree string, m *Model) {
	js.models[tree] = m
	js.modelOrder = append(slices.DeleteFunc(js.modelOrder, func(t string) bool { return t == tree }), tree)
	for len(js.modelOrder) > keepModels {
		delete(js.models, js.modelOrder[0])
		js.modelOrder = js.modelOrder[1:]
	}
}

func modelFailed(m *Model) bool {
	return m.Error != "" || slices.ContainsFunc(m.Hosts, func(h Host) bool { return h.Error != "" }) ||
		slices.ContainsFunc(m.Homes, func(h Home) bool { return h.Error != "" })
}

func (j *Job) finishedState() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state == "passed" || j.state == "failed"
}

// runModel evaluates every configuration of the flake in tree, one nix process
// per machine so one broken machine does not hide the others.
func (js *Jobs) runModel(ctx context.Context, job *Job) error {
	m := &Model{Tree: job.tree, Hosts: []Host{}, Homes: []Home{}, Inputs: []Input{}}
	defer func() {
		js.mu.Lock()
		js.keepModel(job.tree, m)
		js.mu.Unlock()
	}()
	src, cleanup, err := js.snapshot(ctx, job.tree)
	if err != nil {
		m.Error = err.Error()
		return err
	}
	defer cleanup()
	if lock, err := os.ReadFile(filepath.Join(src, "flake.lock")); err == nil {
		if m.Inputs, err = parseInputs(lock); err != nil {
			fmt.Fprintf(job, "%v\n", err)
			m.Inputs = []Input{}
		}
	}
	names, err := js.discover(ctx, src)
	if err != nil {
		m.Error = err.Error()
		return err
	}
	m.Darwin = names["darwin"]
	keptHosts, keptHomes := map[string]Host{}, map[string]Home{}
	js.mu.Lock()
	if js.retry != nil && js.retry.Tree == job.tree {
		for _, h := range js.retry.Hosts {
			if h.Error == "" {
				keptHosts[h.Name] = h
			}
		}
		for _, h := range js.retry.Homes {
			if h.Error == "" {
				keptHomes[h.Name] = h
			}
		}
		js.retry = nil
	}
	js.mu.Unlock()
	// Up to EvalJobs nix processes at once, each writing only its own slot.
	hosts, homes := make([]Host, len(names["nixos"])), make([]Home, len(names["home"]))
	slots := make(chan struct{}, max(1, js.EvalJobs))
	var wg sync.WaitGroup
	start := func(f func()) {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			f()
		})
	}
	for i, n := range names["nixos"] {
		if h, ok := keptHosts[n]; ok {
			fmt.Fprintf(job, "Kept %s: it evaluated last time.\n", n)
			hosts[i] = h
			continue
		}
		start(func() {
			fmt.Fprintf(job, "Evaluating %s…\n", n)
			var out struct { // host.nix returns the detail next to the model's fields
				Host
				HostDetail
			}
			out.Name = n
			h := Host{Name: n}
			if err := js.evalJSON(ctx, src, map[string]string{"host": n}, nixExpr("host.nix"), &out); err != nil {
				h.Error = err.Error()
				fmt.Fprintf(job, "%s: %v\n", n, err)
			} else {
				h = out.Host
				h.Detail = &out.HostDetail
			}
			hosts[i] = normalizeHost(h)
		})
	}
	for i, n := range names["home"] {
		if h, ok := keptHomes[n]; ok {
			homes[i] = h
			continue
		}
		start(func() {
			fmt.Fprintf(job, "Evaluating home %s…\n", n)
			h := Home{Name: n}
			if err := js.evalJSON(ctx, src, map[string]string{"host": n}, nixExpr("home.nix"), &h); err != nil {
				h = Home{Name: n, Error: err.Error()}
			}
			h.Packages, h.Programs = orEmpty(h.Packages), orEmpty(h.Programs)
			homes[i] = h
		})
	}
	wg.Wait()
	m.Hosts, m.Homes = hosts, homes
	slices.SortFunc(m.Hosts, func(a, b Host) int { return strings.Compare(a.Name, b.Name) })
	fmt.Fprintf(job, "Evaluated %d machine(s).\n", len(m.Hosts))
	return nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// normalizeHost gives the UI arrays instead of nulls and a stable order.
func normalizeHost(h Host) Host {
	h.Packages, h.Features, h.Users = orEmpty(h.Packages), orEmpty(h.Features), orEmpty(h.Users)
	h.FileSystems, h.Files = orEmpty(h.FileSystems), orEmpty(h.Files)
	h.Ports.TCP, h.Ports.UDP = orEmpty(h.Ports.TCP), orEmpty(h.Ports.UDP)
	if h.ManagedPackages == nil {
		h.ManagedPackages = map[string]string{}
	}
	slices.SortFunc(h.Packages, func(a, b Package) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(h.Features, func(a, b Feature) int {
		return strings.Compare(a.Group+"."+a.Name, b.Group+"."+b.Name)
	})
	slices.SortFunc(h.Users, func(a, b User) int { return strings.Compare(a.Name, b.Name) })
	for i := range h.Users {
		h.Users[i].Groups = orEmpty(h.Users[i].Groups)
	}
	return h
}

// snapshot exports tree to a temporary folder for Nix.
func (js *Jobs) snapshot(ctx context.Context, tree string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "nixmgr-src-")
	if err != nil {
		return "", nil, err
	}
	src := filepath.Join(tmp, "src")
	if err := js.Repo.export(ctx, tree, src); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	return src, func() { os.RemoveAll(tmp) }, nil
}

func (js *Jobs) discover(ctx context.Context, src string) (map[string][]string, error) {
	var names map[string][]string
	if err := js.evalJSON(ctx, src, nil, nixExpr("discover.nix"), &names); err != nil {
		return nil, err // the UI already says the flake does not evaluate
	}
	return names, nil
}

// evalJSON calls a nix expression, a function of flakeRef and args, for the
// flake in src and decodes its JSON result into out. The evaluation is pure:
// fleet code must not read the server's files, such as the age key.
func (js *Jobs) evalJSON(ctx context.Context, src string, args map[string]string, expr string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ref, err := js.lockedFlake(ctx, src)
	if err != nil {
		return err
	}
	// In the expression: nix eval does not pass --argstr to a top-level function.
	call := "(" + expr + "\n) { flakeRef = " + nixString(ref) + ";"
	for _, k := range slices.Sorted(maps.Keys(args)) {
		call += " " + k + " = " + nixString(args[k]) + ";"
	}
	cmd := js.nixCmd(ctx, js.nixArgs("eval", "--json", "--expr", call+" }")...)
	var stderr bytes.Buffer
	stdout := capped{max: maxEvalOutput}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if stdout.over {
			return fmt.Errorf("nix printed more than %d MB; the evaluation is too large to show", maxEvalOutput>>20)
		}
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return fmt.Errorf("cannot run nix (%v); is Nix installed?", err)
		}
		msg := tail(stderr.String(), 25)
		if strings.Contains(msg, "cannot update unlocked flake input") { // pure mode fetches only what flake.lock pins
			msg += "\nflake.lock does not pin every input of flake.nix: run Lock."
		}
		return errors.New(msg)
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.buf.Bytes()), out); err != nil {
		return fmt.Errorf("unexpected nix output: %v", err)
	}
	return nil
}

// lockedFlake is src as a flake reference pure evaluation accepts: pinned by
// its NAR hash, which nix checks as it copies src to the store.
func (js *Jobs) lockedFlake(ctx context.Context, src string) (string, error) {
	out, err := js.nixCmd(ctx, js.nixArgs("hash", "path", src)...).Output()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return "", errors.New(tail(string(ee.Stderr), 25))
	case err != nil:
		return "", fmt.Errorf("cannot run nix (%v); is Nix installed?", err)
	}
	hash := strings.TrimSpace(string(out))
	if !strings.HasPrefix(hash, "sha256-") {
		return "", fmt.Errorf("unexpected output of nix hash path: %q", tail(hash, 1))
	}
	return "path:" + src + "?narHash=" + url.QueryEscape(hash), nil
}

// nixString quotes s as a Nix string: no escape sequence, no interpolation.
func nixString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`).Replace(s) + `"`
}

const maxEvalOutput = 64 << 20

// capped collects output up to max bytes, then fails the write.
type capped struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.max {
		c.over = true
		return 0, errors.New("output too large")
	}
	return c.buf.Write(p)
}

// tail keeps the last n lines of nix's error output, which hold the reason.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append([]string{"…"}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}
