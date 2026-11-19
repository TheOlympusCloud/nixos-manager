import { describe, expect, it } from 'vitest'
import type { Insight } from './api'
import { bySeverity, fixPlan, insightTitle, missingPackage, nixLines, serious } from './insights'
import { host } from './testmodel'

const ssh: Insight = {
  id: 'ssh-password',
  severity: 'warning',
  title: 'SSH accepts passwords',
  detail: '',
  hosts: ['pi', 'ws', 'gone', 'broken'],
  fix: {
    label: 'Allow SSH keys only',
    settings: [
      { path: ['services', 'openssh', 'settings', 'PasswordAuthentication'], value: false },
      { path: ['nix', 'gc', 'dates'], value: 'weekly' },
    ],
  },
}
const hosts = [host('pi', { managedFile: null }), host('ws'), host('broken', { error: 'boom' })]

describe('insights', () => {
  it('plans a fix only on machines that are Editable here', () => {
    const p = fixPlan(ssh, hosts)
    expect(p.editable).toEqual(['ws'])
    expect(p.codeOnly).toEqual(['pi'])
    expect(p.steps.map((s) => [s.host, s.path.at(-1), s.value])).toEqual([
      ['ws', 'PasswordAuthentication', false],
      ['ws', 'dates', 'weekly'],
    ])
    expect(fixPlan({ ...ssh, fix: { label: 'x', updateInput: 'nixpkgs' } }, hosts).steps).toEqual([])
  })

  it('writes the fix as Nix for Code only machines', () => {
    expect(nixLines(ssh)).toBe('services.openssh.settings.PasswordAuthentication = false;\nnix.gc.dates = "weekly";')
  })

  it('groups by severity, most serious first', () => {
    const info: Insight = { ...ssh, id: 'missing-package:htop', severity: 'info', hosts: ['pi'] }
    const err: Insight = { ...ssh, id: 'eval-error', severity: 'error' }
    expect(bySeverity([info, ssh, err]).map((g) => [g.severity, g.items.length])).toEqual([
      ['error', 1],
      ['warning', 1],
      ['info', 1],
    ])
    expect(serious([info, ssh, err]).map((i) => i.id)).toEqual(['ssh-password', 'eval-error'])
    expect(serious(null)).toEqual([])
    expect(missingPackage(info)).toBe('htop')
    expect(missingPackage(ssh)).toBe('')
  })

  it('uses the console words for Code only machines', () => {
    expect(insightTitle({ ...ssh, id: 'not-managed', hosts: ['pi'] })).toBe('pi is Code only')
    expect(insightTitle({ ...ssh, id: 'not-managed', hosts: ['pi', 'x'] })).toBe('2 machines are Code only')
  })
})
