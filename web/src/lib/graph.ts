// Fleet graph: nodes and edges from the evaluated model, before layout.
import type { Feature, FeatureGroup, Home, Host, Input, Model, User } from './api'

export type Layers = { features: boolean; files: boolean; packages: boolean; users: boolean; inputs: boolean }
export const defaultLayers: Layers = { features: true, files: false, packages: false, users: false, inputs: false }

type Base = { id: string; label: string; width: number; height: number }
export type GNode = Base &
  (
    /** chips: features only this machine has, drawn inside its card (more: how many did not fit). */
    | { kind: 'machine'; host: Host; chips: Feature[]; more: number }
    | { kind: 'feature'; group: FeatureGroup; name: string; description: string; hosts: { host: string; files: string[] }[] }
    | { kind: 'file'; path: string; hosts: string[] }
    | { kind: 'pkgs'; host: string; count: number; preview: string; expanded: boolean }
    | { kind: 'package'; name: string; hosts: { host: string; version: string; files: string[] }[] }
    | { kind: 'user'; name: string; hosts: { host: string; user: User }[] }
    | { kind: 'home'; home: Home }
    | { kind: 'input'; input: Input }
  )
export type GEdge = { id: string; source: string; target: string; group?: FeatureGroup }

export const featureKey = (group: string, name: string) => `${group}.${name}`
/** Files that are not repo paths: "nixos" marks what NixOS or an input sets by default. */
export const isRepoFile = (f: string) => f !== 'nixos'

// Sizes are fixed per kind (estimated from the label) so the layout never waits for the DOM.
const pillWidth = (label: string, extra = 0) => Math.round(Math.min(280, Math.max(96, 58 + extra + label.length * 7.3)))
const MACHINE_SIZE = { width: 252, height: 136 }
const groupRank: Record<string, number> = { services: 0, programs: 1, virtualisation: 2, hardware: 3 }

/** Machine chips: up to `rows` rows of `room` px, the last ending in a "+N" chip when some do not fit. */
export function packChips(features: Feature[], room = 224, rows = 2): { chips: Feature[]; more: number; rows: number } {
  const width = (label: string) => 20 + label.length * 6.2 // 4 px gap included
  const sorted = [...features].sort((a, b) => (groupRank[a.group] ?? 9) - (groupRank[b.group] ?? 9) || a.name.localeCompare(b.name))
  const used = [0]
  const chips: Feature[] = []
  for (const f of sorted) {
    const w = width(f.name)
    if (used.at(-1)! && used.at(-1)! + w > room) {
      if (used.length === rows) break
      used.push(0)
    }
    used[used.length - 1] += w
    chips.push(f)
  }
  // Make room for the "+N" chip at the end of the last row.
  while (chips.length < sorted.length && chips.length && used.at(-1)! + width('+99') > room) used[used.length - 1] -= width(chips.pop()!.name)
  return { chips, more: sorted.length - chips.length, rows: chips.length ? used.length : 0 }
}

/** Machine node height: the card, then a divider and the chip rows. */
export const machineHeight = (chips: number, rows: number) => (chips ? MACHINE_SIZE.height + 19 + rows * 20 + (rows - 1) * 4 : MACHINE_SIZE.height)

