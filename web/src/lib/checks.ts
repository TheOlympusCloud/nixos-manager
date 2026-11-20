// What a check verified, in words for the Checks and Changes panels and the job log.
import type { CheckPass, CheckSummary, Job } from './api'

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/** The machines a passed check built, as the end of a sentence. */
export function builtText(p: CheckPass): string {
  if (p.base === '') return `built every machine (${p.built.length}), as nothing is pushed to compare with`
  if (p.built.length === 0) return 'no machine changes, so nothing needed building'
  if (p.built.length === 1) return `built ${p.built[0]}, the one machine it changes`
  return `built the ${p.built.length} machines it changes: ${p.built.join(', ')}`
}

/** One line per fact of a check's summary; commit: it checked a commit, compared with the last checked one before it. */
export function summaryLines(s: CheckSummary, commit = false): string[] {
  const lines: string[] = []
  const all = commit ? 'every machine: no commit before it passed a check' : 'every machine: nothing is pushed yet'
  if (s.built.length) lines.push(`Built: ${s.built.join(', ')}${s.base === '' ? ` (${all})` : ''}`)
  if (s.failed?.length) lines.push(`Failed: ${s.failed.join(', ')}`)
  if (!s.changed.length && !s.failed?.length) lines.push(`No machine changes compared with ${commit ? 'the last checked commit before it' : 'what is pushed'}`)
  if (s.reboot?.length) lines.push(`Reboot needed: ${s.reboot.join(', ')}`)
  if (s.toBuild || s.toFetch) {
    const parts = []
    if (s.toBuild) parts.push(`build ${count(s.toBuild, 'derivation', 'derivations')}`)
    if (s.toFetch) parts.push(`download ${count(s.toFetch, 'path', 'paths')} (${s.download}, ${s.unpacked} unpacked)`)
    lines.push(`Nix had to ${parts.join(' and ')}`)
  } else if (s.built.length) {
    lines.push('Everything was already in the server’s store')
  }
  return lines
}

const kinds: Record<string, string> = { check: 'Checks', lock: 'Lock inputs', update: 'Update inputs' }

/** A job's name in lists; a check of a commit names the commit, and the branch it was pushed to. */
export function jobTitle(j: Job): string {
  if (j.commit) return `Commit ${j.commit.slice(0, 7)}${j.branch ? ` on ${j.branch}` : ''}`
  return `${kinds[j.kind] ?? j.kind} ${j.id}`
}

/** The newest check of a commit; jobs come newest first. */
export const checkOf = (jobs: Job[], sha: string) => (sha ? jobs.find((j) => j.kind === 'check' && j.commit === sha) : undefined)
