import { describe, expect, it } from 'vitest'
import type { Managed } from './api'
import { cleanOverride, formatArgs, homeRows, homeStateVersion, overriddenAttrs, overrideSummary, packageFile, parseArgs, systemRows } from './packages'
import { host } from './testmodel'

const mf = 'hosts/pc/managed.json'
const managed = (over: Partial<Managed>): Managed => ({ path: mf, exists: true, hash: 'h', packages: [], settings: {}, ...over })

describe('systemRows', () => {
  const pc = host('pc', {
    managedPackages: { nodejs_22: 'nodejs', htop: 'htop' },
    corePackages: ['bash'],
    packages: [
      { name: 'bash', version: '5.3', files: ['nixos'] },
      { name: 'nodejs', version: '22.1', files: [mf] },
      { name: 'htop', version: '3.4.1', files: ['modules/common.nix'] },
      { name: 'rsync', version: '3.4', files: ['nixos'] },
    ],
    removedPackages: [{ name: 'perl', version: '5.40', files: ['nixos'] }],
  })
  it('marks what is set here, changed, removed, pending and core', () => {
    const m = managed({ packages: ['nodejs_22', 'ripgrep'], removedPackages: ['perl', 'rsync', 'gone'], packageOverrides: { htop: { override: { withVimKeys: true } } } })
    const rows = systemRows(pc, m, ['hosts/pc/packages/bash.nix', 'hosts/pc/packages/x/y.nix'])
    const by = Object.fromEntries(rows.map((r) => [r.name, r]))
    expect(rows.map((r) => r.name)).toEqual(['bash', 'gone', 'htop', 'nodejs', 'perl', 'ripgrep', 'rsync'])
    expect(by.nodejs).toMatchObject({ attr: 'nodejs_22', removed: false })
    expect(by.htop).toMatchObject({ attr: undefined, overridden: 'htop' })
    expect(by.bash).toMatchObject({ core: true, overridden: 'bash' })
    expect(by.perl.removed).toBe(true)
    expect(by.rsync.removed).toBe(true) // removed here, not evaluated yet
    expect(by.ripgrep).toMatchObject({ pending: true, attr: 'ripgrep' })
    expect(by.gone).toMatchObject({ removed: true, files: [] })
  })
  it('keeps a package added here when its name is removed: a replacement of the same name', () => {
    // nodejs_20 from a hand-written file replaced by nodejs_24: removedPackages has nodejs, and nodejs stays.
    const node = host('pc', { managedPackages: { nodejs_24: 'nodejs' }, packages: [{ name: 'nodejs', version: '24.1', files: [mf] }] })
    const rows = systemRows(node, managed({ packages: ['nodejs_24'], removedPackages: ['nodejs'] }), [])
    expect(rows).toEqual([expect.objectContaining({ name: 'nodejs', version: '24.1', attr: 'nodejs_24', removed: false })])
    const home = host('pc', { managedPackages: { lua5_4: 'lua' }, users: [{ name: 'pablo', description: '', groups: [], home: { packages: ['lua'], programs: [] } }] })
    expect(homeRows(home, 'pablo', managed({ home: { pablo: { packages: ['lua5_4'], removedPackages: ['lua'] } } }))).toEqual([
      expect.objectContaining({ name: 'lua', attr: 'lua5_4', removed: false }),
    ])
  })
  it('lists changed packages from managed.json and from their Nix files', () => {
    expect(overriddenAttrs(pc, managed({ packageOverrides: { b: {} } }), ['hosts/pc/packages/a.nix', 'hosts/pc/packages/a.txt', 'hosts/other/packages/c.nix'])).toEqual(['a', 'b'])
    expect(packageFile(pc, 'python3Packages.requests')).toBe('hosts/pc/packages/python3Packages.requests.nix')
  })
})

describe('homeRows', () => {
  const pc = host('pc', {
    managedPackages: { ripgrep: 'ripgrep' },
    users: [{ name: 'pablo', description: '', groups: [], home: { packages: ['git', 'htop', 'ripgrep'], programs: ['git'] } }],
  })
  it('marks what is set here, pending and removed', () => {
    const m = managed({ home: { pablo: { packages: ['ripgrep', 'fd'], removedPackages: ['htop', 'nano'] } } })
    const rows = homeRows(pc, 'pablo', m)
    expect(rows.map((r) => [r.name, r.attr ?? '', !!r.pending, !!r.removed])).toEqual([
      ['fd', 'fd', true, false],
      ['git', '', false, false],
      ['htop', '', false, true],
      ['nano', '', false, true],
      ['ripgrep', 'ripgrep', false, false],
    ])
    expect(homeStateVersion(pc)).toBe('26.05')
  })
})

describe('override text', () => {
  it('round-trips arguments', () => {
    const args = { withVimKeys: true, jobs: 4, name: 'x', flags: ['-a'] }
    expect(parseArgs(formatArgs(args))).toEqual({ args })
    expect(parseArgs('# comment\n\n  doCheck = false;  ')).toEqual({ args: { doCheck: false } })
  })
  it('explains a bad line', () => {
    expect(parseArgs('doCheck false')).toEqual({ problem: 'Line 1: write name = value.' })
    expect((parseArgs('a = 1\nb = yes') as { problem: string }).problem).toMatch(/^Line 2: yes is not a JSON value/)
  })
  it('summarises and cleans an override', () => {
    expect(overrideSummary({ override: { a: 1 }, version: '2', patches: [{ file: 'a' }, { url: 'u' }] }, true)).toBe('1 argument · version 2 · 2 patches · Nix file')
    expect(cleanOverride({ override: {}, version: ' ', src: { url: '', github: '' }, patches: [{ url: ' ' }] })).toBeNull()
    expect(cleanOverride({ src: { github: ' a/b ', rev: ' v1 ', url: '' }, patches: [{ file: 'p.patch', hash: 'x' }, { url: 'https://x', hash: '' }] })).toEqual({
      src: { github: 'a/b', rev: 'v1' },
      patches: [{ file: 'p.patch' }, { url: 'https://x' }],
    })
  })
})
