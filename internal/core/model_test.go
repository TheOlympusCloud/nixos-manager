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
