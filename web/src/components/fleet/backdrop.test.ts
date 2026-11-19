import { describe, expect, it } from 'vitest'
import { prng, starField, tooSlow } from './backdrop'

describe('starField', () => {
  it('is the same sky every time', () => {
    expect(Array.from(starField(50).position)).toEqual(Array.from(starField(50).position))
    expect(prng(3)()).toBe(prng(3)())
  })
  it('keeps every star inside the tile, with a depth from far (0) to near (1)', () => {
    const f = starField(400)
    expect(f.position.length).toBe(1200)
    for (let i = 0; i < 400; i++) {
      for (const k of [0, 1, 2]) {
        expect(f.position[i * 3 + k]).toBeGreaterThanOrEqual(0)
        expect(f.position[i * 3 + k]).toBeLessThan(1)
      }
      expect(f.size[i]).toBeGreaterThan(0)
    }
  })
  it('puts most stars far away', () => {
    const f = starField(1000)
    let far = 0
    for (let i = 0; i < 1000; i++) if (f.position[i * 3 + 2] < 0.5) far++
    expect(far).toBeGreaterThan(600)
  })
})

describe('tooSlow', () => {
  it('waits for enough frames before judging', () => {
    expect(tooSlow([90, 90, 90])).toBe(false)
  })
  it('gives up animating when the typical frame blows the budget', () => {
    expect(tooSlow(Array(12).fill(40))).toBe(true)
  })
  it('ignores a few slow frames while shaders compile', () => {
    expect(tooSlow([120, 80, 3, 2, 2, 3, 2, 2, 3, 2, 2, 2])).toBe(false)
  })
})
