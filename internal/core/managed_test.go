package core

import (
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

func TestManagedPackages(t *testing.T) {
	s := newService(t)
	v := must[ManagedView](t)(s.ManagedView("workstation"))
	if v.Exists || v.Path != "hosts/workstation/managed.json" || len(v.Packages) != 0 {
		t.Fatalf("view of a missing file %+v", v)
	}
	must[ManagedView](t)(s.AddPackages("workstation", []string{"htop", "git", "python3Packages.requests", "git"}))
	v = must[ManagedView](t)(s.RemovePackages("workstation", []string{"htop", "not-there"}))
	if !reflect.DeepEqual(v.Packages, []string{"git", "python3Packages.requests"}) || !v.Exists || v.Hash == "" {
		t.Fatalf("packages %+v", v)
	}
	content, _, _ := s.Read("hosts/workstation/managed.json")
	want := "{\n  \"packages\": [\n    \"git\",\n    \"python3Packages.requests\"\n  ],\n  \"settings\": {}\n}\n"
	if content != want {
		t.Fatalf("file content\n%s\nwant\n%s", content, want)
	}
}

func TestManagedSettings(t *testing.T) {
	s := newService(t)
	must[ManagedView](t)(s.SetOption("pi", []string{"services", "openssh", "enable"}, true, nil))
	must[ManagedView](t)(s.SetOption("pi", []string{"services", "openssh", "settings", "PermitRootLogin"}, "no", nil))
	must[ManagedView](t)(s.SetOption("pi", []string{"services", "nginx", "virtualHosts", "home.lan", "root"}, "/srv/www", nil))
	must[ManagedView](t)(s.SetOption("pi", []string{"networking", "firewall", "allowedTCPPorts"}, []any{22, 8123}, nil))
	v := must[ManagedView](t)(s.UnsetOption("pi", []string{"services", "openssh", "settings", "PermitRootLogin"}, nil))
	ssh := v.Settings["services"].(map[string]any)["openssh"].(map[string]any)
	if _, ok := ssh["settings"]; ok || ssh["enable"] != true {
		t.Fatalf("unset did not prune the empty parent: %+v", ssh)
	}
	v = must[ManagedView](t)(s.UnsetOption("pi", []string{"services", "openssh", "enable"}, nil))
	services := v.Settings["services"].(map[string]any)
	if _, ok := services["openssh"]; ok || services["nginx"] == nil {
		t.Fatalf("services after unsets %+v", services)
	}
	content, _, _ := s.Read("hosts/pi/managed.json")
	for _, want := range []string{`"home.lan": {`, `"allowedTCPPorts": [`, "8123"} {
		if !strings.Contains(content, want) {
			t.Fatalf("file lacks %q:\n%s", want, content)
		}
	}
	// Unsetting something that is not there is not an error.
	must[ManagedView](t)(s.UnsetOption("pi", []string{"services", "nope", "enable"}, nil))
}

func TestManagedValidation(t *testing.T) {
	s := newService(t)
	_, err := s.AddPackages("../etc", []string{"git"})
	isErr(t, err, ErrInvalid)
	for _, bad := range []string{"", "a b", "../x", "git;rm", ".hidden", "a..b"} {
		_, err = s.AddPackages("web", []string{bad})
		isErr(t, err, ErrInvalid)
	}
	_, err = s.SetOption("web", nil, true, nil)
	isErr(t, err, ErrInvalid)
	_, err = s.SetOption("web", []string{"services", ""}, true, nil)
	isErr(t, err, ErrInvalid)
	_, err = s.SetOption("web", []string{"time", "timeZone"}, nil, nil)
	isErr(t, err, ErrInvalid)
	must[ManagedView](t)(s.SetOption("web", []string{"time", "timeZone"}, "Europe/Madrid", nil))
	_, err = s.SetOption("web", []string{"time", "timeZone", "x"}, 1, nil)
	isErr(t, err, ErrInvalid) // timeZone is a value, not a set
}

func TestManagedKeepsUnknownKeysAndRefusesBrokenJSON(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "hosts/web/managed.json", "{\n  \"note\": \"kept\",\n  \"packages\": [\"vim\"]\n}\n")
	must[ManagedView](t)(s.AddPackages("web", []string{"git"}))
	content, _, _ := s.Read("hosts/web/managed.json")
	if !strings.Contains(content, `"note": "kept"`) || !strings.Contains(content, `"vim"`) {
		t.Fatalf("an unknown key was lost:\n%s", content)
	}
	write(t, s.Repo, "hosts/web/managed.json", "{ broken")
	_, err := s.AddPackages("web", []string{"htop"})
	isErr(t, err, ErrInvalid)
	if c, _, _ := s.Read("hosts/web/managed.json"); c != "{ broken" {
		t.Fatalf("a broken file was overwritten: %q", c)
	}
	write(t, s.Repo, "hosts/web/managed.json", `{"packages": "git"}`)
	_, err = s.AddPackages("web", []string{"htop"})
	isErr(t, err, ErrInvalid)
}

