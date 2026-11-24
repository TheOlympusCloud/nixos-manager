package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/crypto/ssh"
)

// MachineSpec is what the new-machine wizard asks for.
type MachineSpec struct {
	Name     string `json:"name"`
	System   string `json:"system"`             // x86_64-linux or aarch64-linux
	Role     string `json:"role"`               // minimal, server or desktop
	Hardware string `json:"hardware,omitempty"` // nixos-generate-config output; a placeholder when empty
	Platform string `json:"platform,omitempty" jsonschema:"hardware (default: a physical machine), proxmox-lxc (a NixOS container on Proxmox: no hardware configuration) or proxmox-vm (a Proxmox virtual machine: QEMU guest support plus the hardware configuration)"`
	Disk     string `json:"disk,omitempty" jsonschema:"a disk layout for disko, for a machine nixos-anywhere installs (hardware or proxmox-vm only): ext4, or btrfs (subvolumes for /, /home and /nix). nixos-anywhere then writes hardware-configuration.nix, so leave hardware out. Empty: NixOS is installed by hand"`
	Device   string `json:"device,omitempty" jsonschema:"with a disk layout, the disk nixos-anywhere erases and installs on, such as /dev/sda, /dev/nvme0n1 or /dev/disk/by-id/..."`
	// Packages, when set (an empty list included), replace the role's preset packages.
	Packages []string `json:"packages,omitempty" jsonschema:"nixpkgs attributes managed.json starts with, instead of the role's preset packages"`
	// User, when set, is the machine's first user, written to managed.json. No role creates one.
	User *MachineUser `json:"user,omitempty" jsonschema:"the first user, a normal user in managed.json; the server role allows SSH keys only, so give it a key. Leave it out when users come from hand-written Nix"`
}

// MachineUser is a new machine's first user, as Users → Add user and the Home tab would write it.
type MachineUser struct {
	Name  string   `json:"name" jsonschema:"the user name: lowercase letters, digits, - and _"`
	Keys  []string `json:"keys,omitempty" jsonschema:"authorized SSH public keys, one authorized_keys line each"`
	Wheel bool     `json:"wheel,omitempty" jsonschema:"an administrator: in the wheel group, so sudo works"`
	Home  bool     `json:"home,omitempty" jsonschema:"home-manager on for the user (needs home-manager's NixOS module in the flake, as the starter flake has)"`
}

// The release the templates install (system.stateVersion), which a first user's home starts at too.
const newStateVersion = "26.05"

func (u *MachineUser) check() error {
	if !userName.MatchString(u.Name) {
		return errorf(ErrInvalid, "invalid user name %q: lowercase letters, digits, - and _, starting with a letter or _ (at most 32)", u.Name)
	}
	for _, k := range u.Keys {
		// One key per entry: a line break would smuggle in more authorized_keys lines.
		_, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(k))
		if err != nil || len(rest) > 0 || strings.ContainsAny(k, "\r\n") {
			return errorf(ErrInvalid, "%s's SSH keys: each is one public key line, such as ssh-ed25519 AAAA… me@laptop", u.Name)
		}
	}
	return nil
}

// settings adds the user (and their home-manager release) to a role's settings, without changing the shared preset.
func (u *MachineUser) settings(preset map[string]any) map[string]any {
	settings := maps.Clone(preset)
	user := map[string]any{"isNormalUser": true, "extraGroups": []string{}}
	if u.Wheel {
		user["extraGroups"] = []string{"wheel"}
	}
	if len(u.Keys) > 0 {
		user["openssh"] = map[string]any{"authorizedKeys": map[string]any{"keys": u.Keys}}
	}
	settings["users"] = map[string]any{"users": map[string]any{u.Name: user}}
	if u.Home {
		settings["home-manager"] = map[string]any{"users": map[string]any{u.Name: map[string]any{"home": map[string]any{"stateVersion": newStateVersion}}}}
	}
	return settings
}

type MachineCreated struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
	// Registered: the flake finds machines in hosts/ by itself. Otherwise Snippet is the
	// line to add to its nixosConfigurations.
	Registered bool   `json:"registered"`
	Snippet    string `json:"snippet,omitempty"`
	// InputSnippet: the machine has a disk layout but flake.nix declares no
	// disko input; these lines go in its inputs, then Lock.
	InputSnippet string `json:"inputSnippet,omitempty"`
}

var systems = map[string]bool{"x86_64-linux": true, "aarch64-linux": true}

