package core

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// modeOf is a file's permissions in the working copy.
func modeOf(t *testing.T, s *Service, p string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(filepath.Join(s.Repo.Dir, p))
	ok(t, err)
	return fi.Mode().Perm()
}

// readFile is a file's content, or "" when it does not exist.
func readFile(s *Service, p string) string {
	c, _, _ := s.Read(p)
	return c
}

// A package's customisation reaches the machines that have none, with its
// patch file and Nix file; a machine that changes the package another way is
// refused whole, and one that already has it is left as it is.
func TestCopyPackageOverride(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	o := PackageOverride{Override: map[string]any{"sensorsSupport": false}, Patches: []PackagePatch{{File: "patches/htop.patch"}, {URL: "https://example.org/fix.patch"}}}
	edit(s.SetPackageOverride("web", "htop", &o, nil))
	write(t, s.Repo, "hosts/web/patches/htop.patch", "patch\n")
	write(t, s.Repo, "hosts/web/packages/htop.nix", "{ ... }: pkg: pkg\n")
	for _, h := range []string{"laptop", "ct", "db", "srv"} {
		edit(s.AddPackages(h, []string{"git"}))
	}
	edit(s.SetPackageOverride("ct", "htop", &PackageOverride{Version: "3.4.1"}, nil))
	write(t, s.Repo, "hosts/db/packages/htop.nix", "{ ... }: pkg: pkg.override { }\n")
	edit(s.SetPackageOverride("srv", "htop", &o, nil))
	write(t, s.Repo, "hosts/srv/patches/htop.patch", "patch\n")
	write(t, s.Repo, "hosts/srv/packages/htop.nix", "{ ... }: pkg: pkg\n")

	got := must[[]CopyResult](t)(s.Copy("web", CopyRequest{PackageOverride: "htop", To: []string{"laptop", "ct", "db", "srv", "nowhere"}}))
	want := []CopyResult{
		{Host: "laptop", Changed: true},
		{Host: "ct", Conflicts: []string{"packageOverrides.htop"}},
		{Host: "db", Conflicts: []string{"hosts/db/packages/htop.nix"}},
		{Host: "srv"},
		{Host: "nowhere", Error: "hosts/nowhere/managed.json does not exist: turn on editing on nowhere first"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results\n%+v\nwant\n%+v", got, want)
	}
	laptop := must[ManagedView](t)(s.ManagedView("laptop"))
	if !reflect.DeepEqual(laptop.PackageOverrides["htop"], o) || !reflect.DeepEqual(laptop.Packages, []string{"git"}) ||
		readFile(s, "hosts/laptop/patches/htop.patch") != "patch\n" || readFile(s, "hosts/laptop/packages/htop.nix") != "{ ... }: pkg: pkg\n" {
		t.Fatalf("laptop %+v", laptop)
	}
	// Refused machines got nothing, not even the files they lacked.
	if ct := must[ManagedView](t)(s.ManagedView("ct")); ct.PackageOverrides["htop"].Version != "3.4.1" || readFile(s, "hosts/ct/patches/htop.patch") != "" {
		t.Fatalf("ct %+v", ct)
	}
	if db := must[ManagedView](t)(s.ManagedView("db")); len(db.PackageOverrides) != 0 || readFile(s, "hosts/db/packages/htop.nix") != "{ ... }: pkg: pkg.override { }\n" {
		t.Fatalf("db %+v", db)
	}

	// Only a Nix file: a machine that changes the package in managed.json is refused.
	write(t, s.Repo, "hosts/web/packages/btop.nix", "{ ... }: pkg: pkg\n")
	edit(s.SetPackageOverride("ct", "btop", &PackageOverride{Version: "1"}, nil))
	got = must[[]CopyResult](t)(s.Copy("web", CopyRequest{PackageOverride: "btop", To: []string{"ct", "laptop"}}))
	if !reflect.DeepEqual(got, []CopyResult{{Host: "ct", Conflicts: []string{"packageOverrides.btop"}}, {Host: "laptop", Changed: true}}) || readFile(s, "hosts/laptop/packages/btop.nix") == "" {
		t.Fatalf("btop %+v", got)
	}

	for _, bad := range []struct {
		from string
		req  CopyRequest
	}{
		{"web", CopyRequest{PackageOverride: "nano", To: []string{"laptop"}}},            // not changed on web
		{"web", CopyRequest{PackageOverride: "htop", To: []string{SharedHost}}},          // every machine has its own way
		{"web", CopyRequest{PackageOverride: "htop", To: []string{"web"}}},               // itself
		{"web", CopyRequest{PackageOverride: "htop", To: []string{"../x"}}},              // not a machine
		{"web", CopyRequest{PackageOverride: "htop", Home: "pablo", To: []string{"ct"}}}, // one thing at a time
		{"web", CopyRequest{PackageOverride: "htop"}},                                    // nowhere
		{SharedHost, CopyRequest{PackageOverride: "htop", To: []string{"ct"}}},
	} {
		_, err := s.Copy(bad.from, bad.req)
		isErr(t, err, ErrInvalid)
	}
	// A patch file the override names must be there to be copied.
	edit(s.SetPackageOverride("web", "jq", &PackageOverride{Patches: []PackagePatch{{File: "patches/missing.patch"}}}, nil))
	_, err := s.Copy("web", CopyRequest{PackageOverride: "jq", To: []string{"laptop"}})
	isErr(t, err, ErrNotFound)
}

// A user's home reaches the other machines: its packages and removed packages
// are joined with theirs, its options and files added where they have none.
// home.stateVersion stays each machine's own.
func TestCopyHome(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	hm := func(p ...string) []string { return append([]string{"home-manager", "users", "pablo"}, p...) }
	edit(s.SetHome("web", HomeEdit{User: "pablo", Enable: true, StateVersion: "26.05"}))
	edit(s.EditPackages("web", PackageEdit{User: "pablo", Add: []string{"ripgrep"}, Exclude: []string{"htop"}}))
	edit(s.SetOption("web", hm("programs", "git", "enable"), true, nil))
	edit(s.SetOption("web", hm("home", "sessionVariables", "EDITOR"), "vim", nil))
	write(t, s.Repo, "hosts/web/home/pablo/files/.config/a.conf", "a\n")
	write(t, s.Repo, "hosts/web/home/pablo/custom.nix", "{ }\n")
	edit(s.LinkHomeSecret("web", HomeSecretEdit{User: "pablo", Path: ".netrc", Secret: "netrc"}))
	write(t, s.Repo, "hosts/web/home/pablo/templates/.config/gh/hosts.yml", "oauth_token: @secret:gh@\n")
	write(t, s.Repo, "hosts/web/home/pablo/files/.local/bin/tool", "#!/bin/sh\n")
	ok(t, os.Chmod(filepath.Join(s.Repo.Dir, "hosts/web/home/pablo/files/.local/bin/tool"), 0o755)) // git keeps the bit
	write(t, s.Repo, "hosts/web/home/anna/files/.bashrc", "anna\n")
	write(t, s.Repo, "home/pablo/files/.profile", "not web's\n") // a home/ folder outside web's

	edit(s.AddPackages("laptop", []string{"git"}))
	edit(s.SetHome("srv", HomeEdit{User: "pablo", Enable: true, StateVersion: "25.05"}))
	edit(s.EditPackages("srv", PackageEdit{User: "pablo", Add: []string{"fd"}}))
	edit(s.LinkHomeSecret("srv", HomeSecretEdit{User: "pablo", Path: ".config/rclone/rclone.conf", Secret: "rclone"}))
	write(t, s.Repo, "hosts/srv/home/pablo/files/.bashrc", "srv\n")
	edit(s.SetOption("ct", hm("programs", "git", "enable"), false, nil))
	write(t, s.Repo, "hosts/ct/home/pablo/files/.local/bin/tool", "#!/bin/sh\n") // the same text, not executable
	edit(s.EditPackages("db", PackageEdit{User: "pablo", Add: []string{"htop"}}))
	edit(s.LinkHomeSecret("db", HomeSecretEdit{User: "pablo", Path: ".netrc", Secret: "db-netrc"}))

	got := must[[]CopyResult](t)(s.Copy("web", CopyRequest{Home: "pablo", To: []string{"laptop", "srv", "ct", "db"}}))
	want := []CopyResult{
		{Host: "laptop", Changed: true},
		{Host: "srv", Changed: true},
		{Host: "ct", Conflicts: []string{"settings.home-manager.users.pablo.programs.git.enable", "hosts/ct/home/pablo/files/.local/bin/tool"}},
		{Host: "db", Conflicts: []string{"home.pablo.packages: htop", "home.pablo.secrets: .netrc"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results\n%+v\nwant\n%+v", got, want)
	}
	opts := func(v ManagedView) any {
		return v.Settings["home-manager"].(map[string]any)["users"].(map[string]any)["pablo"]
	}
	laptop := must[ManagedView](t)(s.ManagedView("laptop"))
	if want := (HomeManaged{Packages: []string{"ripgrep"}, RemovedPackages: []string{"htop"}, Secrets: map[string]string{".netrc": "netrc"}}); !reflect.DeepEqual(laptop.Home["pablo"], want) ||
		!reflect.DeepEqual(opts(laptop), map[string]any{"programs": map[string]any{"git": map[string]any{"enable": true}}, "home": map[string]any{"sessionVariables": map[string]any{"EDITOR": "vim"}}}) ||
		readFile(s, "hosts/laptop/home/pablo/files/.config/a.conf") != "a\n" || readFile(s, "hosts/laptop/home/pablo/custom.nix") != "{ }\n" ||
		readFile(s, "hosts/laptop/home/pablo/templates/.config/gh/hosts.yml") != "oauth_token: @secret:gh@\n" ||
		readFile(s, "hosts/laptop/home/anna/files/.bashrc") != "" || readFile(s, "hosts/laptop/home/pablo/files/.profile") != "" ||
		modeOf(t, s, "hosts/laptop/home/pablo/files/.local/bin/tool") != 0o755 || modeOf(t, s, "hosts/laptop/home/pablo/files/.config/a.conf") != 0o644 {
		t.Fatalf("laptop %+v", laptop)
	}
	srv := must[ManagedView](t)(s.ManagedView("srv"))
	if want := (HomeManaged{Packages: []string{"fd", "ripgrep"}, RemovedPackages: []string{"htop"}, Secrets: map[string]string{".netrc": "netrc", ".config/rclone/rclone.conf": "rclone"}}); !reflect.DeepEqual(srv.Home["pablo"], want) ||
		opts(srv).(map[string]any)["home"].(map[string]any)["stateVersion"] != "25.05" || readFile(s, "hosts/srv/home/pablo/files/.bashrc") != "srv\n" {
		t.Fatalf("srv %+v", srv)
	}
	if ct := must[ManagedView](t)(s.ManagedView("ct")); len(ct.Home) != 0 || readFile(s, "hosts/ct/home/pablo/custom.nix") != "" {
		t.Fatalf("ct %+v", ct)
	}
	// Again: nothing left to do.
	got = must[[]CopyResult](t)(s.Copy("web", CopyRequest{Home: "pablo", To: []string{"laptop"}}))
	if !reflect.DeepEqual(got, []CopyResult{{Host: "laptop"}}) {
		t.Fatalf("second copy %+v", got)
	}
	// anna's home is not managed on web: only a home managed there is copied.
	_, err := s.Copy("web", CopyRequest{Home: "anna", To: []string{"laptop"}})
	isErr(t, err, ErrInvalid)
}
