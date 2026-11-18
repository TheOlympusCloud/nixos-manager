import { describe, expect, it } from 'vitest'
import {
  continent, easeInOut, easeOutExpo, fibonacciSphere, noise3, orbitPoint, orbitRadius, orbitTilt, prng, pulseWindow,
} from './math'

describe('fibonacciSphere', () => {
  const n = 4000
  const p = fibonacciSphere(n)

  it('puts every point on the unit sphere', () => {
    for (let i = 0; i < n; i++) {
      const r = Math.hypot(p[i * 3], p[i * 3 + 1], p[i * 3 + 2])
      expect(r).toBeCloseTo(1, 6)
    }
  })

  it('spreads points evenly: every octant holds about an eighth', () => {
    const counts = new Array(8).fill(0)
    for (let i = 0; i < n; i++) {
      counts[(p[i * 3] > 0 ? 1 : 0) + (p[i * 3 + 1] > 0 ? 2 : 0) + (p[i * 3 + 2] > 0 ? 4 : 0)]++
    }
    for (const c of counts) expect(Math.abs(c - n / 8)).toBeLessThan(n * 0.01)
  })

  it('keeps nearest neighbours a consistent distance apart', () => {
    // Each point owns ~4π/n of area, so nearest neighbours sit near sqrt(4π/n) everywhere, poles included.
    const expected = Math.sqrt((4 * Math.PI) / n)
    const nearest: number[] = []
    for (let i = 0; i < n; i += 97) {
      let best = Infinity
      for (let j = 0; j < n; j++) {
        if (j === i) continue
        const d = Math.hypot(p[i * 3] - p[j * 3], p[i * 3 + 1] - p[j * 3 + 1], p[i * 3 + 2] - p[j * 3 + 2])
        best = Math.min(best, d)
      }
      nearest.push(best)
    }
    const mean = nearest.reduce((a, b) => a + b) / nearest.length
    expect(mean).toBeGreaterThan(expected * 0.7)
    expect(mean).toBeLessThan(expected * 1.3)
    expect(Math.max(...nearest) / Math.min(...nearest)).toBeLessThan(2)
  })
})

describe('noise and continents', () => {
  it('noise is deterministic and bounded', () => {
    expect(noise3(1.3, 2.7, 0.4)).toBe(noise3(1.3, 2.7, 0.4))
    for (let i = 0; i < 200; i++) {
      const v = noise3(i * 0.37, i * 0.11, i * 0.53)
      expect(v).toBeGreaterThanOrEqual(0)
      expect(v).toBeLessThanOrEqual(1)
    }
  })

  it('continents cover a plausible fraction of the sphere', () => {
    const p = fibonacciSphere(3000)
    let land = 0
    for (let i = 0; i < 3000; i++) land += continent(p[i * 3], p[i * 3 + 1], p[i * 3 + 2])
    expect(land / 3000).toBeGreaterThan(0.15)
    expect(land / 3000).toBeLessThan(0.6)
  })

  it('prng is seeded and uniform-ish', () => {
    const a = prng(5)
    const b = prng(5)
    expect(a()).toBe(b())
    let sum = 0
    for (let i = 0; i < 1000; i++) sum += a()
    expect(sum / 1000).toBeGreaterThan(0.45)
    expect(sum / 1000).toBeLessThan(0.55)
  })
})

describe('orbits', () => {
  it('stays at the ring radius all the way round', () => {
    const o = { x: 0, y: 0, z: 0 }
    for (let i = 0; i < 3; i++) {
      for (let a = 0; a < Math.PI * 2; a += 0.1) {
        orbitPoint(i, a, o)
        expect(Math.hypot(o.x, o.y, o.z)).toBeCloseTo(orbitRadius(i), 6)
      }
    }
  })

  it('lies in a plane tilted by the ring inclination', () => {
    const o = { x: 0, y: 0, z: 0 }
    for (let i = 0; i < 3; i++) {
      let maxY = 0
      for (let a = 0; a < Math.PI * 2; a += 0.05) {
        orbitPoint(i, a, o)
        maxY = Math.max(maxY, Math.abs(o.y))
      }
      expect(maxY).toBeCloseTo(orbitRadius(i) * Math.sin(orbitTilt(i)), 3)
    }
  })

  it('rings never share a radius', () => {
    expect(new Set([0, 1, 2].map(orbitRadius)).size).toBe(3)
  })
})

describe('easing and deploy timing', () => {
  it('eases run 0 to 1', () => {
    for (const e of [easeInOut, easeOutExpo]) {
      expect(e(0)).toBeCloseTo(0, 6)
      expect(e(1)).toBeCloseTo(1, 6)
      expect(e(0.5)).toBeGreaterThan(0)
      expect(e(0.5)).toBeLessThan(1)
    }
  })

  it('pulses leave in order and all land before the chapter ends', () => {
    for (const n of [1, 3, 6]) {
      let last = 0
      for (let i = 0; i < n; i++) {
        const w = pulseWindow(i, n)
        expect(w.start).toBeGreaterThanOrEqual(last)
        expect(w.arrive).toBeGreaterThan(w.start)
        expect(w.arrive).toBeLessThanOrEqual(1)
        last = w.arrive
      }
    }
  })
})