export function buildGraph(
  model: Model,
  layers: Layers,
  expanded: Iterable<string> = [],
): { nodes: GNode[]; edges: GEdge[]; solo: Map<string, GNode & { kind: 'feature' }> } {
  const open = new Set(expanded)
  const nodes = new Map<string, GNode>()
  const edges = new Map<string, GEdge>()
  const edge = (source: string, target: string, group?: FeatureGroup) => {
    const id = `${source}->${target}`
    if (!edges.has(id)) edges.set(id, { id, source, target, group })
  }
  const ok = model.hosts.filter((h) => !h.error)

  // A file is shared when two or more machines are configured by it; only those become nodes.
  const users = new Map<string, number>()
  for (const h of ok) for (const f of new Set(h.files)) if (isRepoFile(f)) users.set(f, (users.get(f) ?? 0) + 1)
  const shared = (f: string) => layers.files && (users.get(f) ?? 0) > 1

  // A feature only one machine has is a chip in that machine's card, not a node (with two or more machines).
  const featureUsers = new Map<string, number>()
  for (const h of ok) for (const f of h.features) featureUsers.set(featureKey(f.group, f.name), (featureUsers.get(featureKey(f.group, f.name)) ?? 0) + 1)
  const isSolo = (f: Feature) => layers.features && ok.length > 1 && featureUsers.get(featureKey(f.group, f.name)) === 1
  const solo = new Map<string, GNode & { kind: 'feature' }>()

  for (const h of model.hosts) {
    const mine = h.error ? [] : h.features.filter(isSolo)
    const { chips, more, rows } = packChips(mine)
    nodes.set('m:' + h.name, { id: 'm:' + h.name, kind: 'machine', label: h.name, host: h, chips, more, width: MACHINE_SIZE.width, height: machineHeight(chips.length, rows) })
    for (const f of mine) {
      const id = 'f:' + featureKey(f.group, f.name)
      solo.set(id, { id, kind: 'feature', label: f.name, group: f.group, name: f.name, description: f.description, hosts: [{ host: h.name, files: f.files }], width: 0, height: 0 })
    }
  }

  for (const h of ok) {
    const m = 'm:' + h.name
    for (const f of h.files.filter(shared)) {
      const id = 'file:' + f
      const n = nodes.get(id)
      if (n?.kind === 'file') n.hosts.push(h.name)
      else nodes.set(id, { id, kind: 'file', label: f, path: f, hosts: [h.name], width: pillWidth(f, 8), height: 32 })
      edge(m, id)
    }

    if (layers.features)
      for (const ft of h.features.filter((f) => !isSolo(f))) {
        const key = featureKey(ft.group, ft.name)
        const id = 'f:' + key
        const n = nodes.get(id)
        if (n?.kind === 'feature') n.hosts.push({ host: h.name, files: ft.files })
        else
          nodes.set(id, {
            id,
            kind: 'feature',
            label: ft.name,
            group: ft.group,
            name: ft.name,
            description: ft.description,
            hosts: [{ host: h.name, files: ft.files }],
            width: pillWidth(ft.name),
            height: 32,
          })
        const via = ft.files.filter(shared)
        if (via.length) for (const f of via) edge('file:' + f, id, ft.group)
        else edge(m, id, ft.group)
      }

    if (layers.packages && h.packages.length) {
      const id = 'pg:' + h.name
      const names = [...h.packages.filter((p) => p.files.some(isRepoFile)), ...h.packages.filter((p) => !p.files.some(isRepoFile))].map((p) => p.name)
      const preview = names.slice(0, 3).join(', ') + (names.length > 3 ? ` + ${names.length - 3} more` : '')
      nodes.set(id, { id, kind: 'pkgs', label: `${h.packages.length} packages`, host: h.name, count: h.packages.length, preview, expanded: open.has(h.name), width: 236, height: 54 })
      edge(m, id)
      if (open.has(h.name))
        for (const p of h.packages) {
          const pid = 'p:' + p.name
          const n = nodes.get(pid)
          const ref = { host: h.name, version: p.version, files: p.files }
          if (n?.kind === 'package') n.hosts.push(ref)
          else nodes.set(pid, { id: pid, kind: 'package', label: p.name, name: p.name, hosts: [ref], width: pillWidth(p.name), height: 32 })
          edge(id, pid)
        }
    }

    if (layers.users)
      for (const u of h.users) {
        const id = 'u:' + u.name
        const n = nodes.get(id)
        if (n?.kind === 'user') n.hosts.push({ host: h.name, user: u })
        else nodes.set(id, { id, kind: 'user', label: u.name, name: u.name, hosts: [{ host: h.name, user: u }], width: pillWidth(u.name, 40), height: 46 })
        edge(m, id)
      }
  }

  if (layers.users)
    for (const home of model.homes) nodes.set('h:' + home.name, { id: 'h:' + home.name, kind: 'home', label: home.name, home, width: pillWidth(home.name, 40), height: 46 })

  if (layers.inputs)
    for (const input of model.inputs) {
      const id = 'i:' + input.name
      nodes.set(id, { id, kind: 'input', label: input.name, input, width: pillWidth(input.name, 120), height: 46 })
      for (const h of model.hosts) edge(id, 'm:' + h.name)
    }

  // Shared nodes show how many machines use them: make room for the count.
  for (const n of nodes.values()) if ((n.kind === 'feature' || n.kind === 'package' || n.kind === 'file') && n.hosts.length > 1) n.width = Math.min(300, n.width + 26)
  return { nodes: [...nodes.values()], edges: [...edges.values()], solo }
}

/**
 * What to light up when a node is hovered or selected: everything upstream and
 * downstream of it, without passing through another machine (so a shared feature
 * lights its machines, and a machine lights what it runs, not the whole fleet).
 */