// A form saves a whole value; it must not overwrite a change made since it loaded.
func TestManagedSettingsCheckTheHashTheCallerSaw(t *testing.T) {
	s := newService(t)
	seen := must[ManagedView](t)(s.SetOption("web", []string{"networking", "firewall", "allowedTCPPorts"}, []any{22}, nil))
	must[ManagedView](t)(s.SetOption("web", []string{"networking", "firewall", "allowedTCPPorts"}, []any{22, 8080}, nil)) // an agent
	_, err := s.SetOption("web", []string{"networking", "firewall", "allowedTCPPorts"}, []any{22, 443}, &seen.Hash)
	isErr(t, err, ErrConflict)
	_, err = s.UnsetOption("web", []string{"networking", "firewall", "allowedTCPPorts"}, &seen.Hash)
	isErr(t, err, ErrConflict)
	cur := must[ManagedView](t)(s.ManagedView("web"))
	if ports := cur.Settings["networking"].(map[string]any)["firewall"].(map[string]any)["allowedTCPPorts"].([]any); len(ports) != 2 {
		t.Fatalf("the agent's change was lost: %v", ports)
	}
	must[ManagedView](t)(s.SetOption("web", []string{"networking", "firewall", "allowedTCPPorts"}, []any{22, 8080, 443}, &cur.Hash))
	empty := ""
	_, err = s.SetOption("fresh", []string{"time", "timeZone"}, "UTC", &empty) // no file yet: the empty hash matches
	ok(t, err)
}

func TestManagedRefusesTrailingJSON(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "hosts/web/managed.json", "{\"packages\": [\"vim\"]}\n{\"packages\": [\"git\"]}\n")
	_, err := s.AddPackages("web", []string{"htop"})
	isErr(t, err, ErrInvalid)
}

// Any package can go, by name, wherever it is added; restore gives it back, and
// a key nothing uses any more leaves the file.
func TestManagedExcludeAndRestore(t *testing.T) {
	s := newService(t)
	v := must[ManagedView](t)(s.EditPackages("web", PackageEdit{Add: []string{"htop"}, Exclude: []string{"perl", "nano", "python3.13-requests"}}))
	if !reflect.DeepEqual(v.RemovedPackages, []string{"nano", "perl", "python3.13-requests"}) || !reflect.DeepEqual(v.Packages, []string{"htop"}) {
		t.Fatalf("after exclude %+v", v)
	}
	// Replace: one write takes htop out and puts btop in.
	v = must[ManagedView](t)(s.EditPackages("web", PackageEdit{Remove: []string{"htop"}, Add: []string{"btop"}, Restore: []string{"nano", "python3.13-requests"}}))
	if !reflect.DeepEqual(v.RemovedPackages, []string{"perl"}) || !reflect.DeepEqual(v.Packages, []string{"btop"}) {
		t.Fatalf("after replace and restore %+v", v)
	}
	if c, _, _ := s.Read("hosts/web/managed.json"); !strings.Contains(c, "\"removedPackages\": [\n    \"perl\"\n  ]") {
		t.Fatalf("file:\n%s", c)
	}
	// Adding a removed name back clears its removal; a replacement of another name keeps it.
	v = must[ManagedView](t)(s.EditPackages("web", PackageEdit{Exclude: []string{"hello", "nodejs"}, Add: []string{"nodejs_24"}}))
	v = must[ManagedView](t)(s.EditPackages("web", PackageEdit{Add: []string{"hello"}}))
	if !reflect.DeepEqual(v.RemovedPackages, []string{"nodejs", "perl"}) || !slices.Contains(v.Packages, "hello") {
		t.Fatalf("after adding hello back %+v", v)
	}
	must[ManagedView](t)(s.EditPackages("web", PackageEdit{Restore: []string{"perl", "nodejs"}}))
	if c, _, _ := s.Read("hosts/web/managed.json"); strings.Contains(c, "removedPackages") {
		t.Fatalf("an empty removedPackages stays in the file:\n%s", c)
	}
	for _, bad := range []PackageEdit{{Exclude: []string{"a b"}}, {Restore: []string{""}}, {Exclude: []string{"../x"}}, {User: "Not A User", Add: []string{"git"}}} {
		_, err := s.EditPackages("web", bad)
		isErr(t, err, ErrInvalid)
	}
}

