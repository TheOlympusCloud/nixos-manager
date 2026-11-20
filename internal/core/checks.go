package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// CheckPass is a check that passed: the staged tree, the tree that passed a
// check its machines were compared with, and the machines it built.
type CheckPass struct {
	Tree  string   `json:"tree"`
	Base  string   `json:"base"`  // "" when no pushed commit had passed a check: every machine was built
	Built []string `json:"built"` // every machine whose system differs from Base
}

// CheckSummary is what a check verified, for the UI and MCP clients. It is
// kept apart from the log, which the output cap can cut short.
type CheckSummary struct {
	Base    string   `json:"base"`
	Changed []string `json:"changed"`          // machines whose system differs from Base: each one is built
	Built   []string `json:"built"`            // those that built
	Failed  []string `json:"failed,omitempty"` // those that did not build or evaluate
	Reboot  []string `json:"reboot,omitempty"` // changed machines that apply the change only after a reboot
	// From a dry run before the builds: what Nix had to build and to download.
	ToBuild  int    `json:"toBuild"`
	ToFetch  int    `json:"toFetch"`
	Download string `json:"download,omitempty"` // as Nix printed it, e.g. "12.34 MiB"
	Unpacked string `json:"unpacked,omitempty"`
}

// runCheck checks the staged tree: nix flake check, then a real build of every
// machine whose system differs from the tree a push lands on (by system
// derivation, as Changes compares), then a dry build of each extra target.
func (js *Jobs) runCheck(ctx context.Context, job *Job) error {
	tree, base := job.tree, job.Req.Base
	var sum *CheckSummary // once the machines are compared
	defer func() {
		job.mu.Lock()
		job.summary = sum
		job.mu.Unlock()
	}()
	// The other lane evaluates both fleets while nix flake check runs.
	for _, t := range []string{tree, base} {
		if err := js.prepareModel(t); err != nil {
			return err
		}
	}
	tmp, err := os.MkdirTemp("", "nixmgr-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "src")
	if err := js.Repo.export(ctx, tree, src); err != nil {
		return err
	}
	if c := job.Req.Commit; c != "" {
		on := ""
		if job.Req.Branch != "" {
			on = ", pushed to " + job.Req.Branch
		}
		fmt.Fprintf(job, "Checking commit %.12s (tree %.12s)%s.\n", c, tree, on)
	} else {
		fmt.Fprintf(job, "Checking the staged changes (tree %.12s).\n", tree)
	}
	if err := js.step(ctx, job, src, js.nixArgs("flake", "check", "-L", "--keep-going", "--no-update-lock-file", "path:"+src)); err != nil {
		return err
	}

	switch {
	case base == "" && job.Req.Commit != "":
		fmt.Fprintln(job, "\nNo commit before it passed a check, so every machine is built.")
	case base == "":
		fmt.Fprintln(job, "\nNo pushed commit has passed a check, so every machine is built.")
	case job.Req.Commit != "":
		fmt.Fprintf(job, "\nComparing every machine with the newest commit before it that passed a check (tree %.12s).\n", base)
	default:
		fmt.Fprintf(job, "\nComparing every machine with the newest pushed commit that passed a check (tree %.12s).\n", base)
	}
	after, err := js.waitModel(ctx, tree)
	if err == nil && after.Error != "" {
		err = fmt.Errorf("the staged flake does not evaluate:\n%s", after.Error)
	}
	if err != nil {
		return err
	}
	before, err := js.waitModel(ctx, base)
	if err != nil {
		return err
	}
	if before.Error != "" {
		fmt.Fprintln(job, "What it is compared with does not evaluate, so every machine is built.")
	}
	sum = &CheckSummary{Base: base, Changed: []string{}, Built: []string{}}
	hosts := map[string]Host{}
	for _, h := range after.Hosts {
		hosts[h.Name] = h
	}
	var build []Host
	for _, d := range DiffModels(before, after) {
		switch d.Status {
		case "error":
			sum.Failed = append(sum.Failed, d.Host)
			fmt.Fprintf(job, "%s does not evaluate:\n%s\n", d.Host, d.Error)
		case "added", "changed":
			sum.Changed = append(sum.Changed, d.Host)
			build = append(build, hosts[d.Host])
			if d.Reboot {
				sum.Reboot = append(sum.Reboot, d.Host)
			}
		}
	}
	if len(sum.Failed) > 0 {
		return fmt.Errorf("%s: %s", plural(len(sum.Failed), "machine does not evaluate", "machines do not evaluate"), strings.Join(sum.Failed, ", "))
	}

	if len(build) == 0 {
		fmt.Fprintln(job, "No machine's system changes: nothing to build.")
	} else {
		if err := js.buildSystems(ctx, job, src, build, sum); err != nil {
			return err
		}
	}

	for _, t := range job.Req.Targets {
		if slices.ContainsFunc(build, func(h Host) bool { return t == systemAttr(h.Name) }) {
			fmt.Fprintf(job, "\nSkipping the dry build of %s: it was just built.\n", t)
			continue
		}
		if err := js.step(ctx, job, src, js.nixArgs("build", "--dry-run", "--no-link", "-L", "--no-update-lock-file", "path:"+src+"#"+t)); err != nil {
			return err
		}
	}
	js.pruneRoots(after.Hosts)
	fmt.Fprintln(job, "\nAll steps passed.")
	return nil
}

// buildSystems builds each machine's system, after a dry run that tells what
// Nix builds and downloads. A failed machine does not stop the others.
func (js *Jobs) buildSystems(ctx context.Context, job *Job, src string, build []Host, sum *CheckSummary) error {
	names := make([]string, len(build))
	installables := make([]string, len(build))
	for i, h := range build {
		names[i], installables[i] = h.Name, systemInstallable(src, h)
	}
	fmt.Fprintf(job, "\nBuilding the system of %s: %s.\n", plural(len(build), "changed machine", "changed machines"), strings.Join(names, ", "))
	args := js.nixArgs(append([]string{"build", "--dry-run", "--no-link", "--no-update-lock-file"}, installables...)...)
	fmt.Fprintf(job, "$ nix %s\n", strings.Join(args[2:], " "))
	cmd := js.nixCmd(ctx, args...)
	cmd.Dir = src
	out := capped{max: maxOutput}
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		sum.Failed = names
		job.Write([]byte(tail(out.buf.String(), 40) + "\n"))
		if ctx.Err() != nil {
			return fmt.Errorf("stopped: the job ran out of time")
		}
		return fmt.Errorf("failed: %v", err)
	}
	sum.ToBuild, sum.ToFetch, sum.Download, sum.Unpacked = parseDryRun(out.buf.Bytes())
	fmt.Fprintln(job, describeDryRun(sum))

	if js.GCRoots != "" {
		if err := os.MkdirAll(js.GCRoots, 0o700); err != nil {
			return err
		}
	}
	for i, h := range build {
		args := js.nixArgs("build", "--keep-going", "--no-update-lock-file")
		if js.GCRoots != "" {
			args = append(args, "--out-link", filepath.Join(js.GCRoots, rootName(h.Name)))
		} else {
			args = append(args, "--no-link")
		}
		fmt.Fprintf(job, "\nBuilding %s…\n", h.Name)
		if err := js.step(ctx, job, src, append(args, installables[i])); err != nil {
			if ctx.Err() != nil {
				return err
			}
			sum.Failed = append(sum.Failed, h.Name)
			fmt.Fprintf(job, "%s does not build.\n", h.Name)
			if local := LocalSystem(); h.System != "" && h.System != local {
				fmt.Fprintf(job, "%s is %s and this server %s: if the error says that system is not available, give the server emulation or a remote builder for it (README, \"Machines of another architecture\").\n", h.Name, h.System, local)
			}
			continue
		}
		sum.Built = append(sum.Built, h.Name)
	}
	if len(sum.Failed) > 0 {
		return fmt.Errorf("%s: %s", plural(len(sum.Failed), "machine does not build", "machines do not build"), strings.Join(sum.Failed, ", "))
	}
	return nil
}

