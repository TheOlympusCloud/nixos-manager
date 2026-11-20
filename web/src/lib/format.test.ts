import { describe, expect, it } from 'vitest'
import { age, bytes, days, signedBytes } from './format'

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

describe('bytes', () => {
  it('shows sizes in binary units, as Nix does', () => {
    expect(bytes(0)).toBe('0 B')
    expect(bytes(1023)).toBe('1023 B')
    expect(bytes(1536)).toBe('1.5 KiB')
    expect(bytes(18.4 * 1024 * 1024)).toBe('18.4 MiB')
    expect(bytes(1.27 * 1024 ** 3)).toBe('1.27 GiB')
    expect(signedBytes(19293798)).toBe('+18.4 MiB')
    expect(signedBytes(-12288)).toBe('−12.0 KiB')
    expect(signedBytes(0)).toBe('0 B')
  })
})
