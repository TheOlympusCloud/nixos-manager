import { describe, expect, it } from 'vitest'
import go from '../../../internal/core/machines.go?raw'
import { hardwareCheck, machineNameProblem, platforms, previewFiles, roles } from './machines'

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
    expect(files[0].content).toBe(raw('defaultNixTemplate').replace('%[1]q', '"nas"'))
    expect(files[1].content).toBe(raw('hardwarePlaceholder').replace('%q', '"aarch64-linux"'))
    expect(previewFiles({ name: 'nas', system: 'x86_64-linux', role: 'minimal', hardware: '{ }' })[1].content).toBe('{ }')
  })
  it('matches the server templates of every platform', () => {
    const fill = (t: string) => t.replaceAll('%[1]q', '"ct"').replaceAll('%[2]q', '"x86_64-linux"')
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
  it('matches the server presets', () => {
    for (const r of roles) {
      const block = new RegExp(`"${r.role}":\\s*\\{\\s*"packages": \\[\\]string\\{([^}]*)\\}`).exec(go)
      expect(block, r.role).not.toBeNull()
      expect(JSON.parse(`[${block![1]}]`)).toEqual(r.packages)
    }
    expect(JSON.parse(files[2].content).settings.services.openssh.settings).toEqual({ PasswordAuthentication: false, PermitRootLogin: 'no' })
  })
})