func TestManagedHomeManager(t *testing.T) {
	s := newService(t)
	v := must[ManagedView](t)(s.SetHome("web", HomeEdit{User: "pablo", Enable: true, StateVersion: "26.05"}))
	hm := v.Settings["home-manager"].(map[string]any)["users"].(map[string]any)["pablo"].(map[string]any)["home"].(map[string]any)
	if _, ok := v.Home["pablo"]; !ok || hm["stateVersion"] != "26.05" {
		t.Fatalf("after enable %+v", v)
	}
	must[ManagedView](t)(s.SetOption("web", []string{"home-manager", "users", "pablo", "programs", "git", "enable"}, true, nil))
	v = must[ManagedView](t)(s.EditPackages("web", PackageEdit{User: "pablo", Add: []string{"ripgrep"}, Exclude: []string{"htop"}}))
	if want := (HomeManaged{Packages: []string{"ripgrep"}, RemovedPackages: []string{"htop"}}); !reflect.DeepEqual(v.Home["pablo"], want) {
		t.Fatalf("home %+v", v.Home)
	}
	// A user with home-manager by hand gets an entry with the first package, none for nothing.
	v = must[ManagedView](t)(s.EditPackages("web", PackageEdit{User: "anna", Remove: []string{"git"}}))
	if _, ok := v.Home["anna"]; ok {
		t.Fatalf("an empty edit made anna a home-manager user: %+v", v.Home)
	}
	v = must[ManagedView](t)(s.EditPackages("web", PackageEdit{User: "anna", Add: []string{"git"}}))
	if !reflect.DeepEqual(v.Home["anna"].Packages, []string{"git"}) {
		t.Fatalf("anna %+v", v.Home)
	}
	// Off: the user and everything set here for them go; others stay.
	v = must[ManagedView](t)(s.SetHome("web", HomeEdit{User: "pablo"}))
	if _, ok := v.Home["pablo"]; ok || v.Settings["home-manager"] != nil || v.Home["anna"].Packages == nil {
		t.Fatalf("after disable %+v", v)
	}
	for _, bad := range []HomeEdit{{User: "Pablo", Enable: true}, {User: "x", Enable: true, StateVersion: "26"}, {User: "x", StateVersion: "26.05"}} {
		_, err := s.SetHome("web", bad)
		isErr(t, err, ErrInvalid)
	}
}