// Each platform's default.nix. A Proxmox LXC has no hardware file: the container module stands in for it.
var platformTemplates = map[string]string{"hardware": defaultNixTemplate, "proxmox-lxc": proxmoxLXCTemplate, "proxmox-vm": proxmoxVMTemplate}

// Role presets: what managed.json starts with. Everything stays editable by clicking.
var rolePresets = map[string]map[string]any{
	"minimal": {"packages": []string{"git", "htop"}, "settings": map[string]any{}},
	"server": {
		"packages": []string{"btop", "git", "htop", "tmux"},
		"settings": map[string]any{
			"services": map[string]any{
				"openssh":  map[string]any{"enable": true, "settings": map[string]any{"PasswordAuthentication": false, "PermitRootLogin": "no"}},
				"fail2ban": map[string]any{"enable": true},
			},
			"networking": map[string]any{"firewall": map[string]any{"enable": true}},
			"nix":        gcSettings(),
		},
	},
	"desktop": {
		"packages": []string{"firefox", "git", "htop", "vlc"},
		"settings": map[string]any{
			"services": map[string]any{
				"desktopManager": map[string]any{"plasma6": map[string]any{"enable": true}},
				"displayManager": map[string]any{"sddm": map[string]any{"enable": true}},
				"pipewire":       map[string]any{"enable": true, "pulse": map[string]any{"enable": true}},
				"printing":       map[string]any{"enable": true},
			},
			"networking": map[string]any{"networkmanager": map[string]any{"enable": true}},
			"hardware":   map[string]any{"bluetooth": map[string]any{"enable": true}},
			"zramSwap":   map[string]any{"enable": true},
			"nix":        gcSettings(),
		},
	},
}

func gcSettings() map[string]any {
	return map[string]any{
		"gc":       map[string]any{"automatic": true, "dates": "weekly", "options": "--delete-older-than 14d"},
		"optimise": map[string]any{"automatic": true},
	}
}

// %[3]s is the import of disk-config.nix, for a machine with a disk layout.
const defaultNixTemplate = `{ ... }:
{
  imports = [
    ./hardware-configuration.nix%[3]s
  ];

  networking.hostName = %[1]q;

  # The NixOS release this machine was first installed with. Keep it as it is when upgrading.
  system.stateVersion = "26.05";
}
`

const proxmoxLXCTemplate = `{ modulesPath, ... }:
{
  # A NixOS container on Proxmox (unprivileged, nesting on): no disks or
  # boot loader to configure, and Proxmox sets up the network.
  imports = [ (modulesPath + "/virtualisation/proxmox-lxc.nix") ];

  nixpkgs.hostPlatform = %[2]q;

  # comin finds this machine's configuration by its host name,
  # so NixOS sets it instead of Proxmox.
  proxmoxLXC.manageHostName = true;
  networking.hostName = %[1]q;

  # The NixOS release this machine was first installed with. Keep it as it is when upgrading.
  system.stateVersion = "26.05";
}
`

const proxmoxVMTemplate = `{ modulesPath, ... }:
{
  imports = [
    ./hardware-configuration.nix%[3]s
    (modulesPath + "/profiles/qemu-guest.nix")
  ];

  # Lets Proxmox show the VM's address and shut it down cleanly
  # (turn on "QEMU Guest Agent" in the VM's Options).
  services.qemuGuest.enable = true;

  networking.hostName = %[1]q;

  # The NixOS release this machine was first installed with. Keep it as it is when upgrading.
  system.stateVersion = "26.05";
}
`

const hardwarePlaceholder = `# Placeholder: replace with the output of nixos-generate-config --show-hardware-config
# on the machine itself.
{ lib, ... }:
{
  nixpkgs.hostPlatform = lib.mkDefault %q;
  boot.loader.grub.device = "nodev";
  fileSystems."/" = {
    device = "/dev/disk/by-label/nixos";
    fsType = "ext4";
  };
}
`

// The hardware file of a machine with a disk layout until nixos-anywhere
// writes the real one: no file systems or boot loader, which disk-config.nix sets.
const diskoHardwarePlaceholder = `# Placeholder: nixos-anywhere writes the real one when it installs this machine
# (--generate-hardware-config). disk-config.nix sets the disks and the boot loader.
{ lib, ... }:
{
  nixpkgs.hostPlatform = lib.mkDefault %q;
}
`

