package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// fleetService serves the fixture fleet through scripts/dev/fake-nix.
func fleetService(t *testing.T, fixtures string) *Service {
	t.Helper()
	s := NewService(newRepo(t))
	nix, _ := filepath.Abs("../../scripts/dev/fake-nix")
	dir, _ := filepath.Abs(fixtures)
	s.Jobs.Nix = nix
	t.Setenv("NIXMGR_FAKE_FIXTURES", dir)
	lock, err := os.ReadFile("testdata/fleet/flake.lock")
	if err != nil {
		t.Fatal(err)
	}
	write(t, s.Repo, "flake.nix", "{ }\n")
	write(t, s.Repo, "flake.lock", string(lock))
	return s
}

func readyModel(t *testing.T, s *Service, at string) *Model {
	t.Helper()
	m, j, err := s.Jobs.Model(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		wait(t, j)
		if m, _, err = s.Jobs.Model(context.Background(), at); err != nil || m == nil {
			t.Fatalf("model not ready after its job: %v", err)
		}
	}
	return m
}

func TestModelOfAFleet(t *testing.T) {
	s := fleetService(t, "testdata/fleet")
	m := readyModel(t, s, "working")

	var got []string
	for _, h := range m.Hosts {
		got = append(got, h.Name)
	}
	if strings.Join(got, ",") != "homeserver,oldbox,pi,workstation" {
		t.Fatalf("hosts %v", got)
	}
	old := m.Hosts[1]
	if !strings.Contains(old.Error, "infinite recursion") || old.Packages == nil || old.Features == nil {
		t.Fatalf("a failing machine must carry its error and empty lists: %+v", old)
	}
	if hs := m.Hosts[0].Secrets; len(hs) != 2 || hs[1] != (HostSecret{"wifi", "/run/secrets/wifi", "secrets/common.json", "root", "root", "0400"}) {
		t.Fatalf("homeserver's secrets %+v", hs)
	}
	ws := m.Hosts[3]
	if ws.Secrets != nil {
		t.Fatalf("workstation has no sops-nix, yet secrets %+v", ws.Secrets)
	}
	if ws.ManagedFile == nil || *ws.ManagedFile != "hosts/workstation/managed.json" || len(ws.Packages) < 20 {
		t.Fatalf("workstation %+v", ws)
	}
	for i := 1; i < len(ws.Packages); i++ {
		if ws.Packages[i-1].Name > ws.Packages[i].Name {
			t.Fatalf("packages not sorted: %s before %s", ws.Packages[i-1].Name, ws.Packages[i].Name)
		}
	}
	if u := ws.Users[0]; u.Name != "pablo" || u.Home == nil || len(u.Home.Programs) == 0 {
		t.Fatalf("home-manager user %+v", u)
	}
	if m.Hosts[2].ManagedFile != nil {
		t.Fatal("pi does not load a managed file")
	}
	if !strings.HasSuffix(ws.Toplevel, "-nixos-system-workstation-26.05.drv") || ws.Boot == "" || m.Hosts[0].Boot != "" {
		t.Fatalf("system derivation and boot: %q %q, homeserver (a container) %q", ws.Toplevel, ws.Boot, m.Hosts[0].Boot)
	}
	// The diff's detail is decoded but left out of the model every view polls.
	if d := ws.Detail; d == nil || d.SystemPath != "/nix/store/9fw2-nixos-system-workstation-26.05" ||
		d.Units["dhcpcd.service"].Flags["Service/X-RestartIfChanged"] != "false" || d.Etc["hosts"] != "9d2e" {
		t.Fatalf("workstation detail %+v", ws.Detail)
	}
	if d := m.Hosts[2].Detail; d == nil || d.Units != nil || d.Etc != nil {
		t.Fatalf("pi, evaluated without units or /etc: unknown, not empty: %+v", d)
	}
	if b, _ := json.Marshal(ws); bytes.Contains(b, []byte("dhcpcd")) || bytes.Contains(b, []byte("systemPath")) {
		t.Fatalf("the model's JSON carries the detail: %s", b)
	}
	if ws.ManagedPackages["vscode"] != "vscode" || m.Hosts[2].ManagedPackages == nil {
		t.Fatalf("managed package names %+v / %+v", ws.ManagedPackages, m.Hosts[2].ManagedPackages)
	}
	if len(m.Homes) != 1 || m.Homes[0].Name != "pablo@laptop" {
		t.Fatalf("homes %+v", m.Homes)
	}
	if len(m.Inputs) != 3 || m.Inputs[2].Name != "nixpkgs" || m.Inputs[2].URL != "github:NixOS/nixpkgs/nixos-26.05" {
		t.Fatalf("inputs %+v", m.Inputs)
	}
	tree := must[string](t)(s.Repo.WorkingTree(context.Background()))
	if m.Tree != tree {
		t.Fatalf("model tree %s, working tree %s", m.Tree, tree)
	}
}