// A secret linked into a home survives other edits, an empty map leaves the file,
// and only a path inside ~ and a secret name are taken.
func TestManagedHomeSecrets(t *testing.T) {
	s := newService(t)
	edit := must[ManagedView](t)
	edit(s.SetHome("web", HomeEdit{User: "pablo", Enable: true, StateVersion: "26.05"}))
	edit(s.LinkHomeSecret("web", HomeSecretEdit{User: "pablo", Path: ".netrc", Secret: "netrc"}))
	edit(s.LinkHomeSecret("web", HomeSecretEdit{User: "pablo", Path: ".config/rclone/rclone.conf", Secret: "rclone"}))
	v := edit(s.EditPackages("web", PackageEdit{User: "pablo", Add: []string{"ripgrep"}}))
	if want := map[string]string{".netrc": "netrc", ".config/rclone/rclone.conf": "rclone"}; !reflect.DeepEqual(v.Home["pablo"].Secrets, want) {
		t.Fatalf("secrets after a package edit %+v", v.Home["pablo"])
	}
	if c, _, _ := s.Read("hosts/web/managed.json"); !strings.Contains(c, `".netrc": "netrc"`) {
		t.Fatalf("file:\n%s", c)
	}
	// Every machine: modules/managed.json gets the user's entry with the first link.
	if v := edit(s.LinkHomeSecret(SharedHost, HomeSecretEdit{User: "pablo", Path: ".netrc", Secret: "netrc"})); v.Home["pablo"].Secrets[".netrc"] != "netrc" {
		t.Fatalf("shared %+v", v.Home)
	}
	edit(s.LinkHomeSecret("web", HomeSecretEdit{User: "pablo", Path: ".netrc"}))
	edit(s.LinkHomeSecret("web", HomeSecretEdit{User: "pablo", Path: ".config/rclone/rclone.conf"}))
	if c, _, _ := s.Read("hosts/web/managed.json"); strings.Contains(c, "secrets") || !strings.Contains(c, "ripgrep") {
		t.Fatalf("after unlinking both:\n%s", c)
	}
	for _, bad := range []HomeSecretEdit{
		{User: "pablo", Path: "/etc/passwd", Secret: "x"}, {User: "pablo", Path: "../x", Secret: "x"}, {User: "pablo", Path: "a//b", Secret: "x"},
		{User: "pablo", Path: "", Secret: "x"}, {User: "pablo", Path: ".netrc", Secret: "a b"}, {User: "pablo", Path: "a\nb", Secret: "x"},
		{User: "Pablo", Path: ".netrc", Secret: "x"},
	} {
		_, err := s.LinkHomeSecret("web", bad)
		isErr(t, err, ErrInvalid)
	}
	// Written by hand, a bad link is refused when read, as any other value.
	write(t, s.Repo, "hosts/db/managed.json", `{"home": {"pablo": {"packages": [], "removedPackages": [], "secrets": {"../x": "netrc"}}}}`)
	_, err := s.ManagedView("db")
	isErr(t, err, ErrInvalid)
}

func TestPackageOverrides(t *testing.T) {
	s := newService(t)
	o := PackageOverride{
		Override:      map[string]any{"sensorsSupport": false},
		OverrideAttrs: map[string]any{"doCheck": false},
		Version:       "3.4.1",
		Src:           &PackageSource{GitHub: "htop-dev/htop", Rev: "3.4.1"},
		Patches:       []PackagePatch{{File: "patches/htop.patch"}, {URL: "https://example.org/fix.patch", Hash: "sha256-AAAA="}},
	}
	v := must[ManagedView](t)(s.SetPackageOverride("web", "htop", &o, nil))
	got := must[ManagedView](t)(s.ManagedView("web"))
	if !reflect.DeepEqual(got.PackageOverrides["htop"], v.PackageOverrides["htop"]) || got.PackageOverrides["htop"].Src.GitHub != "htop-dev/htop" {
		t.Fatalf("round trip %+v", got.PackageOverrides)
	}
	if c, _, _ := s.Read("hosts/web/managed.json"); !strings.Contains(c, `"sensorsSupport": false`) || !strings.Contains(c, `"file": "patches/htop.patch"`) {
		t.Fatalf("file:\n%s", c)
	}
	bad := []PackageOverride{
		{Src: &PackageSource{URL: "https://x/a.tar.gz", GitHub: "a/b", Rev: "1"}},
		{Src: &PackageSource{URL: "file:///etc/passwd"}},
		{Src: &PackageSource{GitHub: "a/b"}},
		{Src: &PackageSource{GitHub: "not a repo", Rev: "1"}},
		{Src: &PackageSource{URL: "https://x/a.tar.gz", Hash: "md5-abc"}},
		{Patches: []PackagePatch{{File: "../../modules/x.patch"}}},
		{Patches: []PackagePatch{{File: "/etc/x.patch"}}},
		{Patches: []PackagePatch{{File: "a.patch", Hash: "sha256-AAAA="}}},
		{Patches: []PackagePatch{{}}},
		{Override: map[string]any{"": 1}},
		{Version: "1\n2"},
		{From: "nixpkgs unstable"},
		{From: "../nixpkgs"},
	}
	for _, b := range bad {
		_, err := s.SetPackageOverride("web", "htop", &b, nil)
		isErr(t, err, ErrInvalid)
	}
	_, err := s.SetPackageOverride("web", "not valid", &o, nil)
	isErr(t, err, ErrInvalid)
	// A stale form cannot overwrite a newer change; an empty override puts the package back.
	_, err = s.SetPackageOverride("web", "htop", nil, &got.Hash)
	ok(t, err)
	_, err = s.SetPackageOverride("web", "htop", &o, &got.Hash)
	isErr(t, err, ErrConflict)
	must[ManagedView](t)(s.SetPackageOverride("web", "btop", &o, nil))
	v = must[ManagedView](t)(s.SetPackageOverride("web", "btop", &PackageOverride{}, nil))
	if c, _, _ := s.Read("hosts/web/managed.json"); len(v.PackageOverrides) != 0 || strings.Contains(c, "packageOverrides") {
		t.Fatalf("an empty override stays:\n%s", c)
	}
	// Another nixpkgs alone is a change: the package comes from that flake input.
	must[ManagedView](t)(s.SetPackageOverride("web", "firefox", &PackageOverride{From: "nixpkgs-unstable"}, nil))
	if c, _, _ := s.Read("hosts/web/managed.json"); !strings.Contains(c, `"firefox": {
      "from": "nixpkgs-unstable"
    }`) {
		t.Fatalf("file:\n%s", c)
	}
}

