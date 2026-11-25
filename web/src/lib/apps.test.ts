import { describe, expect, it } from 'vitest'
import { appsIn, attributesText, configPathProblem, configTree, countKeys, keyText, kindOf, parseAttributes, parseKey, parseValue } from './apps'

describe('configTree', () => {
  it('joins settings and secrets, objects first', () => {
    const t = configTree({ theme: 'dark', 'Servers[ID=a]': { Hostname: 'h' } }, { 'Servers[ID=a]': { Password: 'music' }, token: 'tok' })
    expect(t).toEqual([
      {
        key: 'Servers[ID=a]',
        path: ['Servers[ID=a]'],
        children: [
          { key: 'Hostname', path: ['Servers[ID=a]', 'Hostname'], value: 'h' },
          { key: 'Password', path: ['Servers[ID=a]', 'Password'], secret: 'music' },
        ],
      },
      { key: 'theme', path: ['theme'], value: 'dark' },
      { key: 'token', path: ['token'], secret: 'tok' },
    ])
    expect(countKeys(t)).toBe(4)
  })
  it('keeps a list as a value', () => {
    expect(configTree({ l: [1, 2] })).toEqual([{ key: 'l', path: ['l'], value: [1, 2] }])
  })
})

describe('parseKey', () => {
  it('splits on dots outside quotes and list items', () => {
    expect(parseKey('Servers[ID=a.b].Hostname', 'toml')).toEqual(['Servers[ID=a.b]', 'Hostname'])
    expect(parseKey('"editor.fontSize"', 'json')).toEqual(['editor.fontSize'])
    expect(parseKey('plugins.Foo.enabled', 'json')).toEqual(['plugins', 'Foo', 'enabled'])
  })
  it('reads INI as section.key and properties as one key', () => {
    expect(parseKey('General.a.b', 'ini')).toEqual(['General', 'a.b'])
    expect(parseKey('leak.until', 'properties')).toEqual(['leak.until'])
    expect(parseKey('size', 'ini')).toBe('An INI key is section.key.')
  })
  it('refuses broken keys', () => {
    expect(parseKey('', 'json')).toBe('Enter a key.')
    expect(parseKey('a..b', 'json')).toMatch(/empty part/)
    expect(parseKey('a[b', 'json')).toMatch(/open quote or bracket/)
    expect(parseKey('servers[id]', 'json')).toMatch(/Name\[Field=Value\]/)
  })
  it('round-trips through keyText', () => {
    for (const [t, f] of [['a."b.c".d', 'json'], ['Servers[ID=a.b].Host', 'toml'], ['General.a.b', 'ini'], ['x.y', 'properties']] as const) {
      const p = parseKey(t, f)
      expect(typeof p).not.toBe('string')
      expect(parseKey(keyText(p as string[], f), f)).toEqual(p)
    }
  })
})

describe('values', () => {
  it('types from the value', () => {
    expect([kindOf(true), kindOf(3), kindOf([1]), kindOf('x')]).toEqual(['boolean', 'number', 'list', 'text'])
  })
  it('parses what is typed', () => {
    expect(parseValue('number', '4.5')).toEqual({ value: 4.5 })
    expect(parseValue('number', 'x')).toEqual({ problem: 'Enter a number.' })
    expect(parseValue('list', '["a", 1]')).toEqual({ value: ['a', 1] })
    expect(parseValue('list', '{}')).toHaveProperty('problem')
    expect(parseValue('boolean', 'true')).toEqual({ value: true })
  })
})

describe('paths and attributes', () => {
  it('checks a path in ~', () => {
    expect(configPathProblem('.config/a.json', [])).toBe('')
    expect(configPathProblem('/etc/x', [])).toMatch(/inside the home/)
    expect(configPathProblem('a/../b', [])).toMatch(/\.\./)
    expect(configPathProblem('a.json', ['a.json'])).toMatch(/already/)
  })
  it('reads key=value lines', () => {
    expect(parseAttributes('service=supersonic\n username = a1 ')).toEqual({ service: 'supersonic', username: 'a1' })
    expect(parseAttributes('nope')).toMatch(/key=value/)
    expect(parseAttributes('')).toMatch(/attributes/)
    expect(attributesText({ a: '1', b: '2' })).toBe('a=1\nb=2')
  })
})

describe('known apps', () => {
  it('finds them by package or by a managed file', () => {
    expect(appsIn(['supersonic-wayland', 'git'], []).map((a) => a.name)).toEqual(['Supersonic'])
    expect(appsIn([], ['.claude/settings.json']).map((a) => a.name)).toEqual(['Claude Code'])
  })
})
