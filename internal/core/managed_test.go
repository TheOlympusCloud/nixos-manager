package core

import (
	"reflect"
	"strings"
	"testing"
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
	must[ManagedView](t)(s.SetOption("pi", []string{"services", "openssh", "enable"}, true))
	must[ManagedView](t)(s.SetOption("pi", []string{"services", "openssh", "settings", "PermitRootLogin"}, "no"))
	must[ManagedView](t)(s.SetOption("pi", []string{"services", "nginx", "virtualHosts", "home.lan", "root"}, "/srv/www"))
	must[ManagedView](t)(s.SetOption("pi", []string{"networking", "firewall", "allowedTCPPorts"}, []any{22, 8123}))
	v := must[ManagedView](t)(s.UnsetOption("pi", []string{"services", "openssh", "settings", "PermitRootLogin"}))
	ssh := v.Settings["services"].(map[string]any)["openssh"].(map[string]any)
	if _, ok := ssh["settings"]; ok || ssh["enable"] != true {
		t.Fatalf("unset did not prune the empty parent: %+v", ssh)
	}
	v = must[ManagedView](t)(s.UnsetOption("pi", []string{"services", "openssh", "enable"}))
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
	must[ManagedView](t)(s.UnsetOption("pi", []string{"services", "nope", "enable"}))
}

func TestManagedValidation(t *testing.T) {
	s := newService(t)
	_, err := s.AddPackages("../etc", []string{"git"})
	isErr(t, err, ErrInvalid)
	for _, bad := range []string{"", "a b", "../x", "git;rm", ".hidden", "a..b"} {
		_, err = s.AddPackages("web", []string{bad})
		isErr(t, err, ErrInvalid)
	}
	_, err = s.SetOption("web", nil, true)
	isErr(t, err, ErrInvalid)
	_, err = s.SetOption("web", []string{"services", ""}, true)
	isErr(t, err, ErrInvalid)
	_, err = s.SetOption("web", []string{"time", "timeZone"}, nil)
	isErr(t, err, ErrInvalid)
	must[ManagedView](t)(s.SetOption("web", []string{"time", "timeZone"}, "Europe/Madrid"))
	_, err = s.SetOption("web", []string{"time", "timeZone", "x"}, 1)
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
