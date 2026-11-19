// The new-machine wizard: names, role presets and a preview of the files it writes.
import type { MachineRole, MachineSpec } from './api'

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
  const hardware = spec.hardware?.trim()
    ? spec.hardware
    : `# Placeholder: replace with the output of nixos-generate-config --show-hardware-config\n# on the machine itself.\n{ lib, ... }:\n{\n  nixpkgs.hostPlatform = lib.mkDefault "${spec.system}";\n  boot.loader.grub.device = "nodev";\n  fileSystems."/" = {\n    device = "/dev/disk/by-label/nixos";\n    fsType = "ext4";\n  };\n}\n`
  return [
    {
      path: `${dir}/default.nix`,
      note: 'The hand-written part: yours to edit in Code.',
      content: `{ ... }:\n{\n  imports = [ ./hardware-configuration.nix ];\n\n  networking.hostName = ${JSON.stringify(spec.name)};\n\n  # The NixOS release this machine was first installed with. Keep it as it is when upgrading.\n  system.stateVersion = "26.05";\n}\n`,
    },
    {
      path: `${dir}/hardware-configuration.nix`,
      note: spec.hardware?.trim() ? 'What you pasted.' : 'A placeholder until you paste the real one.',
      content: hardware,
    },
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
