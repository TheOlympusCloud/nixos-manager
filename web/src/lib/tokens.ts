import type { TokenScope } from './api'

/** What a token may do; each scope includes the ones before it. */
export const scopes: { value: TokenScope; label: string; hint: string }[] = [
  { value: 'read', label: 'Read only', hint: 'Look at files, history, the fleet, checks and deployments. Changes nothing.' },
  { value: 'edit', label: 'Edit, no push', hint: 'Also edit files, run checks, commit, branch and pull. Cannot push, so nothing reaches the machines.' },
  { value: 'full', label: 'Full', hint: 'Also push, which deploys to the machines.' },
]

export const lifetimes = [
  { days: 0, label: 'Never expires' },
  { days: 7, label: '7 days' },
  { days: 30, label: '30 days' },
  { days: 90, label: '90 days' },
  { days: 365, label: '1 year' },
]

/** The expiry to send for a lifetime in days; undefined when it never expires. */
export const expiresAt = (days: number, now = Date.now()) => (days > 0 ? new Date(now + days * 86_400_000).toISOString() : undefined)

/** When a token expires: "never", "expired", or "in 3 d". */
export function expiry(iso: string | undefined, now = Date.now()): string {
  if (!iso) return 'never'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return iso
  const s = (t - now) / 1000
  if (s <= 0) return 'expired'
  if (s < 3600) return `in ${Math.ceil(s / 60)} min`
  if (s < 86400) return `in ${Math.ceil(s / 3600)} h`
  return `in ${Math.ceil(s / 86400)} d`
}
