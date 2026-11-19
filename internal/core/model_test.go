package core

import (
	"reflect"
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