export function related(edges: GEdge[], id: string): { nodes: Set<string>; edges: Set<string> } {
  const nodes = new Set([id])
  const hit = new Set<string>()
  const walk = (from: string, down: boolean) => {
    const queue = [from]
    while (queue.length) {
      const cur = queue.shift()!
      if (cur !== id && cur.startsWith('m:')) continue
      for (const e of edges) {
        const [a, b] = down ? [e.source, e.target] : [e.target, e.source]
        if (a !== cur) continue
        hit.add(e.id)
        if (!nodes.has(b)) {
          nodes.add(b)
          queue.push(b)
        }
      }
    }
  }
  walk(id, true)
  walk(id, false)
  return { nodes, edges: hit }
}

/**
 * A search result anywhere in the model. `select` is what the details panel shows, `at` the
 * node to center on; `layer` and `expand` say what must be drawn first.
 */
export type Hit = { select: string; at: string; label: string; kind: string; sub: string; layer?: keyof Layers; expand?: string[] }

const kindRank = ['Machine', 'Feature', 'Package', 'File', 'User', 'Home', 'Input']
const machinesText = (hosts: string[]) => (hosts.length > 2 ? `${hosts.length} machines` : hosts.join(', '))

/** Search machines, features, packages, files, users, homes and inputs, whatever layers are on. */
export function searchModel(model: Model, query: string, limit = 8): Hit[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const ok = model.hosts.filter((h) => !h.error)
  const hits: Hit[] = []
  const by = <T>(get: (h: Host) => T[], key: (x: T) => string) => {
    const m = new Map<string, { item: T; hosts: string[] }>()
    for (const h of ok)
      for (const x of get(h)) {
        const k = key(x)
        if (!k.toLowerCase().includes(q)) continue
        const e = m.get(k)
        if (e) e.hosts.push(h.name)
        else m.set(k, { item: x, hosts: [h.name] })
      }
    return m
  }

  for (const h of model.hosts)
    if (h.name.toLowerCase().includes(q)) hits.push({ select: 'm:' + h.name, at: 'm:' + h.name, label: h.name, kind: 'Machine', sub: h.error ? 'does not evaluate' : '' })
  for (const [key, { item, hosts }] of by((h) => h.features, (f) => featureKey(f.group, f.name))) {
    // A feature on one machine is a chip in its card: center on the machine.
    const at = hosts.length === 1 && ok.length > 1 ? 'm:' + hosts[0] : 'f:' + key
    hits.push({ select: 'f:' + key, at, label: item.name, kind: 'Feature', sub: `${item.group} · ${machinesText(hosts)}`, layer: 'features' })
  }
  for (const [name, { hosts }] of by((h) => h.packages, (p) => p.name))
    hits.push({ select: 'p:' + name, at: 'p:' + name, label: name, kind: 'Package', sub: machinesText(hosts), layer: 'packages', expand: hosts })
  for (const [path, { hosts }] of by((h) => h.files.filter(isRepoFile), (f) => f))
    hits.push(
      // Only files shared by two or more machines are drawn; a machine's own file leads to the machine.
      hosts.length > 1
        ? { select: 'file:' + path, at: 'file:' + path, label: path, kind: 'File', sub: machinesText(hosts), layer: 'files' }
        : { select: 'm:' + hosts[0], at: 'm:' + hosts[0], label: path, kind: 'File', sub: hosts[0] },
    )
  for (const [name, { hosts }] of by((h) => h.users, (u) => u.name))
    hits.push({ select: 'u:' + name, at: 'u:' + name, label: name, kind: 'User', sub: machinesText(hosts), layer: 'users' })
  for (const home of model.homes)
    if (home.name.toLowerCase().includes(q)) hits.push({ select: 'h:' + home.name, at: 'h:' + home.name, label: home.name, kind: 'Home', sub: 'home-manager', layer: 'users' })
  for (const input of model.inputs)
    if (input.name.toLowerCase().includes(q)) hits.push({ select: 'i:' + input.name, at: 'i:' + input.name, label: input.name, kind: 'Input', sub: input.url, layer: 'inputs' })

  // Exact names first, then names that start with the query, then by kind and name.
  const rank = (h: Hit) => {
    const l = h.label.toLowerCase()
    const base = l.slice(l.lastIndexOf('/') + 1)
    return l === q || base === q ? 0 : l.startsWith(q) || base.startsWith(q) ? 1 : 2
  }
  return hits.sort((a, b) => rank(a) - rank(b) || kindRank.indexOf(a.kind) - kindRank.indexOf(b.kind) || a.label.localeCompare(b.label)).slice(0, limit)
}
