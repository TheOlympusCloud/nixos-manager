import { describe, expect, it } from 'vitest'
import type { Managed } from './api'
import { SHARED_FILE, cleanOverride, copySummary, copyTargets, formatArgs, homeDir, homeEditingLine, homePathProblem, homeRows, homeStateVersion, linksSecrets, nixpkgsInputLine, otherNixpkgs, overriddenAttrs, overrideSummary, packageFile, parseArgs, secretTemplatesDir, standaloneRows, systemRows, takesFromInputs } from './packages'
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

describe('rows with modules/managed.json', () => {
  const shared = (over: Partial<Managed>): Managed => ({ path: SHARED_FILE, exists: true, hash: 's', packages: [], settings: {}, ...over })
  const pc = host('pc', {
    managedVersion: 3,
    managedPackages: { nodejs_22: 'nodejs', jq: 'jq', hello: 'hello', ripgrep: 'ripgrep', fd: 'fd' },
    packages: [
      { name: 'jq', version: '1.8', files: [SHARED_FILE] },
      { name: 'nodejs', version: '22.1', files: [mf] },
      { name: 'htop', version: '3.4.1', files: ['modules/common.nix'] },
      { name: 'bash', version: '5.3', files: ['nixos'] },
    ],
    removedPackages: [
      { name: 'hello', version: '2.12', files: [SHARED_FILE] },
      { name: 'perl', version: '5.40', files: ['nixos'] },
    ],
    users: [
      { name: 'pablo', description: '', groups: [], home: { packages: ['cowsay', 'git'], programs: [] } },
      { name: 'anna', description: '', groups: [], home: { packages: ['git'], programs: [] } },
    ],
  })
  const m = managed({
    packages: ['nodejs_22'],
    removedPackages: ['hello', 'fd'],
    packageOverrides: { bash: { overrideAttrs: { a: 1 } } },
    home: { pablo: { packages: [], removedPackages: ['git'] } },
  })
  const s = shared({
    packages: ['jq', 'hello', 'ripgrep', 'fd'],
    removedPackages: ['perl', 'nodejs', 'gone'],
    packageOverrides: { htop: { version: '3.5' }, bash: { version: '5.4' } },
    home: { pablo: { packages: ['cowsay'], removedPackages: [] }, anna: { packages: ['cowsay'], removedPackages: [] } },
  })
  it('tells the shared packages, removals and changes from the machine’s own', () => {
    const by = Object.fromEntries(systemRows(pc, m, [], s).map((r) => [r.name, r]))
    expect(Object.keys(by).sort()).toEqual(['bash', 'fd', 'gone', 'hello', 'htop', 'jq', 'nodejs', 'perl', 'ripgrep'])
    expect(by.jq).toMatchObject({ sharedAttr: 'jq', attr: undefined, removed: false })
    expect(by.hello).toMatchObject({ sharedAttr: 'hello', removed: true, removedShared: false }) // the machine takes it out
    expect(by.perl).toMatchObject({ removed: true, removedShared: true })
    expect(by.nodejs).toMatchObject({ attr: 'nodejs_22', removed: false }) // added here: a shared removal leaves it
    expect(by.ripgrep).toMatchObject({ pending: true, sharedAttr: 'ripgrep', files: [SHARED_FILE] })
    expect(by.fd).toMatchObject({ removed: true, removedShared: false }) // shared, but removed here: not pending
    expect(by.fd.pending).toBeFalsy()
    expect(by.gone).toMatchObject({ removed: true, removedShared: true })
    expect(by.htop).toMatchObject({ overridden: 'htop', sharedOverride: 'htop' })
    expect(by.bash).toMatchObject({ overridden: 'bash', sharedOverride: undefined }) // its own change wins whole
  })
  it('ignores the shared file when not given (managed.nix older than 3)', () => {
    const rows = systemRows(pc, m, [])
    expect(rows.find((r) => r.name === 'jq')).toMatchObject({ sharedAttr: undefined, removed: false })
    expect(rows.some((r) => r.name === 'ripgrep' || r.name === 'gone')).toBe(false)
  })
  it('joins a user’s shared home only where managed.json has that home', () => {
    expect(homeRows(pc, 'pablo', m, s).map((r) => [r.name, r.sharedAttr ?? '', !!r.removed])).toEqual([
      ['cowsay', 'cowsay', false],
      ['git', '', true],
    ])
    expect(homeRows(pc, 'anna', m, s).map((r) => [r.name, r.sharedAttr ?? ''])).toEqual([['git', '']])
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
  it('keeps the nixpkgs a package comes from, first as managed.json writes it', () => {
    expect(overrideSummary({ from: 'nixpkgs-unstable', patches: [{ url: 'u' }] }, false)).toBe('from nixpkgs-unstable · 1 patch')
    expect(JSON.stringify(cleanOverride({ version: '2', from: ' nixpkgs-unstable ' }))).toBe('{"from":"nixpkgs-unstable","version":"2"}')
    expect(cleanOverride({ from: ' ' })).toBeNull()
  })
})

describe('another nixpkgs', () => {
  const input = (name: string, url: string) => ({ name, type: 'github', url, rev: '', lastModified: 0 })
  it('offers the locked inputs that are a nixpkgs, other than the machine’s own', () => {
    const inputs = [
      input('nixpkgs', 'github:NixOS/nixpkgs/nixos-26.05'),
      input('nixpkgs-unstable', 'github:nixos/nixpkgs/nixos-unstable'),
      input('home-manager', 'github:nix-community/home-manager/release-26.05'),
      input('fork', 'github:pablo/nixpkgs'),
      input('nixpkgs-wayland', 'github:nix-community/nixpkgs-wayland'),
    ]
    expect(otherNixpkgs(inputs)).toEqual(['nixpkgs-unstable', 'fork'])
  })
  it('writes the line flake.nix needs', () => {
    expect(nixpkgsInputLine('nixpkgs-unstable', 'nixos-unstable')).toBe('nixpkgs-unstable.url = "github:NixOS/nixpkgs/nixos-unstable";')
    // A name starting with a digit is not a Nix identifier, so it is quoted.
    expect(nixpkgsInputLine('9p', 'nixos-unstable')).toBe('"9p".url = "github:NixOS/nixpkgs/nixos-unstable";')
  })
  it('needs managed.nix 4', () => {
    expect([3, 4].map((v) => takesFromInputs(host('pc', { managedVersion: v })))).toEqual([false, true])
    expect(takesFromInputs(host('code', { managedFile: null, managedVersion: 4 }))).toBe(false)
  })
})

describe('secrets in homes', () => {
  it('needs managed.nix 5', () => {
    expect([4, 5].map((v) => linksSecrets(host('pc', { managedVersion: v })))).toEqual([false, true])
    expect(linksSecrets(host('code', { managedFile: null, managedVersion: 5 }))).toBe(false)
  })
  it('keeps files with secrets next to the machine\'s managed.json', () => {
    expect(secretTemplatesDir(host('pc', { managedFile: 'machines/pc/managed.json' }), 'pablo')).toBe('machines/pc/home/pablo/templates/')
  })
  it('takes a new path inside ~ that nothing else sets', () => {
    const taken = ['.netrc', '.config/gh/hosts.yml']
    expect(['', ' .config/rclone/rclone.conf '].map((p) => homePathProblem(p, taken))).toEqual(['', ''])
    for (const bad of ['/etc/passwd', '~/.netrc', '../x', 'a//b', './a', 'a/']) expect(homePathProblem(bad, taken)).toMatch(/inside the home folder/)
    expect(homePathProblem('.git/config', taken)).toMatch(/\.git/)
    expect(homePathProblem('.netrc', taken)).toBe('That file is already here.')
  })
})

describe('copy to other machines', () => {
  const pablo = { name: 'pablo', description: '', groups: [], home: null }
  const hosts = [
    host('pc', { managedVersion: 3, homeManager: true, users: [pablo] }),
    host('laptop', { managedVersion: 2, homeManager: true, users: [pablo] }),
    host('old', { managedVersion: 1, homeManager: true, users: [pablo] }),
    host('code', { managedFile: null, managedVersion: 3, homeManager: true, users: [pablo] }),
    host('nohm', { managedVersion: 3, users: [pablo] }),
    host('nouser', { managedVersion: 3, homeManager: true }),
  ]
  it('offers machines whose managed.nix applies customisations, and for a home those with the user and home-manager', () => {
    expect(copyTargets(hosts, 'pc').map((h) => h.name)).toEqual(['laptop', 'nohm', 'nouser'])
    expect(copyTargets(hosts, 'pc', 'pablo').map((h) => h.name)).toEqual(['laptop'])
  })
  it('says where it went, where it was already, and why a machine refused', () => {
    expect(copySummary('htop', [{ host: 'a', changed: true }, { host: 'b', changed: true }])).toEqual({ text: 'Copied htop to a, b.', ok: true })
    expect(
      copySummary('htop', [
        { host: 'a', changed: true },
        { host: 'b', changed: false },
        { host: 'c', changed: false, conflicts: ['packageOverrides.htop', 'hosts/c/packages/htop.nix'] },
        { host: 'd', changed: false, error: 'hosts/d/managed.json is not valid JSON' },
      ]),
    ).toEqual({
      text: 'Copied htop to a. b already has it. Not copied to c, which has another value for packageOverrides.htop, hosts/c/packages/htop.nix. Not copied to d: hosts/d/managed.json is not valid JSON.',
      ok: false,
    })
    expect(copySummary('htop', [{ host: 'a', changed: false }, { host: 'b', changed: false }]).text).toBe('a, b already have it.')
    expect(copySummary('htop', [{ host: 'a', changed: true, error: 'hosts/a/managed.json changed since it was loaded' }])).toEqual({ text: 'Not copied to a: hosts/a/managed.json changed since it was loaded.', ok: false })
  })
})

describe('standalone homes', () => {
  it('give the line for flake.nix, with a string where a Nix path cannot hold the name', () => {
    expect(homeEditingLine('pablo')).toBe('(import ./modules/managed-home.nix ./homes/pablo/managed.json)')
    expect(homeEditingLine('pablo@laptop')).toBe('(import ./modules/managed-home.nix (./homes + "/pablo@laptop/managed.json"))')
  })
  it('keep their files next to the managed.json they load', () => {
    const home = { name: 'pablo@laptop', packages: [], programs: [], files: [], managedFile: null }
    expect(homeDir(home)).toBe('homes/pablo@laptop')
    expect(homeDir({ ...home, managedFile: 'homes/laptop/managed.json' })).toBe('homes/laptop')
  })
  it('list packages as the evaluation has them and as managed.json changes them', () => {
    const home = { name: 'laptop', packages: ['bat', 'nodejs'], programs: [], files: [], managedFile: 'homes/laptop/managed.json', managedPackages: { nodejs_22: 'nodejs' } }
    const m = managed({ path: 'homes/laptop/managed.json', packages: ['nodejs_22', 'jq'], removedPackages: ['nano'] })
    expect(standaloneRows(home, m).map((r) => [r.name, r.attr ?? '', !!r.pending, !!r.removed])).toEqual([
      ['bat', '', false, false],
      ['jq', 'jq', true, false],
      ['nano', '', false, true],
      ['nodejs', 'nodejs_22', false, false],
    ])
  })
})
