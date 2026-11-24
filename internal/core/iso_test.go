package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// isoNix stands in for nix building an ISO: the out-link points at a folder
// with iso/<name>.iso, whose content names the machine. A machine named broken
// fails; the expression is kept next to the script.
const isoNix = `#!/bin/sh
case "$*" in *" hash path "*) echo sha256-fake; exit 0 ;; esac
dir=$(dirname "$0")
printf '%s' "$*" > "$dir/expr"
host=$(printf '%s\n' "$*" | sed -n 's/.* host = "\([^"]*\)";.*/\1/p')
[ "$host" = broken ] && { echo "error: broken does not evaluate" >&2; exit 1; }
out=
prev=
for a in "$@"; do
  [ "$prev" = --out-link ] && out=$a
  prev=$a
done
store=$(mktemp -d "$dir/store-XXXXXX")
mkdir "$store/iso"
printf 'iso of %s\n' "$host" > "$store/iso/nixos-minimal-26.05-x86_64-linux.iso"
ln -s "$store" "$out"
echo "built $store"
`

// pushedFleet is a service whose remote has machines pc (with a disk layout) and
// old (the placeholder, no disk layout), and its ISOs in a folder of their own.
func pushedFleet(t *testing.T) (*Service, *ISOs, string) {
	t.Helper()
	s := newService(t)
	s.Jobs.Nix = script(t, "nix", isoNix)
	for _, h := range []string{"pc", "old", "broken"} {
		write(t, s.Repo, "hosts/"+h+"/default.nix", "{ }\n")
		write(t, s.Repo, "hosts/"+h+"/hardware-configuration.nix", "# Placeholder: nixos-anywhere writes the real one\n{ }\n")
	}
	write(t, s.Repo, "hosts/pc/disk-config.nix", "{ }\n")
	write(t, s.Repo, "hosts/broken/disk-config.nix", "{ }\n")
	sha := commitAll(t, s.Repo, "machines")
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, s.Repo.Dir, "init", "-q", "--bare", "-b", "main", bare)
	gitOut(t, s.Repo.Dir, "remote", "add", "origin", bare)
	must[string](t)(s.Push(context.Background(), true))
	isos := must[*ISOs](t)(OpenISOs(filepath.Join(t.TempDir(), "isos"), s.Jobs))
	return s, isos, sha
}

func buildISO(t *testing.T, isos *ISOs, host string) ISO {
	t.Helper()
	iso := must[ISO](t)(isos.Build(context.Background(), host))
	j, _ := isos.Jobs.Get(iso.Job)
	if info := wait(t, j); info.Kind != "iso" || info.Host != host {
		t.Fatalf("job %+v", info)
	}
	return iso
}

func TestISOBuildKeepsTheLastThree(t *testing.T) {
	s, isos, sha := pushedFleet(t)
	first := buildISO(t, isos, "pc")
	if first.Commit != sha || first.Branch != "main" || first.File != "nixos-pc-"+sha[:7]+".iso" || first.State != "queued" && first.State != "running" {
		t.Fatalf("queued %+v", first)
	}
	expr, _ := os.ReadFile(filepath.Join(filepath.Dir(s.Jobs.Nix), "expr"))
	for _, want := range []string{"build -L --out-link " + filepath.Join(isos.Dir, first.ID, "result"), "nixmgr:iso", `host = "pc";`, `commit = "` + sha + `";`,
		"fleet-install: installs", `gitName = "` + testName + `";`, `gitEmail = "` + testEmail + `";`, `flakeRef = "path:`} {
		if !strings.Contains(string(expr), want) {
			t.Errorf("nix was not called with %q:\n%.400s", want, expr)
		}
	}

	list := must[[]ISO](t)(isos.List())
	sum := sha256.Sum256([]byte("iso of pc\n"))
	if len(list) != 1 || list[0].ID != first.ID || list[0].State != "ready" || list[0].Size != 10 || list[0].SHA256 != hex.EncodeToString(sum[:]) || list[0].Job != "" {
		t.Fatalf("list %+v", list)
	}
	path, iso, err := isos.File(first.ID)
	ok(t, err)
	if b, _ := os.ReadFile(path); string(b) != "iso of pc\n" || iso.ID != first.ID {
		t.Fatalf("file %s: %q %+v", path, b, iso)
	}

	for range 3 {
		buildISO(t, isos, "pc")
	}
	list = must[[]ISO](t)(isos.List())
	if len(list) != keepISOs || list[0].Finished.Before(list[1].Finished) {
		t.Fatalf("kept %+v", list)
	}
	for _, i := range list {
		if i.ID == first.ID {
			t.Fatalf("the oldest ISO was kept: %+v", list)
		}
	}
	if _, err := os.Stat(filepath.Join(isos.Dir, first.ID)); !os.IsNotExist(err) {
		t.Fatalf("the oldest ISO's folder, its GC root, is still there: %v", err)
	}
	_, _, err = isos.File(first.ID)
	isErr(t, err, ErrNotFound)

	// A restart finds the built ones and drops what a stop cut short.
	ok(t, os.MkdirAll(filepath.Join(isos.Dir, "pc-0123456-1"), 0o700))
	again := must[*ISOs](t)(OpenISOs(isos.Dir, s.Jobs))
	if l := must[[]ISO](t)(again.List()); len(l) != keepISOs {
		t.Fatalf("after a restart %+v", l)
	}
	if _, err := os.Stat(filepath.Join(isos.Dir, "pc-0123456-1")); !os.IsNotExist(err) {
		t.Fatalf("an unfinished build's folder is kept: %v", err)
	}
}

