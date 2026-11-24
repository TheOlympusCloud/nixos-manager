import { describe, expect, it } from 'vitest'
import go from '../../../internal/core/machines.go?raw'
import type { MachineSpec } from './api'
import { deviceProblem, diskoInputSnippet, disks, hardwareCheck, hasDiskoInput, machineNameProblem, platforms, previewFiles, roles } from './machines'

describe('machineNameProblem', () => {
  it('wants a valid host name that is not taken', () => {
    expect(machineNameProblem('', [])).toBe('')
    expect(machineNameProblem('nas', ['pi'])).toBe('')
    expect(machineNameProblem('nas-2', [])).toBe('')
    expect(machineNameProblem('NAS', [])).toMatch(/Lowercase/)
    expect(machineNameProblem('-nas', [])).toMatch(/starting and ending/)
    expect(machineNameProblem('nas_', [])).toMatch(/starting and ending/)
    expect(machineNameProblem('my nas', [])).toMatch(/Letters/)
    expect(machineNameProblem('a'.repeat(64), [])).toMatch(/63/)
    expect(machineNameProblem('pi', ['pi'])).toMatch(/already/)
  })
})

describe('hardwareCheck', () => {
  it('blocks what is not Nix and warns without a root file system', () => {
    expect(hardwareCheck('')).toEqual({})
    expect(hardwareCheck('hello')).toHaveProperty('error')
    expect(hardwareCheck('{ boot.initrd.availableKernelModules = [ ]; }')).toHaveProperty('warning')
    expect(hardwareCheck('{ fileSystems."/" = { device = "/dev/sda1"; fsType = "ext4"; }; }')).toEqual({})
  })
})

