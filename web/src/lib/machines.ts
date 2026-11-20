// The new-machine wizard: names, role presets and a preview of the files it writes.
import type { MachineDisk, MachinePlatform, MachineRole, MachineSpec } from './api'

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
// %[1]q is the quoted name, %[2]q the quoted system and %[3]s the import of disk-config.nix.
const defaultNixTemplates: Record<MachinePlatform, string> = {
  hardware: `{ ... }:
{
  imports = [
    ./hardware-configuration.nix%[3]s
  ];

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
`,
}

export const disks: { disk: MachineDisk; label: string; note: string }[] = [
  { disk: 'ext4', label: 'ext4', note: 'One ext4 file system for everything: simple and robust.' },
  { disk: 'btrfs', label: 'Btrfs', note: 'Subvolumes for /, /home and /nix, compressed with zstd.' },
]

/** Problem with the disk to install on ('' when fine); the server checks the same. */
export function deviceProblem(device: string): string {
  if (!device) return ''
  if (!/^\/dev\/[A-Za-z0-9][A-Za-z0-9._:+/-]*$/.test(device) || device.includes('..') || device.length > 200) return 'A disk in /dev, such as /dev/sda, /dev/nvme0n1 or /dev/disk/by-id/…'
  return ''
}

// shortcut: verbatim copies of the disk layout pieces in internal/core/machines.go (a test compares them).
const diskoHardwarePlaceholder = `# Placeholder: nixos-anywhere writes the real one when it installs this machine
# (--generate-hardware-config). disk-config.nix sets the disks and the boot loader.
{ lib, ... }:
{
  nixpkgs.hostPlatform = lib.mkDefault %q;
}
`

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

const diskLayouts: Record<MachineDisk, string> = { ext4: ext4Root, btrfs: btrfsRoot }

function diskConfig(spec: MachineSpec, disk: MachineDisk): string {
  const x86 = spec.system === 'x86_64-linux' // GRUB's BIOS target is x86 only
  const device = spec.device || '/dev/<disk>'
  return diskConfigTemplate
    .replaceAll('%[1]q', JSON.stringify(device))
    .replaceAll('%[1]s', device)
    .replaceAll('%[2]s', x86 ? grubBIOSAndUEFI : grubUEFI)
    .replaceAll('%[3]s', x86 ? biosBootPartition : '')
    .replaceAll('%[4]s', diskLayouts[disk])
    .replaceAll('%%', '%')
}

/** The lines flake.nix needs in its inputs for disk-config.nix. */
export const diskoInputSnippet = `disko = {
  url = "github:nix-community/disko";
  inputs.nixpkgs.follows = "nixpkgs";
};`

/** Whether flake.nix declares an input named disko, outside comments (as the server tells). */
export function hasDiskoInput(flake: string): boolean {
  return /(^|[\s.{;])disko(\.url)?\s*=/.test(flake.replace(/#[^\n]*/g, ''))
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
  const disk = platform !== 'proxmox-lxc' ? spec.disk : undefined
  const hardware = disk
    ? diskoHardwarePlaceholder.replace('%q', JSON.stringify(spec.system))
    : spec.hardware?.trim()
      ? spec.hardware
      : `# Placeholder: replace with the output of nixos-generate-config --show-hardware-config\n# on the machine itself.\n{ lib, ... }:\n{\n  nixpkgs.hostPlatform = lib.mkDefault "${spec.system}";\n  boot.loader.grub.device = "nodev";\n  fileSystems."/" = {\n    device = "/dev/disk/by-label/nixos";\n    fsType = "ext4";\n  };\n}\n`
  return [
    {
      path: `${dir}/default.nix`,
      note: 'The hand-written part: yours to edit in Code.',
      content: defaultNixTemplates[platform]
        .replaceAll('%[1]q', JSON.stringify(spec.name))
        .replaceAll('%[2]q', JSON.stringify(spec.system))
        .replaceAll('%[3]s', disk ? '\n    ./disk-config.nix' : ''),
    },
    ...(platform === 'proxmox-lxc'
      ? []
      : [
          {
            path: `${dir}/hardware-configuration.nix`,
            note: disk ? 'A placeholder: nixos-anywhere writes the real one when it installs.' : spec.hardware?.trim() ? 'What you pasted.' : 'A placeholder until you paste the real one.',
            content: hardware,
          },
        ]),
    ...(disk ? [{ path: `${dir}/disk-config.nix`, note: `The ${disk} layout disko makes when nixos-anywhere installs: it erases the disk.`, content: diskConfig(spec, disk) }] : []),
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
