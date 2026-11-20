import { describe, expect, it } from 'vitest'
import type { Commit, DeployOverview, MachineDeployment } from './api'
import { driftDetail, excerpt, forgetQuestion, installCommand, onSharedToken, localOnly, pollInterval, progress, rollbackOf, rollingOut, rollout, silenceDetail, sortRank, stateDetail, stateLabel, tileOf, trialOf, tryTargets, urlLine } from './deployments'

const commit = (sha: string, subject = sha): Commit => ({ sha, short: sha.slice(0, 7), author: 'Pablo', email: 'p@x', date: '2026-11-19T10:00:00Z', subject })
const m = (host: string, extra: Partial<MachineDeployment> = {}): MachineDeployment => ({ host, state: 'current', branch: 'main', main: 'main', testing: `testing-${host}`, behind: 0, reports: [], silent: false, silentAfter: 2, heartbeats: true, ...extra })
const now = Date.parse('2026-11-19T12:00:00Z')
const ov: DeployOverview = {
  reporting: true,
  sharedToken: false,
  branch: 'main',
  commits: [commit('c3'), commit('c2'), commit('c1')],
  machines: [
    m('web', { commit: 'c3' }),
    m('nas', { state: 'behind', commit: 'c2', behind: 1, behindSince: '2026-11-19T11:50:00Z' }),
    m('pi', { branch: 'testing-pi', commit: 'tp' }),
    m('old', { state: 'behind', silent: true, silentAfter: 168, heartbeats: false, commit: 'c0', behind: 3, lastSeen: '2026-11-09T12:00:00Z' }),
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
    expect(ov.machines.map(stateLabel)).toEqual(['Up to date', '1 behind', 'Up to date', '3 behind', 'Build failed', 'Not reporting'])
    expect(stateLabel(m('box', { state: 'never', lastSeen: '2026-11-19T11:59:00Z' }))).toBe('No deployment yet')
    expect(stateDetail(ov.machines[1])).toBe('1 commit behind main; comin should pick it up within minutes.')
    expect(stateDetail(ov.machines[2])).toBe('Runs the last commit of testing-pi.')
    expect(stateDetail(ov.machines[4])).toBe('comin could not build c3.')
  })

  it('says how long a machine has been silent', () => {
    expect(silenceDetail(ov.machines[3], now)).toBe('No report for 10 d, more than the 168 hours it may stay silent: it may be off, comin or its report timer may have stopped, or it cannot reach this console.')
    expect(silenceDetail(m('pi', { silent: true, lastSeen: '2026-11-19T09:00:00Z' }), now)).toMatch(/^No report for 3 h, more than the 2 hours it may stay silent/)
  })

  it('sorts and counts silent machines apart', () => {
    const quiet = m('quiet', { silent: true }) // up to date, but gone quiet
    expect([...ov.machines, quiet].map(tileOf)).toEqual(['current', 'behind', 'current', 'silent', 'failed', 'silent', 'silent'])
    expect([...ov.machines, quiet].sort((a, b) => sortRank(a) - sortRank(b) || a.host.localeCompare(b.host)).map((x) => x.host)).toEqual([
      'laptop',
      'old',
      'quiet',
      'nas',
      'new',
      'pi',
      'web',
    ])
  })

  it('asks before forgetting a machine', () => {
    const r = { at: '', commit: 'c1', phase: 'deploy' as const, status: 'done' }
    expect(forgetQuestion(m('oldpi', { reports: [r] }))).toBe('Delete the deployment report of oldpi? If it still runs comin and reports again, it comes back.')
    expect(forgetQuestion(m('oldpi', { reports: [r, r] }))).toMatch(/^Delete the 2 deployment reports of oldpi\?/)
  })

  it('cuts long errors', () => {
    expect(excerpt('a\nb\nc\nd\ne')).toBe('a\nb\nc\nd')
    expect(excerpt('x'.repeat(500))).toHaveLength(361)
    expect(excerpt(undefined)).toBe('')
  })

  it('says how what a machine runs differs from its commit', () => {
    expect(driftDetail(m('pi', { drift: 'manual', commit: 'c3abcdef' }))).toBe(
      'Runs a system that no deployment made: not what c3abcde builds, and not what it boots into from comin. Someone ran nixos-rebuild there, or rolled back; the next commit that changes it replaces it.',
    )
    expect(driftDetail(m('pi', { drift: 'unfinished', commit: 'c3abcdef' }))).toMatch(/^Did not finish switching/)
    expect(driftDetail(m('pi'))).toBe('')
  })

  it('lists the machines still on the shared token', () => {
    const shared = { ...ov, sharedToken: true, machines: [m('pi', { sharedToken: true }), m('nas'), m('box', { sharedToken: true })] }
    expect(onSharedToken(shared)).toEqual(['box', 'pi'])
    expect(onSharedToken(ov)).toEqual([])
  })

  it('writes the setup lines', () => {
    expect(urlLine('http://manager.lan:8080')).toBe('fleet.deployReport.url = "http://manager.lan:8080";')
    expect(installCommand('nxh_ABC', 'pi')).toBe("echo 'nxh_ABC' | ssh root@pi 'install -D -m 600 /dev/stdin /var/lib/nixos-manager-report/token'")
    expect(localOnly('localhost')).toBe(true)
    expect(localOnly('nixmgr.home.lan')).toBe(false)
  })

  it('finds the machine a branch tries changes on', () => {
    expect(trialOf(ov, 'testing-pi')?.host).toBe('pi')
    expect(trialOf(ov, 'main')).toBeUndefined()
    expect(trialOf(null, 'testing-pi')).toBeUndefined()
  })

  it('offers a trial on the machines the check built, from the branch their comin follows', () => {
    const fleet = { ...ov, machines: [m('web'), m('nas', { main: 'stable' }), m('pi', { testing: undefined }), m('db')] }
    expect(tryTargets(fleet, 'main', ['web', 'nas', 'pi']).map((x) => x.host)).toEqual(['web'])
    expect(tryTargets(fleet, 'stable', ['web', 'nas']).map((x) => x.host)).toEqual(['nas'])
    expect(tryTargets(fleet, 'main', [])).toEqual([])
    expect(tryTargets(null, 'main', ['web'])).toEqual([])
  })

  it('rolls a failed machine back with the commits since its last deployment', () => {
    const failure = { at: '', commit: 'c3c3c3c3', phase: 'build' as const, status: 'failed' as const }
    const one = m('laptop', { state: 'failed', commit: 'c2c2c2c2', failure, culprits: [commit('c3c3c3c3', 'enable foo')] })
    expect(rollbackOf(one)).toEqual({
      label: 'Revert the commit that broke laptop',
      question: 'laptop failed to build c3c3c3c “enable foo”, the only commit since c2c2c2c, which it runs. This stages the opposite of that commit, runs the checks, and then you commit and push it: every machine it changed gets the revert.',
      list: '',
      message: 'Revert "enable foo"\n\nThis reverts commit c3c3c3c3.',
      rev: 'c3c3c3c3',
      since: 'c2c2c2c2',
    })
    const three = m('laptop', { ...one, failure: { ...failure, phase: 'deploy' }, culprits: [commit('c5c5c5c5', 'e'), commit('c4c4c4c4', 'd'), commit('c3c3c3c3', 'c')] })
    const r = rollbackOf(three)!
    expect(r.label).toBe('Revert the 3 commits since laptop last deployed')
    expect(r.question).toBe('laptop failed to switch to c5c5c5c after 3 commits since c2c2c2c, which it runs. Any of them may have broken it, so this stages the opposite of all 3 as one change, runs the checks, and then you commit and push it. To keep some, revert only the one at fault from History instead.')
    expect(r.list).toBe('c5c5c5c e\nc4c4c4c d\nc3c3c3c c')
    expect(r.message).toBe('Revert the 3 commits that broke laptop\n\nThis reverts commits c5c5c5c5, c4c4c4c4, c3c3c3c3.')
    expect([r.rev, r.since]).toEqual(['c5c5c5c5', 'c2c2c2c2'])
    expect(rollbackOf(m('web', { commit: 'c3' }))).toBeNull()
    expect(rollbackOf(m('laptop', { state: 'failed', failure, culprits: [] }))).toBeNull()
  })
})
