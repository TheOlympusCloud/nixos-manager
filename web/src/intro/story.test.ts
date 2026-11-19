import { describe, expect, it } from 'vitest'
import { STORY, countdown, counterLabel, formatGrouped, lineWindows, locate, window01, type Section } from './story'

const vh = 900
// Sections laid out from STORY: every chapter spans len*vh (the flowing tails of console and outro are extra).
const sections = STORY.reduce<Section[]>((acc, s) => {
  const top = acc.length ? acc[acc.length - 1].top + acc[acc.length - 1].height : 0
  acc.push({ id: s.id, top, height: s.len * vh, pinned: s.len > 0 })
  return acc
}, [])

describe('locate', () => {
  it('starts in the hero at the top', () => {
    expect(locate(0, vh, sections)).toEqual({ id: 'hero', t: 0, index: 0 })
  })
  it('scrubs a pinned chapter over its pinned range', () => {
    const drift = sections[1]
    expect(locate(drift.top, vh, sections).t).toBe(0)
    expect(locate(drift.top + (drift.height - vh) / 2, vh, sections).t).toBeCloseTo(0.5)
    expect(locate(drift.top + drift.height - vh, vh, sections)).toMatchObject({ id: 'drift', t: 1 })
    expect(locate(drift.top + drift.height - 1, vh, sections)).toMatchObject({ id: 'drift', t: 1 })
  })
  it('scrubs an unpinned chapter over its whole height', () => {
    const flow: Section[] = [
      { id: 'hero', top: 0, height: 1350, pinned: true },
      { id: 'console', top: 1350, height: 2000, pinned: false },
    ]
    expect(locate(1350 + 1000, vh, flow)).toMatchObject({ id: 'console', t: 0.5 })
    expect(locate(1350 + 2000, vh, flow)).toMatchObject({ id: 'console', t: 1 })
  })
  it('walks every chapter in order and clamps past the end', () => {
    const seen = sections.map((s) => locate(s.top + 1, vh, sections).id)
    expect(seen).toEqual(STORY.map((s) => s.id))
    expect(locate(1e9, vh, sections)).toMatchObject({ id: 'outro', t: 1 })
    expect(locate(-50, vh, sections)).toMatchObject({ id: 'hero', t: 0 })
  })
})

describe('counter', () => {
  it('collapses 100 000 packages into 1 graph', () => {
    expect(countdown(0)).toBe(100_000)
    expect(countdown(1)).toBe(1)
    let last = Infinity
    for (let t = 0; t <= 1.0001; t += 0.01) {
      const v = countdown(t)
      expect(v).toBeLessThanOrEqual(last)
      last = v
    }
    expect(countdown(0.5)).toBeGreaterThan(100) // still a long way down half-way: the collapse is logarithmic
  })
  it('groups thousands and names the unit', () => {
    expect(formatGrouped(100_000)).toBe('100 000')
    expect(formatGrouped(1)).toBe('1')
    expect(formatGrouped(12_345_678)).toBe('12 345 678')
    expect(counterLabel(0)).toBe('packages')
    expect(counterLabel(1)).toBe('graph')
  })
})

describe('windows', () => {
  it('clamps a sub-window of t', () => {
    expect(window01(0, 0.2, 0.4)).toBe(0)
    expect(window01(0.3, 0.2, 0.4)).toBeCloseTo(0.5)
    expect(window01(1, 0.2, 0.4)).toBe(1)
  })
  it('staggers line reveals inside the range, overlapping', () => {
    const w = lineWindows(3, 0.1, 0.7)
    expect(w).toHaveLength(3)
    expect(w[0][0]).toBeCloseTo(0.1)
    expect(w[2][1]).toBeCloseTo(0.7)
    expect(w[1][0]).toBeLessThan(w[0][1]) // the next line starts before the previous one finishes
    expect(w[1][0]).toBeGreaterThan(w[0][0])
  })
})
