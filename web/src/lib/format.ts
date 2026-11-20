import type { Host } from './api'

/** "x86_64-linux" → "x86_64" */
export const arch = (system: string) => system.replace(/-linux$/, '')
/** First line of an evaluation error, without nix's "error:" prefix. */
export const firstLine = (error: string) => error.trim().split('\n')[0].replace(/^error:\s*/, '')
export const plural = (n: number, word: string, many = word + 's') => `${n} ${n === 1 ? word : many}`
export const initial = (name: string) => (name.match(/[a-z0-9]/i)?.[0] ?? '?').toUpperCase()
export const homeUsers = (h: Host) => h.users.filter((u) => u.home)

/** 1536 → "1.5 KiB": binary units, as Nix shows sizes; two decimals from GiB on. */
export function bytes(n: number): string {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let i = 0
  while (Math.abs(n) >= 1024 && i < units.length - 1) ((n /= 1024), i++)
  return i === 0 ? `${n} B` : `${n.toFixed(i >= 3 ? 2 : 1)} ${units[i]}`
}
/** A size change: "+18.4 MiB", "−12.0 KiB". */
export const signedBytes = (n: number) => (n > 0 ? '+' : n < 0 ? '−' : '') + bytes(Math.abs(n))

/** Seconds since the epoch → "3 d ago". The one age format of the console (see util.ago for ISO dates). */
export function age(unix: number, now = Date.now()): string {
  const s = Math.round(now / 1000 - unix)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  if (s < 86400 * 60) return `${Math.floor(s / 86400)} d ago`
  if (s < 86400 * 730) return `${Math.floor(s / (86400 * 30))} months ago`
  return `${Math.floor(s / (86400 * 365))} years ago`
}
/** Whole days since the epoch time `unix`. */
export const days = (unix: number, now = Date.now()) => Math.max(0, Math.floor((now / 1000 - unix) / 86400))
