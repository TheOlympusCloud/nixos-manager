package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// buildNix plays nix for checks that build. Every file hosts/<name> of the
// evaluated flake is a machine, whose system derivation is named after a hash
// of the file, so editing it changes that machine. In a machine's file,
// "broken" fails its build, "kernel=<x>" is what only a reboot applies,
// "system=<s>" its platform; the file $FAILEVAL makes every machine fail to
// evaluate. A dry run reports 2 derivations to build and 1 path to fetch, and
// a build with --out-link links to a store path named after the machine.
const buildNix = `#!/bin/sh
flake=${NIXMGR_FLAKE#path:}
case "$*" in
  *"nixmgr:discover"*)
    printf '{"nixos":['; sep=
    for f in "$flake"/hosts/*; do [ -e "$f" ] || continue; printf '%s"%s"' "$sep" "$(basename "$f")"; sep=,; done
    printf '],"home":[],"darwin":[]}\n'; exit 0 ;;
  *"nixmgr:host"*)
    [ -n "$FAILEVAL" ] && [ -f "$FAILEVAL" ] && { echo "error: unable to download 'https://cache.nixos.org'" >&2; exit 1; }
    f="$flake/hosts/$NIXMGR_HOST"
    h=$(sha256sum < "$f" | cut -c1-12)
    sys=$(grep -o 'system=[a-z0-9_-]*' "$f" | cut -d= -f2); sys=${sys:-x86_64-linux}
    boot=$(grep -o 'kernel=[a-z0-9]*' "$f")
    printf '{"system":"%s","toplevel":"/nix/store/%s-nixos-system-%s.drv","boot":"%s"}\n' "$sys" "$h" "$NIXMGR_HOST" "$boot"; exit 0 ;;
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
		"flake check", "Comparing every machine with what is pushed", "Building the system of 1 changed machine: web.",
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
		!strings.Contains(info.Output, "Nothing is pushed yet to compare with, so every machine is built.") {
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
