package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// buildNix plays nix for checks that build. Every file hosts/<name> of the
// evaluated flake is a machine, whose system derivation is named after a hash
// of the file, so editing it changes that machine. In a machine's file,
// "broken" fails its build, "kernel=<x>" is what only a reboot applies,
// "system=<s>" its platform; the file $FAILEVAL makes every machine fail to
// evaluate. Every machine runs comin from the remote "remote" on main and its
// testing branch, testing-<name> unless "testing=<x>" names another ("testing="
// none); "nocomin" turns comin off. A dry run reports 2 derivations to build and
// 1 path to fetch, and a build with --out-link links to a store path named after
// the machine.
const buildNix = `#!/bin/sh
flake=$(printf '%s\n' "$*" | sed -n 's/.* flakeRef = "path:\([^"?]*\)?.*/\1/p')
host=$(printf '%s\n' "$*" | sed -n 's/.* host = "\([^"]*\)";.*/\1/p')
case "$*" in
  *" hash path "*) echo sha256-fake; exit 0 ;;
  *"nixmgr:discover"*)
    printf '{"nixos":['; sep=
    for f in "$flake"/hosts/*; do [ -e "$f" ] || continue; printf '%s"%s"' "$sep" "$(basename "$f")"; sep=,; done
    printf '],"home":[],"darwin":[]}\n'; exit 0 ;;
  *"nixmgr:host"*)
    [ -n "$FAILEVAL" ] && [ -f "$FAILEVAL" ] && { echo "error: unable to download 'https://cache.nixos.org'" >&2; exit 1; }
    f="$flake/hosts/$host"
    h=$(sha256sum < "$f" | cut -c1-12)
    sys=$(grep -o 'system=[a-z0-9_-]*' "$f" | cut -d= -f2); sys=${sys:-x86_64-linux}
    boot=$(grep -o 'kernel=[a-z0-9]*' "$f")
    testing=testing-$host; grep -q 'testing=' "$f" && testing=$(grep -o 'testing=[a-z0-9-]*' "$f" | cut -d= -f2)
    comin='"checks":{"comin":true,"cominBranches":[{"url":"remote","main":"main","testing":"'$testing'"}]}'
    grep -q nocomin "$f" && comin='"checks":{"comin":false,"cominBranches":[]}'
    printf '{"system":"%s","toplevel":"/nix/store/%s-nixos-system-%s.drv","boot":"%s",%s}\n' "$sys" "$h" "$host" "$boot" "$comin"; exit 0 ;;
  *"eval --json"*) echo '{}'; exit 0 ;;
esac
echo "nix $*"
case "$*" in *--dry-run*)
  echo "these 2 derivations will be built:" >&2
  echo "  /nix/store/aaa-etc.drv" >&2
  echo "this path will be fetched (0.50 MiB download, 2.00 MiB unpacked):" >&2
  echo "  /nix/store/bbb-hello" >&2
  exit 0 ;;
esac
link=; name=; prev=
for a in "$@"; do
  [ "$prev" = --out-link ] && link=$a
  case "$a" in
    path:*'#nixosConfigurations.'*)
      src=${a#path:}; src=${src%%#*}; name=${a#*#nixosConfigurations.}; name=${name%%.config.*}
      grep -q broken "$src/hosts/$name" && { echo "error: builder for '/nix/store/x-$name.drv' failed with exit code 1" >&2; exit 1; } ;;
    /nix/store/*'^*') name=${a##*-nixos-system-}; name=${name%.drv^*} ;;
  esac
  prev=$a
done
[ -z "$link" ] || ln -sfn "/nix/store/0000-nixos-system-$name" "$link"
`

