// Package labels drawn on the tiles. Pairs with the same name and different versions are the drift.
import { prng } from './math'

export interface PackageLabel {
  name: string
  version: string
  hash: string
  /** Part of a mismatched pair: lit orange in the drift chapter. */
  conflict: boolean
}

const CONFLICTS: [string, string, string][] = [
  ['openssl', '3.0.13', '1.1.1w'],
  ['python3', '3.12.7', '3.9.19'],
  ['glibc', '2.39', '2.35'],
  ['nodejs', '22.11.0', '18.20.4'],
  ['linux', '6.6.58', '5.15.167'],
  ['systemd', '256.7', '249.11'],
]

const PACKAGES: [string, string][] = [
  ['nginx', '1.26.2'], ['jellyfin', '10.9.11'], ['nextcloud', '30.0.1'], ['forgejo', '9.0.1'],
  ['home-assistant', '2024.11.1'], ['firefox', '132.0'], ['git', '2.47.0'], ['openssh', '9.9p1'],
  ['curl', '8.10.1'], ['zlib', '1.3.1'], ['gcc', '13.3.0'], ['rustc', '1.82.0'], ['go', '1.23.2'],
  ['postgresql', '16.4'], ['redis', '7.2.6'], ['sqlite', '3.46.1'], ['ffmpeg', '7.1'], ['mesa', '24.2.5'],
  ['pipewire', '1.2.6'], ['wayland', '1.23.1'], ['podman', '5.2.5'], ['comin', '0.4.1'], ['bash', '5.2p37'],
  ['coreutils', '9.5'], ['tailscale', '1.76.1'], ['wireguard-tools', '1.0.20210914'], ['cups', '2.4.11'],
  ['vim', '9.1.0786'], ['neovim', '0.10.2'], ['tmux', '3.5a'], ['zsh', '5.9'], ['gnupg', '2.4.5'],
  ['perl', '5.40.0'], ['ruby', '3.3.5'], ['openjdk', '21.0.5'], ['cmake', '3.30.5'], ['ninja', '1.12.1'],
  ['llvm', '19.1.3'], ['libxml2', '2.13.4'], ['cairo', '1.18.2'], ['gtk4', '4.16.3'], ['qt6', '6.8.0'],
  ['vulkan-loader', '1.3.296'], ['alsa-lib', '1.2.12'], ['bluez', '5.79'], ['networkmanager', '1.50.0'],
  ['samba', '4.21.1'], ['zfs', '2.2.6'], ['btrfs-progs', '6.11'], ['grub', '2.12'], ['nix', '2.24.10'],
  ['hyprland', '0.45.0'],
]

const NIX32 = '0123456789abcdfghijklmnpqrsvwxyz'

/** A 32-character store hash in Nix's base32 alphabet, deterministic per seed. */
export function storeHash(rand: () => number): string {
  let s = ''
  for (let i = 0; i < 32; i++) s += NIX32[Math.floor(rand() * 32)]
  return s
}

/** The 64 labels of the tile atlas: 12 conflicting tiles first, then the rest. */
export function packageLabels(count = 64): PackageLabel[] {
  const rand = prng(1327)
  const out: PackageLabel[] = []
  for (const [name, a, b] of CONFLICTS) {
    out.push({ name, version: a, hash: storeHash(rand), conflict: true })
    out.push({ name, version: b, hash: storeHash(rand), conflict: true })
  }
  for (const [name, version] of PACKAGES) {
    if (out.length >= count) break
    out.push({ name, version, hash: storeHash(rand), conflict: false })
  }
  return out
}