func TestModelIsCachedPerTree(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	m1 := readyModel(t, s, "working")
	m2, j, err := s.Jobs.Model(ctx, "working")
	if err != nil || m2 != m1 || j != nil {
		t.Fatalf("second call: %p %v %v, want the cached %p", m2, j, err, m1)
	}
	staged := must[Status](t)(s.Status(ctx)).StagedTree
	write(t, s.Repo, "hosts/new.nix", "{ }\n") // untracked files count for the working copy
	m3, j, _ := s.Jobs.Model(ctx, "working")
	if m3 != nil || j == nil {
		t.Fatal("an edit did not start a new evaluation")
	}
	// The same job, or its model when the fake Nix already finished it; never a second job.
	if again, j2, _ := s.Jobs.Model(ctx, "working"); j2 != j && (j2 != nil || again == nil) {
		t.Fatal("a second request started a duplicate evaluation")
	}
	wait(t, j)
	if tree := readyModel(t, s, "working").Tree; tree == m1.Tree || tree == staged {
		t.Fatal("working tree ignores the untracked file")
	}
	head := readyModel(t, s, "head") // no commits yet: an empty fleet, no Nix needed
	if head.Tree != "" || len(head.Hosts) != 0 {
		t.Fatalf("head model of an empty repo %+v", head)
	}
	isErr(t, func() error { _, _, err := s.Jobs.Model(ctx, "yesterday"); return err }(), ErrInvalid)
}

func TestModelOfABrokenFlake(t *testing.T) {
	s := fleetService(t, t.TempDir()) // no fixtures: discovery fails like a flake with a syntax error
	m := readyModel(t, s, "working")
	if m.Error == "" || len(m.Hosts) != 0 {
		t.Fatalf("broken flake model %+v", m)
	}
	if again, j, _ := s.Jobs.Model(context.Background(), "working"); again != m || j != nil {
		t.Fatal("a broken flake is evaluated again on every request")
	}
	// After a transient failure (no network, say), the user can ask again.
	t.Setenv("NIXMGR_FAKE_FIXTURES", must[string](t)(filepath.Abs("testdata/fleet")))
	j := must[*Job](t)(s.Jobs.RefreshModel(context.Background()))
	wait(t, j)
	if fixed := readyModel(t, s, "working"); fixed.Error != "" || len(fixed.Hosts) != 4 {
		t.Fatalf("model after a refresh %+v", fixed)
	}
}

// Quick edits must not pile up full fleet evaluations: a newer request skips
// older ones still waiting in the queue.
func TestNewerModelSkipsQueuedOlderOnes(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	t.Setenv("NIXMGR_FAKE_DELAY", "0.2")
	_, running, _ := s.Jobs.Model(ctx, "working")
	for running.Info(false).State == "queued" {
		time.Sleep(5 * time.Millisecond)
	}
	write(t, s.Repo, "a.nix", "1\n")
	_, older, _ := s.Jobs.Model(ctx, "working")
	write(t, s.Repo, "a.nix", "2\n")
	_, newer, _ := s.Jobs.Model(ctx, "working")
	if older == newer {
		t.Fatal("different trees share a job")
	}
	if info := wait(t, older); info.State != "failed" || !strings.Contains(info.Output, "changed again") {
		t.Fatalf("the outdated evaluation ran: %+v", info)
	}
	if info := wait(t, newer); info.State != "passed" {
		t.Fatalf("latest evaluation: %+v", info)
	}
	if m, j, _ := s.Jobs.Model(ctx, "working"); m == nil || j != nil {
		t.Fatal("latest model not cached")
	}
}

