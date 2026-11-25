// What each machine has installed: the closure it reports, browsed as a tree, and the fleet table.
import type { DepNode, DepRow, DepState, MachineDeps } from './api'

/** A store path's name without the store and the hash: glibc-2.40-66. */
export const label = (n: DepNode) => n.path.slice(n.path.lastIndexOf('/') + 1).replace(/^[0-9a-z]{32}-/, '')

const byName = (d: MachineDeps) => (a: number, b: number) => d.nodes[a].name.localeCompare(d.nodes[b].name) || label(d.nodes[a]).localeCompare(label(d.nodes[b]))

/** What entry i uses, by name. */
export const children = (d: MachineDeps, i: number) => [...new Set(d.nodes[i].refs)].sort(byName(d))

/** For every entry, what uses it, by name. */
export function usedBy(d: MachineDeps): number[][] {
  const out = d.nodes.map(() => [] as number[])
  d.nodes.forEach((n, i) => n.refs.forEach((r) => out[r].push(i)))
  return out.map((l) => l.sort(byName(d)))
}

/** A shortest chain from the system down to entry i, the system first. */
export function why(d: MachineDeps, i: number): number[] {
  const out: number[] = []
  for (let j = i; j >= 0 && out.length <= d.nodes.length; j = d.nodes[j].via) out.push(j)
  return out.reverse()
}

/** The entries whose name contains q, in any case: exact names first, then by name; none for an empty q. */
export function findNodes(d: MachineDeps, q: string, limit = 50): number[] {
  const want = q.trim().toLowerCase()
  if (!want) return []
  const exact = (i: number) => (d.nodes[i].name.toLowerCase() === want ? 0 : 1)
  return d.nodes
    .map((_, i) => i)
    .filter((i) => d.nodes[i].name.toLowerCase().includes(want))
    .sort((a, b) => exact(a) - exact(b) || byName(d)(a, b))
    .slice(0, limit)
}

/** The fleet table's rows whose name contains q, and with differs only those whose machines disagree on the versions. */
export function filterRows(rows: DepRow[], q: string, differs: boolean): DepRow[] {
  const want = q.trim().toLowerCase()
  return rows.filter((r) => (!differs || r.differs) && r.name.toLowerCase().includes(want))
}

/** Why a machine has no tree here, and what to do about it; '' when it has one. */
export function stateMessage(d: { state: DepState; message?: string }, host: string): string {
  switch (d.state) {
    case 'ready':
      return ''
    case 'none':
      return `${host} does not report to this console. Give it its report token (Deployments → Setup): what it has installed arrives with its first report.`
    case 'shared':
      return `${host} reports with the shared fleet token, which cannot send what it has installed. Give it its own token (Deployments → Setup).`
    case 'old':
      return `${host}'s report module is older and does not send what it has installed. Copy report-deployment.sh and report-deployments.nix from the starter fleet's modules into the repository's modules; the tree arrives with the first report after ${host} deploys them.`
    case 'waiting':
      return `This console asked ${host} for what it has installed: it arrives with ${host}'s next report, within 15 minutes.`
    case 'refused':
      return `This console refused what ${host} sent: ${d.message ?? 'it was not valid'}. It asks again once ${host} runs another system.`
    case 'error':
      return `${host} could not read what it has installed; it tries again with its next report.`
  }
}
