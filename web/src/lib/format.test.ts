import { describe, expect, it } from 'vitest'
import { age, days } from './format'

describe('age', () => {
  const now = Date.parse('2026-11-19T12:00:00Z')
  const ago = (s: number) => age(now / 1000 - s, now)
  it('uses one format from seconds to years', () => {
    expect(ago(20)).toBe('just now')
    expect(ago(300)).toBe('5 min ago')
    expect(ago(3 * 3600)).toBe('3 h ago')
    expect(ago(47 * 86400)).toBe('47 d ago')
    expect(ago(127 * 86400)).toBe('4 months ago')
    expect(ago(800 * 86400)).toBe('2 years ago')
    expect(ago(-50)).toBe('just now')
  })
  it('counts whole days', () => {
    expect(days(now / 1000 - 31 * 86400 - 5, now)).toBe(31)
    expect(days(now / 1000 + 100, now)).toBe(0)
  })
})