// History asking for the commit the working copy matches shares its queued job;
// a later edit must not skip that job, or History shows a failure for the commit.
func TestEditsDoNotSkipAnEvaluationHistoryWaitsOn(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	t.Setenv("NIXMGR_FAKE_DELAY", "0.2")
	_, running, _ := s.Jobs.Model(ctx, "working")
	for running.Info(false).State == "queued" {
		time.Sleep(5 * time.Millisecond)
	}
	write(t, s.Repo, "a.nix", "1\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "one", true))
	_, shared, _ := s.Jobs.Model(ctx, "working")
	if _, j, _ := s.Jobs.Model(ctx, "head"); j != shared {
		t.Fatal("the commit's evaluation should share the queued job for the same tree")
	}
	write(t, s.Repo, "a.nix", "2\n")
	s.Jobs.Model(ctx, "working")
	if info := wait(t, shared); info.State != "passed" {
		t.Fatalf("the evaluation History waits on was skipped: %+v", info)
	}
}

// countingNix wraps the fixture fake-nix: each machine evaluation lasts 0.4 s
// and logs its name, and how many were running half way, to $COUNTS.
func countingNix(t *testing.T, s *Service) (counts string) {
	t.Helper()
	dir := t.TempDir()
	counts = filepath.Join(dir, "counts")
	t.Setenv("COUNTS", counts)
	s.Jobs.Nix = script(t, "nix", `#!/bin/sh
case "$*" in *"nixmgr:host"*)
  host=$(printf '%s\n' "$*" | sed -n 's/.* host = "\([^"]*\)";.*/\1/p')
  run="$COUNTS.run.$host"
  touch "$run"; sleep 0.2
  echo "$host $(ls "$COUNTS".run.* | wc -l)" >> "$COUNTS"
  sleep 0.2; rm "$run" ;;
esac
exec `+s.Jobs.Nix+` "$@"
`)
	return counts
}

// evaluated reads countingNix's log: the machines evaluated, and the most at once.
func evaluated(t *testing.T, counts string) (hosts []string, most int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(fileOf(t, counts)), "\n") {
		var h string
		var n int
		if _, err := fmt.Sscan(line, &h, &n); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		hosts, most = append(hosts, h), max(most, n)
	}
	slices.Sort(hosts)
	return hosts, most
}