func TestISOBuildFailsAndRefuses(t *testing.T) {
	s, isos, _ := pushedFleet(t)
	ctx := context.Background()

	failed := buildISO(t, isos, "broken")
	list := must[[]ISO](t)(isos.List())
	if len(list) != 1 || list[0].ID != failed.ID || list[0].State != "failed" || list[0].Job == "" {
		t.Fatalf("list %+v", list)
	}
	if _, err := os.Stat(filepath.Join(isos.Dir, failed.ID)); !os.IsNotExist(err) {
		t.Fatalf("a failed build left its folder: %v", err)
	}

	_, err := isos.Build(ctx, "old")
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("why: %v", err)
	}
	_, err = isos.Build(ctx, "../pc")
	isErr(t, err, ErrInvalid)
	write(t, s.Repo, "hosts/new/default.nix", "{ }\n") // made, not pushed
	_, err = isos.Build(ctx, "new")
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "not pushed") {
		t.Errorf("why: %v", err)
	}
	_, _, err = isos.File("../../etc")
	isErr(t, err, ErrNotFound)

	// One build of a machine at a commit at a time.
	s.Jobs.Nix = script(t, "nix", strings.Replace(isoNix, "out=\n", "sleep 1\nout=\n", 1))
	building := must[ISO](t)(isos.Build(ctx, "pc"))
	_, err = isos.Build(ctx, "pc")
	isErr(t, err, ErrConflict)
	j, _ := s.Jobs.Get(building.Job)
	wait(t, j)

	bare := newService(t)
	write(t, bare.Repo, "hosts/pc/default.nix", "{ }\n")
	commitAll(t, bare.Repo, "not pushed")
	_, err = must[*ISOs](t)(OpenISOs(t.TempDir(), bare.Jobs)).Build(ctx, "pc")
	isErr(t, err, ErrInvalid)
}

// fleetInstall assembles the installer as iso.nix does, for the commit of the repository at url.
func fleetInstall(t *testing.T, url, baked, commit string) string {
	t.Helper()
	header := "#!/usr/bin/env bash\nset -o errexit\nset -o nounset\nset -o pipefail\n" +
		"host='pc'\ncommit='" + commit + "'\nurl='" + url + "'\nbranch='main'\nbaked=" + baked + "\ngit_name='Pablo'\ngit_email='pablo@example.invalid'\n"
	return script(t, "fleet-install", header+nixExpr("fleet-install.sh"))
}