// Keys managed.nix would not read are refused, so they cannot be lost on the next write.
func TestManagedRefusesUnknownOverrideFields(t *testing.T) {
	s := newService(t)
	for _, doc := range []string{
		`{"packageOverrides": {"htop": {"overide": {}}}}`,
		`{"packageOverrides": {"htop": {"src": {"url": "ftp://x"}}}}`,
		`{"home": {"pablo": {"pkgs": []}}}`,
		`{"removedPackages": "perl"}`,
	} {
		write(t, s.Repo, "hosts/web/managed.json", doc)
		_, err := s.ManagedView("web")
		isErr(t, err, ErrInvalid)
	}
	write(t, s.Repo, "hosts/web/managed.json", `{"packageOverrides": {"htop": {"override": {"jobs": 12345678901234567890}}}}`)
	must[ManagedView](t)(s.AddPackages("web", []string{"git"}))
	if c, _, _ := s.Read("hosts/web/managed.json"); !strings.Contains(c, "12345678901234567890") {
		t.Fatalf("a number lost its digits:\n%s", c)
	}
}

// SharedHost edits modules/managed.json with the same calls, and a machine's
// own file stays untouched; home-manager is still turned on per machine.
func TestManagedSharedByEveryMachine(t *testing.T) {
	s := newService(t)
	v := must[ManagedView](t)(s.ManagedView(SharedHost))
	if v.Exists || v.Path != "modules/managed.json" || v.Host != SharedHost {
		t.Fatalf("view of a missing shared file %+v", v)
	}
	edit := must[ManagedView](t)
	edit(s.EditPackages(SharedHost, PackageEdit{Add: []string{"htop"}, Exclude: []string{"perl"}}))
	edit(s.EditPackages(SharedHost, PackageEdit{User: "pablo", Add: []string{"ripgrep"}}))
	edit(s.SetPackageOverride(SharedHost, "htop", &PackageOverride{Patches: []PackagePatch{{File: "patches/htop.patch"}}}, nil))
	v = edit(s.SetOption(SharedHost, []string{"home-manager", "users", "pablo", "programs", "git", "enable"}, true, nil))
	if !reflect.DeepEqual(v.Packages, []string{"htop"}) || !reflect.DeepEqual(v.RemovedPackages, []string{"perl"}) ||
		!reflect.DeepEqual(v.Home["pablo"].Packages, []string{"ripgrep"}) || v.PackageOverrides["htop"].Patches[0].File != "patches/htop.patch" {
		t.Fatalf("shared view %+v", v)
	}
	if c, _, err := s.Read("modules/managed.json"); err != nil || !strings.Contains(c, `"perl"`) || !strings.Contains(c, `"ripgrep"`) {
		t.Fatalf("modules/managed.json %q %v", c, err)
	}
	if _, _, err := s.Read("hosts/web/managed.json"); err == nil {
		t.Fatal("a shared edit wrote a machine's managed.json")
	}
	_, err := s.SetHome(SharedHost, HomeEdit{User: "pablo", Enable: true, StateVersion: "26.05"})
	isErr(t, err, ErrInvalid)
	// The name stands only for the shared file: no folder of that name is read or written.
	_, err = s.ManagedView("**")
	isErr(t, err, ErrInvalid)
}

