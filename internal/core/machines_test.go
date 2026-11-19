package core

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/TheOlympusCloud/nixos-manager/templates"
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

func TestCreateMachineOnProxmox(t *testing.T) {
	s := newService(t)
	lxc := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "ct", System: "x86_64-linux", Role: "server", Platform: "proxmox-lxc"}))
	if strings.Join(lxc.Files, " ") != "hosts/ct/default.nix hosts/ct/managed.json" {
		t.Fatalf("a container has no hardware file: %v", lxc.Files)
	}
	def, _, _ := s.Read("hosts/ct/default.nix")
	for _, want := range []string{`(modulesPath + "/virtualisation/proxmox-lxc.nix")`, `nixpkgs.hostPlatform = "x86_64-linux";`, "proxmoxLXC.manageHostName = true;", `networking.hostName = "ct";`} {
		if !strings.Contains(def, want) {
			t.Errorf("container default.nix lacks %s:\n%s", want, def)
		}
	}

	vm := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "vm", System: "aarch64-linux", Role: "minimal", Platform: "proxmox-vm"}))
	def, _, _ = s.Read("hosts/vm/default.nix")
	hw, _, _ := s.Read("hosts/vm/hardware-configuration.nix")
	if len(vm.Files) != 3 || !strings.Contains(def, "./hardware-configuration.nix") || !strings.Contains(def, `(modulesPath + "/profiles/qemu-guest.nix")`) ||
		!strings.Contains(def, "services.qemuGuest.enable = true;") || !strings.Contains(def, `networking.hostName = "vm";`) || !strings.Contains(hw, `"aarch64-linux"`) {
		t.Fatalf("vm files %v:\n%s\n%s", vm.Files, def, hw)
	}

	// No platform is a physical machine, as before platforms existed.
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "pc", System: "x86_64-linux", Role: "minimal"}))
	def, _, _ = s.Read("hosts/pc/default.nix")
	if strings.Contains(def, "modulesPath") || strings.Contains(def, "%!") || !strings.Contains(def, `networking.hostName = "pc";`) {
		t.Fatalf("hardware default.nix:\n%s", def)
	}

	_, err := s.CreateMachine(MachineSpec{Name: "ok", System: "x86_64-linux", Role: "minimal", Platform: "xen"})
	isErr(t, err, ErrInvalid)
	_, err = s.CreateMachine(MachineSpec{Name: "ok", System: "x86_64-linux", Role: "minimal", Platform: "proxmox-lxc", Hardware: "{ }"})
	isErr(t, err, ErrInvalid)
}

// The platform templates must make machines Nix can evaluate: instantiate a whole container and VM
// system from the starter flake.
func TestCreatedMachinesEvaluate(t *testing.T) {
	if os.Getenv("NIXMGR_NIX_TESTS") == "" {
		t.Skip("needs real Nix and network access: set NIXMGR_NIX_TESTS=1")
	}
	s := newService(t)
	if _, err := s.Repo.Seed(templates.Fleet); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []MachineSpec{
		{Name: "ct", System: "x86_64-linux", Role: "server", Platform: "proxmox-lxc"},
		{Name: "vm", System: "x86_64-linux", Role: "server", Platform: "proxmox-vm"},
	} {
		must[MachineCreated](t)(s.CreateMachine(spec))
		attr := "path:" + s.Repo.Dir + "#nixosConfigurations." + spec.Name + ".config.system.build.toplevel.drvPath"
		if out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes", "eval", "--raw", attr).CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", spec.Platform, err, out)
		}
	}
}
