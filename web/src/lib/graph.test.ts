import { describe, expect, it } from 'vitest'
import { buildGraph, defaultLayers, related, type GNode } from './graph'
import { testModel } from './testmodel'

const ids = (xs: { id: string }[]) => xs.map((x) => x.id).sort()
const find = (nodes: GNode[], id: string) => nodes.find((n) => n.id === id)

describe('buildGraph', () => {
  it('draws machines and shares one node per feature between machines', () => {
    const g = buildGraph(testModel, defaultLayers)
    expect(ids(g.nodes)).toEqual(['f:programs.steam', 'f:services.home-assistant', 'f:services.openssh', 'm:a', 'm:b', 'm:broken'])
    expect(ids(g.edges)).toEqual(['m:a->f:programs.steam', 'm:a->f:services.openssh', 'm:b->f:services.home-assistant', 'm:b->f:services.openssh'])
    const ssh = find(g.nodes, 'f:services.openssh')
    expect(ssh?.kind === 'feature' && ssh.hosts.map((h) => h.host)).toEqual(['a', 'b'])
    expect(g.edges.find((e) => e.target === 'f:programs.steam')?.group).toBe('programs')
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
      'm:a->f:programs.steam',
      'm:a->file:flake.nix',
      'm:a->file:modules/common.nix',
      'm:b->f:services.home-assistant',
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
    expect(r.nodes.has('f:services.home-assistant')).toBe(true)
    expect(r.nodes.has('i:nixpkgs')).toBe(true)
    expect(r.nodes.has('m:a')).toBe(false)
    expect(r.nodes.has('f:programs.steam')).toBe(false)
    expect(r.edges.has('m:b->file:modules/common.nix')).toBe(true)
  })

  it('stops an input at the machines', () => {
    expect([...related(g.edges, 'i:comin').nodes].sort()).toEqual(['i:comin', 'm:a', 'm:b', 'm:broken'])
  })
})
