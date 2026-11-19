// Effective option values: where they come from, how to show them, what can be copied.
import type { HostValue } from './api'
import { pathName } from './options'

/** Where a value comes from: managed.json ("Set here"), a hand-written file, or NixOS and the inputs. */
export type Source = { kind: 'here' } | { kind: 'code'; file: string; files: string[] } | { kind: 'default' }

const isRepo = (f: string) => f !== 'nixos'

/** files: the repo files that define the value (from /api/fleet/values). */
export function sourceOf(files: string[], managedFile: string | null): Source {
  // The module that applies managed.json reports its definitions as the JSON file; older setups as the module.
  if (managedFile && files.some((f) => f === managedFile || f === 'modules/managed.nix')) return { kind: 'here' }
  const code = files.filter(isRepo)
  return code.length ? { kind: 'code', file: code[0], files: code } : { kind: 'default' }
}

export const sourceLabel = (s: Source) => (s.kind === 'here' ? 'Set here' : s.kind === 'code' ? `In code · ${s.file}` : 'Default')

export type Effective =
  | { state: 'value'; value: unknown; source: Source; pending: boolean }
  | { state: 'missing' } // the machine has no such option
  | { state: 'error'; error: string }

/**
 * A machine's final value. A value set here wins over the evaluation (it applies at priority 90),
 * so it is shown at once; a value reset here but still in the last evaluation is pending.
 */
export function effective(hv: HostValue | undefined, managed: unknown, managedFile: string | null, managedLoaded: boolean): Effective | null {
  if (managed !== undefined) return { state: 'value', value: managed, source: { kind: 'here' }, pending: false }
  if (!hv) return null
  if (hv.error) return { state: 'error', error: hv.error }
  if (!hv.exists) return { state: 'missing' }
  const source = sourceOf(hv.files, managedFile)
  return { state: 'value', value: hv.value, source, pending: source.kind === 'here' && managedLoaded }
}

const marker = (v: unknown) => typeof v === 'string' && v.startsWith('«') && v.endsWith('»')

/** A value as short Nix: "Europe/Madrid" → "Europe/Madrid" in quotes, [22 80] → [ 22 80 ]. */
export function showValue(v: unknown): string {
  if (v === undefined) return '—'
  if (v === null) return 'null'
  if (typeof v === 'string') return marker(v) ? v : JSON.stringify(v)
  if (Array.isArray(v)) return v.length ? `[ ${v.map(showValue).join(' ')} ]` : '[ ]'
  if (typeof v === 'object') {
    const e = Object.entries(v as Record<string, unknown>)
    return e.length ? `{ ${e.map(([k, x]) => `${pathName([k])} = ${showValue(x)};`).join(' ')} }` : '{ }'
  }
  return String(v)
}

/** Can this value be written to managed.json as it is? Not null, not a package or function marker. */
export function copyable(v: unknown): boolean {
  if (v === null || v === undefined || marker(v)) return false
  if (Array.isArray(v)) return v.every(copyable)
  if (typeof v === 'object') return Object.values(v as Record<string, unknown>).every(copyable)
  return true
}

export const sameValue = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)

const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)

/** Every value in a managed.json `settings` tree, as option names: services.openssh.enable = true. */
export function managedOptions(settings: Record<string, unknown> | undefined): { name: string; path: string[]; value: unknown }[] {
  const out: { name: string; path: string[]; value: unknown }[] = []
  const walk = (node: Record<string, unknown>, path: string[]) => {
    for (const [k, v] of Object.entries(node)) {
      if (isObject(v) && Object.keys(v).length) walk(v, [...path, k])
      else out.push({ name: pathName([...path, k]), path: [...path, k], value: v })
    }
  }
  if (settings) walk(settings, [])
  return out
}

export const featureGroups = ['services', 'programs', 'virtualisation', 'hardware'] as const

/** `….enable` switches set in managed.json, as features: services.desktopManager.plasma6.enable → desktopManager.plasma6. */
export function managedFeatures(settings: Record<string, unknown> | undefined) {
  return managedOptions(settings)
    .filter((o) => (featureGroups as readonly string[]).includes(o.path[0]) && o.path.length > 2 && o.path.at(-1) === 'enable' && typeof o.value === 'boolean')
    .map((o) => ({ group: o.path[0] as (typeof featureGroups)[number], name: o.path.slice(1, -1).join('.'), on: o.value as boolean }))
}

/**
 * Where "turn on editing" adds its line in flake.nix: after the line that imports hosts/<name>,
 * with some context around it. Null when no such line is found.
 */
export function editingPreview(flake: string, host: string): { before: string[]; line: string; after: string[]; at: number } | null {
  const lines = flake.split('\n')
  const ref = new RegExp(`\\./hosts/${host.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?=[/\\s\\]";)]|$)`)
  const i = lines.findIndex((l) => ref.test(l) && !l.includes('managed.nix') && !l.trim().startsWith('#'))
  if (i < 0) return null
  const indent = /^\s*/.exec(lines[i])![0]
  return { before: lines.slice(Math.max(0, i - 3), i + 1), line: `${indent}(import ./modules/managed.nix ./hosts/${host}/managed.json)`, after: lines.slice(i + 1, i + 3), at: i + 2 }
}
