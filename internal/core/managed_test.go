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

func TestUpdateManagedModule(t *testing.T) {
	s := newService(t)
	must[string](t)(s.UpdateManagedModule("")) // a repository without one gets it
	write(t, s.Repo, "modules/managed.nix", "# an older one\nfile: { ... }: { }\n")
	_, err := s.UpdateManagedModule("")
	isErr(t, err, ErrExists)
	_, err = s.UpdateManagedModule("stale")
	isErr(t, err, ErrConflict)
	_, cur, _ := s.Read("modules/managed.nix")
	must[string](t)(s.UpdateManagedModule(cur))
	want, _ := fs.ReadFile(templates.Fleet, "modules/managed.nix")
	if c, _, _ := s.Read("modules/managed.nix"); c != string(want) || !strings.Contains(c, "nixosManager.version") {
		t.Fatalf("not the template:\n%s", c)
	}
}
