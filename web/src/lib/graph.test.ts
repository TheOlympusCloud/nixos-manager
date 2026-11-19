import { describe, expect, it } from 'vitest'
import { buildGraph, defaultLayers, packChips, related, searchModel, type GNode } from './graph'
import { testModel } from './testmodel'

const ids = (xs: { id: string }[]) => xs.map((x) => x.id).sort()
const find = (nodes: GNode[], id: string) => nodes.find((n) => n.id === id)

describe('buildGraph', () => {
  it('draws machines, one node per shared feature, and single-machine features as chips', () => {
    const g = buildGraph(testModel, defaultLayers)
    expect(ids(g.nodes)).toEqual(['f:services.openssh', 'm:a', 'm:b', 'm:broken'])
    expect(ids(g.edges)).toEqual(['m:a->f:services.openssh', 'm:b->f:services.openssh'])
    const ssh = find(g.nodes, 'f:services.openssh')
    expect(ssh?.kind === 'feature' && ssh.hosts.map((h) => h.host)).toEqual(['a', 'b'])
    expect(g.edges[0].group).toBe('services')
    const a = find(g.nodes, 'm:a')
    expect(a?.kind === 'machine' && [a.chips.map((f) => f.name), a.more, a.height]).toEqual([['steam'], 0, 175])
    expect(find(g.nodes, 'm:broken')?.height).toBe(136)
    // Not drawn, but the details panel can still show them.
    expect([...g.solo.keys()].sort()).toEqual(['f:programs.steam', 'f:services.home-assistant'])
  })

  it('keeps every feature a node with a single healthy machine, and no chips without the layer', () => {
    const one = { ...testModel, hosts: testModel.hosts.filter((h) => h.name !== 'b') }
    expect(ids(buildGraph(one, defaultLayers).nodes)).toEqual(['f:programs.steam', 'f:services.openssh', 'm:a', 'm:broken'])
    const off = buildGraph(testModel, { ...defaultLayers, features: false })
    expect(off.nodes.every((n) => n.kind !== 'machine' || n.chips.length === 0)).toBe(true)
  })

  it('keeps a machine that does not evaluate, without anything attached', () => {
    const g = buildGraph(testModel, { features: true, files: true, packages: true, users: true, inputs: false })
    expect(g.edges.some((e) => e.source === 'm:broken' || e.target === 'm:broken')).toBe(false)
    expect(find(g.nodes, 'm:broken')?.kind).toBe('machine')
  })

  it('routes features through shared files when the files layer is on', () => {
    const g = buildGraph(testModel, { ...defaultLayers, files: true })
    // Shared by a and b: flake.nix and modules/common.nix. Single-machine files stay off the graph.
    expect(ids(g.nodes).filter((id) => id.startsWith('file:'))).toEqual(['file:flake.nix', 'file:modules/common.nix'])
    expect(ids(g.edges)).toEqual([
      'file:modules/common.nix->f:services.openssh',
      'm:a->file:flake.nix',
      'm:a->file:modules/common.nix',
      'm:b->file:flake.nix',
      'm:b->file:modules/common.nix',
    ])
  })

  it('groups packages per machine and shares expanded packages between machines', () => {
    const layers = { ...defaultLayers, features: false, packages: true }
    const g = buildGraph(testModel, layers)
    expect(ids(g.nodes)).toEqual(['m:a', 'm:b', 'm:broken', 'pg:a', 'pg:b'])
    const pa = find(g.nodes, 'pg:a')
    // Repo packages come first in the preview, NixOS defaults last.
    expect(pa?.kind === 'pkgs' && [pa.count, pa.preview]).toEqual([5, 'git, htop, vlc + 2 more'])

    const open = buildGraph(testModel, layers, ['a', 'b'])
    expect(ids(open.nodes).filter((id) => id.startsWith('p:'))).toEqual(['p:git', 'p:htop', 'p:nano', 'p:vlc', 'p:vscode'])
    const git = find(open.nodes, 'p:git')
    expect(git?.kind === 'package' && git.hosts.map((h) => h.host)).toEqual(['a', 'b'])
    expect(open.edges.filter((e) => e.target === 'p:git').map((e) => e.source)).toEqual(['pg:a', 'pg:b'])
    const only = buildGraph(testModel, layers, ['b'])
    expect(ids(only.nodes).filter((id) => id.startsWith('p:'))).toEqual(['p:git', 'p:nano'])
  })

  it('adds users, standalone homes and inputs feeding every machine', () => {
    const g = buildGraph(testModel, { features: false, files: false, packages: false, users: true, inputs: true })
    expect(ids(g.nodes)).toEqual(['h:pablo@laptop', 'i:comin', 'i:nixpkgs', 'm:a', 'm:b', 'm:broken', 'u:pablo'])
    const pablo = find(g.nodes, 'u:pablo')
    expect(pablo?.kind === 'user' && pablo.hosts.map((h) => [h.host, h.user.groups.length])).toEqual([
      ['a', 1],
      ['b', 2],
    ])
    expect(g.edges.filter((e) => e.source === 'i:nixpkgs').map((e) => e.target)).toEqual(['m:a', 'm:b', 'm:broken'])
  })
})

