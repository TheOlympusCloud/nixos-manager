import { describe, expect, it } from 'vitest'
import type { Commit, DeployOverview, MachineDeployment } from './api'
import { excerpt, installCommand, localOnly, pollInterval, progress, rollingOut, rollout, stateDetail, stateLabel, urlLine } from './deployments'

const commit = (sha: string, subject = sha): Commit => ({ sha, short: sha.slice(0, 7), author: 'Pablo', email: 'p@x', date: '2026-11-19T10:00:00Z', subject })
const m = (host: string, extra: Partial<MachineDeployment> = {}): MachineDeployment => ({ host, state: 'current', branch: 'main', behind: 0, reports: [], ...extra })
const now = Date.parse('2026-11-19T12:00:00Z')
const ov: DeployOverview = {
  reporting: true,
  branch: 'main',
  commits: [commit('c3'), commit('c2'), commit('c1')],
  machines: [
    m('web', { commit: 'c3' }),
    m('nas', { state: 'behind', commit: 'c2', behind: 1, behindSince: '2026-11-19T11:50:00Z' }),
    m('pi', { branch: 'testing-pi', commit: 'tp' }),
    m('old', { state: 'silent', commit: 'c0', behind: 3 }),
    m('laptop', { state: 'failed', failure: { at: '', commit: 'c3', phase: 'build', status: 'failed', error: 'boom' } }),
    m('new', { state: 'never' }),
  ],
}

describe('deployments', () => {
  it('places each machine at the commit it runs', () => {
    const r = rollout(ov)
    expect(r.rows.map((row) => [row.commit.sha, row.machines.map((x) => x.host)])).toEqual([
      ['c3', ['web']],
      ['c2', ['nas']],
      ['c1', []],
    ])
    expect(r.testing.map((x) => x.host)).toEqual(['pi'])
    expect(r.older.map((x) => x.host)).toEqual(['old', 'laptop'])
    expect(r.waiting.map((x) => x.host)).toEqual(['new'])
  })

  it('counts the reporting machines on the branch that run its last commit', () => {
    expect(progress(ov)).toEqual({ tip: ov.commits[0], done: 1, total: 4 })
    expect(progress({ ...ov, commits: [] })).toEqual({ tip: undefined, done: 0, total: 4 })
  })

  it('polls quickly only while a recent commit is rolling out', () => {
    expect(rollingOut(ov, now)).toBe(true)
    expect(pollInterval(ov, now)).toBe(5_000)
    const later = now + 2 * 3600_000
    expect(rollingOut(ov, later)).toBe(false)
    expect(pollInterval(ov, later)).toBe(15_000)
    expect(pollInterval({ ...ov, machines: [m('web', { commit: 'c3' })] }, now)).toBe(60_000)
    expect(pollInterval(null, now)).toBe(60_000)
  })

  it('words each state', () => {
    expect(ov.machines.map(stateLabel)).toEqual(['Up to date', '1 behind', 'Up to date', 'Silent', 'Build failed', 'Not reporting'])
    expect(stateDetail(ov.machines[1])).toBe('1 commit behind main; comin should pick it up within minutes.')
    expect(stateDetail(ov.machines[2])).toBe('Runs the last commit of testing-pi.')
    expect(stateDetail(ov.machines[4])).toBe('comin could not build c3.')
  })

  it('cuts long errors', () => {
    expect(excerpt('a\nb\nc\nd\ne')).toBe('a\nb\nc\nd')
    expect(excerpt('x'.repeat(500))).toHaveLength(361)
    expect(excerpt(undefined)).toBe('')
  })

  it('writes the setup lines', () => {
    expect(urlLine('http://manager.lan:8080')).toBe('fleet.deployReport.url = "http://manager.lan:8080";')
    expect(installCommand('nxd_ABC', 'pi')).toBe("echo 'nxd_ABC' | ssh root@pi 'install -D -m 600 /dev/stdin /var/lib/nixos-manager-report/token'")
    expect(localOnly('localhost')).toBe(true)
    expect(localOnly('nixmgr.home.lan')).toBe(false)
  })
})