// The installer, run with --yes against stand-ins for the tools of the ISO: it
// fetches the repository, partitions with the disk layout, writes the hardware
// configuration, keeps the host keys, installs, and pushes the new hardware
// configuration; offline, it installs the ISO's own copy and pushes nothing.
func TestFleetInstallScript(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("needs bash")
	}
	repo := newRepo(t)
	write(t, repo, "hosts/pc/default.nix", "{ }\n")
	write(t, repo, "hosts/pc/hardware-configuration.nix", "# Placeholder: nixos-anywhere writes the real one\n{ }\n")
	sha := commitAll(t, repo, "pc")
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, repo.Dir, "init", "-q", "--bare", "-b", "main", bare)
	gitOut(t, repo.Dir, "push", "-q", bare, "HEAD:refs/heads/main")
	baked := t.TempDir()
	ok(t, repo.export(context.Background(), gitOut(t, repo.Dir, "rev-parse", "HEAD^{tree}"), filepath.Join(baked, "src")))
	ok(t, os.Chmod(filepath.Join(baked, "src"), 0o555)) // a store path
	t.Cleanup(func() { os.Chmod(filepath.Join(baked, "src"), 0o755) })

	root := t.TempDir()
	ok(t, os.MkdirAll(filepath.Join(root, "etc", "ssh"), 0o755))
	ok(t, os.MkdirAll(filepath.Join(root, "mnt"), 0o755))
	ok(t, os.WriteFile(filepath.Join(root, "etc", "ssh", "ssh_host_ed25519_key"), []byte("private\n"), 0o600))
	ok(t, os.WriteFile(filepath.Join(root, "etc", "ssh", "ssh_host_ed25519_key.pub"), []byte("ssh-ed25519 AAAA root@nixos\n"), 0o644))
	bin := t.TempDir()
	log := filepath.Join(root, "log")
	tool := func(name, body string) {
		ok(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\necho \""+name+" $*\" >> "+log+"\n"+body), 0o755))
	}
	tool("id", "echo 0\n")
	tool("sudo", "exit 1\n")
	tool("ssh-keygen", "")
	tool("ip", "")
	tool("lsblk", "")
	tool("mountpoint", "exit 1\n")
	tool("findmnt", "exit 1\n") // no /iso: not booted from a stick
	tool("reboot", "")
	tool("nixos-install", "")
	tool("nixos-enter", "")
	tool("nixos-generate-config", "echo '# generated for this machine'\n")
	tool("disko", "")
	tool("nix", `case "$*" in
  *disko.devices*) echo /dev/vda ;;
  *isNormalUser*) echo pablo ;;
  *"build --no-link --print-out-paths"*diskoScript) echo `+filepath.Join(bin, "disko")+` ;;
esac
`)
	run := func(url string, args ...string) string {
		t.Helper()
		cmd := exec.Command(fleetInstall(t, url, filepath.Join(baked, "src"), sha), args...)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FLEET_INSTALL_ROOT="+root)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fleet-install %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	out := run(bare, "--yes")
	calls, _ := os.ReadFile(log)
	for _, want := range []string{"disko \n", "nixos-generate-config --root " + root + "/mnt --no-filesystems --show-hardware-config",
		"/fleet#pc --root " + root + "/mnt --no-root-passwd --no-channel-copy", "reboot"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("no %q in the calls:\n%s\n%s", want, calls, out)
		}
	}
	if !strings.Contains(string(calls), "nixos-install --flake /") || strings.Contains(string(calls), "nixos-enter") {
		t.Errorf("install calls:\n%s", calls)
	}
	if st, err := os.Stat(filepath.Join(root, "mnt", "etc", "ssh", "ssh_host_ed25519_key")); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("the host key was not kept for the machine: %v %v", st, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "mnt", "etc", "nixos", "hardware-configuration.nix")); string(b) != "# generated for this machine\n" {
		t.Errorf("hardware configuration on the machine: %q", b)
	}
	if got := gitOut(t, bare, "show", "main:hosts/pc/hardware-configuration.nix"); got != "# generated for this machine" {
		t.Errorf("pushed hardware configuration: %q\n%s", got, out)
	}
	if got := gitOut(t, bare, "log", "-1", "--format=%an <%ae>: %s", "main"); got != "Pablo <pablo@example.invalid>: pc: hardware configuration from the installer" {
		t.Errorf("pushed commit: %s", got)
	}

	// Offline: the ISO's copy, read-only as in the store, and nothing to push.
	ok(t, os.Remove(log))
	pushed := gitOut(t, bare, "rev-parse", "main")
	out = run(bare, "--yes", "--offline")
	if !strings.Contains(out, "Installing commit "+sha[:7]+", as this ISO has it.") || !strings.Contains(out, "still has the placeholder") {
		t.Errorf("offline:\n%s", out)
	}
	if gitOut(t, bare, "rev-parse", "main") != pushed {
		t.Error("offline pushed")
	}
	// Nothing is erased or installed before the machine's name is typed, nor on the stick it runs from.
	refused := func(stdin, want string, args ...string) {
		t.Helper()
		os.Remove(log)
		cmd := exec.Command(fleetInstall(t, bare, filepath.Join(baked, "src"), sha), args...)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FLEET_INSTALL_ROOT="+root)
		cmd.Stdin = strings.NewReader(stdin)
		out, err := cmd.CombinedOutput()
		calls, _ := os.ReadFile(log)
		if err == nil || !strings.Contains(string(out), want) || strings.Contains(string(calls), "\ndisko") || strings.Contains(string(calls), "nixos-install") {
			t.Errorf("%v: %v\n%s\ncalls:\n%s", args, err, out, calls)
		}
	}
	refused("\nnot-pc\n", "Stopped: nothing was changed.")
	tool("findmnt", "echo /dev/vda1\n")
	tool("lsblk", `case "$*" in *PKNAME*) echo vda ;; esac
`)
	refused("", "is the USB stick this installer runs from", "--yes")
	tool("findmnt", "exit 1\n")

	// Without a disk layout, it wants the disks mounted at /mnt first.
	tool("nix", "")
	cmd := exec.Command(fleetInstall(t, "", filepath.Join(baked, "src"), sha), "--yes")
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FLEET_INSTALL_ROOT="+root)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "mount them at /mnt") {
		t.Errorf("no disk layout, nothing mounted: %v\n%s", err, out)
	}
}