// disk-config.nix: %[1]q is the disk, %[2]s the boot loader for the system,
// %[3]s the BIOS boot partition (x86_64 only), %[4]s the root partition's content.
const diskConfigTemplate = `# The disks: disko partitions and formats them when nixos-anywhere installs this
# machine, erasing everything on %[1]s, and mounts them from then on. A change
# here does not repartition an installed machine.
{ inputs, ... }:
{
  imports = [ inputs.disko.nixosModules.disko ];
%[2]s
  disko.devices.disk.main = {
    type = "disk";
    device = %[1]q;
    content = {
      type = "gpt";
      partitions = {%[3]s
        ESP = {
          size = "1G";
          type = "EF00";
          content = {
            type = "filesystem";
            format = "vfat";
            mountpoint = "/boot";
            mountOptions = [ "umask=0077" ];
          };
        };
        root = {
          size = "100%%";
          content = {%[4]s
          };
        };
      };
    };
  };
}
`

const grubBIOSAndUEFI = `
  # GRUB boots BIOS and UEFI machines alike: disko installs it on the disk with
  # the BIOS boot partition, and UEFI firmware finds it at the removable-media path.
  boot.loader.grub = {
    efiSupport = true;
    efiInstallAsRemovable = true;
  };
`

const grubUEFI = `
  # GRUB on UEFI, found at the removable-media path.
  boot.loader.grub = {
    device = "nodev";
    efiSupport = true;
    efiInstallAsRemovable = true;
  };
`

const biosBootPartition = `
        boot = {
          size = "1M";
          type = "EF02";
        };`

const ext4Root = `
            type = "filesystem";
            format = "ext4";
            mountpoint = "/";`

const btrfsRoot = `
            type = "btrfs";
            extraArgs = [ "-f" ];
            subvolumes = {
              "/root" = {
                mountpoint = "/";
                mountOptions = [
                  "compress=zstd"
                  "noatime"
                ];
              };
              "/home" = {
                mountpoint = "/home";
                mountOptions = [
                  "compress=zstd"
                  "noatime"
                ];
              };
              "/nix" = {
                mountpoint = "/nix";
                mountOptions = [
                  "compress=zstd"
                  "noatime"
                ];
              };
            };`

var diskLayouts = map[string]string{"ext4": ext4Root, "btrfs": btrfsRoot}

// diskDevice is a disk's path: plain characters only, as it goes into Nix and is shown in a shell command.
var diskDevice = regexp.MustCompile(`^/dev/[A-Za-z0-9][A-Za-z0-9._:+/-]*$`)

const diskoInputSnippet = `disko = {
  url = "github:nix-community/disko";
  inputs.nixpkgs.follows = "nixpkgs";
};`

// hasDiskoInput tells whether flake.nix declares an input named disko (disko = …, disko.url = …), outside comments.
// shortcut: any "disko =" counts, a let binding in outputs too; that flake then fails to evaluate the machine,
// which says what is missing. Parse the inputs attribute set if this misleads someone.
func hasDiskoInput(flake string) bool {
	return diskoInput.MatchString(nixComment.ReplaceAllString(flake, ""))
}

var (
	diskoInput = regexp.MustCompile(`(^|[\s.{;])disko(\.url)?\s*=`)
	nixComment = regexp.MustCompile(`#[^\n]*`)
)

