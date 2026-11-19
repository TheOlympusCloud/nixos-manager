// The new-machine wizard: names, role presets and a preview of the files it writes.
import type { MachinePlatform, MachineRole, MachineSpec } from './api'

/** Problem with a new machine name ('' when fine). It becomes networking.hostName, so it must be a valid host name. */
export function machineNameProblem(name: string, existing: string[]): string {
  if (!name) return ''
  if (/[A-Z]/.test(name)) return 'Lowercase only.'
  if (!/^[a-z0-9]([a-z0-9_-]{0,61}[a-z0-9])?$/.test(name)) return 'Letters, digits, - and _, starting and ending with a letter or digit (at most 63).'
  if (existing.includes(name)) return `There is already a machine called ${name}.`
  return ''
}

export const architectures: { system: MachineSpec['system']; label: string; note: string }[] = [
  { system: 'x86_64-linux', label: 'x86_64', note: 'Intel and AMD PCs, most servers and virtual machines' },
  { system: 'aarch64-linux', label: 'aarch64', note: 'Raspberry Pi 4 and 5, ARM servers and boards' },
]

export const platforms: { platform: MachinePlatform; label: string; note: string }[] = [
  { platform: 'hardware', label: 'Physical machine', note: 'A PC, laptop, server or board. NixOS needs its hardware configuration.' },
  { platform: 'proxmox-vm', label: 'Proxmox VM', note: 'A virtual machine: QEMU guest drivers and the guest agent, plus its hardware configuration.' },
  { platform: 'proxmox-lxc', label: 'Proxmox LXC', note: 'A NixOS container: light and fast, with no disks or boot loader to set up.' },
]

// shortcut: verbatim copies of the default.nix templates in internal/core/machines.go (a test compares them);
// %[1]q is the quoted name and %[2]q the quoted system.
const defaultNixTemplates: Record<MachinePlatform, string> = {
  hardware: `{ ... }:
{
  imports = [ ./hardware-configuration.nix ];

  networking.hostName = %[1]q;

  # The NixOS release this machine was first installed with. Keep it as it is when upgrading.
  system.stateVersion = "26.05";
}
`,
  'proxmox-lxc': `{ modulesPath, ... }:
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
`,
  'proxmox-vm': `{ modulesPath, ... }:
{
  imports = [
    ./hardware-configuration.nix
    (modulesPath + "/profiles/qemu-guest.nix")
  ];

  # Lets Proxmox show the VM's address and shut it down cleanly
  # (turn on "QEMU Guest Agent" in the VM's Options).
  services.qemuGuest.enable = true;

  networking.hostName = %[1]q;

  # The NixOS release this machine was first installed with. Keep it as it is when upgrading.
  system.stateVersion = "26.05";
}
`,
}

// shortcut: these mirror the presets in internal/core/machines.go; update both together.
export const roles: { role: MachineRole; label: string; note: string; gets: string[]; packages: string[]; settings: Record<string, unknown> }[] = [
  {
    role: 'server',
    label: 'Server',
    note: 'Runs unattended and is reached over SSH.',
    gets: ['SSH with keys only, no root login', 'fail2ban', 'Firewall on', 'Weekly garbage collection and store optimisation'],
    packages: ['btop', 'git', 'htop', 'tmux'],
    settings: {
      services: { openssh: { enable: true, settings: { PasswordAuthentication: false, PermitRootLogin: 'no' } }, fail2ban: { enable: true } },
      networking: { firewall: { enable: true } },
      nix: { gc: { automatic: true, dates: 'weekly', options: '--delete-older-than 14d' }, optimise: { automatic: true } },
    },
  },
  {
    role: 'desktop',
    label: 'Desktop',
    note: 'A workstation or laptop with a graphical session.',
    gets: ['KDE Plasma 6 with SDDM', 'PipeWire sound', 'NetworkManager', 'Bluetooth and printing', 'zram swap', 'Weekly garbage collection'],
    packages: ['firefox', 'git', 'htop', 'vlc'],
    settings: {
      services: {
        desktopManager: { plasma6: { enable: true } },
        displayManager: { sddm: { enable: true } },
        pipewire: { enable: true, pulse: { enable: true } },
        printing: { enable: true },
      },
      networking: { networkmanager: { enable: true } },
      hardware: { bluetooth: { enable: true } },
      zramSwap: { enable: true },
      nix: { gc: { automatic: true, dates: 'weekly', options: '--delete-older-than 14d' }, optimise: { automatic: true } },
    },
  },
  {
    role: 'minimal',
    label: 'Minimal',
    note: 'A bare system to build up yourself.',
    gets: ['Nothing switched on beyond NixOS defaults'],
    packages: ['git', 'htop'],
    settings: {},
  },
]

/** The files the server writes for this spec, for the review step. */
export function previewFiles(spec: MachineSpec): { path: string; content: string; note: string }[] {
  const dir = `hosts/${spec.name || '<name>'}`
  const preset = roles.find((r) => r.role === spec.role) ?? roles[2]
  const platform = spec.platform ?? 'hardware'
  const hardware = spec.hardware?.trim()
    ? spec.hardware
    : `# Placeholder: replace with the output of nixos-generate-config --show-hardware-config\n# on the machine itself.\n{ lib, ... }:\n{\n  nixpkgs.hostPlatform = lib.mkDefault "${spec.system}";\n  boot.loader.grub.device = "nodev";\n  fileSystems."/" = {\n    device = "/dev/disk/by-label/nixos";\n    fsType = "ext4";\n  };\n}\n`
  return [
    {
      path: `${dir}/default.nix`,
      note: 'The hand-written part: yours to edit in Code.',
      content: defaultNixTemplates[platform].replaceAll('%[1]q', JSON.stringify(spec.name)).replaceAll('%[2]q', JSON.stringify(spec.system)),
    },
    ...(platform === 'proxmox-lxc'
      ? []
      : [
          {
            path: `${dir}/hardware-configuration.nix`,
            note: spec.hardware?.trim() ? 'What you pasted.' : 'A placeholder until you paste the real one.',
            content: hardware,
          },
        ]),
    {
      path: `${dir}/managed.json`,
      note: `The ${preset.label.toLowerCase()} preset: what you change by clicking.`,
      content: JSON.stringify({ packages: preset.packages, settings: preset.settings }, null, 2) + '\n',
    },
  ]
}

/** Light checks of a pasted hardware-configuration.nix: errors block, warnings inform. */
export function hardwareCheck(text: string): { error?: string; warning?: string } {
  const t = text.trim()
  if (!t) return {}
  if (new Blob([text]).size > 64 << 10) return { error: 'This is larger than 64 KiB; paste only hardware-configuration.nix.' }
  if (!t.includes('{') || !t.includes('}')) return { error: 'This does not look like a Nix file. Paste the whole output of the command.' }
  if (!t.includes('fileSystems') || !t.includes('"/"')) return { warning: 'No root file system (fileSystems."/") in it; the machine will not boot without one.' }
  return {}
}