func systemAttr(host string) string {
	return "nixosConfigurations." + attrName(host) + ".config.system.build.toplevel"
}

// storeHas reports whether a store path is present; a variable for tests.
var storeHas = func(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// systemInstallable is what builds a machine's system: the derivation its
// evaluation wrote, which needs no second evaluation, or the flake attribute
// when the garbage collector has removed that file since.
func systemInstallable(src string, h Host) string {
	if strings.HasPrefix(h.Toplevel, "/") && strings.HasSuffix(h.Toplevel, ".drv") && storeHas(h.Toplevel) {
		return h.Toplevel + "^*"
	}
	return "path:" + src + "#" + systemAttr(h.Name)
}

var (
	dryBuild = regexp.MustCompile(`(?m)^(?:these (\d+) derivations|this derivation) will be built`)
	dryFetch = regexp.MustCompile(`(?m)^(?:these (\d+) paths|this path) will be fetched \(([^,]+) download, ([^)]+) unpacked\)`)
)

// parseDryRun reads what nix build --dry-run says it would build and fetch.
func parseDryRun(out []byte) (toBuild, toFetch int, download, unpacked string) {
	count := func(m [][]byte) int {
		if len(m[1]) == 0 { // "this derivation", "this path"
			return 1
		}
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}
	if m := dryBuild.FindSubmatch(out); m != nil {
		toBuild = count(m)
	}
	if m := dryFetch.FindSubmatch(out); m != nil {
		toFetch, download, unpacked = count(m), string(bytes.TrimSpace(m[2])), string(bytes.TrimSpace(m[3]))
	}
	return toBuild, toFetch, download, unpacked
}

func describeDryRun(s *CheckSummary) string {
	if s.ToBuild == 0 && s.ToFetch == 0 {
		return "Everything is already in this server's store."
	}
	parts := []string{}
	if s.ToBuild > 0 {
		parts = append(parts, "builds "+plural(s.ToBuild, "derivation", "derivations"))
	}
	if s.ToFetch > 0 {
		parts = append(parts, fmt.Sprintf("downloads %s (%s, %s unpacked)", plural(s.ToFetch, "path", "paths"), s.Download, s.Unpacked))
	}
	return "Nix " + strings.Join(parts, " and ") + "."
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// LocalSystem is this server's Nix system, e.g. x86_64-linux.
func LocalSystem() string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
	if arch == "" {
		arch = runtime.GOARCH
	}
	return arch + "-" + runtime.GOOS
}

var unsafeRootChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// rootName is a machine's GC root file name: its name, with anything that is
// not a plain file name character (a dot or slash) replaced.
func rootName(host string) string { return unsafeRootChars.ReplaceAllString(host, "_") }

// pruneRoots drops the GC roots of machines the flake no longer has.
func (js *Jobs) pruneRoots(hosts []Host) {
	if js.GCRoots == "" {
		return
	}
	keep := map[string]bool{}
	for _, h := range hosts {
		keep[rootName(h.Name)] = true
	}
	entries, _ := os.ReadDir(js.GCRoots)
	for _, e := range entries {
		if !keep[e.Name()] {
			os.Remove(filepath.Join(js.GCRoots, e.Name()))
		}
	}
}
