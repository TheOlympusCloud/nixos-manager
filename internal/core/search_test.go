package core

import (
	"context"
	"os"
	"testing"
)

func TestNixpkgsRef(t *testing.T) {
	lock, _ := os.ReadFile("testdata/fleet/flake.lock")
	if got := nixpkgsRef(lock); got != "github:NixOS/nixpkgs/9e1f0a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f" {
		t.Fatalf("github ref %q", got)
	}
	git := []byte(`{"nodes":{"root":{"inputs":{"nixpkgs":"np"}},"np":{"locked":{"type":"git","url":"https://forgejo.lan/mirror/nixpkgs.git","rev":"abc"}}},"root":"root"}`)
	if got := nixpkgsRef(git); got != "git+https://forgejo.lan/mirror/nixpkgs.git?rev=abc" {
		t.Fatalf("git ref %q", got)
	}
	for _, none := range [][]byte{nil, []byte(`{"nodes":{"root":{"inputs":{}}},"root":"root"}`), []byte("{bad")} {
		if got := nixpkgsRef(none); got != "nixpkgs" {
			t.Fatalf("fallback ref %q", got)
		}
	}
}

func TestRankPackages(t *testing.T) {
	idx := []PackageResult{
		{Attr: "firefox-esr", Pname: "firefox-esr", Description: "Web browser (ESR)"},
		{Attr: "vimPlugins.fire", Pname: "fire", Description: "x"},
		{Attr: "librewolf", Pname: "librewolf", Description: "A fork of Firefox"},
		{Attr: "firefox", Pname: "firefox", Description: "Web browser"},
		{Attr: "python3Packages.firefox-helper", Pname: "firefox-helper", Description: "y"},
	}
	got := rankPackages(idx, "Firefox", 10)
	want := []string{"firefox", "firefox-esr", "python3Packages.firefox-helper", "librewolf"}
	if len(got) != len(want) {
		t.Fatalf("results %+v", got)
	}
	for i, w := range want {
		if got[i].Attr != w {
			t.Fatalf("rank %d = %s, want %s (all: %+v)", i, got[i].Attr, w, got)
		}
	}
	if len(rankPackages(idx, "fire", 2)) != 2 || len(rankPackages(idx, "", 10)) != 0 {
		t.Fatal("limit or empty query ignored")
	}
}

func TestRankOptions(t *testing.T) {
	str := func(s string) *string { return &s }
	idx := []OptionDoc{
		{Name: "services.openssh.settings.PermitRootLogin"},
		{Name: "services.openssh.enable", Description: str("Whether to enable the OpenSSH daemon.")},
		{Name: "programs.ssh.startAgent", Description: str("Whether to start the OpenSSH agent.")},
		{Name: "services.openssh.ports"},
	}
	got := rankOptions(idx, "services.openssh.", 10)
	if len(got) != 3 || got[0].Name != "services.openssh.enable" || got[1].Name != "services.openssh.ports" {
		t.Fatalf("prefix browse %+v", got)
	}
	if got := rankOptions(idx, "openssh agent", 10); len(got) != 1 || got[0].Name != "programs.ssh.startAgent" {
		t.Fatalf("all words must match %+v", got)
	}
}

func TestSearchIndexesBuildInTheBackground(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")

	res, job, err := s.Jobs.SearchPackages(ctx, "fire", 10)
	if err != nil || res != nil || job == nil || job.Req.Kind != "packages" {
		t.Fatalf("first package search: %v %v %v", res, job, err)
	}
	wait(t, job)
	res, job, err = s.Jobs.SearchPackages(ctx, "fire", 10)
	if err != nil || job != nil || len(res) != 2 || res[0].Attr != "firefox" || res[0].Version != "145.0" {
		t.Fatalf("package results %+v %v %v", res, job, err)
	}

	opts, job, err := s.Jobs.SearchOptions(ctx, "workstation", "services.openssh", 10)
	if err != nil || opts != nil || job == nil {
		t.Fatalf("first option search: %v %v %v", opts, job, err)
	}
	wait(t, job)
	opts, _, err = s.Jobs.SearchOptions(ctx, "workstation", "services.openssh", 10)
	if err != nil || len(opts) != 4 || opts[0].Name != "services.openssh.enable" || opts[0].Type != "boolean" {
		t.Fatalf("option results %+v %v", opts, err)
	}
	_, _, err = s.Jobs.SearchOptions(ctx, "../x", "a", 10)
	isErr(t, err, ErrInvalid)
}

func TestSearchIndexFailureIsReportedOnce(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, t.TempDir()) // no fixtures: nix search fails
	_, job, _ := s.Jobs.SearchPackages(ctx, "git", 10)
	wait(t, job)
	_, job, err := s.Jobs.SearchPackages(ctx, "git", 10)
	if err == nil || job != nil {
		t.Fatalf("a failed index must report its error, not start again: %v %v", job, err)
	}
	job, err = s.Jobs.RefreshIndex(ctx, "packages", "")
	if err != nil || job == nil {
		t.Fatalf("refresh after a failure: %v %v", job, err)
	}
}

// Options declared by the repo's own modules appear once committed, without a manual refresh.
func TestOptionsIndexFollowsCommits(t *testing.T) {
	ctx := context.Background()
	s := fleetService(t, "testdata/fleet")
	_, job, _ := s.Jobs.SearchOptions(ctx, "workstation", "openssh", 10)
	wait(t, job)
	if _, again, _ := s.Jobs.SearchOptions(ctx, "workstation", "openssh", 10); again != nil {
		t.Fatal("index rebuilt without a change")
	}
	write(t, s.Repo, "modules/mine.nix", "{ lib, ... }: { options.my.thing = lib.mkEnableOption \"thing\"; }\n")
	ok(t, s.Stage(ctx, nil))
	must[string](t)(s.Commit(ctx, "feat: my module", true))
	_, rebuild, _ := s.Jobs.SearchOptions(ctx, "workstation", "openssh", 10)
	if rebuild == nil {
		t.Fatal("a commit did not refresh the option index")
	}
	wait(t, rebuild)
	// Each commit builds a new index: only the newest one per machine is kept.
	s.Jobs.mu.Lock()
	defer s.Jobs.mu.Unlock()
	if n := len(s.Jobs.indexes); n != 1 {
		t.Fatalf("%d indexes kept for one machine, want 1", n)
	}
}