// buildService is a service on a repository whose remote has the machines
// web and db, as pushed; with remote false it has no remote at all.
func buildService(t *testing.T, remote bool) *Service {
	t.Helper()
	ctx := context.Background()
	var s *Service
	if remote {
		r, _ := remotePair(t)
		s = NewService(r)
	} else {
		s = NewService(newRepo(t))
	}
	s.Jobs.Nix = script(t, "nix", buildNix)
	write(t, s.Repo, "hosts/web", "web kernel=1\n")
	write(t, s.Repo, "hosts/db", "db\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "machines", true))
	if remote {
		must[string](t)(s.Push(ctx, true))
	}
	return s
}

func runCheck(t *testing.T, s *Service, targets ...string) JobInfo {
	t.Helper()
	ctx := context.Background()
	ok(t, s.Stage(ctx, nil))
	return wait(t, must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check", Targets: targets})))
}

// The check builds exactly the machines whose system differs from what is
// pushed, and the pass records them with the tree they were compared with.
func TestCheckBuildsTheMachinesAChangeTouches(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	runCheck(t, s) // what is pushed passes
	pushed := strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", "origin/main^{tree}"))
	write(t, s.Repo, "hosts/web", "web kernel=2\n")

	info := runCheck(t, s, systemAttr("web"), systemAttr("db"))
	if info.State != "passed" {
		t.Fatalf("check %s:\n%s", info.State, info.Output)
	}
	sum := info.Summary
	if sum == nil || !slices.Equal(sum.Changed, []string{"web"}) || !slices.Equal(sum.Built, []string{"web"}) || sum.Base != pushed ||
		!slices.Equal(sum.Reboot, []string{"web"}) || sum.Failed != nil {
		t.Fatalf("summary %+v", sum)
	}
	if sum.ToBuild != 2 || sum.ToFetch != 1 || sum.Download != "0.50 MiB" || sum.Unpacked != "2.00 MiB" {
		t.Fatalf("dry run summary %+v", sum)
	}
	for _, want := range []string{
		"flake check", "Comparing every machine with the newest pushed commit that passed a check", "Building the system of 1 changed machine: web.",
		"nix --extra-experimental-features nix-command flakes build --keep-going --no-update-lock-file --no-link path:",
		"Nix builds 2 derivations and downloads 1 path (0.50 MiB, 2.00 MiB unpacked).",
		"Skipping the dry build of " + systemAttr("web"), "build --dry-run --no-link -L --no-update-lock-file path:",
	} {
		if !strings.Contains(info.Output, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(info.Output, "Building db") {
		t.Errorf("built db, which the change does not touch")
	}
	if !strings.Contains(info.Output, "#"+systemAttr("db")) {
		t.Errorf("the extra target db was not dry-built:\n%s", info.Output)
	}
	if t.Failed() {
		t.Fatalf("output:\n%s", info.Output)
	}
	pass, ok := s.Jobs.PassOf(info.Tree)
	if !ok || pass.Base != pushed || !slices.Equal(pass.Built, []string{"web"}) {
		t.Fatalf("pass %+v %v", pass, ok)
	}
	st := must[Status](t)(s.Status(ctx))
	if !st.StagedChecked || st.StagedCheck == nil || !slices.Equal(st.StagedCheck.Built, []string{"web"}) {
		t.Fatalf("status %+v", st)
	}
	must[string](t)(s.Commit(ctx, "web: new kernel", false))
	must[string](t)(s.Push(ctx, false))

	// Nothing changes a machine: nothing is built, and the check passes.
	write(t, s.Repo, "README", "notes\n")
	info = runCheck(t, s)
	if info.State != "passed" || len(info.Summary.Changed) != 0 || !strings.Contains(info.Output, "No machine's system changes") {
		t.Fatalf("check %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
}

// With nothing pushed there is nothing to compare with: every machine is built.
func TestCheckWithNothingPushedBuildsEveryMachine(t *testing.T) {
	s := buildService(t, false)
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	info := runCheck(t, s)
	if info.State != "passed" || !slices.Equal(info.Summary.Built, []string{"db", "web"}) || info.Summary.Base != "" ||
		!strings.Contains(info.Output, "No pushed commit has passed a check, so every machine is built.") {
		t.Fatalf("check %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
	if p, _ := s.Jobs.PassOf(info.Tree); p.Base != "" || len(p.Built) != 2 {
		t.Fatalf("pass %+v", p)
	}
}

// A machine that evaluates but does not build fails the check; the others
// still build, and the push stays refused.
func TestCheckFailsWhenAChangedMachineDoesNotBuild(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	write(t, s.Repo, "hosts/db", "db broken system=riscv64-linux\n")
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	info := runCheck(t, s)
	if info.State != "failed" || !slices.Equal(info.Summary.Failed, []string{"db"}) || !slices.Equal(info.Summary.Built, []string{"web"}) {
		t.Fatalf("check %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
	for _, want := range []string{"error: builder for", "db does not build.", "1 machine does not build: db",
		`db is riscv64-linux and this server ` + localSystem(), `"Machines of another architecture"`} {
		if !strings.Contains(info.Output, want) {
			t.Fatalf("output lacks %q:\n%s", want, info.Output)
		}
	}
	if s.Jobs.Passed(info.Tree) {
		t.Fatal("a check whose machine did not build passed")
	}
	must[string](t)(s.Commit(ctx, "broken db", true))
	_, err := s.Push(ctx, false)
	isErr(t, err, ErrNotChecked)
}

// A machine that does not evaluate fails the check before anything builds.
// A check never trusts a model that failed before (no network): once the
// cause is gone, the next check evaluates again and passes.
func TestCheckEvaluatesAgainAfterAFailedEvaluation(t *testing.T) {
	s := buildService(t, true)
	runCheck(t, s) // what is pushed passes
	failEval := filepath.Join(t.TempDir(), "fail")
	t.Setenv("FAILEVAL", failEval)
	ok(t, os.WriteFile(failEval, nil, 0o644))
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	info := runCheck(t, s)
	if info.State != "failed" || !strings.Contains(info.Output, "unable to download") || strings.Contains(info.Output, "Building") {
		t.Fatalf("check %s:\n%s", info.State, info.Output)
	}
	ok(t, os.Remove(failEval))
	if info := runCheck(t, s); info.State != "passed" || !slices.Equal(info.Summary.Built, []string{"web"}) {
		t.Fatalf("check after the failure went away %s %+v:\n%s", info.State, info.Summary, info.Output)
	}
}

// The derivation a model evaluated is built as it is, without evaluating the
// machine again, while it is in the store.
func TestCheckBuildsTheEvaluatedDerivation(t *testing.T) {
	old := storeHas
	storeHas = func(string) bool { return true }
	t.Cleanup(func() { storeHas = old })
	s := buildService(t, true)
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	info := runCheck(t, s)
	if info.State != "passed" || !strings.Contains(info.Output, "-nixos-system-web.drv^*") || strings.Contains(info.Output, "#"+systemAttr("web")) {
		t.Fatalf("check %s:\n%s", info.State, info.Output)
	}
}

// Each machine's last built system stays out of the garbage collector's reach,
// under a name that cannot leave the folder; machines removed from the flake
// lose theirs.
func TestCheckKeepsAGCRootPerMachine(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, false)
	s.Jobs.GCRoots = filepath.Join(t.TempDir(), "gcroots")
	if info := runCheck(t, s); info.State != "passed" {
		t.Fatalf("check:\n%s", info.Output)
	}
	for _, h := range []string{"web", "db"} {
		if dest, err := os.Readlink(filepath.Join(s.Jobs.GCRoots, h)); err != nil || dest != "/nix/store/0000-nixos-system-"+h {
			t.Fatalf("root of %s: %q %v", h, dest, err)
		}
	}
	ok(t, s.Repo.Delete("hosts/db"))
	ok(t, s.Stage(ctx, nil))
	if info := runCheck(t, s); info.State != "passed" {
		t.Fatalf("check:\n%s", info.Output)
	}
	if _, err := os.Lstat(filepath.Join(s.Jobs.GCRoots, "db")); !os.IsNotExist(err) {
		t.Fatalf("the removed machine's root stayed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(s.Jobs.GCRoots, "web")); err != nil {
		t.Fatalf("web's root went: %v", err)
	}
	if got := rootName("../a.b/c"); got != "___a_b_c" {
		t.Fatalf("rootName = %q", got)
	}
}

// The push needs the check of its commit to have compared the machines with
// what the push lands on now, unless that check built every machine.
func TestPushNeedsTheChangedMachinesBuilt(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	ok(t, s.Stage(ctx, nil))
	tree := must[Status](t)(s.Status(ctx)).StagedTree
	must[string](t)(s.Commit(ctx, "web", true))

	s.Jobs.mu.Lock()
	s.Jobs.pass(CheckPass{Tree: tree, Base: strings.Repeat("0", 40), Built: []string{}})
	s.Jobs.mu.Unlock()
	_, err := s.Push(ctx, false)
	isErr(t, err, ErrNotChecked)
	if !strings.Contains(err.Error(), "moved since") {
		t.Fatalf("message %q", err)
	}

	s.Jobs.mu.Lock()
	s.Jobs.pass(CheckPass{Tree: tree, Base: "", Built: []string{"db", "web"}})
	s.Jobs.mu.Unlock()
	must[string](t)(s.Push(ctx, false))
}

// What a push lands on: the branch on the remote, else the remote's default
// branch, which a new testing-<host> branch is deployed on top of.
func TestPushBase(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, true)
	main := strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", "origin/main^{tree}"))
	base := func() string {
		s.Repo.mu.Lock()
		defer s.Repo.mu.Unlock()
		return must[string](t)(s.Repo.pushBase(ctx))
	}
	if got := base(); got != main {
		t.Fatalf("on main: %q, want %q", got, main)
	}
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	commitAll(t, s.Repo, "web")
	ok(t, s.Repo.CreateBranch(ctx, "testing-web", ""))
	if got := base(); got != main {
		t.Fatalf("on a new branch: %q, want main's %q", got, main)
	}
	gitOut(t, s.Repo.Dir, "remote", "set-head", "origin", "-d")
	if got := base(); got != "" {
		t.Fatalf("no remote branch and no default: %q", got)
	}
}

// Passes from before checks built the machines only evaluated: they are read
// without error but no longer count, and the file is rewritten in the new form.
func TestEvaluationOnlyPassesNoLongerCount(t *testing.T) {
	ctx := context.Background()
	s := buildService(t, false)
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	ok(t, s.Stage(ctx, nil))
	tree := must[Status](t)(s.Status(ctx)).StagedTree
	path := filepath.Join(t.TempDir(), "checks.json")
	ok(t, os.WriteFile(path, []byte(`{"passed":["`+tree+`"]}`), 0o600))
	ok(t, s.Jobs.PersistChecks(path))
	if s.Jobs.Passed(tree) {
		t.Fatal("an evaluation-only pass counted")
	}
	runCheck(t, s)
	var f checksFile
	ok(t, json.Unmarshal(must[[]byte](t)(os.ReadFile(path)), &f))
	if len(f.Checks) != 1 || f.Checks[0].Tree != tree || len(f.Checks[0].Built) != 2 || f.Passed != nil {
		t.Fatalf("checks.json %+v", f)
	}
}

func TestParseDryRun(t *testing.T) {
	for _, c := range []struct {
		out                string
		build, fetch       int
		download, unpacked string
	}{
		{"", 0, 0, "", ""},
		{"this derivation will be built:\n  /nix/store/a.drv\n", 1, 0, "", ""},
		{"these 14 derivations will be built:\n  /nix/store/a.drv\nthese 230 paths will be fetched (512.30 MiB download, 2101.12 MiB unpacked):\n  /nix/store/b\n", 14, 230, "512.30 MiB", "2101.12 MiB"},
		{"this path will be fetched (0.01 MiB download, 0.05 MiB unpacked):\n  /nix/store/b\n", 0, 1, "0.01 MiB", "0.05 MiB"},
		{"warning: Git tree is dirty\nthese 3 paths will be fetched (1.2 GiB download, 4.5 GiB unpacked):\n", 0, 3, "1.2 GiB", "4.5 GiB"},
	} {
		b, f, d, u := parseDryRun([]byte(c.out))
		if b != c.build || f != c.fetch || d != c.download || u != c.unpacked {
			t.Errorf("parseDryRun(%q) = %d %d %q %q", c.out, b, f, d, u)
		}
	}
	if got := describeDryRun(&CheckSummary{}); got != "Everything is already in this server's store." {
		t.Errorf("nothing to do: %q", got)
	}
	if got := describeDryRun(&CheckSummary{ToBuild: 1}); got != "Nix builds 1 derivation." {
		t.Errorf("one build: %q", got)
	}
}

// With real Nix, on the starter fleet plus a container: a check builds the one
// machine an edit changes and keeps its system from the garbage collector, and
// a system that evaluates but does not build fails the check and the push.
func TestCheckBuildsTheChangedMachineWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	s.Jobs.GCRoots = filepath.Join(t.TempDir(), "gcroots")
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "ct", System: "x86_64-linux", Role: "minimal", Platform: "proxmox-lxc"}))
	finish := func(j *Job) JobInfo {
		t.Helper()
		select {
		case <-j.Done():
		case <-time.After(40 * time.Minute):
			t.Fatal("the job did not finish")
		}
		return j.Info(true)
	}
	if info := finish(must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "machines", true))
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, s.Repo.Dir, "init", "-q", "--bare", "-b", "main", bare)
	gitOut(t, s.Repo.Dir, "remote", "add", "origin", bare)
	must[string](t)(s.Push(ctx, true))
	// What is pushed passed a check before; building both machines for it is not what this test is about.
	s.Jobs.mu.Lock()
	s.Jobs.pass(CheckPass{Tree: gitOut(t, s.Repo.Dir, "rev-parse", "HEAD^{tree}"), Built: []string{"ct", "example"}})
	s.Jobs.mu.Unlock()

	def := fileText(t, s.Repo, "hosts/example/default.nix")
	edit := func(line string) JobInfo {
		t.Helper()
		write(t, s.Repo, "hosts/example/default.nix",
			strings.Replace(strings.Replace(def, "{ ... }:", "{ config, ... }:", 1), "system.stateVersion", line+"\n  system.stateVersion", 1))
		ok(t, s.Stage(ctx, nil))
		return finish(must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check"})))
	}

	info := edit(`boot.kernel.sysctl."vm.swappiness" = 10;`)
	if info.State != "passed" || !slices.Equal(info.Summary.Built, []string{"example"}) || !slices.Equal(info.Summary.Changed, []string{"example"}) {
		t.Fatalf("check %s %+v:\n%s", info.State, info.Summary, tail(info.Output, 60))
	}
	root, err := os.Readlink(filepath.Join(s.Jobs.GCRoots, "example"))
	if err != nil || !strings.Contains(root, "-nixos-system-example-") {
		t.Fatalf("GC root of example: %q %v", root, err)
	}
	if _, err := os.Stat(filepath.Join(root, "activate")); err != nil {
		t.Fatalf("the built system has no activate script: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(s.Jobs.GCRoots, "ct")); !os.IsNotExist(err) {
		t.Fatalf("ct does not change but was built: %v", err)
	}
	must[string](t)(s.Commit(ctx, "example: swappiness", false))
	must[string](t)(s.Push(ctx, false))
	checked := gitOut(t, s.Repo.Dir, "rev-parse", "HEAD^{tree}")

	// A commit pushed from elsewhere is built against the last checked one.
	clone := filepath.Join(t.TempDir(), "laptop")
	gitOut(t, s.Repo.Dir, "clone", "-q", bare, clone)
	cdef := filepath.Join(clone, "hosts/example/default.nix")
	ok(t, os.WriteFile(cdef, []byte(strings.Replace(fileText(t, s.Repo, "hosts/example/default.nix"), `"vm.swappiness" = 10;`, `"vm.swappiness" = 20;`, 1)), 0o644))
	gitOut(t, clone, "-c", "user.name=Laptop", "-c", "user.email=laptop@example.invalid", "commit", "-q", "-am", "example: swappiness 20")
	gitOut(t, clone, "push", "-q")
	gitOut(t, s.Repo.Dir, "fetch", "-q", "origin")
	job, _, err := s.Jobs.CheckCommit(ctx, gitOut(t, clone, "rev-parse", "HEAD"), "main")
	ok(t, err)
	if info := finish(job); info.State != "passed" || info.Summary.Base != checked || !slices.Equal(info.Summary.Built, []string{"example"}) {
		t.Fatalf("check of the pushed commit %s %+v:\n%s", info.State, info.Summary, tail(info.Output, 60))
	}

	// Both systems are now built here: the diff adds every dependency and the size.
	write(t, s.Repo, "hosts/example/default.nix",
		strings.Replace(strings.Replace(def, "{ ... }:", "{ pkgs, ... }:", 1), "system.stateVersion", "boot.kernel.sysctl.\"vm.swappiness\" = 10;\n  environment.systemPackages = [ pkgs.hello ];\n  system.stateVersion", 1))
	ok(t, s.Stage(ctx, nil))
	if info := finish(must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check"}))); info.State != "passed" {
		t.Fatalf("check with hello: %s", tail(info.Output, 60))
	}
	model := func(at string) *Model {
		t.Helper()
		m, j, err := s.Jobs.Model(ctx, at)
		ok(t, err)
		if j != nil {
			finish(j)
			m, _, err = s.Jobs.Model(ctx, at)
			ok(t, err)
		}
		return m
	}
	head, work := model("head"), model("working")
	ds := DiffModels(head, work)
	s.Jobs.AddClosures(ctx, head, work, ds)
	var ex HostDiff
	for _, d := range ds {
		if d.Host == "example" {
			ex = d
		}
	}
	c := ex.Closure
	if c == nil || c.SizeFrom == 0 || c.SizeTo <= c.SizeFrom || !slices.ContainsFunc(c.Changes, func(x ClosureChange) bool { return x.Name == "hello" && x.From == "" && x.To != "" && x.Delta > 0 }) {
		t.Fatalf("closure of example: %+v (diff %+v)", c, ex)
	}

	info = edit(`environment.etc.nixmgr-broken.source = derivation { name = "nixmgr-broken"; system = config.nixpkgs.hostPlatform.system; builder = "/bin/sh"; args = [ "-c" "exit 1" ]; };`)
	if info.State != "failed" || !slices.Equal(info.Summary.Failed, []string{"example"}) || !strings.Contains(info.Output, "nixmgr-broken") {
		t.Fatalf("a system that does not build: check %s %+v:\n%s", info.State, info.Summary, tail(info.Output, 60))
	}
	must[string](t)(s.Commit(ctx, "example: broken", true))
	_, err = s.Push(ctx, false)
	isErr(t, err, ErrNotChecked)
}

// A check that fails before it compared the machines has no summary to show,
// so nothing claims that no machine changes.
func TestCheckThatFailsBeforeComparingHasNoSummary(t *testing.T) {
	s := buildService(t, true)
	s.Jobs.Nix = script(t, "nix", strings.Replace(buildNix, "echo \"nix $*\"\n", "echo \"nix $*\"\ncase \"$*\" in *\"flake check\"*) exit 1 ;; esac\n", 1))
	write(t, s.Repo, "hosts/web", "web kernel=2\n")
	if info := runCheck(t, s); info.State != "failed" || info.Summary != nil {
		t.Fatalf("check %s, summary %+v:\n%s", info.State, info.Summary, info.Output)
	}
}