func fileOf(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Machines evaluate side by side, never more than EvalJobs at once, and the
// model lists them in the same order whatever finished first.
func TestMachinesEvaluateInParallelWithinTheLimit(t *testing.T) {
	s := fleetService(t, "testdata/fleet")
	counts := countingNix(t, s)
	s.Jobs.EvalJobs = 2
	m := readyModel(t, s, "working")
	hosts, most := evaluated(t, counts)
	if strings.Join(hosts, ",") != "homeserver,oldbox,pi,workstation" || most != 2 {
		t.Fatalf("evaluated %v, at most %d at once; want all four, 2 at once", hosts, most)
	}
	if got := strings.Join(names(m.Hosts, func(h Host) string { return h.Name }), ","); got != "homeserver,oldbox,pi,workstation" {
		t.Fatalf("model hosts %s", got)
	}
	if m.Hosts[1].Error == "" || m.Hosts[3].Error != "" || len(m.Homes) != 1 {
		t.Fatalf("one broken machine must not hide the others: %+v", m)
	}
}

// Evaluate again keeps the machines that evaluated: the files did not change,
// so neither did they. It evaluates again only what failed.
func TestEvaluateAgainOnlyRetriesWhatFailed(t *testing.T) {
	fixtures := t.TempDir()
	ok(t, os.CopyFS(fixtures, os.DirFS("testdata/fleet")))
	s := fleetService(t, fixtures)
	counts := countingNix(t, s)
	before := readyModel(t, s, "working")
	if before.Hosts[1].Name != "oldbox" || before.Hosts[1].Error == "" {
		t.Fatalf("oldbox should fail first: %+v", before.Hosts[1])
	}
	// What was missing (the network, say) is back.
	pi := strings.Replace(fileOf(t, filepath.Join(fixtures, "host-pi.json")), `"pi"`, `"oldbox"`, -1)
	ok(t, os.WriteFile(filepath.Join(fixtures, "host-oldbox.json"), []byte(pi), 0o644))
	ok(t, os.Remove(counts))
	wait(t, must[*Job](t)(s.Jobs.RefreshModel(context.Background())))
	after := readyModel(t, s, "working")
	if hosts, _ := evaluated(t, counts); strings.Join(hosts, ",") != "oldbox" {
		t.Fatalf("evaluated again %v, want only oldbox", hosts)
	}
	if after.Hosts[1].Error != "" || after.Hosts[1].Name != "oldbox" || !reflect.DeepEqual(after.Hosts[3], before.Hosts[3]) || len(after.Homes) != 1 {
		t.Fatalf("model after evaluating again: %+v", after)
	}
}

// History can show what one commit changed: the model at any revision.
func TestModelAtRevision(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	ok(t, s.Stage(ctx, nil))
	c1 := must[string](t)(s.Commit(ctx, "first", true))
	write(t, s.Repo, "hosts/a.nix", "{ }\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "second", true))

	m := readyModel(t, s, "rev:"+c1)
	tree := strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", c1+"^{tree}"))
	if m.Tree != tree || len(m.Hosts) != 4 {
		t.Fatalf("model at %s: tree %s, want %s", c1, m.Tree, tree)
	}
	if p, ok := s.Repo.Parent(ctx, c1); ok || p != "" {
		t.Fatalf("root commit has a parent: %q", p)
	}
	if p, ok := s.Repo.Parent(ctx, "HEAD"); !ok || p != c1 {
		t.Fatalf("parent of HEAD: %q %v", p, ok)
	}
	_, _, err := s.Jobs.Model(ctx, "rev:nope")
	isErr(t, err, ErrNotFound)
}

func TestCappedOutput(t *testing.T) {
	c := capped{max: 4}
	if _, err := c.Write([]byte("abc")); err != nil || c.over {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("de")); err == nil || !c.over || c.buf.String() != "abc" {
		t.Fatalf("over the cap: %v %q", err, c.buf.String())
	}
}

func TestNixString(t *testing.T) {
	for in, want := range map[string]string{
		"pi":           `"pi"`,
		`a"b\c`:        `"a\"b\\c"`,
		"${x} $y\n''z": `"\${x} \$y` + "\n" + `''z"`,
	} {
		if got := nixString(in); got != want {
			t.Errorf("nixString(%q) = %s, want %s", in, got, want)
		}
	}
}

// Fleet code is evaluated in pure mode: the flake is pinned by its NAR hash and
// the arguments are in the expression, never --impure or the environment.
func TestEvalIsPure(t *testing.T) {
	ctx := context.Background()
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("NIXMGR_CALLS", calls)
	js := &Jobs{Nix: script(t, "nix", `#!/bin/sh
printf '%s\n' "$*" >> "$NIXMGR_CALLS"
case "$*" in *" hash path /src") echo sha256-a+b/c=; exit 0 ;; esac
case "$*" in *"unlocked"*) echo "error: cannot update unlocked flake input 'nixpkgs' in pure mode" >&2; exit 1 ;; esac
echo '{"ok":true}'
`)}
	var out struct{ OK bool }
	ok(t, js.evalJSON(ctx, "/src", map[string]string{"host": "pi", "pathsJSON": `[["a"]]`}, "{ flakeRef, host, pathsJSON }: { }", &out))
	got := fileOf(t, calls)
	want := `) { flakeRef = "path:/src?narHash=sha256-a%2Bb%2Fc%3D"; host = "pi"; pathsJSON = "[[\"a\"]]"; }` + "\n"
	if !out.OK || strings.Contains(got, "--impure") || !strings.HasSuffix(got, want) {
		t.Fatalf("calls:\n%s\nwant the call to end with %s", got, want)
	}
	if err := js.evalJSON(ctx, "/src", nil, "unlocked", &out); err == nil || !strings.Contains(err.Error(), "run Lock") {
		t.Fatalf("an input missing from flake.lock: %v", err)
	}
	// No hash, no evaluation.
	ok(t, os.Remove(calls))
	err := js.evalJSON(ctx, "/other", nil, "{ flakeRef }: { }", &out)
	if err == nil || !strings.Contains(err.Error(), "nix hash path") || strings.Count(fileOf(t, calls), "\n") != 1 {
		t.Fatalf("a failed hash: %v\n%s", err, fileOf(t, calls))
	}
}

// With real Nix: fleet code cannot read the server's files, and the arguments
// reach the expression as they are. No network: the flake has no inputs.
func TestEvalIsPureWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	secret := filepath.Join(t.TempDir(), "sops-age-key")
	ok(t, os.WriteFile(secret, []byte("AGE-SECRET-KEY-1TEST"), 0o600))
	src := t.TempDir()
	ok(t, os.WriteFile(filepath.Join(src, "flake.nix"), []byte(`{ outputs = _: { leak = builtins.readFile `+nixString(secret)+`; }; }`), 0o644))
	js := &Jobs{Nix: "nix"}
	odd := `a"b\c ${d} $e` + "\n'' f"
	var got string
	ok(t, js.evalJSON(ctx, src, map[string]string{"s": odd}, `{ flakeRef, s }: builtins.seq (builtins.getFlake flakeRef).outPath s`, &got))
	if got != odd {
		t.Fatalf("the argument came back as %q", got)
	}
	err := js.evalJSON(ctx, src, nil, `{ flakeRef }: (builtins.getFlake flakeRef).leak`, &got)
	if err == nil || !strings.Contains(err.Error(), "forbidden in pure eval") || strings.Contains(err.Error(), "AGE-SECRET") {
		t.Fatalf("reading a file of the server: %v", err)
	}
}

// With real Nix: a machine whose configuration reads the server's age key does
// not evaluate, and the key shows nowhere in the model.
func TestHostCannotReadServerFilesWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	secret := filepath.Join(t.TempDir(), "sops-age-key")
	ok(t, os.WriteFile(secret, []byte("AGE-SECRET-KEY-1TEST"), 0o600))
	s := NewService(newRepo(t))
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	def := fileText(t, s.Repo, "hosts/example/default.nix")
	write(t, s.Repo, "hosts/example/default.nix", strings.Replace(def, "system.stateVersion",
		"users.motd = builtins.readFile "+nixString(secret)+";\n  system.stateVersion", 1))
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if info := wait40(t, lock); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	_, j, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	wait40(t, j)
	m, _, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	b, _ := json.Marshal(m)
	var example *Host
	for i := range m.Hosts {
		if m.Hosts[i].Name == "example" {
			example = &m.Hosts[i]
		}
	}
	if example == nil || !strings.Contains(example.Error, "forbidden in pure eval") || bytes.Contains(b, []byte("AGE-SECRET")) {
		t.Fatalf("model: %s\n%s", b, j.Info(true).Output)
	}
}

