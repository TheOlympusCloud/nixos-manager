package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// A patch that only adds a file applies to any source tree.
const nixmgrPatch = `diff --git a/nixmgr-patched b/nixmgr-patched
new file mode 100755
--- /dev/null
+++ b/nixmgr-patched
@@ -0,0 +1,2 @@
+#!/bin/sh
+echo patched
`

// The whole customisation path with real Nix, on two machines of the starter
// fleet: NixOS's default packages removed and rebuilt, a package changed from
// managed.json and from a raw Nix file, a machine's own custom.nix, and
// home-manager users with packages, an option, a dotfile and a module of
// their own, a home copied from one machine to the other, a package from
// another nixpkgs input, and modules/managed.json for both. The check builds both machines, the built
// systems hold exactly that, the model says so, and comin builds the same
// systems from the push.
func TestCustomisedFleetWithRealNix(t *testing.T) {
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
	edit := must[ManagedView](t)

	// example: NixOS's perl goes and its strace is patched; pablo's hand-written
	// home loses htop and gets hello, a variable, a dotfile and a module.
	edit(s.EditPackages("example", PackageEdit{Exclude: []string{"perl"}}))
	edit(s.SetPackageOverride("example", "strace", &PackageOverride{
		OverrideAttrs: map[string]any{"postInstall": "install -m755 $NIX_BUILD_TOP/$sourceRoot/nixmgr-patched $out/bin/nixmgr-patched"},
		Patches:       []PackagePatch{{File: "patches/nixmgr.patch"}},
	}, nil))
	write(t, s.Repo, "hosts/example/patches/nixmgr.patch", nixmgrPatch)
	write(t, s.Repo, "hosts/example/packages/strace.nix", "{ pkgs, prev, lib, ... }:\npkg:\npkg.overrideAttrs (old: {\n  postInstall = (old.postInstall or \"\") + \"\\nln -s nixmgr-patched $out/bin/nixmgr-raw\\n\";\n})\n")
	write(t, s.Repo, "hosts/example/custom.nix", "{ environment.etc.\"nixmgr-custom\".text = \"custom.nix\"; }\n")
	edit(s.EditPackages("example", PackageEdit{User: "pablo", Add: []string{"hello"}, Exclude: []string{"htop"}}))
	edit(s.SetOption("example", []string{"home-manager", "users", "pablo", "home", "sessionVariables", "NIXMGR_TEST"}, "managed", nil))
	write(t, s.Repo, "hosts/example/home/pablo/files/.config/nixmgr/test.conf", "dotfile\n")
	write(t, s.Repo, "hosts/example/home/pablo/custom.nix", "{ home.file.\"nixmgr-custom\".text = \"home custom.nix\"; }\n")

	// ct: two NixOS defaults go, htop gets vim keys, a hand-written lua5_3 is replaced
	// by lua5_4 of the same name (lua), and a new user gets a home.
	def := fileText(t, s.Repo, "hosts/ct/default.nix")
	write(t, s.Repo, "hosts/ct/default.nix", strings.Replace(strings.Replace(def, "{ modulesPath, ... }:", "{ modulesPath, pkgs, ... }:", 1),
		"system.stateVersion", "environment.systemPackages = [ pkgs.lua5_3 ];\n  system.stateVersion", 1))
	if !strings.Contains(fileText(t, s.Repo, "hosts/ct/default.nix"), "pkgs.lua5_3") {
		t.Fatalf("ct's default.nix did not take lua5_3:\n%s", def)
	}
	edit(s.EditPackages("ct", PackageEdit{Exclude: []string{"rsync", "strace", "jq"}}))
	edit(s.EditPackages("ct", PackageEdit{Exclude: []string{"lua"}, Add: []string{"lua5_4"}}))
	edit(s.SetPackageOverride("ct", "htop", &PackageOverride{Override: map[string]any{"withVimKeys": true}}, nil))
	edit(s.SetOption("ct", []string{"users", "users", "anna", "isNormalUser"}, true, nil))
	edit(s.SetHome("ct", HomeEdit{User: "anna", Enable: true, StateVersion: "26.05"}))
	edit(s.EditPackages("ct", PackageEdit{User: "anna", Add: []string{"hello"}}))
	write(t, s.Repo, "hosts/ct/home/anna/files/.config/nixmgr/anna.conf", "anna\n")

	// example gets anna too, and her home copied from ct; home.stateVersion stays
	// example's own, set as the web sets it after a copy.
	edit(s.SetOption("example", []string{"users", "users", "anna", "isNormalUser"}, true, nil))
	if r := must[[]CopyResult](t)(s.Copy("ct", CopyRequest{Home: "anna", To: []string{"example"}})); len(r) != 1 || !r[0].Changed {
		t.Fatalf("copy anna's home: %+v", r)
	}
	edit(s.SetHome("example", HomeEdit{User: "anna", Enable: true, StateVersion: "26.05"}))

	// example takes tree from another nixpkgs, with the input line the Customise
	// drawer shows added to flake.nix (the same branch here, to keep the test light).
	flake := fileText(t, s.Repo, "flake.nix")
	write(t, s.Repo, "flake.nix", strings.Replace(flake, "    # Add more inputs here", "    nixpkgs-other.url = \"github:NixOS/nixpkgs/nixos-26.05\";\n    # Add more inputs here", 1))
	edit(s.AddPackages("example", []string{"tree"}))
	edit(s.SetPackageOverride("example", "tree", &PackageOverride{From: "nixpkgs-other"}, nil))

	// Every machine: jq, which ct takes out, and cowsay for pablo, whose home only
	// example manages: ct has no pablo and must not get a home for him.
	edit(s.EditPackages(SharedHost, PackageEdit{Add: []string{"jq"}}))
	edit(s.EditPackages(SharedHost, PackageEdit{User: "pablo", Add: []string{"cowsay"}}))

	if info := wait40(t, must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	ok(t, s.Stage(ctx, nil))
	info := wait40(t, must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "check"})))
	if info.State != "passed" || !slices.Equal(info.Summary.Built, []string{"ct", "example"}) {
		t.Fatalf("check %s %+v:\n%s", info.State, info.Summary, tail(info.Output, 80))
	}

	root := func(h string) string {
		t.Helper()
		p, err := os.Readlink(filepath.Join(s.Jobs.GCRoots, h))
		ok(t, err)
		return p
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		ok(t, err)
		return string(b)
	}
	expect := func(cond bool, what string) {
		t.Helper()
		if !cond {
			t.Errorf("built systems: %s", what)
		}
	}
	ex, ct := root("example"), root("ct")
	expect(!exists(ex+"/sw/bin/perl") && exists(ex+"/sw/bin/rsync") && exists(ex+"/sw/bin/strace"), "example has no perl, and keeps rsync and strace")
	out, err := exec.Command(ex + "/sw/bin/nixmgr-patched").Output()
	expect(err == nil && string(out) == "patched\n", "example's strace has the patch and the overrideAttrs from managed.json")
	expect(exists(ex+"/sw/bin/nixmgr-raw"), "example's strace has packages/strace.nix on top")
	expect(read(ex+"/etc/nixmgr-custom") == "custom.nix", "example has its custom.nix")
	pablo := ex + "/etc/profiles/per-user/pablo"
	expect(exists(pablo+"/bin/hello") && !exists(pablo+"/bin/htop") && exists(pablo+"/bin/git"), "pablo has hello and git (by hand), and no htop")
	expect(exists(ex+"/sw/bin/jq") && !exists(ct+"/sw/bin/jq"), "example has the shared jq, which ct takes out")
	expect(exists(pablo+"/bin/cowsay") && !exists(ct+"/etc/profiles/per-user/anna/bin/cowsay"), "pablo has the shared cowsay, anna does not")
	expect(strings.Contains(read(pablo+"/etc/profile.d/hm-session-vars.sh"), `NIXMGR_TEST="managed"`), "pablo's home has the option set by clicking")
	expect(!exists(ct+"/sw/bin/rsync") && !exists(ct+"/sw/bin/strace") && exists(ct+"/sw/bin/perl") && exists(ct+"/sw/bin/htop"), "ct has no rsync or strace, and keeps perl and htop")
	expect(exists(ct+"/etc/profiles/per-user/anna/bin/hello"), "anna has hello")
	expect(exists(ex+"/etc/profiles/per-user/anna/bin/hello"), "anna has hello on example too, from the copy")
	expect(exists(ex+"/sw/bin/tree") && !exists(ct+"/sw/bin/tree"), "example has tree, from the other nixpkgs")
	lua, err := filepath.EvalSymlinks(ct + "/sw/bin/lua")
	expect(err == nil && strings.Contains(lua, "-lua-5.4"), "ct runs lua 5.4, the replacement of the same name, not lua 5.3 nor no lua: "+lua)

	// What the console shows of the working copy is what was built.
	var m *Model
	for m == nil {
		var j *Job
		m, j, err = s.Jobs.Model(ctx, "working")
		ok(t, err)
		if m == nil {
			wait40(t, j)
		}
	}
	hosts := map[string]Host{}
	for _, h := range m.Hosts {
		hosts[h.Name] = h
	}
	pkgNames := func(ps []Package) []string { return names(ps, func(p Package) string { return p.Name }) }
	home := func(h Host, user string) HomeInfo {
		for _, u := range h.Users {
			if u.Name == user && u.Home != nil {
				return *u.Home
			}
		}
		t.Fatalf("%s has no home for %s: %+v", h.Name, user, h.Users)
		return HomeInfo{}
	}
	e, c := hosts["example"], hosts["ct"]
	expect(e.ManagedVersion == 4 && e.HomeManager && c.ManagedVersion == 4, "the model sees managed.nix 4 and home-manager")
	expect(slices.ContainsFunc(m.Inputs, func(i Input) bool { return i.Name == "nixpkgs-other" }) && slices.Contains(pkgNames(e.Packages), "tree"), "the model lists the locked nixpkgs-other and example's tree")
	for _, p := range e.Packages {
		if p.Name == "jq" {
			expect(slices.Equal(p.Files, []string{"modules/managed.json"}), "the model lists example's jq from modules/managed.json: "+strings.Join(p.Files, ","))
		}
	}
	expect(slices.Contains(pkgNames(e.Packages), "jq") && slices.Contains(pkgNames(c.RemovedPackages), "jq"), "the model lists jq on example and as removed on ct")
	expect(slices.Contains(pkgNames(e.RemovedPackages), "perl") && !slices.Contains(pkgNames(e.Packages), "perl"), "the model lists perl as removed on example")
	expect(slices.Contains(pkgNames(c.RemovedPackages), "rsync") && slices.Contains(pkgNames(c.RemovedPackages), "strace"), "the model lists rsync and strace as removed on ct")
	p := home(e, "pablo")
	expect(slices.Contains(p.Packages, "hello") && !slices.Contains(p.Packages, "htop"), "the model lists pablo's packages")
	expect(slices.Contains(p.Files, ".config/nixmgr/test.conf") && slices.Contains(p.Files, "nixmgr-custom"), "the model lists pablo's files")
	expect(slices.Contains(home(c, "anna").Packages, "hello"), "the model lists anna's hello")
	for _, p := range c.Packages {
		if p.Name == "lua" {
			expect(strings.HasPrefix(p.Version, "5.4") && slices.Equal(p.Files, []string{"hosts/ct/managed.json"}), "the model lists ct's lua as 5.4 from managed.json: "+p.Version+" "+strings.Join(p.Files, ","))
		}
	}
	expect(slices.Contains(pkgNames(c.Packages), "lua") && !slices.Contains(pkgNames(c.RemovedPackages), "lua"), "the model lists lua as installed on ct, not removed")

	// Pushed, comin builds exactly what the check built and the console expects.
	must[string](t)(s.Commit(ctx, "customise both machines", false))
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitOut(t, s.Repo.Dir, "init", "-q", "--bare", "-b", "main", bare)
	gitOut(t, s.Repo.Dir, "remote", "add", "origin", bare)
	must[string](t)(s.Push(ctx, true))
	tip := strings.TrimSpace(gitOut(t, s.Repo.Dir, "rev-parse", "HEAD"))
	expected := waitSystems(t, s.Jobs, tip, []string{"ct", "example"}, 40*time.Minute)
	comin := filepath.Join(t.TempDir(), "comin")
	gitOut(t, s.Repo.Dir, "clone", "-q", bare, comin)
	ref := "git+file://" + comin + "?rev=" + tip
	for _, h := range []string{"ct", "example"} {
		want := nixRaw(t, ref+"#nixosConfigurations."+h+".config.system.build.toplevel.outPath")
		if expected[h].Path != want || root(h) != want {
			t.Errorf("%s: comin builds %s, the check built %s, the console expects %s (%s)", h, want, root(h), expected[h].Path, expected[h].Error)
		}
	}
	files := nixRaw(t, ref+"#nixosConfigurations.example.config.home-manager.users.pablo.home-files.outPath")
	expect(read(files+"/.config/nixmgr/test.conf") == "dotfile\n" && read(files+"/nixmgr-custom") == "home custom.nix", "pablo's dotfile and custom.nix are in his home files")
	annaFiles := nixRaw(t, ref+"#nixosConfigurations.example.config.home-manager.users.anna.home-files.outPath")
	expect(read(annaFiles+"/.config/nixmgr/anna.conf") == "anna\n", "anna's dotfile, copied from ct, is in her home files on example")
	tree := nixRaw(t, ref+"#nixosConfigurations.example.pkgs.tree.outPath")
	resolved, err := filepath.EvalSymlinks(ex + "/sw/bin/tree")
	expect(err == nil && strings.HasPrefix(resolved, tree+"/"), "example runs the tree its pkgs has, the one from nixpkgs-other")
	vim := nixRaw(t, ref+"#nixosConfigurations.ct.pkgs.htop.outPath")
	resolved, err = filepath.EvalSymlinks(ct + "/sw/bin/htop")
	expect(err == nil && strings.HasPrefix(resolved, vim+"/"), "ct runs the htop built with vim keys")
	plain := nixRaw(t, ref+"#nixosConfigurations.example.pkgs.htop.outPath")
	expect(vim != plain, "htop with vim keys is another build than example's plain htop")
}