// machineName becomes networking.hostName, so it follows NixOS's host name rule (lowercase here).
var machineName = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$`)

// CreateMachine adds hosts/<name>/ with a configuration for its platform, its hardware file (none for a
// Proxmox LXC) and a managed.json preset.
func (s *Service) CreateMachine(spec MachineSpec) (MachineCreated, error) {
	if !machineName.MatchString(spec.Name) {
		return MachineCreated{}, errorf(ErrInvalid, "a machine name is lowercase letters, digits, - and _, starting and ending with a letter or digit (at most 63)")
	}
	if !systems[spec.System] {
		return MachineCreated{}, errorf(ErrInvalid, "unsupported system %q (x86_64-linux or aarch64-linux)", spec.System)
	}
	preset, ok := rolePresets[spec.Role]
	if !ok {
		return MachineCreated{}, errorf(ErrInvalid, "unknown role %q (minimal, server or desktop)", spec.Role)
	}
	if spec.Platform == "" {
		spec.Platform = "hardware"
	}
	template, ok := platformTemplates[spec.Platform]
	if !ok {
		return MachineCreated{}, errorf(ErrInvalid, "unknown platform %q (hardware, proxmox-lxc or proxmox-vm)", spec.Platform)
	}
	if len(spec.Hardware) > 64<<10 {
		return MachineCreated{}, errorf(ErrInvalid, "the hardware configuration is too large")
	}
	lxc := spec.Platform == "proxmox-lxc"
	if lxc && strings.TrimSpace(spec.Hardware) != "" {
		return MachineCreated{}, errorf(ErrInvalid, "a Proxmox LXC has no hardware configuration: leave it out")
	}
	root, disko := diskLayouts[spec.Disk]
	switch {
	case spec.Disk == "" && spec.Device != "":
		return MachineCreated{}, errorf(ErrInvalid, "a disk is only used with a disk layout")
	case spec.Disk == "":
	case !disko:
		return MachineCreated{}, errorf(ErrInvalid, "unknown disk layout %q (ext4 or btrfs)", spec.Disk)
	case lxc:
		return MachineCreated{}, errorf(ErrInvalid, "a Proxmox LXC has no disk to lay out")
	case !diskDevice.MatchString(spec.Device) || strings.Contains(spec.Device, "..") || len(spec.Device) > 200:
		return MachineCreated{}, errorf(ErrInvalid, "the disk to install on is a path in /dev, such as /dev/sda or /dev/disk/by-id/…")
	case strings.TrimSpace(spec.Hardware) != "":
		return MachineCreated{}, errorf(ErrInvalid, "with a disk layout, nixos-anywhere writes the hardware configuration: leave it out")
	}
	dir := "hosts/" + spec.Name
	hardware := spec.Hardware
	if strings.TrimSpace(hardware) == "" {
		hardware = fmt.Sprintf(hardwarePlaceholder, spec.System)
	}
	diskImport := ""
	if disko {
		hardware = fmt.Sprintf(diskoHardwarePlaceholder, spec.System)
		diskImport = "\n    ./disk-config.nix"
	}
	if spec.Packages != nil {
		if err := checkPackages(spec.Packages); err != nil {
			return MachineCreated{}, err
		}
		pkgs := orEmpty(slices.Compact(slices.Sorted(slices.Values(spec.Packages))))
		preset = map[string]any{"packages": pkgs, "settings": preset["settings"]}
	}
	if u := spec.User; u != nil {
		if err := u.check(); err != nil {
			return MachineCreated{}, err
		}
		preset = map[string]any{"packages": preset["packages"], "settings": u.settings(preset["settings"].(map[string]any))}
		if u.Home {
			preset["home"] = map[string]HomeManaged{u.Name: {Packages: []string{}, RemovedPackages: []string{}}}
		}
	}
	var managed bytes.Buffer
	enc := json.NewEncoder(&managed)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false) // a key's comment may hold < or &
	if err := enc.Encode(preset); err != nil {
		return MachineCreated{}, err
	}
	files := []struct{ path, content string }{
		{dir + "/default.nix", fmt.Sprintf(template, spec.Name, spec.System, diskImport)},
		{dir + "/hardware-configuration.nix", hardware},
		{dir + "/managed.json", managed.String()},
	}
	if lxc {
		files = slices.Delete(files, 1, 2)
	}
	if disko {
		grub, bios := grubBIOSAndUEFI, biosBootPartition
		if spec.System != "x86_64-linux" {
			grub, bios = grubUEFI, "" // GRUB's BIOS target is x86 only
		}
		files = slices.Insert(files, 2, struct{ path, content string }{dir + "/disk-config.nix", fmt.Sprintf(diskConfigTemplate, spec.Device, grub, bios, root)})
	}
	// Check every file first: stopping halfway would leave a machine the flake picks up.
	for _, f := range files {
		if _, _, err := s.Read(f.path); err == nil {
			return MachineCreated{}, errorf(ErrExists, "%s already exists", f.path)
		}
	}
	res := MachineCreated{Name: spec.Name, Files: []string{}}
	for _, f := range files {
		if _, err := s.Write(f.path, f.content, ""); err != nil {
			return res, err
		}
		res.Files = append(res.Files, f.path)
	}
	flake, _, _ := s.Read("flake.nix")
	res.Registered = strings.Contains(flake, "readDir ./hosts")
	if !res.Registered {
		res.Snippet = fmt.Sprintf("%s = mkHost %q;", spec.Name, spec.Name)
	}
	if disko && !hasDiskoInput(flake) {
		res.InputSnippet = diskoInputSnippet
	}
	return res, nil
}
