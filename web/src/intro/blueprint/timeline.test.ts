import { describe, expect, it } from 'vitest'
import { EDGES, FAILING, NODES, STEPS, blueprintAt, stepAt } from './timeline'

describe('blueprint timeline', () => {
  it('is one contiguous sequence of acts from 0 to 1', () => {
    expect(STEPS[0].from).toBe(0)
    expect(STEPS[STEPS.length - 1].to).toBe(1)
    for (let i = 1; i < STEPS.length; i++) expect(STEPS[i].from).toBe(STEPS[i - 1].to)
    expect(STEPS.map((s) => s.id)).toEqual(['draw', 'evaluate', 'fail', 'fix', 'stamp'])
    expect(stepAt(0)).toBe('draw')
    expect(stepAt(0.6)).toBe('fail')
    expect(stepAt(1)).toBe('stamp')
  })

  it('references only known nodes', () => {
    const ids = new Set(NODES.map((n) => n.id))
    for (const [a, b] of EDGES) expect(ids.has(a) && ids.has(b), `${a} -> ${b}`).toBe(true)
    expect(ids.has(FAILING)).toBe(true)
  })

  it('starts blank and draws everything by the end of the first act', () => {
    const start = blueprintAt(0)
    expect(Object.values(start.nodes).every((n) => n.draw === 0 && n.status === 'idle')).toBe(true)
    expect(start.edges.every((e) => e === 0)).toBe(true)
    expect(start.stamp).toBe(0)
    const drawn = blueprintAt(STEPS[0].to)
    expect(Object.values(drawn.nodes).every((n) => n.draw === 1)).toBe(true)
    expect(drawn.edges.every((e) => e === 1)).toBe(true)
    // The root is drawn before the leaves.
    const mid = blueprintAt(0.1)
    expect(mid.nodes.flake.draw).toBeGreaterThan(mid.nodes.hass.draw)
  })

  it('scans every node and marks it ok, then fails the one machine', () => {
    const half = blueprintAt(0.42)
    expect(Object.values(half.nodes).some((n) => n.status === 'scanning')).toBe(true)
    const evaluated = blueprintAt(STEPS[1].to)
    for (const n of NODES) if (n.id !== FAILING) expect(evaluated.nodes[n.id].status, n.id).toBe('ok')
    const failed = blueprintAt(0.6)
    expect(failed.nodes[FAILING].status).toBe('fail')
    expect(failed.error).toBeCloseTo(1)
    expect(failed.fix).toBe(0)
  })

  it('fixes the failing node and stamps the drawing', () => {
    const fixing = blueprintAt(0.74)
    expect(fixing.fix).toBeGreaterThan(0)
    expect(fixing.nodes[FAILING].status).toBe('fixing')
    const done = blueprintAt(1)
    expect(Object.values(done.nodes).every((n) => n.status === 'ok' && n.scan === 1)).toBe(true)
    expect(done.stamp).toBe(1)
    expect(done.step).toBe('stamp')
  })

  it('clamps out-of-range input', () => {
    expect(blueprintAt(-1)).toEqual(blueprintAt(0))
    expect(blueprintAt(2)).toEqual(blueprintAt(1))
  })
})
