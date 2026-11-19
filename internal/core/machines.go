package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MachineSpec is what the new-machine wizard asks for.
type MachineSpec struct {
	Name     string `json:"name"`
	System   string `json:"system"`             // x86_64-linux or aarch64-linux
	Role     string `json:"role"`               // minimal, server or desktop
	Hardware string `json:"hardware,omitempty"` // nixos-generate-config output; a placeholder when empty
}

type MachineCreated struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
	// Registered: the flake finds machines in hosts/ by itself. Otherwise Snippet is the
	// line to add to its nixosConfigurations.
	Registered bool   `json:"registered"`
	Snippet    string `json:"snippet,omitempty"`
}

var systems = map[string]bool{"x86_64-linux": true, "aarch64-linux": true}

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

const defaultNixTemplate = `{ ... }:
{
  imports = [ ./hardware-configuration.nix ];

  networking.hostName = %q;

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

// machineName becomes networking.hostName, so it follows NixOS's host name rule (lowercase here).
var machineName = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$`)

// CreateMachine adds hosts/<name>/ with a configuration, its hardware file and a managed.json preset.
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
	if len(spec.Hardware) > 64<<10 {
		return MachineCreated{}, errorf(ErrInvalid, "the hardware configuration is too large")
	}
	dir := "hosts/" + spec.Name
	hardware := spec.Hardware
	if strings.TrimSpace(hardware) == "" {
		hardware = fmt.Sprintf(hardwarePlaceholder, spec.System)
	}
	var managed bytes.Buffer
	enc := json.NewEncoder(&managed)
	enc.SetIndent("", "  ")
	if err := enc.Encode(preset); err != nil {
		return MachineCreated{}, err
	}
	files := []struct{ path, content string }{
		{dir + "/default.nix", fmt.Sprintf(defaultNixTemplate, spec.Name)},
		{dir + "/hardware-configuration.nix", hardware},
		{dir + "/managed.json", managed.String()},
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
	return res, nil
}
