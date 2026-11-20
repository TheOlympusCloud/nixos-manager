import { describe, expect, it } from 'vitest'
import { expiresAt, expiry, lifetimes, scopes } from './tokens'

const now = Date.parse('2026-01-10T12:00:00Z')

describe('tokens', () => {
  it('offers the three scopes, least access first', () => {
    expect(scopes.map((s) => s.value)).toEqual(['read', 'edit', 'full'])
  })

  it('turns a lifetime into the expiry to send', () => {
    expect(expiresAt(0, now)).toBeUndefined()
    expect(expiresAt(30, now)).toBe('2026-02-09T12:00:00.000Z')
    expect(lifetimes[0].days).toBe(0)
  })

  it('describes when a token expires', () => {
    expect(expiry(undefined, now)).toBe('never')
    expect(expiry('', now)).toBe('never')
    expect(expiry('2026-01-10T11:59:59Z', now)).toBe('expired')
    expect(expiry('2026-01-10T12:00:00Z', now)).toBe('expired')
    expect(expiry('2026-01-10T12:30:00Z', now)).toBe('in 30 min')
    expect(expiry('2026-01-10T17:00:00Z', now)).toBe('in 5 h')
    expect(expiry('2026-02-09T12:00:00Z', now)).toBe('in 30 d')
    expect(expiry('garbage', now)).toBe('garbage')
  })
})
