import type { StatusFile, TreeEntry } from './api'

export type TreeNode = {
  name: string
  path: string
  dir: boolean
  children: TreeNode[]
  /** Git status letter shown next to the entry ('' when clean). */
  badge: string
  /** Deleted in git but still listed so the deletion is visible. */
  deleted?: boolean
  /** A folder containing changed files. */
  changed?: boolean
}

/** One letter per file; untracked/added/deleted/renamed say more than "modified". */
export function badgeFor(f: StatusFile): string {
  for (const l of ['?', 'A', 'D', 'R', 'M']) if (f.staged === l || f.unstaged === l) return l
  return ''
}

/** Turn the flat, recursive entry list into nested nodes (folders first, then by name). */
export function buildTree(entries: TreeEntry[], files: StatusFile[] = []): TreeNode[] {
  const root: TreeNode = { name: '', path: '', dir: true, children: [], badge: '' }
  const nodes = new Map<string, TreeNode>([['', root]])
  const add = (path: string, dir: boolean): TreeNode => {
    const found = nodes.get(path)
    if (found) return found
    const i = path.lastIndexOf('/')
    const parent = add(i < 0 ? '' : path.slice(0, i), true)
    const node: TreeNode = { name: path.slice(i + 1), path, dir, children: [], badge: '' }
    nodes.set(path, node)
    parent.children.push(node)
    return node
  }
  for (const e of entries) if (e.path) add(e.path, e.dir)

  for (const f of files) {
    const badge = badgeFor(f)
    const path = f.path.replace(/\/+$/, '') // git reports untracked folders as "dir/"
    if (!badge || !path) continue
    let node = nodes.get(path)
    if (!node) {
      if (badge !== 'D') continue
      node = add(path, false)
      node.deleted = true
    }
    node.badge = badge
    for (let p = path; p.includes('/'); ) {
      p = p.slice(0, p.lastIndexOf('/'))
      const parent = nodes.get(p)
      if (parent) parent.changed = true
    }
  }

  const sort = (list: TreeNode[]) => {
    list.sort((a, b) => (a.dir === b.dir ? (a.name < b.name ? -1 : a.name > b.name ? 1 : 0) : a.dir ? -1 : 1))
    for (const n of list) sort(n.children)
  }
  sort(root.children)
  return root.children
}

