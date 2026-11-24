package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Installer ISOs: the NixOS installer for one machine, built from the commit
// the machines deploy, with fleet-install, which installs that machine from the
// repository (nix/iso.nix, nix/fleet-install.sh). Builds run in their own job
// lane, so they never hold up checks or the fleet views. The last keepISOs are
// kept as GC roots in Dir and served for download. Nothing secret goes into
// one: no age key, no token; the SSH host key the machine keeps is made when
// the ISO boots, in memory.

const (
	keepISOs = 3
	isoMeta  = "iso.json"
)

// ISO is an installer image: built (ready), or a build since the server started.
type ISO struct {
	ID       string    `json:"id"`
	Host     string    `json:"host"`
	Commit   string    `json:"commit"`
	Branch   string    `json:"branch"`
	State    string    `json:"state"`         // queued, running, failed, ready
	Job      string    `json:"job,omitempty"` // the build, while the server remembers it
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`
	File     string    `json:"file,omitempty"` // the download's name
	Size     int64     `json:"size,omitempty"`
	SHA256   string    `json:"sha256,omitempty"`
}

type ISOs struct {
	Dir  string // one folder per ISO: iso.json and result, the GC root
	Jobs *Jobs

	mu     sync.Mutex
	builds []*isoBuild // unfinished and failed builds, oldest first
}

type isoBuild struct {
	iso ISO
	job *Job
}

// OpenISOs keeps the ISOs in dir, removing the folders of builds a stop cut short.
func OpenISOs(dir string, jobs *Jobs) (*ISOs, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Name(), isoMeta)); errors.Is(err, os.ErrNotExist) {
			os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
	return &ISOs{Dir: dir, Jobs: jobs}, nil
}

var isoID = regexp.MustCompile(`^[a-z0-9_-]+-[0-9a-f]{7}-[0-9]+$`)

// isPlaceholder tells a hardware-configuration.nix the wizard or the starter flake wrote.
func isPlaceholder(hardware string) bool { return strings.HasPrefix(hardware, "# Placeholder") }

// Build queues the ISO of host at the commit a push lands on, as last fetched:
// what comin deploys, and what fleet-install installs when it cannot fetch the
// repository itself.
func (is *ISOs) Build(ctx context.Context, host string) (ISO, error) {
	if !machineName.MatchString(host) {
		return ISO{}, errorf(ErrInvalid, "invalid machine name %q", host)
	}
	r := is.Jobs.Repo
	r.mu.Lock()
	commit, branch := r.pushedCommit(ctx)
	var tree, hw string
	var hwErr, diskErr, pushedErr error
	_, _, localErr := r.Read("hosts/" + host + "/default.nix")
	if commit != "" {
		out, err := r.git(ctx, "rev-parse", "--verify", "-q", commit+"^{tree}")
		tree = strings.TrimSpace(out)
		if err != nil {
			r.mu.Unlock()
			return ISO{}, err
		}
		hw, hwErr = r.FileAt(ctx, commit, "hosts/"+host+"/hardware-configuration.nix")
		_, diskErr = r.FileAt(ctx, commit, "hosts/"+host+"/disk-config.nix")
		_, pushedErr = r.FileAt(ctx, commit, "hosts/"+host+"/default.nix")
	}
	r.mu.Unlock()
	switch {
	case commit == "":
		return ISO{}, errorf(ErrInvalid, "nothing is pushed yet: an ISO installs what the machines deploy, so commit and push first")
	case localErr == nil && pushedErr != nil: // a machine the wizard just made; other layouts fail in the build's log
		return ISO{}, errorf(ErrInvalid, "%s is not pushed yet: an ISO installs what the machines deploy, so commit and push hosts/%s/ first", host, host)
	case hwErr == nil && isPlaceholder(hw) && diskErr != nil:
		return ISO{}, errorf(ErrInvalid, "%s has no disk layout, and its hardware-configuration.nix is still the placeholder at %.7s: the installer would know neither its disks nor its boot loader. Give it its real hardware configuration and boot loader, or create it in New machine with a disk layout; commit and push, then build it again", host, commit)
	}

	is.mu.Lock()
	defer is.mu.Unlock()
	for _, b := range is.builds {
		if b.iso.Host == host && b.iso.Commit == commit && !b.job.finishedState() {
			return ISO{}, errorf(ErrConflict, "the ISO of %s at %.7s is already being built", host, commit)
		}
	}
	now := time.Now()
	b := &isoBuild{iso: ISO{ID: fmt.Sprintf("%s-%.7s-%d", host, commit, now.UnixMilli()), Host: host, Commit: commit, Branch: branch, Started: now,
		File: fmt.Sprintf("nixos-%s-%.7s.iso", host, commit)}}
	iso := b.iso // the build's own copy: b changes under is.mu
	is.Jobs.mu.Lock()
	job, err := is.Jobs.enqueueDo(JobRequest{Kind: "iso", Host: host}, tree, "", func(ctx context.Context, job *Job) error { return is.run(ctx, job, iso) })
	is.Jobs.mu.Unlock()
	if err != nil {
		return ISO{}, err
	}
	b.job, b.iso.Job = job, job.ID
	is.builds = append(is.builds, b)
	// The failed builds shown are the last few; the jobs keep only so many logs anyway.
	for len(is.builds) > 10 && is.builds[0].job.finishedState() {
		is.builds = is.builds[1:]
	}
	return is.info(b), nil
}

// pushedCommit is the commit a push lands on, as last fetched (see pushBase),
// and its branch; "" when nothing is pushed. Needs r.mu.
func (r *Repo) pushedCommit(ctx context.Context) (sha, branch string) {
	refs := []string{"refs/remotes/origin/HEAD"}
	if cur, err := r.git(ctx, "symbolic-ref", "--short", "-q", "HEAD"); err == nil {
		refs = append([]string{"refs/remotes/origin/" + strings.TrimSpace(cur)}, refs...)
	}
	for _, ref := range refs {
		if out, err := r.git(ctx, "rev-parse", "--verify", "-q", ref+"^{commit}"); err == nil {
			name, _ := r.git(ctx, "rev-parse", "--abbrev-ref", ref)
			return strings.TrimSpace(out), strings.TrimPrefix(strings.TrimSpace(name), "origin/")
		}
	}
	return "", ""
}

func (is *ISOs) info(b *isoBuild) ISO {
	out := b.iso
	if b.job != nil {
		out.State = b.job.Info(false).State
	}
	return out
}

// isoCall is the Nix expression of host's installer ISO, from the locked flake ref
// of commit; fleet-install commits as gitName <gitEmail>.
func isoCall(ref, host, commit, gitName, gitEmail string) string {
	return "(" + nixExpr("iso.nix") + "\n) { flakeRef = " + nixString(ref) + "; host = " + nixString(host) + "; commit = " + nixString(commit) +
		"; installer = " + nixString(nixExpr("fleet-install.sh")) + "; gitName = " + nixString(gitName) + "; gitEmail = " + nixString(gitEmail) + "; }"
}

// run builds the ISO into its folder, then records its size and SHA-256 and
// keeps only the newest keepISOs. A failed build leaves no folder.
func (is *ISOs) run(ctx context.Context, job *Job, iso ISO) (err error) {
	fmt.Fprintf(job, "Building the installer ISO of %s at commit %.12s (%s).\n", iso.Host, iso.Commit, iso.Branch)
	src, cleanup, err := is.Jobs.snapshot(ctx, job.tree)
	if err != nil {
		return err
	}
	defer cleanup()
	ref, err := is.Jobs.lockedFlake(ctx, src)
	if err != nil {
		return err
	}
	dir := filepath.Join(is.Dir, iso.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(dir)
		}
	}()
	result := filepath.Join(dir, "result")
	r := is.Jobs.Repo
	call := isoCall(ref, iso.Host, iso.Commit, r.AuthorName, r.AuthorEmail)
	fmt.Fprintf(job, "$ nix build -L --out-link %s --expr <nix/iso.nix>\n", result)
	cmd := is.Jobs.nixCmd(ctx, is.Jobs.nixArgs("build", "-L", "--out-link", result, "--expr", call)...)
	cmd.Dir = src
	cmd.Stdout, cmd.Stderr = job, job
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("stopped: the build ran out of time")
		}
		return fmt.Errorf("failed: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(result, "iso", "*.iso"))
	if len(files) != 1 {
		return fmt.Errorf("the build has %d ISO files in iso/, not one", len(files))
	}
	f, err := os.Open(files[0])
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if iso.Size, err = io.Copy(h, f); err != nil {
		return err
	}
	iso.SHA256, iso.State, iso.Job, iso.Finished = hex.EncodeToString(h.Sum(nil)), "ready", "", time.Now()
	b, _ := json.Marshal(iso) // plain fields always marshal
	if err := writeAtomic(filepath.Join(dir, isoMeta), b); err != nil {
		return err
	}
	fmt.Fprintf(job, "\n%s: %d bytes, SHA-256 %s.\n", iso.File, iso.Size, iso.SHA256)
	is.mu.Lock()
	is.builds = slices.DeleteFunc(is.builds, func(b *isoBuild) bool { return b.iso.ID == iso.ID })
	is.mu.Unlock()
	is.prune()
	return nil
}

// ready lists the built ISOs, newest first.
func (is *ISOs) ready() ([]ISO, error) {
	entries, err := os.ReadDir(is.Dir)
	if err != nil {
		return nil, err
	}
	out := []ISO{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(is.Dir, e.Name(), isoMeta))
		if err != nil {
			continue // a build under way
		}
		var iso ISO
		if err := json.Unmarshal(b, &iso); err != nil || iso.ID != e.Name() {
			slog.Warn("unreadable installer ISO record", "dir", e.Name(), "err", err)
			continue
		}
		out = append(out, iso)
	}
	slices.SortFunc(out, func(a, b ISO) int { return b.Finished.Compare(a.Finished) })
	return out, nil
}

// prune drops all but the newest keepISOs: their GC roots go, and Nix's
// garbage collector frees the space. Only the iso lane calls it.
func (is *ISOs) prune() {
	isos, err := is.ready()
	if err != nil || len(isos) <= keepISOs {
		return
	}
	for _, iso := range isos[keepISOs:] {
		os.RemoveAll(filepath.Join(is.Dir, iso.ID))
	}
}

// List gives the builds under way or failed, then the built ISOs, newest first.
func (is *ISOs) List() ([]ISO, error) {
	out := []ISO{}
	is.mu.Lock()
	for i := len(is.builds) - 1; i >= 0; i-- {
		out = append(out, is.info(is.builds[i]))
	}
	is.mu.Unlock()
	ready, err := is.ready()
	return append(out, ready...), err
}

// File is the image of a built ISO, for download.
func (is *ISOs) File(id string) (string, ISO, error) {
	if !isoID.MatchString(id) {
		return "", ISO{}, errorf(ErrNotFound, "no such ISO")
	}
	b, err := os.ReadFile(filepath.Join(is.Dir, id, isoMeta))
	if err != nil {
		return "", ISO{}, errorf(ErrNotFound, "no such ISO: only the last %d are kept", keepISOs)
	}
	var iso ISO
	if err := json.Unmarshal(b, &iso); err != nil {
		return "", ISO{}, err
	}
	files, _ := filepath.Glob(filepath.Join(is.Dir, id, "result", "iso", "*.iso"))
	if len(files) != 1 {
		return "", ISO{}, errorf(ErrNotFound, "the ISO file is gone")
	}
	return files[0], iso, nil
}
