import type { Host } from './api'

/** "x86_64-linux" → "x86_64" */
export const arch = (system: string) => system.replace(/-linux$/, '')
/** First line of an evaluation error, without nix's "error:" prefix. */
export const firstLine = (error: string) => error.trim().split('\n')[0].replace(/^error:\s*/, '')
export const plural = (n: number, word: string, many = word + 's') => `${n} ${n === 1 ? word : many}`
export const initial = (name: string) => (name.match(/[a-z0-9]/i)?.[0] ?? '?').toUpperCase()
export const homeUsers = (h: Host) => h.users.filter((u) => u.home)

/** Seconds since the epoch → "3 d ago" style age. */
export function age(unix: number, now = Date.now()): string {
  const s = Math.max(0, Math.round(now / 1000 - unix))
  if (s < 3600) return `${Math.max(1, Math.floor(s / 60))} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`
  if (s < 86400 * 60) return `${Math.floor(s / 86400)} d ago`
  return `${Math.floor(s / (86400 * 30))} months ago`
}
