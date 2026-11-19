// Small fleet used by the unit tests only.
import type { Host, Model } from './api'

export function host(name: string, over: Partial<Host> = {}): Host {
  return {
    name,
    hostName: name,
    system: 'x86_64-linux',
    release: '26.05',
    stateVersion: '26.05',
    timeZone: null,
    kernel: null,
    managedFile: `hosts/${name}/managed.json`,
    packages: [],
    features: [],
    users: [],
    ports: { tcp: [], udp: [], firewall: true },
    fileSystems: [],
    files: ['flake.nix', `hosts/${name}/default.nix`],
    error: null,
    ...over,
  }
}

const common = 'modules/common.nix'
export const testModel: Model = {
  tree: 't',
  hosts: [
    host('a', {
      files: ['flake.nix', 'hosts/a/default.nix', common, 'hosts/a/managed.json'],
      packages: [
        { name: 'git', version: '2.51.0', files: [common] },
        { name: 'htop', version: '3.4.1', files: [common] },
        { name: 'vlc', version: '3.0.21', files: ['hosts/a/default.nix'] },
        { name: 'vscode', version: '1.105.1', files: ['hosts/a/managed.json'] },
        { name: 'nano', version: '8.6', files: ['nixos'] },
      ],
      features: [
        { group: 'services', name: 'openssh', files: [common], description: 'ssh' },
        { group: 'programs', name: 'steam', files: ['hosts/a/managed.json'], description: 'steam' },
      ],
      users: [{ name: 'pablo', description: 'Pablo', groups: ['wheel'], home: { packages: ['bat'], programs: ['git'] } }],
      ports: { tcp: [22], udp: [41641], firewall: true },
    }),
    host('b', {
      managedFile: null,
      files: ['flake.nix', 'hosts/b/default.nix', common],
      packages: [
        { name: 'git', version: '2.51.0', files: [common] },
        { name: 'nano', version: '8.6', files: ['nixos'] },
      ],
      features: [
        { group: 'services', name: 'openssh', files: [common], description: 'ssh' },
        { group: 'services', name: 'home-assistant', files: ['hosts/b/default.nix'], description: 'ha' },
      ],
      users: [{ name: 'pablo', description: 'Pablo', groups: ['wheel', 'gpio'], home: null }],
      ports: { tcp: [22, 8123], udp: [], firewall: true },
    }),
    host('broken', { error: 'error: infinite recursion encountered\n  at /nix/store/x-source/hosts/broken/default.nix:12:3', files: [], hostName: '' }),
  ],
  homes: [{ name: 'pablo@laptop', packages: ['neovim'], programs: ['git'] }],
  inputs: [
    { name: 'nixpkgs', type: 'github', url: 'github:NixOS/nixpkgs/nixos-26.05', rev: '9e1f0a2b3c4d', lastModified: 1763400000 },
    { name: 'comin', type: 'github', url: 'github:nlewo/comin', rev: 'c84768279d6c', lastModified: 1762300000 },
  ],
}
