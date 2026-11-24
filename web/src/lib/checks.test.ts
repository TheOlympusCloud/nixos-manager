import { describe, expect, it } from 'vitest'
import type { CheckSummary, Job } from './api'
import { builtText, checkOf, jobTitle, summaryLines } from './checks'

describe('builtText', () => {
  it('says which machines the check built, compared with what', () => {
    expect(builtText({ base: 'abc', built: [] })).toBe('no machine changes, so nothing needed building')
    expect(builtText({ base: 'abc', built: ['pi'] })).toBe('built pi, the one machine it changes')
    expect(builtText({ base: 'abc', built: ['pi', 'web'] })).toBe('built the 2 machines it changes: pi, web')
    expect(builtText({ base: '', built: ['pi', 'web'] })).toBe('built every machine (2), as no pushed commit had passed a check')
  })
})

describe('summaryLines', () => {
  const sum = (s: Partial<CheckSummary>): CheckSummary => ({ base: 'abc', changed: [], built: [], toBuild: 0, toFetch: 0, ...s })
  it('lists the builds, failures, reboots and what Nix did', () => {
    expect(summaryLines(sum({ changed: ['pi', 'web'], built: ['web'], failed: ['pi'], reboot: ['web'], toBuild: 14, toFetch: 230, download: '512.30 MiB', unpacked: '2101.12 MiB' }))).toEqual([
      'Built: web',
      'Failed: pi',
      'Reboot needed: web',
      'Nix had to build 14 derivations and download 230 paths (512.30 MiB, 2101.12 MiB unpacked)',
    ])
  })
  it('says when no machine changes, and when everything was already built', () => {
    expect(summaryLines(sum({}))).toEqual(['No machine changes compared with the newest pushed commit that passed a check'])
    expect(summaryLines(sum({ base: '', changed: ['web'], built: ['web'] }))).toEqual(['Built: web (every machine: no pushed commit had passed a check)', 'Everything was already in the server’s store'])
  })
  it('names a machine that did not evaluate, without claiming anything was built', () => {
    expect(summaryLines(sum({ failed: ['pi'] }))).toEqual(['Failed: pi'])
  })
})

describe('checks of a commit', () => {
  const sha = 'abcdef0123456789abcdef0123456789abcdef01'
  const job = (j: Partial<Job>): Job => ({ id: 'j1', kind: 'check', state: 'passed', ...j })
  it('names the commit and where it was pushed', () => {
    expect(jobTitle(job({ commit: sha, branch: 'main' }))).toBe('Commit abcdef0 on main')
    expect(jobTitle(job({ commit: sha }))).toBe('Commit abcdef0')
    expect(jobTitle(job({ id: 'j7' }))).toBe('Checks j7')
    expect(jobTitle(job({ id: 'j8', kind: 'update' }))).toBe('Update inputs j8')
    expect(jobTitle(job({ id: 'j9', kind: 'iso', host: 'pc' }))).toBe('Installer ISO of pc')
  })
  it('finds the newest check of a commit', () => {
    const jobs = [job({ id: 'j9', kind: 'update' }), job({ id: 'j8', commit: sha, state: 'running' }), job({ id: 'j3', commit: sha, state: 'failed' }), job({ id: 'j2' })]
    expect(checkOf(jobs, sha)?.id).toBe('j8')
    expect(checkOf(jobs, 'other')).toBeUndefined()
    expect(checkOf(jobs, '')).toBeUndefined()
  })
  it('compares a commit with the last checked one, not with what is pushed', () => {
    const s: CheckSummary = { base: '', changed: ['web'], built: ['web'], toBuild: 0, toFetch: 0 }
    expect(summaryLines(s, true)[0]).toBe('Built: web (every machine: no commit before it passed a check)')
    expect(summaryLines({ ...s, base: 'abc', changed: [], built: [] }, true)).toEqual(['No machine changes compared with the last checked commit before it'])
  })
})