// The preview must show what the server writes: compare with the Go templates and presets.
describe('previewFiles', () => {
  const raw = (name: string) => new RegExp(`const ${name} = \`([^\`]*)\``).exec(go)![1]
  const files = previewFiles({ name: 'nas', system: 'aarch64-linux', role: 'server' })
  it('matches the server templates', () => {
    expect(files.map((f) => f.path)).toEqual(['hosts/nas/default.nix', 'hosts/nas/hardware-configuration.nix', 'hosts/nas/managed.json'])
    expect(files[0].content).toBe(raw('defaultNixTemplate').replace('%[1]q', '"nas"').replace('%[3]s', ''))
    expect(files[1].content).toBe(raw('hardwarePlaceholder').replace('%q', '"aarch64-linux"'))
    expect(previewFiles({ name: 'nas', system: 'x86_64-linux', role: 'minimal', hardware: '{ }' })[1].content).toBe('{ }')
  })
  it('shows the packages chosen instead of the preset, sorted, as the server writes them', () => {
    const managed = (packages?: string[]) => JSON.parse(previewFiles({ name: 'nas', system: 'x86_64-linux', role: 'server', packages }).at(-1)!.content)
    expect(managed(['tmux', 'restic', 'tmux']).packages).toEqual(['restic', 'tmux'])
    expect(managed([]).packages).toEqual([])
    expect(managed().packages).toEqual(['btop', 'git', 'htop', 'tmux'])
  })
  it('shows the first user and their home as the server writes them', () => {
    const key = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl pablo@laptop'
    const managed = (user: MachineSpec['user']) => JSON.parse(previewFiles({ name: 'nas', system: 'x86_64-linux', role: 'server', user }).at(-1)!.content)
    const m = managed({ name: 'pablo', keys: [key], wheel: true, home: true })
    expect(m.settings.users.users.pablo).toEqual({ isNormalUser: true, extraGroups: ['wheel'], openssh: { authorizedKeys: { keys: [key] } } })
    expect(m.settings['home-manager'].users.pablo.home.stateVersion).toBe(/const newStateVersion = "([^"]+)"/.exec(go)![1])
    expect(m.home).toEqual({ pablo: { packages: [], removedPackages: [] } })
    expect(m.settings.services.openssh.enable).toBe(true)
    const anna = managed({ name: 'anna' })
    expect(anna.settings.users.users.anna).toEqual({ isNormalUser: true, extraGroups: [] })
    expect(anna.home).toBeUndefined()
    expect(anna.settings['home-manager']).toBeUndefined()
    // The preset is shared by every preview.
    expect(roles[0].settings).not.toHaveProperty('users')
  })
  it('matches the server templates of every platform', () => {
    const fill = (t: string) => t.replaceAll('%[1]q', '"ct"').replaceAll('%[2]q', '"x86_64-linux"').replaceAll('%[3]s', '')
    const lxc = previewFiles({ name: 'ct', system: 'x86_64-linux', role: 'server', platform: 'proxmox-lxc' })
    expect(lxc.map((f) => f.path)).toEqual(['hosts/ct/default.nix', 'hosts/ct/managed.json'])
    expect(lxc[0].content).toBe(fill(raw('proxmoxLXCTemplate')))
    expect(lxc[0].content).toContain('networking.hostName = "ct";')
    const vm = previewFiles({ name: 'ct', system: 'x86_64-linux', role: 'server', platform: 'proxmox-vm' })
    expect(vm.map((f) => f.path)).toEqual(['hosts/ct/default.nix', 'hosts/ct/hardware-configuration.nix', 'hosts/ct/managed.json'])
    expect(vm[0].content).toBe(fill(raw('proxmoxVMTemplate')))
    expect(previewFiles({ name: 'ct', system: 'x86_64-linux', role: 'server', platform: 'hardware' })[0].content).toBe(fill(raw('defaultNixTemplate')))
    for (const p of platforms) expect(go).toContain(`"${p.platform}":`)
  })
  it('matches the server templates of every disk layout', () => {
    const layout = (system: 'x86_64-linux' | 'aarch64-linux', disk: 'ext4' | 'btrfs', platform: 'hardware' | 'proxmox-vm' = 'hardware') =>
      previewFiles({ name: 'nas', system, role: 'minimal', platform, disk, device: '/dev/sda' })
    const x86 = layout('x86_64-linux', 'ext4')
    expect(x86.map((f) => f.path)).toEqual(['hosts/nas/default.nix', 'hosts/nas/hardware-configuration.nix', 'hosts/nas/disk-config.nix', 'hosts/nas/managed.json'])
    expect(x86[0].content).toBe(raw('defaultNixTemplate').replace('%[1]q', '"nas"').replace('%[3]s', '\n    ./disk-config.nix'))
    expect(x86[1].content).toBe(raw('diskoHardwarePlaceholder').replace('%q', '"x86_64-linux"'))
    const disk = (grub: string, bios: string, root: string) =>
      raw('diskConfigTemplate')
        .replaceAll('%[1]q', '"/dev/sda"')
        .replaceAll('%[1]s', '/dev/sda')
        .replace('%[2]s', raw(grub))
        .replace('%[3]s', bios ? raw(bios) : '')
        .replace('%[4]s', raw(root))
        .replace('%%', '%')
    expect(x86[2].content).toBe(disk('grubBIOSAndUEFI', 'biosBootPartition', 'ext4Root'))
    expect(layout('x86_64-linux', 'btrfs')[2].content).toBe(disk('grubBIOSAndUEFI', 'biosBootPartition', 'btrfsRoot'))
    expect(layout('aarch64-linux', 'btrfs')[2].content).toBe(disk('grubUEFI', '', 'btrfsRoot'))
    const vm = layout('aarch64-linux', 'ext4', 'proxmox-vm')
    expect(vm[0].content).toBe(raw('proxmoxVMTemplate').replace('%[1]q', '"nas"').replace('%[3]s', '\n    ./disk-config.nix'))
    expect(vm[2].content).toBe(disk('grubUEFI', '', 'ext4Root'))
    for (const d of disks) expect(go).toContain(`"${d.disk}": `)
    expect(raw('diskoInputSnippet')).toBe(diskoInputSnippet)
  })
  it('matches the server presets', () => {
    for (const r of roles) {
      const block = new RegExp(`"${r.role}":\\s*\\{\\s*"packages": \\[\\]string\\{([^}]*)\\}`).exec(go)
      expect(block, r.role).not.toBeNull()
      expect(JSON.parse(`[${block![1]}]`)).toEqual(r.packages)
    }
    expect(JSON.parse(files[2].content).settings.services.openssh.settings).toEqual({ PasswordAuthentication: false, PermitRootLogin: 'no' })
  })
})

describe('disks', () => {
  it('wants a plain path in /dev, as the server does', () => {
    for (const ok of ['', '/dev/sda', '/dev/nvme0n1', '/dev/vda', '/dev/disk/by-id/nvme-Samsung_SSD_980_1TB_S6', '/dev/disk/by-path/pci-0000:00:1f.2-ata-1']) expect(deviceProblem(ok), ok).toBe('')
    for (const bad of ['sda', '/dev/', '/dev/../etc/passwd', '/dev/sda"; x = "', '/dev/${x}', '/dev/sd a']) expect(deviceProblem(bad), bad).not.toBe('')
  })
  it('finds the disko input in flake.nix as the server does', () => {
    expect(hasDiskoInput('{ inputs.disko.url = "github:nix-community/disko"; }')).toBe(true)
    expect(hasDiskoInput('{\n  inputs = {\n    disko = {\n      url = "x";\n    };\n  };\n}')).toBe(true)
    expect(hasDiskoInput('{\n  # Add more inputs here (disko, nixos-hardware, ...)\n}')).toBe(false)
    expect(hasDiskoInput('# disko = later\n{ }')).toBe(false)
    expect(hasDiskoInput('{ inputs.mydisko.url = "x"; }')).toBe(false)
  })
})
