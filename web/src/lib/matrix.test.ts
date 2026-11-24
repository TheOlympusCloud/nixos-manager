import { describe, expect, it } from 'vitest'
import { featureMatrix, packageMatrix } from './matrix'
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

  it('marks packages set for every machine', () => {
    const shared = { ...a, packages: [{ name: 'jq', version: '1.8', files: ['modules/managed.json'] }] }
    expect(packageMatrix([shared])[0].cells[0]).toMatchObject({ shared: true, managed: false })
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
