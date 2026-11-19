// Fleet insights: grouping, wording and who a fix can reach.
import type { Host, Insight } from './api'

export const severities = ['error', 'warning', 'info'] as const
export const severityLabel: Record<Insight['severity'], string> = { error: 'Needs attention', warning: 'Warnings', info: 'Suggestions' }

/** Insights by severity, most serious first; empty groups are left out. */
export function bySeverity(insights: Insight[]): { severity: Insight['severity']; items: Insight[] }[] {
  return severities.map((severity) => ({ severity, items: insights.filter((i) => i.severity === severity) })).filter((g) => g.items.length)
}

/** The console's words for findings whose server title uses older ones. */
export function insightTitle(i: Insight): string {
  if (i.id === 'not-managed') return i.hosts.length === 1 ? `${i.hosts[0]} is Code only` : `${i.hosts.length} machines are Code only`
  return i.title
}
export function insightDetail(i: Insight): string {
  if (i.id === 'not-managed') return 'They do not load their managed.json, so nothing on them can be changed by clicking here. Open a machine to turn on editing.'
  return i.detail
}

/** A package that every machine but one has: the package name, from the insight id. */
export const missingPackage = (i: Insight) => (i.id.startsWith('missing-package:') ? i.id.slice('missing-package:'.length) : '')

export type FixPlan = {
  /** Machines the settings can be written to (Editable here). */
  editable: string[]
  /** Affected machines that only change in code. */
  codeOnly: string[]
  /** Editable machines left out: the fix allows SSH keys only, and no user there has one. */
  noKey: string[]
  /** One step per machine and setting, in order. */
  steps: { host: string; path: string[]; value: unknown }[]
}

/** Who a settings fix can reach. Machines that are not in the model (or do not evaluate) are left out. */
export function fixPlan(i: Insight, hosts: Host[]): FixPlan {
  const known = i.hosts.map((n) => hosts.find((h) => h.name === n)).filter((h): h is Host => !!h && !h.error)
  const codeOnly = known.filter((h) => !h.managedFile).map((h) => h.name)
  const writable = known.filter((h) => h.managedFile)
  const noKey = restrictsSSH(i) ? writable.filter((h) => !((h.checks?.sshUserKeys ?? 0) > 0)).map((h) => h.name) : []
  const editable = writable.map((h) => h.name).filter((n) => !noKey.includes(n))
  const steps = editable.flatMap((host) => (i.fix?.settings ?? []).map((s) => ({ host, path: s.path, value: s.value })))
  return { editable, codeOnly, noKey, steps }
}

const SSH_LOGINS = ['services.openssh.settings.PasswordAuthentication', 'services.openssh.settings.PermitRootLogin']
/** The fix turns SSH password or root logins off, which locks out a machine where no user has a key. */
const restrictsSSH = (i: Insight) => (i.fix?.settings ?? []).some((s) => SSH_LOGINS.includes(s.path.join('.')) && s.value !== true && s.value !== 'yes')

/** `services.openssh.settings.PasswordAuthentication = false;` for doing it by hand. */
export function nixLines(i: Insight): string {
  return (i.fix?.settings ?? []).map((s) => `${s.path.join('.')} = ${JSON.stringify(s.value)};`).join('\n')
}

/** Error and warning findings, for the badges. */
export const serious = (insights: Insight[] | null) => (insights ?? []).filter((i) => i.severity !== 'info')
