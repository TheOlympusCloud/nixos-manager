// What a change does to the machines, in NixOS words: "firefox 145.0 → 146.0", "jellyfin turned on".
import type { HostDiff, InputChange } from './api'
import { plural } from './format'

export type ImpactItem = { text: string; title?: string; kind: 'add' | 'del' | 'up' }
export type ImpactGroup = { label: string; items: ImpactItem[] }
/** An option set here (managed.json) that a change adds, changes or resets. */
export type SettingChange = { path: string[]; from?: unknown; to?: unknown }
/** A machine's diff plus the settings its managed.json changes. */
export type Impact = HostDiff & { settings?: SettingChange[] }

const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)

/** Leaf settings (lists are leaves) that differ between two managed.json `settings` objects, in path order. */
export function settingChanges(before: unknown, after: unknown, path: string[] = []): SettingChange[] {
  const b = isObj(before)
  const a = isObj(after)
  if (!b && !a) return JSON.stringify(before) === JSON.stringify(after) ? [] : [{ path, from: before, to: after }]
  // A leaf that became a set of options, or the reverse, counts as reset (or set) on its own.
  const out: SettingChange[] = []
  if (!b && before !== undefined) out.push({ path, from: before })
  if (!a && after !== undefined) out.push({ path, to: after })
  const bo = b ? before : {}
  const ao = a ? after : {}
  const keys = [...new Set([...Object.keys(bo), ...Object.keys(ao)])].sort()
  return [...out, ...keys.flatMap((k) => settingChanges(bo[k], ao[k], [...path, k]))]
}

/** `"weekly"`, `true`, `[ "a" "b" ]`: a value as Nix would show it, shortened. */
export function nixValue(v: unknown): string {
  const s = Array.isArray(v) ? `[ ${v.map((x) => JSON.stringify(x)).join(' ')} ]` : JSON.stringify(v)
  return s.length > 48 ? s.slice(0, 47) + '…' : s
}

const settingText = (c: SettingChange) => (c.to === undefined ? `${c.path.join('.')} reset` : `${c.path.join('.')} = ${nixValue(c.to)}`)
/** "fail2ban turned on", "PasswordAuthentication = false": one setting, in few words. */
function settingShort(c: SettingChange): string {
  const last = c.path.at(-1) ?? ''
  if (last === 'enable' && c.path.length > 1 && typeof c.to === 'boolean') return `${c.path.at(-2)} turned ${c.to ? 'on' : 'off'}`
  return c.to === undefined ? `${last} reset to its default` : `${last} = ${nixValue(c.to)}`
}

/** Fold managed.json setting changes into the machine diffs; an unchanged machine with settings becomes changed. */
export function withSettings(ds: HostDiff[], settings: Record<string, SettingChange[]>): Impact[] {
  return ds.map((d) => {
    const s = settings[d.host]
    if (!s?.length || d.status === 'error' || d.status === 'removed') return d
    return { ...d, settings: s, status: d.status === 'unchanged' ? 'changed' : d.status }
  })
}

/** `services.desktopManager.plasma6` → `desktopManager.plasma6` (the group is shown by the label). */
const featureName = (key: string) => key.replace(/^[a-z]+\./, '')

/** The changes of one machine, grouped for the expanded view. Empty groups are left out. */
export function impactGroups(d: Impact): ImpactGroup[] {
  const items = (xs: string[] | undefined, kind: ImpactItem['kind'], name = (s: string) => s) => (xs ?? []).map((s) => ({ text: name(s), title: s, kind }))
  return [
    { label: 'Updated', items: (d.versionsChanged ?? []).map((v) => ({ text: `${v.name} ${v.from} → ${v.to}`, kind: 'up' as const })) },
    {
      label: 'Settings',
      items: (d.settings ?? []).map((c) => ({ text: settingText(c), title: c.from === undefined ? 'Was not set here' : `Was ${nixValue(c.from)}`, kind: c.to === undefined ? ('del' as const) : ('up' as const) })),
    },
    { label: 'Installed', items: items(d.packagesAdded, 'add') },
    { label: 'Removed', items: items(d.packagesRemoved, 'del') },
    { label: 'Turned on', items: items(d.featuresAdded, 'add', featureName) },
    { label: 'Turned off', items: items(d.featuresRemoved, 'del', featureName) },
    { label: 'Users', items: [...items(d.usersAdded, 'add'), ...items(d.usersRemoved, 'del')] },
    { label: 'Ports', items: [...items(d.portsAdded, 'add'), ...items(d.portsRemoved, 'del')] },
  ].filter((g) => g.items.length)
}