func TestUpdateManagedModule(t *testing.T) {
	s := newService(t)
	must[string](t)(s.UpdateManagedModule("", false)) // a repository without one gets it
	write(t, s.Repo, "modules/managed.nix", "# an older one\nfile: { ... }: { }\n")
	_, err := s.UpdateManagedModule("", false)
	isErr(t, err, ErrExists)
	_, err = s.UpdateManagedModule("stale", false)
	isErr(t, err, ErrConflict)
	_, cur, _ := s.Read("modules/managed.nix")
	must[string](t)(s.UpdateManagedModule(cur, false))
	want, _ := fs.ReadFile(templates.Fleet, "modules/managed.nix")
	if c, _, _ := s.Read("modules/managed.nix"); c != string(want) || !strings.Contains(c, "nixosManager.version") {
		t.Fatalf("not the template:\n%s", c)
	}
}

// A standalone home is edited as home:<name>, in homes/<name>/managed.json: its own
// packages, removed packages and home-manager options at the top, as a machine's.
func TestManagedStandaloneHome(t *testing.T) {
	s := newService(t)
	const h = HomePrefix + "pablo@laptop"
	v := must[ManagedView](t)(s.EditPackages(h, PackageEdit{Add: []string{"ripgrep"}, Exclude: []string{"nano"}}))
	if v.Path != "homes/pablo@laptop/managed.json" || !reflect.DeepEqual(v.Packages, []string{"ripgrep"}) || !reflect.DeepEqual(v.RemovedPackages, []string{"nano"}) {
		t.Fatalf("view %+v", v)
	}
	must[ManagedView](t)(s.SetOption(h, []string{"programs", "git", "enable"}, true, nil))
	content, _, _ := s.Read("homes/pablo@laptop/managed.json")
	want := "{\n  \"packages\": [\n    \"ripgrep\"\n  ],\n  \"removedPackages\": [\n    \"nano\"\n  ],\n  \"settings\": {\n    \"programs\": {\n      \"git\": {\n        \"enable\": true\n      }\n    }\n  }\n}\n"
	if content != want {
		t.Fatalf("file content\n%s\nwant\n%s", content, want)
	}

	// Users, secrets and package overrides are a NixOS machine's.
	_, err := s.EditPackages(h, PackageEdit{User: "pablo", Add: []string{"jq"}})
	isErr(t, err, ErrInvalid)
	_, err = s.SetHome(h, HomeEdit{User: "pablo", Enable: true})
	isErr(t, err, ErrInvalid)
	_, err = s.LinkHomeSecret(h, HomeSecretEdit{User: "pablo", Path: ".netrc", Secret: "netrc"})
	isErr(t, err, ErrInvalid)
	_, err = s.SetPackageOverride(h, "htop", &PackageOverride{Version: "1"}, nil)
	isErr(t, err, ErrInvalid)
	_, err = s.Copy("workstation", CopyRequest{Home: "pablo", To: []string{h}})
	isErr(t, err, ErrInvalid)
	write(t, s.Repo, "homes/pablo@laptop/managed.json", `{"packages": [], "home": {"pablo": {"packages": []}}}`)
	_, err = s.ManagedView(h)
	isErr(t, err, ErrInvalid)

	for _, bad := range []string{HomePrefix, HomePrefix + "../x", HomePrefix + "a/b", HomePrefix + ".hidden", "pablo@laptop"} {
		if _, err := s.ManagedView(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
		if validTarget(bad) {
			t.Errorf("validTarget(%q)", bad)
		}
	}
	if !validTarget(h) || !validTarget("workstation") {
		t.Fatal("validTarget refuses a home or a machine")
	}
}

func TestUpdateManagedHomeModule(t *testing.T) {
	s := newService(t)
	must[string](t)(s.UpdateManagedModule("", true))
	want, _ := fs.ReadFile(templates.Fleet, "modules/managed-home.nix")
	if c, _, _ := s.Read("modules/managed-home.nix"); c != string(want) || !strings.Contains(c, "nixosManager.managedFile") {
		t.Fatalf("not the template:\n%s", c)
	}
	if _, _, err := s.Read("modules/managed.nix"); err == nil {
		t.Fatal("managed.nix was written too")
	}
}
