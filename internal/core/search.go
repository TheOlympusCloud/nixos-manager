package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"
)

// PackageResult is one nixpkgs package from the package index.
type PackageResult struct {
	Attr        string `json:"attr"`
	Pname       string `json:"pname"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// OptionDoc is one documented NixOS option of a machine.
type OptionDoc struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description *string `json:"description"`
	Default     *string `json:"default"`
	Example     *string `json:"example"`
	ReadOnly    bool    `json:"readOnly"`
}

// nixpkgsRef is the exact nixpkgs the flake locks, so package search matches
// what the machines get. Without a lock it falls back to the registry.
func nixpkgsRef(lock []byte) string {
	var l flakeLock
	if json.Unmarshal(lock, &l) != nil {
		return "nixpkgs"
	}
	var node string
	if json.Unmarshal(l.Nodes[l.Root].Inputs["nixpkgs"], &node) != nil {
		return "nixpkgs"
	}
	in := l.Nodes[node].Locked
	switch in.Type {
	case "github", "gitlab", "sourcehut":
		return fmt.Sprintf("%s:%s/%s/%s", in.Type, in.Owner, in.Repo, in.Rev)
	case "git":
		return "git+" + in.URL + "?rev=" + in.Rev
	case "tarball", "file":
		return in.URL
	case "path":
		return "path:" + in.Path
	}
	return "nixpkgs"
}

// score ranks a match: exact name, name prefix, name contains, description only.
// Every word of the query must appear in the name or the description.
func score(name, desc, q string) int {
	name, desc = strings.ToLower(name), strings.ToLower(desc)
	for _, w := range strings.Fields(q) {
		if !strings.Contains(name, w) && !strings.Contains(desc, w) {
			return -1
		}
	}
	switch {
	case name == q:
		return 0
	case strings.HasPrefix(name, q):
		return 1
	case strings.Contains(name, q):
		return 2
	}
	return 3
}

func rankPackages(idx []PackageResult, q string, limit int) []PackageResult {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return []PackageResult{}
	}
	type hit struct {
		p PackageResult
		s int
	}
	var hits []hit
	for _, p := range idx {
		s := score(p.Attr, p.Description, q)
		if ps := score(p.Pname, p.Description, q); ps >= 0 && (s < 0 || ps < s) {
			s = ps
		}
		if s >= 0 {
			hits = append(hits, hit{p, s})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int {
		if a.s != b.s {
			return a.s - b.s
		}
		if len(a.p.Attr) != len(b.p.Attr) {
			return len(a.p.Attr) - len(b.p.Attr)
		}
		return strings.Compare(a.p.Attr, b.p.Attr)
	})
	out := []PackageResult{}
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].p)
	}
	return out
}

// rankOptions ranks like packages, then puts .enable first and shallow options
// before deep ones, which is what browsing a prefix such as services.openssh needs.
func rankOptions(idx []OptionDoc, q string, limit int) []OptionDoc {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return []OptionDoc{}
	}
	type hit struct {
		o OptionDoc
		s int
	}
	var hits []hit
	for _, o := range idx {
		desc := ""
		if o.Description != nil {
			desc = *o.Description
		}
		if s := score(o.Name, desc, q); s >= 0 {
			hits = append(hits, hit{o, s})
		}
	}
	enable := func(o OptionDoc) int {
		if strings.HasSuffix(o.Name, ".enable") {
			return 0
		}
		return 1
	}
	slices.SortFunc(hits, func(a, b hit) int {
		if a.s != b.s {
			return a.s - b.s
		}
		if d := enable(a.o) - enable(b.o); d != 0 {
			return d
		}
		if d := strings.Count(a.o.Name, ".") - strings.Count(b.o.Name, "."); d != 0 {
			return d
		}
		return strings.Compare(a.o.Name, b.o.Name)
	})
	out := []OptionDoc{}
	for i := 0; i < len(hits) && i < limit; i++ {
		out = append(out, hits[i].o)
	}
	return out
}

func (js *Jobs) lockFile() []byte {
	b, _ := js.Repo.root.ReadFile("flake.lock")
	return b
}

// indexKey names an index. Options also depend on the repo's own modules, so
// their key follows the last commit as well as flake.lock.
func (js *Jobs) indexKey(ctx context.Context, kind, host string) string {
	lock := js.lockFile()
	if kind == "packages" {
		return "packages " + nixpkgsRef(lock)
	}
	h := sha256.Sum256(lock)
	head, _ := js.Repo.headTree(ctx)
	return "options " + host + " " + hex.EncodeToString(h[:8]) + " " + head
}

// index returns a ready index, or the job building it, or the error that
// stopped the last build (until RefreshIndex).
func (js *Jobs) index(ctx context.Context, kind, host string) (any, *Job, error) {
	if kind == "options" && !validTarget(host) {
		return nil, nil, errorf(ErrInvalid, "invalid machine name %q", host)
	}
	key := js.indexKey(ctx, kind, host)
	js.mu.Lock()
	defer js.mu.Unlock()
	if idx, ok := js.indexes[key]; ok {
		return idx, nil, nil
	}
	if err, ok := js.indexErrs[key]; ok {
		return nil, nil, err
	}
	for _, j := range js.jobs {
		if j.key == key && !j.finishedState() {
			return nil, j, nil
		}
	}
	return js.startIndex(ctx, kind, host, key)
}

// startIndex queues an index build; js.mu must be held.
func (js *Jobs) startIndex(ctx context.Context, kind, host, key string) (any, *Job, error) {
	tree := ""
	if kind == "options" {
		js.mu.Unlock() // WorkingTree takes the repo lock; never hold both
		var err error
		tree, err = js.Repo.WorkingTree(ctx)
		js.mu.Lock()
		if err != nil {
			return nil, nil, err
		}
		for _, j := range js.jobs { // another request may have started it while unlocked
			if j.key == key && !j.finishedState() {
				return nil, j, nil
			}
		}
	}
	j, err := js.enqueue(JobRequest{Kind: kind, Host: host}, tree, key)
	return nil, j, err
}

// SearchPackages searches the locked nixpkgs. While the index is being built
// (minutes the first time) it returns the job instead.
func (js *Jobs) SearchPackages(ctx context.Context, q string, limit int) ([]PackageResult, *Job, error) {
	idx, job, err := js.index(ctx, "packages", "")
	if idx == nil {
		return nil, job, err
	}
	return rankPackages(idx.([]PackageResult), q, clampLimit(limit)), nil, nil
}

// SearchOptions searches the documented options of one machine.
func (js *Jobs) SearchOptions(ctx context.Context, host, q string, limit int) ([]OptionDoc, *Job, error) {
	idx, job, err := js.index(ctx, "options", host)
	if idx == nil {
		return nil, job, err
	}
	return rankOptions(idx.([]OptionDoc), q, clampLimit(limit)), nil, nil
}

// Option returns the option named exactly name, if the machine has it.
func (js *Jobs) Option(ctx context.Context, host, name string) (*OptionDoc, *Job, error) {
	idx, job, err := js.index(ctx, "options", host)
	if idx == nil {
		return nil, job, err
	}
	for _, o := range idx.([]OptionDoc) {
		if o.Name == name {
			return &o, nil, nil
		}
	}
	return nil, nil, errorf(ErrNotFound, "%s has no option %s", host, name)
}

// RefreshIndex forgets an index (or its last error) and builds it again.
func (js *Jobs) RefreshIndex(ctx context.Context, kind, host string) (*Job, error) {
	if kind != "packages" && kind != "options" {
		return nil, errorf(ErrInvalid, "unknown index %q (want packages or options)", kind)
	}
	if kind == "options" && !validTarget(host) {
		return nil, errorf(ErrInvalid, "invalid machine name %q", host)
	}
	key := js.indexKey(ctx, kind, host)
	js.mu.Lock()
	defer js.mu.Unlock()
	delete(js.indexes, key)
	delete(js.indexErrs, key)
	_, j, err := js.startIndex(ctx, kind, host, key)
	return j, err
}

func clampLimit(n int) int {
	if n <= 0 || n > 200 {
		return 50
	}
	return n
}

func (js *Jobs) runIndex(ctx context.Context, job *Job) (err error) {
	defer func() {
		js.mu.Lock()
		// Every lock change or commit asks for a new index: keep only the newest
		// of this kind (and machine), as each can take tens of megabytes.
		family := job.Req.Kind + " "
		if job.Req.Kind == "options" {
			family += job.Req.Host + " "
		}
		older := func(k string) bool { return k != job.key && strings.HasPrefix(k, family) }
		maps.DeleteFunc(js.indexes, func(k string, _ any) bool { return older(k) })
		maps.DeleteFunc(js.indexErrs, func(k string, _ error) bool { return older(k) })
		if err != nil {
			js.indexErrs[job.key] = fmt.Errorf("building the %s index failed: %w", job.Req.Kind, err)
		}
		js.mu.Unlock()
	}()
	switch job.Req.Kind {
	case "packages":
		ref := nixpkgsRef(js.lockFile())
		fmt.Fprintf(job, "Indexing the packages of %s; the first time this takes a few minutes.\n", ref)
		cmd := js.nixCmd(ctx, js.nixArgs("search", "--json", ref, "^")...)
		var stdout bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, job
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				return fmt.Errorf("cannot run nix (%v); is Nix installed?", err)
			}
			return err
		}
		var raw map[string]PackageResult
		if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
			return fmt.Errorf("unexpected nix search output: %v", err)
		}
		idx := make([]PackageResult, 0, len(raw))
		for key, p := range raw {
			// legacyPackages.<system>.<attr> or packages.<system>.<attr>
			if parts := strings.SplitN(key, ".", 3); len(parts) == 3 {
				p.Attr = parts[2]
			} else {
				p.Attr = key
			}
			idx = append(idx, p)
		}
		fmt.Fprintf(job, "Indexed %d packages.\n", len(idx))
		js.mu.Lock()
		js.indexes[job.key] = idx
		js.mu.Unlock()
	case "options":
		fmt.Fprintf(job, "Indexing the NixOS options of %s.\n", job.Req.Host)
		src, cleanup, err := js.snapshot(ctx, job.tree)
		if err != nil {
			return err
		}
		defer cleanup()
		var idx []OptionDoc
		if err := js.evalJSON(ctx, src, map[string]string{"host": job.Req.Host}, nixExpr("options.nix"), &idx); err != nil {
			return err
		}
		fmt.Fprintf(job, "Indexed %d options.\n", len(idx))
		js.mu.Lock()
		js.indexes[job.key] = idx
		js.mu.Unlock()
	}
	return nil
}
