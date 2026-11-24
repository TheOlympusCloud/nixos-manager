// Customising packages: the rows of a machine's (or a home's) packages, and
// the text forms of a package override.
import type { Host, Managed, Package, PackageOverride } from './api'

/** One row of a packages table. */
export type PackageRow = {
  name: string
  version: string
  files: string[]
  /** In managed.json's list, by this attribute. */
  attr?: string
  /** Changed by managed.json or packages/<attr>.nix, by this attribute. */
  overridden?: string
  /** Added in managed.json, not in the evaluation yet. */
  pending?: boolean
  /** Taken out by name: removedPackages, in the evaluation or not yet. */
  removed?: boolean
  /** NixOS's own package for a working system. */
  core?: boolean
}

/** The folder of a machine's managed.json, where its own Nix files go: hosts/pc. */
export const machineDir = (host: Host) => (host.managedFile ? host.managedFile.replace(/\/[^/]*$/, '') : `hosts/${host.name}`)
export const packageFile = (host: Host, attr: string) => `${machineDir(host)}/packages/${attr}.nix`
export const hostNixFile = (host: Host) => `${machineDir(host)}/custom.nix`
export const homeNixFile = (host: Host, user: string) => `${machineDir(host)}/home/${user}/custom.nix`
export const dotfilesDir = (host: Host, user: string) => `${machineDir(host)}/home/${user}/files/`

/** The attribute of a package name in managed.json's lists or overrides: nodejs → nodejs_22. */
export function attrOf(host: Host, name: string, attrs: string[]): string | undefined {
  const pname = (a: string) => host.managedPackages?.[a]
  return attrs.find((a) => a === name) ?? attrs.find((a) => pname(a) === name) ?? attrs.find((a) => a.split('.').pop() === name)
}

/** Attributes changed on the machine: in managed.json, or by a packages/<attr>.nix file. */
export function overriddenAttrs(host: Host, managed: Managed | undefined, paths: string[]): string[] {
  const dir = `${machineDir(host)}/packages/`
  const files = paths.filter((p) => p.startsWith(dir) && p.endsWith('.nix') && !p.slice(dir.length).includes('/')).map((p) => p.slice(dir.length, -4))
  return [...new Set([...Object.keys(managed?.packageOverrides ?? {}), ...files])].sort()
}

/**
 * The machine's packages: installed ones (the evaluation's), ones added since
 * (pending), and ones taken out by name, whether the evaluation shows it yet.
 */
export function systemRows(host: Host, managed: Managed | undefined, paths: string[]): PackageRow[] {
  const attrs = managed?.packages ?? []
  const changed = overriddenAttrs(host, managed, paths)
  const removedNames = new Set(managed?.removedPackages ?? [])
  const core = new Set(host.corePackages ?? [])
  const known = new Set([...host.packages, ...(host.removedPackages ?? [])].map((p) => p.name))
  const row = (p: Package, extra: Partial<PackageRow> = {}): PackageRow => ({
    ...p,
    attr: host.managedFile && p.files.includes(host.managedFile) ? attrOf(host, p.name, attrs) : undefined,
    overridden: attrOf(host, p.name, changed),
    core: core.has(p.name),
    ...extra,
  })
  const pname = (a: string) => host.managedPackages?.[a]
  const pending = attrs
    .filter((a) => !known.has(a) && !known.has(a.split('.').pop()!) && !known.has(pname(a) ?? ''))
    .map((a) => ({ name: a, version: '', files: host.managedFile ? [host.managedFile] : [], attr: a, pending: true, overridden: attrOf(host, a, changed) }))
  const rows = [
    ...host.packages.map((p) => row(p, { removed: removedNames.has(p.name) })),
    ...(host.removedPackages ?? []).map((p) => row(p, { removed: true })),
    ...pending,
  ]
  // Removed by name, but the evaluation does not know the package: still listed, to give it back.
  for (const n of removedNames) if (!known.has(n)) rows.push({ name: n, version: '', files: [], removed: true })
  return rows.sort((a, b) => a.name.localeCompare(b.name))
}

/** A home-manager user's packages, as the evaluation lists them and as managed.json changes them. */
export function homeRows(host: Host, user: string, managed: Managed | undefined): PackageRow[] {
  const u = host.users.find((x) => x.name === user)
  const mine = managed?.home?.[user]
  const attrs = mine?.packages ?? []
  const removedNames = new Set(mine?.removedPackages ?? [])
  const names = u?.home?.packages ?? []
  const rows: PackageRow[] = names.map((n) => ({ name: n, version: '', files: [], attr: attrOf(host, n, attrs), removed: removedNames.has(n) }))
  const known = new Set(names)
  const pname = (a: string) => host.managedPackages?.[a]
  for (const a of attrs) if (!known.has(a) && !known.has(pname(a) ?? '') && !known.has(a.split('.').pop()!)) rows.push({ name: a, version: '', files: [], attr: a, pending: true })
  for (const n of removedNames) if (!known.has(n)) rows.push({ name: n, version: '', files: [], removed: true })
  return rows.sort((a, b) => a.name.localeCompare(b.name))
}

