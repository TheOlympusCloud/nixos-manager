package core

import (
	"strings"
	"testing"
)

func TestCreateMachine(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "flake.nix", "{ outputs = { nixpkgs, ... }: { nixosConfigurations = nixpkgs.lib.genAttrs (builtins.attrNames (builtins.readDir ./hosts)) mkHost; }; }\n")

	res := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "nas", System: "aarch64-linux", Role: "server"}))
	if !res.Registered || res.Snippet != "" || len(res.Files) != 3 {
		t.Fatalf("result %+v", res)
	}
	def, _, _ := s.Read("hosts/nas/default.nix")
	hw, _, _ := s.Read("hosts/nas/hardware-configuration.nix")
	if !strings.Contains(def, `networking.hostName = "nas";`) || !strings.Contains(hw, `"aarch64-linux"`) {
		t.Fatalf("files:\n%s\n%s", def, hw)
	}
	v := must[ManagedView](t)(s.ManagedView("nas"))
	ssh := v.Settings["services"].(map[string]any)["openssh"].(map[string]any)
	if ssh["enable"] != true || ssh["settings"].(map[string]any)["PasswordAuthentication"] != false {
		t.Fatalf("server preset: %+v", v.Settings)
	}

	pasted := "{ ... }: { fileSystems.\"/\".device = \"/dev/sda1\"; }\n"
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "desk", System: "x86_64-linux", Role: "desktop", Hardware: pasted}))
	if hw, _, _ := s.Read("hosts/desk/hardware-configuration.nix"); hw != pasted {
		t.Fatalf("pasted hardware config not kept: %q", hw)
	}

	_, err := s.CreateMachine(MachineSpec{Name: "nas", System: "x86_64-linux", Role: "minimal"})
	isErr(t, err, ErrExists)
	for _, bad := range []MachineSpec{
		{Name: "Bad Name", System: "x86_64-linux", Role: "minimal"},
		{Name: "ok", System: "sparc-linux", Role: "minimal"},
		{Name: "ok", System: "x86_64-linux", Role: "gaming-rig"},
		{Name: strings.Repeat("a", 64), System: "x86_64-linux", Role: "minimal"},
		{Name: "_x", System: "x86_64-linux", Role: "minimal"},
		{Name: "x-", System: "x86_64-linux", Role: "minimal"},
	} {
		_, err := s.CreateMachine(bad)
		isErr(t, err, ErrInvalid)
	}
}

func TestCreateMachineInAHandWrittenFlake(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "flake.nix", "{ outputs = { ... }: { nixosConfigurations.web = mkHost \"web\"; }; }\n")
	res := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "db", System: "x86_64-linux", Role: "minimal"}))
	if res.Registered || !strings.Contains(res.Snippet, `db = mkHost "db";`) {
		t.Fatalf("a flake that lists machines by hand needs the snippet: %+v", res)
	}
}

func TestCreateMachineOverAHalfMadeOne(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "hosts/half/managed.json", "{}\n")
	_, err := s.CreateMachine(MachineSpec{Name: "half", System: "x86_64-linux", Role: "minimal"})
	isErr(t, err, ErrExists)
	if _, _, err := s.Read("hosts/half/default.nix"); err == nil {
		t.Fatal("default.nix was written before the clash was found")
	}
}