describe('related', () => {
  const g = buildGraph(testModel, { ...defaultLayers, files: true, inputs: true })

  it('lights a shared feature, its file and every machine that has it, but not the inputs', () => {
    expect([...related(g.edges, 'f:services.openssh').nodes].sort()).toEqual(['f:services.openssh', 'file:modules/common.nix', 'm:a', 'm:b'])
  })

  it('lights what a machine runs and the inputs it uses, without crossing into other machines', () => {
    const r = related(g.edges, 'm:b')
    expect(r.nodes.has('f:services.openssh')).toBe(true)
    expect(r.nodes.has('i:nixpkgs')).toBe(true)
    expect(r.nodes.has('m:a')).toBe(false)
    expect(r.edges.has('m:b->file:modules/common.nix')).toBe(true)
  })

  it('stops an input at the machines', () => {
    expect([...related(g.edges, 'i:comin').nodes].sort()).toEqual(['i:comin', 'm:a', 'm:b', 'm:broken'])
  })
})

describe('packChips', () => {
  const f = (name: string, group: 'services' | 'programs' = 'services') => ({ group, name, files: [], description: '' })

  it('sorts by group, wraps onto a second row and ends with a +N chip when full', () => {
    expect(packChips([f('zsh', 'programs'), f('nginx')])).toEqual({ chips: [f('nginx'), f('zsh', 'programs')], more: 0, rows: 1 })
    const many = Array.from({ length: 12 }, (_, i) => f('svc-' + String(i).padStart(2, '0')))
    const r = packChips(many)
    expect(r.rows).toBe(2)
    expect(r.chips.length + r.more).toBe(12)
    expect(r.more).toBeGreaterThan(0)
    // Three chips fit per row, and the +N chip still fits after the sixth.
    expect([r.chips.length, r.more]).toEqual([6, 6])
  })

  it('gives up a chip of a full last row for the +N chip', () => {
    const r = packChips(Array.from({ length: 7 }, (_, i) => f('feature' + i)))
    expect([r.chips.length, r.more, r.rows]).toEqual([5, 2, 2])
  })

  it('is empty without features', () => {
    expect(packChips([])).toEqual({ chips: [], more: 0, rows: 0 })
  })
})

describe('searchModel', () => {
  it('finds things in hidden layers and says what to turn on', () => {
    const [vlc] = searchModel(testModel, 'vlc')
    expect(vlc).toMatchObject({ select: 'p:vlc', at: 'p:vlc', kind: 'Package', layer: 'packages', expand: ['a'] })
    const [ha] = searchModel(testModel, 'home-assist')
    // Only b has it: a chip in b's card, so the graph centers on b.
    expect(ha).toMatchObject({ select: 'f:services.home-assistant', at: 'm:b', layer: 'features' })
    expect(searchModel(testModel, 'openssh')[0]).toMatchObject({ at: 'f:services.openssh', sub: 'services · a, b' })
    expect(searchModel(testModel, 'comin')[0]).toMatchObject({ select: 'i:comin', layer: 'inputs' })
    expect(searchModel(testModel, 'laptop')[0]).toMatchObject({ select: 'h:pablo@laptop', kind: 'Home', layer: 'users' })
  })

  it('leads a single-machine file to its machine and a shared file to its node', () => {
    expect(searchModel(testModel, 'hosts/a/default')[0]).toMatchObject({ select: 'm:a', kind: 'File', sub: 'a' })
    expect(searchModel(testModel, 'common.nix')[0]).toMatchObject({ select: 'file:modules/common.nix', layer: 'files' })
  })

  it('ranks exact and prefix matches first and caps the list', () => {
    expect(searchModel(testModel, 'a').length).toBeLessThanOrEqual(8)
    expect(searchModel(testModel, 'a')[0]).toMatchObject({ select: 'm:a', kind: 'Machine' })
    expect(searchModel(testModel, 'git').map((h) => h.label)).toEqual(['git'])
    expect(searchModel(testModel, '  ')).toEqual([])
  })
})
