// What a check verified, in words for the Checks and Changes panels and the job log.
import type { CheckPass, CheckSummary } from './api'

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/** The machines a passed check built, as the end of a sentence. */
export function builtText(p: CheckPass): string {
  if (p.base === '') return `built every machine (${p.built.length}), as nothing is pushed to compare with`
  if (p.built.length === 0) return 'no machine changes, so nothing needed building'
  if (p.built.length === 1) return `built ${p.built[0]}, the one machine it changes`
  return `built the ${p.built.length} machines it changes: ${p.built.join(', ')}`
}

/** One line per fact of a check's summary. */
export function summaryLines(s: CheckSummary): string[] {
  const lines: string[] = []
  if (s.built.length) lines.push(`Built: ${s.built.join(', ')}${s.base === '' ? ' (every machine: nothing is pushed yet)' : ''}`)
  if (s.failed?.length) lines.push(`Failed: ${s.failed.join(', ')}`)
  if (!s.changed.length && !s.failed?.length) lines.push('No machine changes compared with what is pushed')
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