/** The release a new home starts at (home.stateVersion): the machine's NixOS release, 26.05. */
export const homeStateVersion = (host: Host) => /^\d{2}\.\d{2}/.exec(host.release)?.[0] ?? host.stateVersion

// ---- override arguments as text: one `name = <JSON value>` per line ----

/** {withVimKeys: true} → "withVimKeys = true" */
export const formatArgs = (args: Record<string, unknown> | undefined) =>
  Object.entries(args ?? {})
    .map(([k, v]) => `${k} = ${JSON.stringify(v)}`)
    .join('\n')

/**
 * "withVimKeys = true" lines → {withVimKeys: true}. Values are JSON (true, 3,
 * "text", [..], {..}), as managed.json keeps them; a trailing ; is allowed.
 */
export function parseArgs(text: string): { args: Record<string, unknown> } | { problem: string } {
  const args: Record<string, unknown> = {}
  for (const [i, raw] of text.split('\n').entries()) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) continue
    const m = /^([A-Za-z_][A-Za-z0-9_'-]*)\s*=\s*(.+?)\s*;?$/.exec(line)
    if (!m) return { problem: `Line ${i + 1}: write name = value.` }
    try {
      args[m[1]] = JSON.parse(m[2])
    } catch {
      return { problem: `Line ${i + 1}: ${m[2]} is not a JSON value: use true, 3, "text" or [ … ].` }
    }
  }
  return { args }
}

/** What an override changes, in a few words. */
export function overrideSummary(o: PackageOverride | undefined, rawFile: boolean): string {
  const parts: string[] = []
  const n = (x: object | undefined) => Object.keys(x ?? {}).length
  if (n(o?.override)) parts.push(`${n(o?.override)} argument${n(o?.override) > 1 ? 's' : ''}`)
  if (n(o?.overrideAttrs)) parts.push(`${n(o?.overrideAttrs)} attribute${n(o?.overrideAttrs) > 1 ? 's' : ''}`)
  if (o?.version) parts.push(`version ${o.version}`)
  else if (o?.src) parts.push('another source')
  if (o?.patches?.length) parts.push(`${o.patches.length} patch${o.patches.length > 1 ? 'es' : ''}`)
  if (rawFile) parts.push('Nix file')
  return parts.join(' · ')
}

/** Drops empty parts, so an override with nothing left puts the package back. */
export function cleanOverride(o: PackageOverride): PackageOverride | null {
  const out: PackageOverride = {}
  if (Object.keys(o.override ?? {}).length) out.override = o.override
  if (Object.keys(o.overrideAttrs ?? {}).length) out.overrideAttrs = o.overrideAttrs
  if (o.version?.trim()) out.version = o.version.trim()
  if (o.src && (o.src.url?.trim() || o.src.github?.trim())) {
    const s = o.src
    out.src = s.url?.trim() ? { url: s.url.trim() } : { github: s.github!.trim(), rev: s.rev?.trim() ?? '' }
    if (s.hash?.trim()) out.src.hash = s.hash.trim()
  }
  const patches = (o.patches ?? []).filter((p) => p.url?.trim() || p.file?.trim()).map((p) => (p.file?.trim() ? { file: p.file.trim() } : { url: p.url!.trim(), ...(p.hash?.trim() ? { hash: p.hash.trim() } : {}) }))
  if (patches.length) out.patches = patches
  return Object.keys(out).length ? out : null
}

// ---- starters for the files next to managed.json ----

export const packageFileTemplate = (attr: string, host: string) => `# How ${attr} is built on ${host}, applied after what is set by clicking.
# pkg is ${attr} as nixpkgs has it, with those changes; return the package to use.
# pkgs has every change on this machine (do not read pkgs.${attr} here: use pkg); prev has none.
{ pkgs, prev, lib, ... }:
pkg:
pkg.overrideAttrs (old: {
  # patches = (old.patches or [ ]) ++ [ ./../patches/fix.patch ];
  # configureFlags = (old.configureFlags or [ ]) ++ [ "--enable-something" ];
})
`

export const hostNixTemplate = (host: string) => `# ${host}'s own NixOS settings, next to what is set by clicking in managed.json.
# Anything NixOS can do goes here; modules/managed.nix imports this file.
{ config, lib, pkgs, ... }:
{
}
`

export const homeNixTemplate = (user: string, host: string) => `# ${user}'s home-manager settings on ${host}, next to what is set by clicking.
# Anything home-manager can do goes here; modules/managed.nix imports this file.
{ config, lib, pkgs, ... }:
{
}
`
