import { describe, expect, it } from 'vitest'
import { countAt } from './motion'

describe('countAt', () => {
  it('starts at the old value and ends exactly at the new one', () => {
    expect(countAt(3, 22, 0)).toBe(3)
    expect(countAt(3, 22, 1)).toBe(22)
    expect(countAt(3, 22, 1.4)).toBe(22)
    expect(countAt(3, 22, -0.2)).toBe(3)
  })
  it('never overshoots, counting up or down', () => {
    for (let t = 0; t <= 1; t += 0.05) {
      const up = countAt(0, 40, t)
      const down = countAt(40, 0, t)
      expect(up).toBeGreaterThanOrEqual(0)
      expect(up).toBeLessThanOrEqual(40)
      expect(down).toBeGreaterThanOrEqual(0)
      expect(down).toBeLessThanOrEqual(40)
    }
  })
  it('moves fastest at the start', () => {
    expect(countAt(0, 100, 0.25)).toBeGreaterThan(50)
  })
})