// A standalone home-manager configuration with real Nix. Its programs include removed
// options, which throw when read (programs.octant): they must not hide the rest.
func TestHomeModelWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	write(t, s.Repo, "flake.nix", `{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  inputs.home-manager = {
    url = "github:nix-community/home-manager/release-26.05";
    inputs.nixpkgs.follows = "nixpkgs";
  };
  outputs =
    { nixpkgs, home-manager, ... }:
    let
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in
    {
      homeConfigurations.pablo = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [
          {
            home.username = "pablo";
            home.homeDirectory = "/home/pablo";
            home.stateVersion = "26.05";
            home.packages = [ pkgs.hello ];
            programs.git.enable = true;
          }
        ];
      };
    };
}
`)
	// Pure evaluation fetches only the inputs flake.lock pins.
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if info := wait40(t, lock); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	_, j, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	select {
	case <-j.Done():
	case <-time.After(15 * time.Minute):
		t.Fatal("the model did not finish")
	}
	m, _, err := s.Jobs.Model(ctx, "working")
	ok(t, err)
	if m == nil || m.Error != "" || len(m.Homes) != 1 {
		t.Fatalf("model: %+v\n%s", m, j.Info(true).Output)
	}
	h := m.Homes[0]
	if h.Error != "" || !slices.Contains(h.Packages, "hello") || !slices.Contains(h.Programs, "git") {
		t.Fatalf("home: %+v", h)
	}
}

