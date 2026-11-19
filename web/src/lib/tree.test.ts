import { describe, expect, it } from 'vitest'
import { badgeFor, buildTree, type TreeNode } from './tree'

const names = (nodes: TreeNode[]) => nodes.map((n) => n.name)

describe('buildTree', () => {
  const entries = [
    { path: 'flake.nix', dir: false },
    { path: 'hosts', dir: true },
    { path: 'hosts/web', dir: true },
    { path: 'hosts/web/default.nix', dir: false },
    { path: 'hosts/empty', dir: true },
    { path: 'README.md', dir: false },
    { path: 'home', dir: true },
  ]

  it('nests entries with folders first, then by name', () => {
    const tree = buildTree(entries)
    expect(names(tree)).toEqual(['home', 'hosts', 'README.md', 'flake.nix'])
    const hosts = tree[1]
    expect(names(hosts.children)).toEqual(['empty', 'web'])
    expect(hosts.children[1].children[0]).toMatchObject({ name: 'default.nix', path: 'hosts/web/default.nix', dir: false })
    expect(hosts.children[0].children).toEqual([])
  })

  it('creates missing parent folders', () => {
    const tree = buildTree([{ path: 'a/b/c.nix', dir: false }])
    expect(tree[0]).toMatchObject({ name: 'a', dir: true })
    expect(tree[0].children[0]).toMatchObject({ name: 'b', path: 'a/b', dir: true })
  })

  it('adds status badges, deleted files and changed-folder markers', () => {
    const tree = buildTree(entries, [
      { path: 'hosts/web/default.nix', staged: '', unstaged: 'M' },
      { path: 'hosts/old.nix', staged: 'D', unstaged: '' },
      { path: 'hosts/empty/', staged: '', unstaged: '?' },
      { path: 'flake.nix', staged: '', unstaged: '' },
    ])
    const hosts = tree.find((n) => n.name === 'hosts')!
    expect(hosts.changed).toBe(true)
    expect(names(hosts.children)).toEqual(['empty', 'web', 'old.nix'])
    expect(hosts.children[0].badge).toBe('?')
    expect(hosts.children[1].children[0].badge).toBe('M')
    expect(hosts.children[2]).toMatchObject({ badge: 'D', deleted: true })
    expect(tree.find((n) => n.name === 'flake.nix')!.badge).toBe('')
    expect(tree.find((n) => n.name === 'home')!.changed).toBeUndefined()
  })

  it('ignores status entries for missing paths unless deleted', () => {
    expect(buildTree([], [{ path: 'ghost.nix', staged: '', unstaged: 'M' }])).toEqual([])
  })
})

describe('badgeFor', () => {
  it('prefers the most telling letter', () => {
    expect(badgeFor({ path: 'a', staged: 'A', unstaged: 'M' })).toBe('A')
    expect(badgeFor({ path: 'a', staged: 'M', unstaged: 'M' })).toBe('M')
    expect(badgeFor({ path: 'a', staged: 'R', unstaged: '' })).toBe('R')
    expect(badgeFor({ path: 'a', staged: '', unstaged: '?' })).toBe('?')
    expect(badgeFor({ path: 'a', staged: '', unstaged: '' })).toBe('')
  })
})
