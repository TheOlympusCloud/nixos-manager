// Fleet matrices: packages × machines and features × machines.
import type { Feature, FeatureGroup, Host, HostDiff } from './api'
import { isRepoFile } from './graph'

export type PkgCell = { version: string; files: string[]; managed: boolean }
export type PkgRow = { name: string; cells: (PkgCell | null)[]; count: number; nixos: boolean }

/** One row per package name, one cell per machine (in the order given). */
export function packageMatrix(hosts: Host[]): PkgRow[] {
  const rows = new Map<string, PkgRow>()
  hosts.forEach((h, i) => {
    for (const p of h.packages) {
      let row = rows.get(p.name)
      if (!row) rows.set(p.name, (row = { name: p.name, cells: hosts.map(() => null), count: 0, nixos: true }))
      row.cells[i] = { version: p.version, files: p.files, managed: !!h.managedFile && p.files.includes(h.managedFile) }
      row.count++
      if (p.files.some(isRepoFile)) row.nixos = false
    }
  })
  return [...rows.values()].sort((a, b) => a.name.localeCompare(b.name))
}

export const groupOrder: FeatureGroup[] = ['services', 'programs', 'virtualisation', 'hardware']
export const groupLabel: Record<FeatureGroup, string> = { services: 'Services', programs: 'Programs', virtualisation: 'Virtualisation', hardware: 'Hardware' }

export type FeatCell = Feature & { managed: boolean }
export type FeatRow = { key: string; group: FeatureGroup; name: string; description: string; cells: (FeatCell | null)[]; count: number }

/** One row per feature (`group.name`), sorted by group then name; one cell per machine. */
export function featureMatrix(hosts: Host[]): FeatRow[] {
  const rows = new Map<string, FeatRow>()
  hosts.forEach((h, i) => {
    for (const f of h.features) {
      const key = `${f.group}.${f.name}`
      let row = rows.get(key)
      if (!row) rows.set(key, (row = { key, group: f.group, name: f.name, description: f.description, cells: hosts.map(() => null), count: 0 }))
      row.cells[i] = { ...f, managed: !!h.managedFile && f.files.includes(h.managedFile) }
      row.count++
    }
  })
  const rank = (g: FeatureGroup) => (groupOrder.indexOf(g) + 5) % 5 // unknown groups last
  return [...rows.values()].sort((a, b) => rank(a.group) - rank(b.group) || a.name.localeCompare(b.name))
}

// ---- model diff (working copy against the last commit) ----

export type DiffPart = { label: string; added: string[]; removed: string[] }

/** The non-empty parts of a machine's diff, in a fixed order. */
export function diffParts(d: HostDiff): DiffPart[] {
  return [
    { label: 'Packages', added: d.packagesAdded ?? [], removed: d.packagesRemoved ?? [] },
    { label: 'Features', added: d.featuresAdded ?? [], removed: d.featuresRemoved ?? [] },
    { label: 'Users', added: d.usersAdded ?? [], removed: d.usersRemoved ?? [] },
    { label: 'Ports', added: d.portsAdded ?? [], removed: d.portsRemoved ?? [] },
  ].filter((p) => p.added.length || p.removed.length)
}

/** One line for a machine's diff, e.g. "+2 −1 packages · +1 features". */
export function diffSummary(d: HostDiff): string {
  if (d.status === 'unchanged') return 'No changes'
  if (d.status === 'error') return 'Does not evaluate'
  const parts = diffParts(d).map((p) => [p.added.length && `+${p.added.length}`, p.removed.length && `−${p.removed.length}`, p.label.toLowerCase()].filter(Boolean).join(' '))
  const lead = d.status === 'added' ? 'New machine' : d.status === 'removed' ? 'Machine removed' : ''
  return [lead, ...parts].filter(Boolean).join(' · ') || 'Changed'
}

/** Changed machines first (errors, then added, changed, removed), unchanged ones apart. */
export function splitDiffs(ds: HostDiff[]): { changed: HostDiff[]; unchanged: HostDiff[] } {
  const order = ['error', 'added', 'changed', 'removed']
  const changed = ds.filter((d) => d.status !== 'unchanged').sort((a, b) => order.indexOf(a.status) - order.indexOf(b.status) || a.host.localeCompare(b.host))
  return { changed, unchanged: ds.filter((d) => d.status === 'unchanged') }
}