// The model must tell any change of a machine's system, and which ones need a
// reboot, from a real NixOS evaluation: a container has no kernel of its own,
// a sysctl changes the system but not what it boots, kernel parameters do.
func TestChangeDetectionWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []MachineSpec{
		{Name: "ct", System: "x86_64-linux", Role: "minimal", Platform: "proxmox-lxc"},
		{Name: "vm", System: "x86_64-linux", Role: "minimal", Platform: "proxmox-vm"},
	} {
		must[MachineCreated](t)(s.CreateMachine(spec))
	}
	// Locked, so nixpkgs cannot move between the evaluations compared.
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	select {
	case <-lock.Done():
	case <-time.After(10 * time.Minute):
		t.Fatal("the lock did not finish")
	}
	if info := lock.Info(true); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "machines", true))
	model := func(at string) *Model {
		t.Helper()
		_, j, err := s.Jobs.Model(ctx, at)
		ok(t, err)
		if j != nil {
			select {
			case <-j.Done():
			case <-time.After(20 * time.Minute):
				t.Fatal("the model did not finish")
			}
		}
		m, _, err := s.Jobs.Model(ctx, at)
		ok(t, err)
		if m == nil || m.Error != "" || len(m.Hosts) != 3 {
			t.Fatalf("model at %s: %+v", at, m)
		}
		for _, h := range m.Hosts {
			if h.Error != "" || !strings.HasPrefix(h.Toplevel, "/nix/store/") || !strings.HasSuffix(h.Toplevel, ".drv") {
				t.Fatalf("%s: toplevel %q, error %s", h.Name, h.Toplevel, h.Error)
			}
		}
		return m
	}
	head := model("head")
	for _, h := range head.Hosts {
		if (h.Name == "ct") != (h.Boot == "") {
			t.Fatalf("%s boots %q; only the container boots no kernel of its own", h.Name, h.Boot)
		}
	}
	edit := func(line string) []HostDiff {
		t.Helper()
		def := fileText(t, s.Repo, "hosts/example/default.nix")
		write(t, s.Repo, "hosts/example/default.nix", strings.Replace(def, "system.stateVersion", line+"\n  system.stateVersion", 1))
		return DiffModels(head, model("working"))
	}
	byHost := func(ds []HostDiff) map[string]HostDiff {
		out := map[string]HostDiff{}
		for _, d := range ds {
			out[d.Host] = d
		}
		return out
	}
	d := byHost(edit(`boot.kernel.sysctl."vm.swappiness" = 10;`))
	if ex := d["example"]; ex.Status != "changed" || ex.Reboot || ex.PackagesAdded != nil || ex.FeaturesAdded != nil || ex.VersionsChanged != nil {
		t.Fatalf("a sysctl must show as another change without a reboot: %+v", ex)
	}
	if d["ct"].Status != "unchanged" || d["vm"].Status != "unchanged" {
		t.Fatalf("machines the edit does not touch: %+v %+v", d["ct"], d["vm"])
	}
	if ex := byHost(edit(`boot.kernelParams = [ "quiet" ];`))["example"]; ex.Status != "changed" || !ex.Reboot {
		t.Fatalf("kernel parameters need a reboot: %+v", ex)
	}

	// Units and /etc, as switch-to-configuration and the etc builder see them.
	demo := func(v string) string {
		return `systemd.services.nixmgr-restart = { wantedBy = [ "multi-user.target" ]; script = "sleep ` + v + `"; };
  systemd.services.nixmgr-keep = { restartIfChanged = false; script = "sleep ` + v + `"; };
  systemd.services.nixmgr-reload = { reloadTriggers = [ "` + v + `" ]; script = "sleep 1"; };
  systemd.services.nixmgr-quiet = { description = "quiet ` + v + `"; script = "sleep 1"; };
  systemd.services.nixmgr-` + v + ` = { script = "sleep 1"; };
  environment.etc."nixmgr-text".text = "` + v + `";
  environment.etc."nixmgr-file".source = ./demo.conf;`
	}
	def := fileText(t, s.Repo, "hosts/example/default.nix")
	write(t, s.Repo, "hosts/example/demo.conf", "same\n")
	write(t, s.Repo, "hosts/example/default.nix", strings.Replace(def, "system.stateVersion", demo("old")+"\n  system.stateVersion", 1))
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "demo units", true))
	before := model("head")
	write(t, s.Repo, "hosts/example/default.nix", strings.Replace(def, "system.stateVersion", demo("new")+"\n  system.stateVersion", 1))
	write(t, s.Repo, "hosts/example/other.txt", "another file of the flake\n") // moves the flake's own store path
	ex := byHost(DiffModels(before, model("working")))["example"]
	units := map[string]string{}
	for _, u := range ex.Units {
		units[u.Name] = u.Action
	}
	want := map[string]string{"nixmgr-restart.service": "restart", "nixmgr-keep.service": "keep", "nixmgr-reload.service": "reload",
		"nixmgr-old.service": "stop", "nixmgr-new.service": "start"}
	if !reflect.DeepEqual(units, want) {
		t.Fatalf("units: got %v, want %v (a changed description alone restarts nothing)", units, want)
	}
	if !reflect.DeepEqual(ex.Etc, []EtcChange{{Path: "/etc/nixmgr-text", Status: "changed"}}) {
		t.Fatalf("/etc: %+v (a file of the flake that did not change must not show)", ex.Etc)
	}
	for _, h := range before.Hosts {
		if h.Detail == nil || !strings.Contains(h.Detail.SystemPath, "-nixos-system-"+h.Name) || len(h.Detail.Units) < 20 || h.Detail.Etc["hosts"] == "" {
			t.Fatalf("%s: detail %+v", h.Name, h.Detail)
		}
	}
}

