import { describe, expect, it } from 'vitest'
import type { HostDiff } from './api'
import { diffParts, diffSummary, featureMatrix, packageMatrix, splitDiffs } from './matrix'
import { testModel } from './testmodel'

const [a, b] = testModel.hosts

describe('packageMatrix', () => {
  it('builds one row per package with a cell per machine', () => {
    const rows = packageMatrix([a, b])
    expect(rows.map((r) => r.name)).toEqual(['git', 'htop', 'nano', 'vlc', 'vscode'])
    const git = rows[0]
    expect(git.count).toBe(2)
    expect(git.cells.map((c) => c?.version)).toEqual(['2.51.0', '2.51.0'])
    expect(rows.find((r) => r.name === 'htop')?.cells[1]).toBeNull()
  })

  it('marks NixOS defaults and packages from the managed file', () => {
    const rows = packageMatrix([a, b])
    expect(rows.filter((r) => r.nixos).map((r) => r.name)).toEqual(['nano'])
    expect(rows.find((r) => r.name === 'vscode')?.cells[0]?.managed).toBe(true)
    expect(rows.find((r) => r.name === 'git')?.cells[0]?.managed).toBe(false)
  })
})

describe('featureMatrix', () => {
  it('groups features in a fixed order and shares rows between machines', () => {
    const rows = featureMatrix([a, b])
    expect(rows.map((r) => r.key)).toEqual(['services.home-assistant', 'services.openssh', 'programs.steam'])
    expect(rows[1].cells.map((c) => !!c)).toEqual([true, true])
    expect(rows[1].count).toBe(2)
    expect(rows[2].cells[0]?.managed).toBe(true)
    expect(rows[0].cells[0]).toBeNull()
  })
})

describe('host diffs', () => {
  const changed: HostDiff = { host: 'a', status: 'changed', packagesAdded: ['firefox', 'vlc'], packagesRemoved: ['nano'], featuresAdded: ['services.jellyfin'], portsRemoved: ['tcp/80'] }

  it('lists only the parts that changed', () => {
    expect(diffParts(changed).map((p) => p.label)).toEqual(['Packages', 'Features', 'Ports'])
    expect(diffParts({ host: 'x', status: 'unchanged' })).toEqual([])
  })

  it('summarises a machine in one line', () => {
    expect(diffSummary(changed)).toBe('+2 −1 packages · +1 features · −1 ports')
    expect(diffSummary({ host: 'n', status: 'added', usersAdded: ['pablo'] })).toBe('New machine · +1 users')
    expect(diffSummary({ host: 'x', status: 'unchanged' })).toBe('No changes')
    expect(diffSummary({ host: 'x', status: 'error', error: 'boom' })).toBe('Does not evaluate')
  })

  it('puts changed machines first and sets unchanged ones apart', () => {
    const { changed: c, unchanged: u } = splitDiffs([
      { host: 'z', status: 'unchanged' },
      changed,
      { host: 'b', status: 'error' },
      { host: 'n', status: 'added' },
    ])
    expect(c.map((d) => d.host)).toEqual(['b', 'n', 'a'])
    expect(u.map((d) => d.host)).toEqual(['z'])
  })
})
