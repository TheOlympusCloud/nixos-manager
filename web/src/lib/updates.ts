// Inputs → Scheduled update: how the schedule and the latest update read.
import type { Update, UpdateSchedule } from './api'
import { plural } from './format'

/** The schedule's days, Monday first; -1 is every day. */
export const scheduleDays: { value: number; label: string }[] = [
  { value: -1, label: 'Every day' },
  ...['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((label, i) => ({ value: (i + 1) % 7, label: 'Every ' + label })),
]

export const hourText = (h: number) => String(h).padStart(2, '0') + ':00'

/** "Every Monday at 04:00 (CET)", or "Off". */
export function scheduleText(s: UpdateSchedule, zone: string): string {
  if (!s.enabled) return 'Off'
  const day = scheduleDays.find((d) => d.value === s.day)?.label ?? 'Every day'
  return `${day} at ${hourText(s.hour)}${zone ? ` (${zone})` : ''}`
}

export type UpdateLine = { tone: 'ok' | 'busy' | 'bad' | 'muted'; title: string; detail: string }

/** What the latest update's state says, given whether its branch moved on since. */
export function updateLine(u: Update, outdated = false): UpdateLine {
  const branch = u.branch || 'the fleet branch'
  const failed = u.summary?.failed ?? []
  switch (u.state) {
    case 'updating':
      return { tone: 'busy', title: 'Updating the inputs…', detail: `nix flake update, on top of ${branch} as it is on the remote.` }
    case 'checking':
      return { tone: 'busy', title: 'Checking the update…', detail: 'nix flake check, then a build of every machine it changes.' }
    case 'ready': {
      if (outdated) return { tone: 'bad', title: `${branch} moved on since this update`, detail: 'Update again to make one on top of it.' }
      const built = u.summary?.built ?? []
      return {
        tone: 'ok',
        title: `Ready to merge into ${branch}`,
        detail: built.length ? `Passed its check, which built ${plural(built.length, 'machine')}: ${built.join(', ')}.` : 'Passed its check; no machine had to be built.',
      }
    }
    case 'failed':
      return {
        tone: 'bad',
        title: 'The update failed its check',
        detail: failed.length ? `${failed.join(', ')} ${failed.length === 1 ? 'does' : 'do'} not build with it. The log says why.` : 'The log says why.',
      }
    case 'current':
      return { tone: 'ok', title: 'Every input is up to date', detail: 'Nothing moved, so there is nothing to merge.' }
    case 'error':
      return { tone: 'bad', title: 'The update could not run', detail: u.error ?? '' }
    case 'merged':
      return { tone: 'ok', title: `Merged into ${branch}`, detail: 'comin deploys it to the machines it changes.' }
    case 'discarded':
      return { tone: 'muted', title: 'Discarded', detail: 'Its branch stays until the next update replaces it.' }
  }
}

/** The update is being made or checked. */
export const updateBusy = (u?: Update) => u?.state === 'updating' || u?.state === 'checking'
