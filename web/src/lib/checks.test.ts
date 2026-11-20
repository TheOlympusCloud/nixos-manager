import { describe, expect, it } from 'vitest'
import type { CheckSummary } from './api'
import { builtText, summaryLines } from './checks'

describe('builtText', () => {
  it('says which machines the check built, compared with what', () => {
    expect(builtText({ base: 'abc', built: [] })).toBe('no machine changes, so nothing needed building')
    expect(builtText({ base: 'abc', built: ['pi'] })).toBe('built pi, the one machine it changes')
    expect(builtText({ base: 'abc', built: ['pi', 'web'] })).toBe('built the 2 machines it changes: pi, web')
    expect(builtText({ base: '', built: ['pi', 'web'] })).toBe('built every machine (2), as nothing is pushed to compare with')
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
    expect(summaryLines(sum({}))).toEqual(['No machine changes compared with what is pushed'])
    expect(summaryLines(sum({ base: '', changed: ['web'], built: ['web'] }))).toEqual(['Built: web (every machine: nothing is pushed yet)', 'Everything was already in the server’s store'])
  })
  it('names a machine that did not evaluate, without claiming anything was built', () => {
    expect(summaryLines(sum({ failed: ['pi'] }))).toEqual(['Failed: pi'])
  })
})
