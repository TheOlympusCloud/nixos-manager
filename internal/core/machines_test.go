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

// The platform templates and disk layouts must make machines Nix can evaluate: instantiate whole
// systems from the starter flake, and the disko script nixos-anywhere runs.
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
		// What nixos-anywhere builds: the system and the disko script that partitions the disk.
		{Name: "pc", System: "x86_64-linux", Role: "desktop", Disk: "ext4", Device: "/dev/nvme0n1"},
		{Name: "vmb", System: "x86_64-linux", Role: "server", Platform: "proxmox-vm", Disk: "btrfs", Device: "/dev/sda"},
		{Name: "arm", System: "aarch64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/disk/by-id/nvme-example"},
	} {
		res := must[MachineCreated](t)(s.CreateMachine(spec))
		if res.InputSnippet != "" {
			t.Errorf("the starter flake has the disko input: %+v", res)
		}
		attrs := []string{"toplevel"}
		if spec.Disk != "" {
			attrs = append(attrs, "diskoScript")
		}
		for _, a := range attrs {
			attr := "path:" + s.Repo.Dir + "#nixosConfigurations." + spec.Name + ".config.system.build." + a + ".drvPath"
			if out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes", "eval", "--raw", attr).CombinedOutput(); err != nil {
				t.Errorf("%s %s %s: %v\n%s", spec.Platform, spec.Disk, a, err, out)
			}
		}
	}
	// The file systems and boot loader come from the disk layout alone.
	attr := "path:" + s.Repo.Dir + "#nixosConfigurations.pc.config"
	out, err := exec.Command("nix", "--extra-experimental-features", "nix-command flakes", "eval", "--json", attr,
		"--apply", `c: { root = c.fileSystems."/".fsType; boot = c.fileSystems."/boot".fsType; grub = c.boot.loader.grub.devices; efi = c.boot.loader.grub.efiSupport; }`).Output()
	if ee, ok := err.(*exec.ExitError); ok {
		out = append(out, ee.Stderr...)
	}
	if err != nil || strings.TrimSpace(string(out)) != `{"boot":"vfat","efi":true,"grub":["/dev/nvme0n1"],"root":"ext4"}` {
		t.Errorf("pc: %v\n%s", err, out)
	}
}

