package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const keepClosures = 256 // pairs of systems remembered; the oldest are forgotten first

// closureRetry is how long a failed comparison is not run again; a variable for tests.
var closureRetry = 10 * time.Minute

// closureEntry is a remembered comparison: its result, or when it failed.
type closureEntry struct {
	diff   *ClosureDiff
	failed time.Time
}

// ClosureDiff is what changes between two built systems.
type ClosureDiff struct {
	SizeFrom int64           `json:"sizeFrom"` // closure sizes in bytes
	SizeTo   int64           `json:"sizeTo"`
	Changes  []ClosureChange `json:"changes"`
}

// ClosureChange is one line of nix store diff-closures.
type ClosureChange struct {
	Name  string `json:"name"`
	From  string `json:"from,omitempty"`  // versions before; "" when absent
	To    string `json:"to,omitempty"`    // versions after; "" when absent
	Delta int64  `json:"delta,omitempty"` // bytes; Nix leaves out changes under 8 KiB
}

// AddClosures adds to each changed machine what changes between its two built
// systems, every dependency's version and the size, when this server's store
// has both (a check built them). A system that was not built here, or was
// collected since, leaves the machine without one.
func (js *Jobs) AddClosures(ctx context.Context, head, work *Model, ds []HostDiff) {
	paths := func(m *Model) map[string]string {
		out := map[string]string{}
		for _, h := range m.Hosts {
			if h.Error == "" && h.Detail != nil && h.Detail.SystemPath != "" && reportPath.MatchString(h.Detail.SystemPath) {
				out[h.Name] = h.Detail.SystemPath
			}
		}
		return out
	}
	before, after := paths(head), paths(work)
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4) // a few machines at once: a nixpkgs update changes all of them
	for i, d := range ds {
		from, to := before[d.Host], after[d.Host]
		if d.Status != "changed" || from == "" || to == "" || from == to || !storeHas(from) || !storeHas(to) {
			continue
		}
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			c, err := js.closureDiff(ctx, from, to)
			if err != nil {
				slog.Warn("cannot compare the built systems of a machine", "host", d.Host, "err", err)
			}
			ds[i].Closure = c // each goroutine writes only its own machine
		})
	}
	wg.Wait()
}

// closureDiff compares two systems in the store. Two store paths never change,
// so the result is kept; a failure is kept for closureRetry and then returns
// nil without an error, as it was logged when it happened.
func (js *Jobs) closureDiff(ctx context.Context, from, to string) (*ClosureDiff, error) {
	key := from + " " + to
	js.mu.Lock()
	e, ok := js.closures[key]
	js.mu.Unlock()
	if ok && (e.diff != nil || time.Since(e.failed) < closureRetry) {
		return e.diff, nil
	}
	c, err := js.compareSystems(ctx, from, to)
	if err != nil && ctx.Err() != nil { // the request went away: nothing to remember
		return nil, err
	}
	e = closureEntry{diff: c}
	if err != nil {
		e.failed = time.Now()
	}
	js.mu.Lock()
	if js.closures == nil {
		js.closures = map[string]closureEntry{}
	}
	if _, ok := js.closures[key]; !ok {
		js.closureOrder = append(js.closureOrder, key)
	}
	js.closures[key] = e
	for len(js.closureOrder) > keepClosures {
		delete(js.closures, js.closureOrder[0])
		js.closureOrder = js.closureOrder[1:]
	}
	js.mu.Unlock()
	return c, err
}

// compareSystems runs nix store diff-closures and nix path-info on two systems.
func (js *Jobs) compareSystems(ctx context.Context, from, to string) (*ClosureDiff, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	out, err := js.nixOutput(ctx, "store", "diff-closures", from, to)
	if err != nil {
		return nil, err
	}
	sizes, err := js.nixOutput(ctx, "path-info", "--closure-size", from, to)
	if err != nil {
		return nil, err
	}
	size := parseClosureSizes(sizes)
	c := &ClosureDiff{SizeFrom: size[from], SizeTo: size[to], Changes: parseDiffClosures(out)}
	if c.SizeFrom == 0 || c.SizeTo == 0 {
		return nil, fmt.Errorf("nix path-info printed no closure size: %.200q", sizes)
	}
	return c, nil
}

// nixOutput runs a quick nix command and returns what it printed.
func (js *Jobs) nixOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, js.Nix, js.nixArgs(args...)...)
	cmd.Env = append(cmd.Environ(), "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil, fmt.Errorf("cannot run nix (%v); is Nix installed?", err)
		}
		return nil, fmt.Errorf("nix %s: %s", strings.Join(args[:min(2, len(args))], " "), tail(stderr.String(), 5))
	}
	return stdout.Bytes(), nil
}

var (
	ansiEscape  = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	closureSize = regexp.MustCompile(`(?:^|, )([+-]?\d+(?:\.\d+)?) ([KMGTPE])iB$`)
)

// parseDiffClosures reads nix store diff-closures: "name: 1.0 → 1.1, 12.3 KiB",
// "name: -1.5 MiB" when only the size moved, ∅ for absent. Older Nix prints
// "+12.3 KiB", always in KiB. Store path names hold no spaces or colons, so
// the separators are unambiguous.
func parseDiffClosures(out []byte) []ClosureChange {
	changes := []ClosureChange{}
	for _, line := range strings.Split(ansiEscape.ReplaceAllString(string(out), ""), "\n") {
		name, rest, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok || strings.ContainsAny(name, " \t") {
			continue
		}
		c := ClosureChange{Name: name}
		if m := closureSize.FindStringSubmatchIndex(rest); m != nil {
			n, _ := strconv.ParseFloat(rest[m[2]:m[3]], 64)
			power := strings.Index("KMGTPE", rest[m[4]:m[5]]) + 1
			c.Delta = int64(math.Round(n * math.Pow(1024, float64(power))))
			rest = rest[:m[0]]
		}
		if rest != "" {
			from, to, ok := strings.Cut(rest, " → ")
			if !ok {
				continue
			}
			absent := func(v string) string {
				if v == "∅" {
					return ""
				}
				return v
			}
			c.From, c.To = absent(from), absent(to)
		}
		changes = append(changes, c)
	}
	return changes
}

// parseClosureSizes reads nix path-info --closure-size: a path and its size in
// bytes per line.
func parseClosureSizes(out []byte) map[string]int64 {
	sizes := map[string]int64{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || !strings.HasPrefix(f[0], "/") {
			continue
		}
		if n, err := strconv.ParseInt(f[len(f)-1], 10, 64); err == nil {
			sizes[f[0]] = n
		}
	}
	return sizes
}
