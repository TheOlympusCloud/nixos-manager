// Fleet graph: nodes and edges from the evaluated model, before layout.
import type { FeatureGroup, Home, Host, Input, Model, User } from './api'

export type Layers = { features: boolean; files: boolean; packages: boolean; users: boolean; inputs: boolean }
export const defaultLayers: Layers = { features: true, files: false, packages: false, users: false, inputs: false }

type Base = { id: string; label: string; width: number; height: number }
export type GNode = Base &
  (
    | { kind: 'machine'; host: Host }
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

export function buildGraph(model: Model, layers: Layers, expanded: Iterable<string> = []): { nodes: GNode[]; edges: GEdge[] } {
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

  for (const h of model.hosts) nodes.set('m:' + h.name, { id: 'm:' + h.name, kind: 'machine', label: h.name, host: h, ...MACHINE_SIZE })

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
      for (const ft of h.features) {
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
  return { nodes: [...nodes.values()], edges: [...edges.values()] }
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