func TestFleetInstallScriptPassesShellcheck(t *testing.T) {
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("needs shellcheck")
	}
	if out, err := exec.Command("shellcheck", fleetInstall(t, "https://forgejo.lan/p/fleet.git", "/nix/store/x-source", strings.Repeat("a", 40))).CombinedOutput(); err != nil {
		t.Fatalf("shellcheck: %v\n%s", err, out)
	}
}

// The ISO of a machine the wizard made with a disk layout evaluates, with the starter
// flake's nixpkgs, and its fleet-install builds (writeShellApplication runs shellcheck).
func TestISOEvaluatesWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	s := newService(t)
	s.Jobs.Nix = "nix"
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "pc", System: "x86_64-linux", Role: "desktop", Disk: "ext4", Device: "/dev/vda"}))
	ctx := context.Background()
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if info := wait(t, lock); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	sha := commitAll(t, s.Repo, "pc")
	src, cleanup, err := s.Jobs.snapshot(ctx, gitOut(t, s.Repo.Dir, "rev-parse", "HEAD^{tree}"))
	ok(t, err)
	defer cleanup()
	ref := must[string](t)(s.Jobs.lockedFlake(ctx, src))
	call := isoCall(ref, "pc", sha, "Pablo", "pablo@example.invalid")
	nix := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("nix", append([]string{"--extra-experimental-features", "nix-command flakes"}, args...)...).Output()
		if ee, isExit := err.(*exec.ExitError); isExit {
			out = append(out, ee.Stderr...)
		}
		if err != nil {
			t.Fatalf("nix %s: %v\n%s", args[0], err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if drv := nix("eval", "--raw", "--expr", "("+call+").drvPath"); !strings.HasSuffix(drv, ".iso.drv") {
		t.Errorf("ISO derivation %s", drv)
	}
	bin := nix("build", "--no-link", "--print-out-paths", "--expr", "("+call+").fleetInstall") + "/bin/fleet-install"
	b, err := os.ReadFile(bin)
	ok(t, err)
	// lib.escapeShellArg quotes only what needs it, so either form is right.
	for _, v := range [][2]string{{"host", "pc"}, {"url", "https://forgejo.example.lan/pablo/nixos-config.git"}, {"branch", "main"}} {
		if !strings.Contains(string(b), v[0]+"="+v[1]+"\n") && !strings.Contains(string(b), v[0]+"='"+v[1]+"'\n") {
			t.Errorf("fleet-install lacks %s=%s", v[0], v[1])
		}
	}
	if !strings.Contains(string(b), "baked=/nix/store/") {
		t.Error("fleet-install lacks baked=/nix/store/…")
	}
}

// A customised machine installs from its installer ISO and starts, as on a real
// computer but in QEMU (testdata/install-vm.nix): fleet-install --yes --offline
// installs it on an empty disk, and its home-manager user logs in with the
// packages clicked for them on the PATH and their file in ~. It takes KVM, a few
// GB of downloads and about six minutes, so it runs only with
// NIXMGR_VM_TESTS=1 (.github/workflows/vm-tests.yml).
func TestInstallInVMWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_VM_TESTS") == "" {
		t.Skip("needs real Nix, KVM and network access: set NIXMGR_VM_TESTS=1")
	}
	ctx := context.Background()
	s := newService(t)
	s.Jobs.Nix = "nix"
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "pc", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/vda",
		User: &MachineUser{Name: "pablo", Wheel: true, Home: true}}))
	// Customised as by clicking: packages added, a NixOS default removed, a package
	// built differently, the user's own package and a file in their ~.
	must[ManagedView](t)(s.EditPackages("pc", PackageEdit{Add: []string{"hello", "jq"}, Exclude: []string{"strace"}}))
	must[ManagedView](t)(s.SetPackageOverride("pc", "hello", &PackageOverride{OverrideAttrs: map[string]any{"postInstall": "ln -s hello $out/bin/hello-customised"}}, nil))
	must[ManagedView](t)(s.EditPackages("pc", PackageEdit{User: "pablo", Add: []string{"cowsay"}}))
	must[ManagedView](t)(s.SetOption("pc", []string{"users", "users", "pablo", "initialPassword"}, "nixmgr", nil))
	write(t, s.Repo, "hosts/pc/home/pablo/files/.config/nixmgr/hello.txt", "from managed.json's folder\n")
	// The VM's own hardware configuration, not the placeholder, which fleet-install would
	// replace: the VM, without network, then installs what was built here, building nothing.
	write(t, s.Repo, "hosts/pc/hardware-configuration.nix", `# The test VM's.
{ lib, modulesPath, ... }:
{
  imports = [ (modulesPath + "/profiles/qemu-guest.nix") ];
  nixpkgs.hostPlatform = lib.mkDefault "x86_64-linux";
}
`)
	// The test driver's shell on the installed machine, as in nixpkgs' installer tests.
	write(t, s.Repo, "hosts/pc/custom.nix", `{ modulesPath, ... }:
{
  imports = [ (modulesPath + "/testing/test-instrumentation.nix") ];
}
`)
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	<-lock.Done() // fetching the inputs takes longer than wait allows
	if info := lock.Info(true); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	sha := commitAll(t, s.Repo, "pc")
	src, cleanup, err := s.Jobs.snapshot(ctx, gitOut(t, s.Repo.Dir, "rev-parse", "HEAD^{tree}"))
	ok(t, err)
	defer cleanup()
	ref := must[string](t)(s.Jobs.lockedFlake(ctx, src))
	vm, err := os.ReadFile("testdata/install-vm.nix")
	ok(t, err)
	session := `echo ~; echo "$PATH"; command -v jq hello-customised cowsay git strace; cat ~/.config/nixmgr/hello.txt`
	call := "(" + string(vm) + "\n) { flakeRef = " + nixString(ref) + "; host = \"pc\"; user = \"pablo\"; password = \"nixmgr\"; command = " + nixString(session) +
		"; iso = " + isoCall(ref, "pc", sha, "Pablo", "pablo@example.invalid") + "; }"
	// Stopped before go test's own timeout, which would leave nix running and its output open.
	if d, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, d.Add(-time.Minute))
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "build", "-L", "--no-link", "--print-out-paths", "--expr", call)
	cmd.Stderr = os.Stderr // the VMs' consoles, as they run
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("nix build: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "session.txt"))
	ok(t, err)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{
		"/home/pablo",
		"", // PATH, below
		"/run/current-system/sw/bin/jq",
		"/run/current-system/sw/bin/hello-customised",
		"/etc/profiles/per-user/pablo/bin/cowsay",
		"/run/current-system/sw/bin/git", // and no strace
		"from managed.json's folder",
	}
	if len(lines) != len(want) {
		t.Fatalf("the login session printed:\n%s", b)
	}
	for _, dir := range []string{"/etc/profiles/per-user/pablo/bin", "/run/current-system/sw/bin"} {
		if !slices.Contains(strings.Split(lines[1], ":"), dir) {
			t.Errorf("PATH %s lacks %s", lines[1], dir)
		}
	}
	lines[1] = ""
	if !slices.Equal(lines, want) {
		t.Errorf("the login session printed:\n%s", b)
	}
}
