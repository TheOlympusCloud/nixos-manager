import { describe, expect, it } from 'vitest'
import { ago, isWithin, minimalChange, stripAnsi } from './util'

describe('minimalChange', () => {
  it('replaces only the differing middle', () => {
    expect(minimalChange('{ a = 1; }', '{ a = 2; }')).toEqual({ from: 6, to: 7, insert: '2' })
    expect(minimalChange('abc', 'abXc')).toEqual({ from: 2, to: 2, insert: 'X' })
    expect(minimalChange('abc', 'ac')).toEqual({ from: 1, to: 2, insert: '' })
    expect(minimalChange('aaa', 'aa')).toEqual({ from: 2, to: 3, insert: '' })
    expect(minimalChange('', 'new')).toEqual({ from: 0, to: 0, insert: 'new' })
    expect(minimalChange('same', 'same')).toBeNull()
  })

  it('always reproduces the target', () => {
    const cases: [string, string][] = [['abcabc', 'abc'], ['x', 'xxxx'], ['hello world', 'help world'], ['ab', 'ba']]
    for (const [a, b] of cases) {
      const c = minimalChange(a, b)!
      expect(a.slice(0, c.from) + c.insert + a.slice(c.to)).toBe(b)
    }
  })
})

describe('helpers', () => {
  it('isWithin matches the path itself and its descendants only', () => {
    expect(isWithin('hosts/web.nix', 'hosts')).toBe(true)
    expect(isWithin('hosts', 'hosts')).toBe(true)
    expect(isWithin('hosts2/a.nix', 'hosts')).toBe(false)
  })

  it('ago formats relative times', () => {
    const now = Date.parse('2026-01-10T12:00:00Z')
    expect(ago('2026-01-10T11:59:30Z', now)).toBe('just now')
    expect(ago('2026-01-10T11:55:00Z', now)).toBe('5 min ago')
    expect(ago('2026-01-10T09:00:00Z', now)).toBe('3 h ago')
    expect(ago('2026-01-08T12:00:00Z', now)).toBe('2 d ago')
    expect(ago('', now)).toBe('')
    expect(ago('garbage', now)).toBe('garbage')
  })

  it('stripAnsi removes color codes', () => {
    expect(stripAnsi('\x1b[31;1merror:\x1b[0m boom')).toBe('error: boom')
  })
})