/** "a", "a and b", or "3 packages" when there are more than two. */
function few(xs: string[], what: string, many = what + 's') {
  return xs.length <= 2 ? xs.join(' and ') : plural(xs.length, what, many)
}

/** One readable line per machine, e.g. "curl 8.16.0 → 8.17.0 and 2 more updated · jellyfin turned on". */
export function impactLine(d: Impact): string {
  if (d.status === 'error') return 'Does not evaluate'
  if (d.status === 'unchanged') return 'No change'
  if (d.status === 'removed') return 'Removed from the flake'
  if (d.status === 'added') {
    const pk = d.packagesAdded?.length ?? 0
    const ft = d.featuresAdded?.length ?? 0
    return ['New machine', pk && plural(pk, 'package'), ft && plural(ft, 'service or program', 'services and programs')].filter(Boolean).join(' · ')
  }
  const parts: string[] = []
  const v = d.versionsChanged ?? []
  if (v.length) parts.push(`${v[0].name} ${v[0].from} → ${v[0].to}${v.length > 1 ? ` and ${v.length - 1} more updated` : ''}`)
  const st = d.settings ?? []
  if (st.length) parts.push(st.length === 1 ? settingShort(st[0]) : `${st.length} settings changed`)
  if (d.packagesAdded?.length) parts.push(`${few(d.packagesAdded, 'package')} installed`)
  if (d.packagesRemoved?.length) parts.push(`${few(d.packagesRemoved, 'package')} removed`)
  if (d.featuresAdded?.length) parts.push(`${few(d.featuresAdded.map(featureName), 'service or program', 'services and programs')} turned on`)
  if (d.featuresRemoved?.length) parts.push(`${few(d.featuresRemoved.map(featureName), 'service or program', 'services and programs')} turned off`)
  if (d.usersAdded?.length) parts.push(`${d.usersAdded.length <= 2 ? 'user ' : ''}${few(d.usersAdded, 'user')} added`)
  if (d.usersRemoved?.length) parts.push(`${d.usersRemoved.length <= 2 ? 'user ' : ''}${few(d.usersRemoved, 'user')} removed`)
  if (d.portsAdded?.length) parts.push(`${d.portsAdded.length <= 2 ? 'port ' : ''}${few(d.portsAdded, 'port')} opened`)
  if (d.portsRemoved?.length) parts.push(`${d.portsRemoved.length <= 2 ? 'port ' : ''}${few(d.portsRemoved, 'port')} closed`)
  return parts.join(' · ') || 'Changed'
}

/** Changed machines (new, changed, removed), the ones that do not evaluate, and the unchanged ones. */
export function splitImpact<T extends HostDiff>(ds: T[]): { changed: T[]; broken: T[]; unchanged: T[] } {
  const order = ['added', 'changed', 'removed']
  return {
    changed: ds.filter((d) => order.includes(d.status)).sort((a, b) => order.indexOf(a.status) - order.indexOf(b.status) || a.host.localeCompare(b.host)),
    broken: ds.filter((d) => d.status === 'error'),
    unchanged: ds.filter((d) => d.status === 'unchanged'),
  }
}

export const shortRev = (rev?: string) => (rev ?? '').slice(0, 7)

/** "9 days newer", "same day", "3 days older" between an input's two revisions. */
export function revDistance(c: InputChange): string {
  if (!c.fromDate || !c.toDate) return ''
  const d = Math.round((c.toDate - c.fromDate) / 86400)
  if (d === 0) return 'same day'
  return `${plural(Math.abs(d), 'day')} ${d > 0 ? 'newer' : 'older'}`
}
