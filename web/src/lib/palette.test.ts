import { describe, expect, it } from 'vitest'
import { fuzzy, highlight, rank } from './palette'

describe('fuzzy', () => {
  it('matches subsequences and rejects the rest', () => {
    expect(fuzzy('hsrv', 'homeserver')?.hits).toEqual([0, 4, 6, 7])
    expect(fuzzy('xyz', 'homeserver')).toBeNull()
    expect(fuzzy('', 'anything')).toEqual({ score: 0, hits: [] })
  })

  it('ranks prefixes over word starts over scattered letters', () => {
    const s = (q: string, t: string) => fuzzy(q, t)!.score
    expect(s('fire', 'firefox')).toBeGreaterThan(s('fire', 'networking.firewall.enable'))
    expect(s('fire', 'networking.firewall.enable')).toBeGreaterThan(s('fire', 'xfirex'))
    expect(s('ssh', 'services.openssh.enable')).toBeGreaterThan(s('soe', 'services.openssh.enable'))
    expect(s('soe', 'services.openssh.enable')).toBeGreaterThan(0)
  })

  it('prefers word starts for scattered letters', () => {
    expect(fuzzy('nfe', 'networking.firewall.enable')?.hits).toEqual([0, 11, 20])
  })
})

describe('highlight', () => {
  it('splits text into matched and plain runs', () => {
    expect(highlight('firefox', [0, 1, 4])).toEqual([
      { text: 'fi', hit: true },
      { text: 're', hit: false },
      { text: 'f', hit: true },
      { text: 'ox', hit: false },
    ])
  })
})

describe('rank', () => {
  const items = [
    { id: 'm:pi', group: 'Machines', label: 'pi' },
    { id: 'm:ws', group: 'Machines', label: 'workstation' },
    { id: 'p:fx', group: 'Packages', label: 'firefox', keywords: 'web browser' },
    { id: 'p:ff', group: 'Packages', label: 'ffmpeg' },
    { id: 'v:fl', group: 'Views', label: 'Fleet' },
  ]
  it('orders by group, then score, and caps each group', () => {
    expect(rank(items, 'f', ['Views', 'Packages', 'Machines'], { Packages: 1 }).map((r) => r.id)).toEqual(['v:fl', 'p:ff'])
    expect(rank(items, 'ws', ['Machines']).map((r) => r.id)).toEqual(['m:ws'])
  })
  it('matches keywords below labels, at word starts only, without highlights', () => {
    const r = rank(items, 'browser', ['Packages'])
    expect(r.map((x) => x.id)).toEqual(['p:fx'])
    expect(r[0].match.hits).toEqual([])
    expect(rank(items, 'rowser', ['Packages'])).toEqual([])
    expect(rank(items, 'wbr', ['Packages'])).toEqual([])
  })
})
