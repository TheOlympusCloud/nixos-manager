package core

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseInputs(t *testing.T) {
	lock := []byte(`{
  "nodes": {
    "root": { "inputs": { "nixpkgs": "nixpkgs", "home-manager": "home-manager", "comin": "comin", "hm-nixpkgs": ["home-manager", "nixpkgs"] } },
    "nixpkgs": { "locked": { "type": "github", "owner": "NixOS", "repo": "nixpkgs", "rev": "abc123", "lastModified": 1731900000 },
                 "original": { "type": "github", "owner": "NixOS", "repo": "nixpkgs", "ref": "nixos-26.05" } },
    "home-manager": { "locked": { "type": "github", "owner": "nix-community", "repo": "home-manager", "rev": "def456", "lastModified": 1731800000 } },
    "comin": { "locked": { "type": "git", "url": "https://forgejo.lan/pablo/comin.git", "rev": "0011", "lastModified": 1731700000 } }
  },
  "root": "root",
  "version": 7
}`)
	got, err := parseInputs(lock)
	if err != nil {
		t.Fatal(err)
	}
	want := []Input{
		{Name: "comin", Type: "git", URL: "https://forgejo.lan/pablo/comin.git", Rev: "0011", LastModified: 1731700000},
		{Name: "home-manager", Type: "github", URL: "github:nix-community/home-manager", Rev: "def456", LastModified: 1731800000},
		{Name: "nixpkgs", Type: "github", URL: "github:NixOS/nixpkgs/nixos-26.05", Rev: "abc123", LastModified: 1731900000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inputs\n got %+v\nwant %+v", got, want)
	}
	if in, err := parseInputs(nil); err != nil || len(in) != 0 {
		t.Fatalf("no flake.lock: %v %v", in, err)
	}
	if _, err := parseInputs([]byte("{not json")); err == nil {
		t.Fatal("broken flake.lock accepted")
	}
}

func host(name string, pkgs []string, features []string, tcp ...int) Host {
	h := Host{Name: name, Ports: Ports{TCP: tcp}}
	for _, p := range pkgs {
		h.Packages = append(h.Packages, Package{Name: p})
	}
	for _, f := range features {
		h.Features = append(h.Features, Feature{Group: "services", Name: f})
	}
	return h
}

func TestDiffModels(t *testing.T) {
	head := &Model{Hosts: []Host{
		host("server", []string{"git", "htop"}, []string{"openssh"}, 22),
		host("old", nil, nil),
		host("laptop", []string{"firefox"}, nil),
	}}
	work := &Model{Hosts: []Host{
		host("server", []string{"git", "btop"}, []string{"openssh", "jellyfin"}, 22, 8096),
		host("laptop", []string{"firefox"}, nil),
		host("pi", []string{"vim"}, nil),
	}}
	work.Hosts[1].Users = []User{{Name: "pablo"}}
	got := DiffModels(head, work)
	want := []HostDiff{
		{Host: "laptop", Status: "changed", UsersAdded: []string{"pablo"}},
		{Host: "old", Status: "removed"},
		{Host: "pi", Status: "added", PackagesAdded: []string{"vim"}},
		{Host: "server", Status: "changed", PackagesAdded: []string{"btop"}, PackagesRemoved: []string{"htop"},
			FeaturesAdded: []string{"services.jellyfin"}, PortsAdded: []string{"tcp/8096"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got %+v\nwant %+v", got, want)
	}
	broken := &Model{Hosts: []Host{{Name: "server", Error: "infinite recursion"}}}
	if d := DiffModels(head, broken); d[len(d)-1].Status != "error" || d[len(d)-1].Host != "server" {
		t.Fatalf("a machine that no longer evaluates: %+v", d)
	}
	same := DiffModels(head, head)
	for _, d := range same {
		if d.Status != "unchanged" {
			t.Fatalf("diff of a model with itself: %+v", d)
		}
	}
}

func TestDiffModelsReportsVersionChanges(t *testing.T) {
	head := &Model{Hosts: []Host{{Name: "web", Packages: []Package{{Name: "firefox", Version: "145.0"}, {Name: "git", Version: "2.51.0"}}}}}
	work := &Model{Hosts: []Host{{Name: "web", Packages: []Package{{Name: "firefox", Version: "146.0"}, {Name: "git", Version: "2.51.0"}}}}}
	d := DiffModels(head, work)[0]
	if d.Status != "changed" || !reflect.DeepEqual(d.VersionsChanged, []VersionChange{{Name: "firefox", From: "145.0", To: "146.0"}}) {
		t.Fatalf("version change %+v", d)
	}
}

func TestDiffInputs(t *testing.T) {
	head := []Input{
		{Name: "nixpkgs", Type: "github", URL: "github:NixOS/nixpkgs/nixos-26.05", Rev: "aaa", LastModified: 100},
		{Name: "home-manager", Type: "github", URL: "github:nix-community/home-manager", Rev: "hm1", LastModified: 50},
		{Name: "old", Type: "git", URL: "https://x/old.git", Rev: "o1"},
	}
	work := []Input{
		{Name: "nixpkgs", Type: "github", URL: "github:NixOS/nixpkgs/nixos-26.05", Rev: "bbb", LastModified: 200},
		{Name: "home-manager", Type: "github", URL: "github:nix-community/home-manager", Rev: "hm1", LastModified: 50},
		{Name: "sops-nix", Type: "github", URL: "github:Mic92/sops-nix", Rev: "s1", LastModified: 300},
	}
	got := DiffInputs(head, work)
	want := []InputChange{
		{Name: "nixpkgs", Status: "updated", FromRev: "aaa", ToRev: "bbb", FromDate: 100, ToDate: 200, CompareURL: "https://github.com/NixOS/nixpkgs/compare/aaa...bbb"},
		{Name: "old", Status: "removed", FromRev: "o1"},
		{Name: "sops-nix", Status: "added", ToRev: "s1", ToDate: 300},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input changes\n got %+v\nwant %+v", got, want)
	}
}

func TestVersionChangesWithSeveralVersionsOfAName(t *testing.T) {
	pkgs := func(nv ...string) []Package {
		var out []Package
		for i := 0; i < len(nv); i += 2 {
			out = append(out, Package{Name: nv[i], Version: nv[i+1]})
		}
		return out
	}
	before := pkgs("python3", "3.12.8", "python3", "3.13.1", "curl", "8.16.0")
	if got := versionChanges(before, pkgs("python3", "3.13.1", "python3", "3.12.8", "curl", "8.16.0")); len(got) != 0 {
		t.Fatalf("same versions in another order: %+v", got)
	}
	got := versionChanges(before, pkgs("python3", "3.12.9", "python3", "3.13.1", "curl", "8.17.0"))
	want := []VersionChange{{Name: "curl", From: "8.16.0", To: "8.17.0"}, {Name: "python3", From: "3.12.8, 3.13.1", To: "3.12.9, 3.13.1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

// The lists show what is easy to name; the system derivation shows that anything
// changed at all: a unit, an /etc file, a sysctl. The kernel, initrd and kernel
// parameters take effect only after a reboot.
func TestDiffModelsSeesEveryChangeOfTheSystem(t *testing.T) {
	sys := func(name, toplevel, boot, kernel string) Host {
		h := Host{Name: name, Toplevel: "/nix/store/" + toplevel + "-nixos-system-" + name + ".drv", Boot: boot}
		if kernel != "" {
			h.Kernel = &kernel
		}
		return h
	}
	head := &Model{Hosts: []Host{
		sys("web", "a1", "k1 i1", "6.12.1"),
		sys("kern", "a2", "k1 i1", "6.12.1"),
		sys("initrd", "a3", "k1 i1", "6.12.1"),
		sys("ct", "a4", "", "6.12.1"),
		sys("same", "a5", "k1 i1", "6.12.1"),
		{Name: "broken", Error: "infinite recursion"},
	}}
	work := &Model{Hosts: []Host{
		sys("web", "b1", "k1 i1", "6.12.1"),
		sys("kern", "b2", "k2 i2", "6.12.2"),
		sys("initrd", "b3", "k1 i2", "6.12.1"),
		sys("ct", "b4", "", "6.12.2"), // a container runs its host's kernel
		sys("same", "a5", "k1 i1", "6.12.1"),
		sys("broken", "b6", "k1 i1", "6.12.1"),
		sys("new", "b7", "k1 i1", "6.12.1"),
	}}
	got := DiffModels(head, work)
	want := []HostDiff{
		{Host: "broken", Status: "changed"}, // its last commit did not evaluate: nothing to say about a reboot
		{Host: "ct", Status: "changed"},
		{Host: "initrd", Status: "changed", Reboot: true},
		{Host: "kern", Status: "changed", Reboot: true, Kernel: &VersionChange{Name: "kernel", From: "6.12.1", To: "6.12.2"}},
		{Host: "new", Status: "added"},
		{Host: "same", Status: "unchanged"},
		{Host: "web", Status: "changed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff\n got %+v\nwant %+v", got, want)
	}
}

// What switch-to-configuration does to each changed unit, if it runs, and
// which /etc files change: from the evaluated units and /etc entries.
func TestDiffModelsListsUnitsAndEtc(t *testing.T) {
	u := func(hash string, kv ...string) UnitInfo {
		i := UnitInfo{Hash: hash, Reload: map[string]string{}, Flags: map[string]string{}}
		for n := 0; n < len(kv); n += 2 {
			if strings.HasPrefix(kv[n], "flag:") {
				i.Flags[strings.TrimPrefix(kv[n], "flag:")] = kv[n+1]
			} else {
				i.Reload[kv[n]] = kv[n+1]
			}
		}
		return i
	}
	before := map[string]UnitInfo{
		"nginx.service":               u("a"),
		"phpfpm.service":              u("a"),
		"dhcpcd.service":              u("a"),
		"journald.service":            u("a"),
		"explicit.service":            u("a"),
		"data.mount":                  u("a"),
		"-.mount":                     u("a"),
		"nix.mount":                   u("a"),
		"sshd.socket":                 u("a"),
		"multi-user.target":           u("a"),
		"machine.slice":               u("a"),
		"watch.path":                  u("a"),
		"caddy.service":               u("a", "Unit/X-Reload-Triggers", "r1"),
		"trigger-added.service":       u("a"),
		"trigger-removed.service":     u("a", "Unit/X-Reload-Triggers", "r1"),
		"execreload-removed.service":  u("a", "Service/ExecReload", "e1"),
		"execreload-changed.service":  u("a", "Service/ExecReload", "e1"),
		"opts.mount":                  u("a", "Mount/Options", "o1"),
		"opts-added.mount":            u("a"),
		"nix-opts-added.mount":        u("a"),
		"same.service":                u("a", "Unit/X-Reload-Triggers", "r1"),
		"old.service":                 u("a"),
		"kept.service":                u("a", "flag:Unit/X-StopOnRemoval", "no"),
		"getty@.service":              u("a"),
		"reload-flag-trigger.service": u("a", "Unit/X-Reload-Triggers", "r1"),
	}
	after := map[string]UnitInfo{
		"nginx.service":               u("b"),
		"phpfpm.service":              u("b", "flag:Service/X-ReloadIfChanged", "true"),
		"dhcpcd.service":              u("b", "flag:Service/X-RestartIfChanged", "false"),
		"journald.service":            u("b", "flag:Unit/RefuseManualStop", "yes"),
		"explicit.service":            u("b", "flag:Service/X-RestartIfChanged", "on", "flag:Unit/X-OnlyManualStart", "0"),
		"data.mount":                  u("b"),
		"-.mount":                     u("b"),
		"nix.mount":                   u("b"),
		"sshd.socket":                 u("b"),
		"multi-user.target":           u("b"),
		"machine.slice":               u("b"),
		"watch.path":                  u("b"),
		"caddy.service":               u("a", "Unit/X-Reload-Triggers", "r2"),
		"trigger-added.service":       u("a", "Unit/X-Reload-Triggers", "r1"),
		"trigger-removed.service":     u("a"),
		"execreload-removed.service":  u("a"),
		"execreload-changed.service":  u("a", "Service/ExecReload", "e2"),
		"opts.mount":                  u("a", "Mount/Options", "o2"),
		"opts-added.mount":            u("a", "Mount/Options", "o1"),
		"nix-opts-added.mount":        u("a", "Mount/Options", "o1"),
		"same.service":                u("a", "Unit/X-Reload-Triggers", "r1"),
		"new.service":                 u("a"),
		"new.target":                  u("a"),
		"getty@.service":              u("b"),
		"reload-flag-trigger.service": u("a", "Unit/X-Reload-Triggers", "r2", "flag:Service/X-RestartIfChanged", "false"),
	}
	// A mount named nix-opts-added.mount is not /nix: only -.mount and nix.mount are reloaded.
	head := &Model{Hosts: []Host{{Name: "web", Toplevel: "/nix/store/a.drv", Detail: &HostDetail{Units: before,
		Etc: map[string]string{"hosts": "h1", "ssh/sshd_config": "s1", "old.conf": "o1"}}}}}
	work := &Model{Hosts: []Host{{Name: "web", Toplevel: "/nix/store/b.drv", Detail: &HostDetail{Units: after,
		Etc: map[string]string{"hosts": "h2", "ssh/sshd_config": "s1", "new.conf": "n1"}}}}}
	d := DiffModels(head, work)[0]
	want := []UnitChange{
		{Name: "-.mount", Action: "reload"},
		{Name: "caddy.service", Action: "reload"},
		{Name: "data.mount", Action: "restart"},
		{Name: "dhcpcd.service", Action: "keep"},
		{Name: "execreload-changed.service", Action: "reload"},
		{Name: "explicit.service", Action: "restart"},
		{Name: "getty@.service", Action: "restart"},
		{Name: "journald.service", Action: "keep"},
		{Name: "kept.service", Action: "keep"}, // removed, but X-StopOnRemoval=false: it keeps running
		{Name: "new.service", Action: "start"},
		{Name: "new.target", Action: "start"},
		{Name: "nginx.service", Action: "restart"},
		{Name: "nix-opts-added.mount", Action: "restart"},
		{Name: "nix.mount", Action: "reload"},
		{Name: "old.service", Action: "stop"},
		{Name: "opts-added.mount", Action: "restart"},
		{Name: "opts.mount", Action: "reload"},
		{Name: "phpfpm.service", Action: "reload"},
		{Name: "reload-flag-trigger.service", Action: "reload"},
		{Name: "sshd.socket", Action: "keep"},
		{Name: "trigger-added.service", Action: "reload"},
	}
	if !reflect.DeepEqual(d.Units, want) {
		t.Errorf("units\n got %+v\nwant %+v", d.Units, want)
	}
	wantEtc := []EtcChange{{Path: "/etc/hosts", Status: "changed"}, {Path: "/etc/new.conf", Status: "added"}, {Path: "/etc/old.conf", Status: "removed"}}
	if !reflect.DeepEqual(d.Etc, wantEtc) {
		t.Errorf("etc\n got %+v\nwant %+v", d.Etc, wantEtc)
	}
	if d.Status != "changed" {
		t.Errorf("status %s", d.Status)
	}

	// Without what to compare (an older evaluation, a new or broken machine) nothing is listed.
	unknown := &Model{Hosts: []Host{{Name: "web", Toplevel: "/nix/store/a.drv"}}}
	if d := DiffModels(unknown, work)[0]; d.Units != nil || d.Etc != nil {
		t.Errorf("no detail before: %+v", d)
	}
	if d := DiffModels(&Model{}, work)[0]; d.Status != "added" || d.Units != nil || d.Etc != nil {
		t.Errorf("a new machine lists no unit: %+v", d)
	}
	noUnits := &Model{Hosts: []Host{{Name: "web", Toplevel: "/nix/store/a.drv", Detail: &HostDetail{Etc: map[string]string{"hosts": "h1"}}}}}
	if d := DiffModels(noUnits, work)[0]; d.Units != nil || len(d.Etc) == 0 {
		t.Errorf("units unknown before, /etc known: %+v", d)
	}
}

// Home-manager users show in NixOS terms too: packages, programs and the files in ~.
func TestDiffModelsListsHomeManager(t *testing.T) {
	user := func(name string, pkgs, progs []string) User {
		return User{Name: name, Home: &HomeInfo{Packages: pkgs, Programs: progs}}
	}
	head := &Model{Hosts: []Host{{
		Name: "pc", Toplevel: "a",
		Users:  []User{user("pablo", []string{"htop", "git"}, []string{"git"}), {Name: "guest"}},
		Detail: &HostDetail{HomeFiles: map[string]map[string]string{"pablo": {".bashrc": "1", ".config/git/config": "1"}}},
	}}}
	working := &Model{Hosts: []Host{{
		Name: "pc", Toplevel: "b",
		Users: []User{user("pablo", []string{"git", "ripgrep"}, []string{"git", "starship"}), user("anna", []string{"htop"}, nil)},
		Detail: &HostDetail{HomeFiles: map[string]map[string]string{
			"pablo": {".bashrc": "1", ".config/git/config": "2", ".config/starship.toml": "1"},
			"anna":  {".profile": "1"},
		}},
	}}}
	d := DiffModels(head, working)[0]
	if d.Status != "changed" ||
		!reflect.DeepEqual(d.HomePackagesAdded, []string{"anna: htop", "pablo: ripgrep"}) ||
		!reflect.DeepEqual(d.HomePackagesRemoved, []string{"pablo: htop"}) ||
		!reflect.DeepEqual(d.HomeProgramsAdded, []string{"pablo: starship"}) || d.HomeProgramsRemoved != nil {
		t.Fatalf("diff %+v", d)
	}
	want := []EtcChange{{"anna: ~/.profile", "added"}, {"pablo: ~/.config/git/config", "changed"}, {"pablo: ~/.config/starship.toml", "added"}}
	if !reflect.DeepEqual(d.HomeFiles, want) {
		t.Fatalf("home files %+v", d.HomeFiles)
	}
	// Only a home package moving is a change even when the system derivation is the same.
	working.Hosts[0].Toplevel = "a"
	working.Hosts[0].Users = []User{user("pablo", []string{"htop"}, []string{"git"})}
	if d := DiffModels(head, working)[0]; d.Status != "changed" || !reflect.DeepEqual(d.HomePackagesRemoved, []string{"pablo: git"}) {
		t.Fatalf("diff %+v", d)
	}
}
