package core

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/templates"
)

// A user's own home-manager module (custom.nix next to managed.json) declares an
// option: search, the option itself and its value on the machine all find it under
// home-manager.users.<user>, though home-manager's own option list does not have it.
func TestOwnHomeOptionsWithRealNix(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	ctx := context.Background()
	s := NewService(newRepo(t))
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "ct", System: "x86_64-linux", Role: "minimal", Platform: "proxmox-lxc",
		User: &MachineUser{Name: "pablo", Home: true}}))
	write(t, s.Repo, "hosts/ct/home/pablo/custom.nix", `{ lib, ... }:
{
  options.olympus.example.enable = lib.mkEnableOption "the example app";
  config.olympus.example.enable = true;
}
`)
	lock := must[*Job](t)(s.Jobs.Submit(ctx, JobRequest{Kind: "lock"}))
	if info := wait40(t, lock); info.State != "passed" {
		t.Fatalf("lock: %s", info.Output)
	}
	name := "home-manager.users.pablo.olympus.example.enable"
	hits, j, err := s.Jobs.SearchOptions(ctx, "ct", "olympus", 50)
	ok(t, err)
	if hits == nil {
		if info := wait40(t, j); info.State != "passed" {
			t.Fatalf("options of ct: %s", info.Output)
		}
		hits, _, err = s.Jobs.SearchOptions(ctx, "ct", "olympus", 50)
		ok(t, err)
	}
	if len(hits) != 1 || hits[0].Name != name || hits[0].Type != "boolean" {
		t.Fatalf("search olympus: %+v", hits)
	}
	if hits, _, _ := s.Jobs.SearchOptions(ctx, "ct", "home-manager.users.<name>.programs.git.enable", 5); len(hits) == 0 {
		t.Fatal("home-manager's own options are gone")
	}
	res, vj, err := s.Jobs.OptionValues(ctx, []string{name, "users.users.pablo.isNormalUser"}, []string{"ct"})
	ok(t, err)
	if res == nil {
		if info := wait40(t, vj); info.State != "passed" {
			t.Fatalf("values of ct: %s", info.Output)
		}
		res, _, err = s.Jobs.OptionValues(ctx, []string{name, "users.users.pablo.isNormalUser"}, []string{"ct"})
		ok(t, err)
	}
	for _, r := range res {
		if v := r.Hosts[0]; !v.Exists || v.Value != true {
			t.Errorf("%s on ct: %+v", r.Name, v)
		}
	}
	if v := res[0].Hosts[0]; !slices.Contains(v.Files, "hosts/ct/home/pablo/custom.nix") {
		t.Errorf("%s is set in %v", name, v.Files)
	}
}
