import { describe, expect, it } from 'vitest'
import { Buf } from '../lib/ascii'
import { LAYERS, SHEET, blueprint, bufs, planet, snowflake, story, trace } from './scenes'

const text = (b: Buf) => b.toString()

describe('text scenes', () => {
  it('traces the first part of a polyline, joining its corners', () => {
    const b = new Buf(12, 4)
    trace(b, [[0, 0], [9, 0], [9, 3]], 0.5)
    expect(text(b).split('\n')[0]).toBe('───────     ')
    trace(b, [[0, 0], [9, 0], [9, 3]], 1)
    expect(text(b).split('\n')[0].slice(9, 10)).toBe('╮')
  })

  it('draws the planet and the snowflake inside their buffers', () => {
    const L = bufs(['globe', 'hot', 'warm'], 80, 30)
    planet(L, 40, 15, 10, 0)
    expect(text(L.globe) + text(L.hot)).toMatch(/[@#%*+=]/)
    snowflake({ hot: L.hot, globe: L.globe }, 40, 15, 8, 0)
    expect(text(L.hot)).toMatch(/█/)
    // Scattered packages leave the sphere for the warm layer.
    const S = bufs(['globe', 'hot', 'warm'], 80, 30)
    planet(S, 40, 15, 10, 0, 0.9)
    expect(text(S.warm)).toMatch(/[▪▫]/)
  })

  it('ticks every machine once the deploy has switched, and keeps the bars clear', () => {
    const L = bufs(LAYERS, 170, 52)
    story(L, 'deploy', 1, 60, ['workstation', 'homeserver', 'pi'])
    expect(text(L.good).match(/✓/g)).toHaveLength(3)
    expect(text(L.fg)).toContain('⌥ forge')
    for (const n of LAYERS) expect(L[n].ch[0].join('').trim() + L[n].ch[51].join('').trim()).toBe('')
  })

  it('draws the five acts of the blueprint', () => {
    const at = (t: number) => {
      const L = bufs(SHEET, 170, 52)
      blueprint(L, t, 0)
      return L
    }
    expect(text(at(0.42).hot)).toContain('┃') // the scan
    const fail = at(0.6)
    expect(text(fail.bad)).toContain('hosts/pi')
    expect(text(fail.bad)).toContain('error: infinite recursion encountered')
    expect(text(fail.fg)).toContain('STATUS failing')
    expect(text(at(0.7).good)).toContain('patch · hosts/pi/default.nix')
    const done = at(1)
    expect(text(done.good)).toContain('NIX FLAKE CHECK · PASSED')
    expect(text(done.good)).toContain('BUILT · 3 MACHINES')
    expect(text(done.bad)).not.toContain('✖')
    expect(text(done.good).match(/✓/g)).toHaveLength(15)
  })

  it('lays the callouts on the sheet when the window is short', () => {
    const at = (t: number) => {
      const L = bufs(SHEET, 151, 45)
      blueprint(L, t, 0)
      return L
    }
    const fail = at(0.6)
    expect(text(fail.bad)).toContain('hosts/pi')
    expect(text(fail.bad)).toContain('error: infinite recursion encountered')
    expect(text(fail.fg)).toContain('STATUS failing')
    const fix = at(0.7)
    expect(text(fix.good)).toContain('patch · hosts/pi/default.nix')
    expect(text(fix.bad)).not.toContain('error:')
  })
})