// settle waits until the files are older than the racy window, so the next
// WorkingTree can be cached.
func settle(t *testing.T) {
	t.Helper()
	old := racyWindow
	racyWindow = 30 * time.Millisecond
	t.Cleanup(func() { racyWindow = old })
	time.Sleep(2 * racyWindow)
}

// cachedTree is WorkingTree while the repository lock is held by someone else:
// only a cached answer can come back.
func cachedTree(t *testing.T, r *Repo) (string, bool) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	got := make(chan string, 1)
	go func() {
		tree, _ := r.WorkingTree(context.Background())
		got <- tree
	}()
	select {
	case tree := <-got:
		return tree, true
	case <-time.After(2 * time.Second): // a hit takes microseconds; only a miss waits for the lock
		r.mu.Unlock()
		<-got // let it finish with the lock
		r.mu.Lock()
		return "", false
	}
}

// freshTree is the working tree computed without the cache.
func freshTree(t *testing.T, r *Repo) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	return must[string](t)(r.indexTree(context.Background(), []string{"-A", "--", "."}))
}

// Polls reuse the working tree without the repository lock while no file
// changed, and see every change: content of the same size with the
// modification time put back, renames, new and deleted files, the executable
// bit, ignore rules and the index.
func TestWorkingTreeIsCachedUntilAnythingChanges(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	write(t, r, "a.nix", "aaaa\n")
	write(t, r, "ignored.nix", "tracked though ignored\n")
	gitOut(t, r.Dir, "add", "-A")
	write(t, r, ".gitignore", "ignored.nix\n")
	commitAll(t, r, "init")
	abs := func(p string) string { return filepath.Join(r.Dir, p) }

	for _, c := range []struct {
		name   string
		change func()
	}{
		{"same size, same mtime", func() {
			fi, _ := os.Stat(abs("a.nix"))
			added, _, _ := fileID(fi)
			// git compares change times in whole seconds: in the second the file was
			// added, git itself would not see this content. Write until the change
			// time (the kernel's coarse clock) is in another second.
			for {
				ok(t, os.WriteFile(abs("a.nix"), []byte("bbbb\n"), 0o644))
				now, _ := os.Stat(abs("a.nix"))
				if c, _, _ := fileID(now); c.Unix() != added.Unix() {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			ok(t, os.Chtimes(abs("a.nix"), fi.ModTime(), fi.ModTime()))
		}},
		{"rename", func() { ok(t, os.Rename(abs("a.nix"), abs("b.nix"))) }},
		{"new file", func() { ok(t, os.WriteFile(abs("c.nix"), []byte("c\n"), 0o644)) }},
		{"executable", func() { ok(t, os.Chmod(abs("c.nix"), 0o755)) }},
		{"delete", func() { ok(t, os.Remove(abs("c.nix"))) }},
		{"info/exclude", func() { ok(t, os.WriteFile(abs(".git/info/exclude"), []byte("b.nix\n"), 0o644)) }},
		{"index", func() { gitOut(t, r.Dir, "rm", "-q", "--cached", "ignored.nix") }},
	} {
		settle(t)
		before := must[string](t)(r.WorkingTree(ctx))
		if tree, hit := cachedTree(t, r); !hit || tree != before {
			t.Fatalf("%s: an unchanged working copy was not served from the cache (%v)", c.name, hit)
		}
		c.change()
		settle(t) // past the racy window, only the fingerprint can tell
		want := freshTree(t, r)
		if want == before {
			t.Fatalf("%s: the change does not change the tree; the test is wrong", c.name)
		}
		if got := must[string](t)(r.WorkingTree(ctx)); got != want {
			t.Fatalf("%s: stale working tree %s, want %s", c.name, got, want)
		}
	}
}

// The index and info/exclude are found in a repository whose path has spaces.
func TestWorkingTreeCacheWatchesTheIndexAnywhere(t *testing.T) {
	r := must[*Repo](t)(OpenRepo(filepath.Join(t.TempDir(), "my fleet"), "", testName, testEmail))
	if len(r.gitFiles) != 2 || filepath.Base(r.gitFiles[0]) != "index" || filepath.Base(r.gitFiles[1]) != "exclude" || !strings.Contains(r.gitFiles[0], "my fleet") {
		t.Fatalf("git files %q", r.gitFiles)
	}
}

// A file changed moments ago may change again within the same timestamp, so
// the tree is not cached until it is older than the racy window.
func TestRecentChangesAreNotCached(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	settle(t)
	racyWindow = time.Hour
	write(t, r, "a.nix", "a\n")
	must[string](t)(r.WorkingTree(ctx))
	if _, hit := cachedTree(t, r); hit {
		t.Fatal("a tree with a file changed just now was cached")
	}
}

// The push dialog compares what is pushed with the commit to push: the model
// of origin's copy of the branch, else of origin's default branch, else none.
func TestModelOfWhatIsPushed(t *testing.T) {
	ctx := context.Background()
	r, _ := remotePair(t)
	s := NewService(r)
	nix, _ := filepath.Abs("../../scripts/dev/fake-nix")
	fixtures, _ := filepath.Abs("testdata/fleet")
	s.Jobs.Nix = nix
	t.Setenv("NIXMGR_FAKE_FIXTURES", fixtures)
	pushed := strings.TrimSpace(gitOut(t, r.Dir, "rev-parse", "origin/main^{tree}"))
	write(t, r, "b.nix", "2\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "local", true))
	if m := readyModel(t, s, "pushed"); m.Tree != pushed {
		t.Fatalf("pushed model of tree %q, want %q", m.Tree, pushed)
	}
	// A new branch is deployed on top of the default branch.
	ok(t, s.Repo.CreateBranch(ctx, "testing-web", ""))
	if m := readyModel(t, s, "pushed"); m.Tree != pushed {
		t.Fatalf("new branch: pushed model of tree %q, want %q", m.Tree, pushed)
	}
	// Nothing pushed: every machine is new.
	alone := NewService(newRepo(t))
	if m := readyModel(t, alone, "pushed"); m.Tree != "" || len(m.Hosts) != 0 {
		t.Fatalf("no remote: %+v", m)
	}
}
