// Deployments as comin reports them: wording, the rollout picture and the setup commands.
import type { Commit, DeployOverview, DeployState, MachineDeployment } from './api'

export const short = (sha?: string) => (sha ?? '').slice(0, 7)

export const stateTone: Record<DeployState, 'ok' | 'warn' | 'err' | 'off'> = {
  current: 'ok',
  behind: 'warn',
  failed: 'err',
  silent: 'off',
  unknown: 'off',
  never: 'off',
}

export function stateLabel(m: MachineDeployment): string {
  switch (m.state) {
    case 'current':
      return 'Up to date'
    case 'behind':
      return `${m.behind} behind`
    case 'failed':
      return m.failure?.phase === 'build' ? 'Build failed' : 'Deploy failed'
    case 'silent':
      return 'Silent'
    case 'unknown':
      return 'Unknown commit'
    case 'never':
      return 'Not reporting'
  }
}

/** One sentence on what the state means, for titles and the machine page. */
export function stateDetail(m: MachineDeployment): string {
  const c = (n: number) => `${n} commit${n === 1 ? '' : 's'}`
  switch (m.state) {
    case 'current':
      return `Runs the last commit of ${m.branch}.`
    case 'behind':
      return `${c(m.behind)} behind ${m.branch}; comin should pick ${m.behind === 1 ? 'it' : 'them'} up within minutes.`
    case 'failed':
      return m.failure?.phase === 'build' ? `comin could not build ${short(m.failure?.commit)}.` : `comin could not switch to ${short(m.failure?.commit)}.`
    case 'silent':
      return `${c(m.behind)} behind ${m.branch} and no report for over 7 days: probably off, or comin stopped.`
    case 'unknown':
      return `Runs ${short(m.commit)}, which is not on ${m.branch} in this working copy: pull, or it ran a branch that is gone.`
    case 'never':
      return 'Has not reported a deployment: the report token or the address is missing there.'
  }
}

export type Rollout = {
  /** The newest commits of the branch, newest first, with the machines that run each. */
  rows: { commit: Commit; machines: MachineDeployment[] }[]
  /** Machines that follow another branch: their testing branch, or another main branch. */
  testing: MachineDeployment[]
  /** Machines on a commit older than the rows, or none that is known. */
  older: MachineDeployment[]
  /** Machines that never reported. */
  waiting: MachineDeployment[]
}

export function rollout(ov: DeployOverview): Rollout {
  const onBranch = ov.machines.filter((m) => m.branch === ov.branch && m.state !== 'never')
  const rows = ov.commits.map((commit) => ({ commit, machines: onBranch.filter((m) => m.commit === commit.sha) }))
  const placed = new Set(rows.flatMap((r) => r.machines))
  return {
    rows,
    testing: ov.machines.filter((m) => m.branch !== ov.branch && m.state !== 'never'),
    older: onBranch.filter((m) => !placed.has(m)),
    waiting: ov.machines.filter((m) => m.state === 'never'),
  }
}

/** How many reporting machines on the branch run its last commit. */
export function progress(ov: DeployOverview): { tip?: Commit; done: number; total: number } {
  const tip = ov.commits[0]
  const on = ov.machines.filter((m) => m.branch === ov.branch && m.state !== 'never')
  return { tip, done: tip ? on.filter((m) => m.commit === tip.sha).length : 0, total: on.length }
}

/** Something is catching up: a machine lags a commit made less than an hour ago. */
export function rollingOut(ov: DeployOverview | null, now = Date.now()): boolean {
  return !!ov?.machines.some((m) => m.state === 'behind' && m.behindSince && now - Date.parse(m.behindSince) < 3600_000)
}

/** Poll every 5 s while a rollout runs, every 15 s while machines lag, else every minute. */
export function pollInterval(ov: DeployOverview | null, now = Date.now()): number {
  if (rollingOut(ov, now)) return 5_000
  return ov?.machines.some((m) => m.state === 'behind') ? 15_000 : 60_000
}

/** The question before forgetting a machine's reports. */
export function forgetQuestion(m: MachineDeployment): string {
  const n = m.reports.length
  return `Delete ${n === 1 ? 'the deployment report' : `the ${n} deployment reports`} of ${m.host}? If it still runs comin and reports again, it comes back.`
}

/** The first lines of an error, for cards and table cells. */
export function excerpt(error: string | undefined, lines = 4, chars = 360): string {
  const text = (error ?? '').trim().split('\n').slice(0, lines).join('\n')
  return text.length > chars ? text.slice(0, chars).trimEnd() + '…' : text
}

export const TOKEN_FILE = '/var/lib/nixos-manager-report/token'

/** The line that turns reporting on, for modules/common.nix. */
export const urlLine = (url: string) => `fleet.deployReport.url = "${url}";`

/** Puts the token on a machine over SSH. It travels on stdin, so it is in no command line on either side. */
export const installCommand = (token: string, host: string) => `echo '${token}' | ssh root@${host} 'install -D -m 600 /dev/stdin ${TOKEN_FILE}'`

/** Addresses only this computer can reach: the machines need the manager's LAN or Tailscale address. */
export const localOnly = (hostname: string) => ['localhost', '127.0.0.1', '[::1]', '::1'].includes(hostname)