func TestCreateMachineWithADiskLayout(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "flake.nix", "{ inputs.disko.url = \"github:nix-community/disko\"; outputs = { ... }: { }; # readDir ./hosts\n}\n")

	res := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "nas", System: "x86_64-linux", Role: "server", Disk: "ext4", Device: "/dev/disk/by-id/nvme-Samsung_SSD_980_1TB_S6"}))
	if strings.Join(res.Files, " ") != "hosts/nas/default.nix hosts/nas/hardware-configuration.nix hosts/nas/disk-config.nix hosts/nas/managed.json" || res.InputSnippet != "" {
		t.Fatalf("result %+v", res)
	}
	def, _, _ := s.Read("hosts/nas/default.nix")
	hw, _, _ := s.Read("hosts/nas/hardware-configuration.nix")
	disk, _, _ := s.Read("hosts/nas/disk-config.nix")
	if !strings.Contains(def, "./hardware-configuration.nix\n    ./disk-config.nix\n") || strings.Contains(def, "%!") {
		t.Errorf("default.nix does not import disk-config.nix:\n%s", def)
	}
	// nixos-anywhere writes the real hardware file; until then nothing in it may clash with disko's.
	if strings.Contains(hw, "fileSystems") || strings.Contains(hw, "grub") || !strings.Contains(hw, `nixpkgs.hostPlatform = lib.mkDefault "x86_64-linux";`) {
		t.Errorf("placeholder:\n%s", hw)
	}
	for _, want := range []string{
		"inputs.disko.nixosModules.disko", `device = "/dev/disk/by-id/nvme-Samsung_SSD_980_1TB_S6";`,
		`type = "EF02";`, `type = "EF00";`, "efiSupport = true;", "efiInstallAsRemovable = true;", `format = "ext4";`, `mountpoint = "/";`,
	} {
		if !strings.Contains(disk, want) {
			t.Errorf("disk-config.nix lacks %s:\n%s", want, disk)
		}
	}
	if strings.Contains(disk, "nodev") || strings.Contains(disk, "%!") {
		t.Errorf("x86_64 boots BIOS through the EF02 partition:\n%s", disk)
	}

	// aarch64 boots UEFI only; btrfs has its subvolumes.
	must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "arm", System: "aarch64-linux", Role: "minimal", Platform: "proxmox-vm", Disk: "btrfs", Device: "/dev/vda"}))
	def, _, _ = s.Read("hosts/arm/default.nix")
	disk, _, _ = s.Read("hosts/arm/disk-config.nix")
	if !strings.Contains(def, "./disk-config.nix") || !strings.Contains(def, "qemu-guest.nix") {
		t.Errorf("vm default.nix:\n%s", def)
	}
	for _, want := range []string{`device = "nodev";`, `type = "btrfs";`, `"/nix" = {`, `"compress=zstd"`, `device = "/dev/vda";`} {
		if !strings.Contains(disk, want) {
			t.Errorf("aarch64 btrfs disk-config.nix lacks %s:\n%s", want, disk)
		}
	}
	if strings.Contains(disk, "EF02") {
		t.Errorf("no BIOS boot partition on aarch64:\n%s", disk)
	}

	// Without a layout nothing changes.
	plain := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "pc", System: "x86_64-linux", Role: "minimal"}))
	def, _, _ = s.Read("hosts/pc/default.nix")
	if len(plain.Files) != 3 || strings.Contains(def, "disk-config") || strings.Contains(def, "%!") {
		t.Fatalf("%v\n%s", plain.Files, def)
	}

	for _, bad := range []MachineSpec{
		{Name: "a", System: "x86_64-linux", Role: "minimal", Disk: "ext4"},                                              // no disk
		{Name: "b", System: "x86_64-linux", Role: "minimal", Device: "/dev/sda"},                                        // a disk without a layout
		{Name: "c", System: "x86_64-linux", Role: "minimal", Disk: "zfs", Device: "/dev/sda"},                           // unknown layout
		{Name: "d", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/sda", Platform: "proxmox-lxc"}, // a container has no disk
		{Name: "e", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/sda", Hardware: "{ }"},         // nixos-anywhere writes it
		{Name: "f", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "sda"},
		{Name: "g", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/../etc/passwd"},
		{Name: "h", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: `/dev/sda"; x = "`},
		{Name: "i", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/${x}"},
		{Name: "j", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/"},
	} {
		_, err := s.CreateMachine(bad)
		isErr(t, err, ErrInvalid)
		if _, _, err := s.Read("hosts/" + bad.Name + "/default.nix"); err == nil {
			t.Errorf("%+v was written", bad)
		}
	}
}

func TestCreateMachineWithADiskLayoutNeedsTheDiskoInput(t *testing.T) {
	s := newService(t)
	write(t, s.Repo, "flake.nix", "{\n  inputs = {\n    nixpkgs.url = \"github:NixOS/nixpkgs/nixos-26.05\";\n    # Add more inputs here (disko, nixos-hardware, ...)\n  };\n}\n")
	res := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "nas", System: "x86_64-linux", Role: "minimal", Disk: "ext4", Device: "/dev/sda"}))
	if !strings.Contains(res.InputSnippet, `url = "github:nix-community/disko";`) || !strings.Contains(res.InputSnippet, `inputs.nixpkgs.follows = "nixpkgs";`) {
		t.Fatalf("a flake without disko needs the input: %+v", res)
	}
	// Only a layout needs it.
	if res := must[MachineCreated](t)(s.CreateMachine(MachineSpec{Name: "pc", System: "x86_64-linux", Role: "minimal"})); res.InputSnippet != "" {
		t.Fatalf("%+v", res)
	}
	for _, flake := range []string{
		"{ inputs = { disko = { url = \"github:nix-community/disko\"; }; }; }",
		"{ inputs.disko.url = \"github:nix-community/disko\"; }",
		"{\n  inputs = {\n    disko.url = \"git+https://example.org/disko\";\n  };\n}",
	} {
		if !hasDiskoInput(flake) {
			t.Errorf("disko is an input of %s", flake)
		}
	}
	for _, flake := range []string{"{ outputs = { disko-tools, ... }: { }; }", "# disko = later\n{ }", "{ inputs.mydisko.url = \"x\"; }"} {
		if hasDiskoInput(flake) {
			t.Errorf("disko is no input of %s", flake)
		}
	}
}
