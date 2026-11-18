import { describe, expect, it } from 'vitest'
import { controlFor, docsHTML, fillPath, optionSegments, parseNix, pathName } from './options'

describe('controlFor', () => {
  it('maps each documented option type to its control', () => {
    expect(controlFor('boolean')).toEqual({ kind: 'switch' })
    expect(controlFor('null or boolean')).toEqual({ kind: 'tristate' })
    expect(controlFor('signed integer')).toEqual({ kind: 'number', nullable: false })
    expect(controlFor('16 bit unsigned integer; between 0 and 65535 (both inclusive)')).toEqual({ kind: 'number', min: 0, max: 65535, nullable: false })
    expect(controlFor('32 bit signed integer; between -2147483648 and 2147483647 (both inclusive)')).toMatchObject({ min: -2147483648, max: 2147483647 })
    expect(controlFor('positive integer, meaning >0')).toEqual({ kind: 'number', min: 1, nullable: false })
    expect(controlFor('unsigned integer, meaning >=0')).toEqual({ kind: 'number', min: 0, nullable: false })
    expect(controlFor('integer between 1 and 10 (both inclusive)')).toMatchObject({ kind: 'number', min: 1, max: 10 })
    expect(controlFor('null or signed integer')).toEqual({ kind: 'number', nullable: true })
    expect(controlFor('string')).toEqual({ kind: 'text', noSpaces: false, nullable: false })
    expect(controlFor('string without spaces')).toEqual({ kind: 'text', noSpaces: true, nullable: false })
    expect(controlFor('null or string')).toEqual({ kind: 'text', noSpaces: false, nullable: true })
    expect(controlFor('null or string without spaces')).toEqual({ kind: 'text', noSpaces: true, nullable: true })
    expect(controlFor('null or absolute path')).toEqual({ kind: 'text', noSpaces: false, nullable: true })
    expect(controlFor('one of "yes", "without-password", "no"')).toEqual({ kind: 'select', choices: ['yes', 'without-password', 'no'], nullable: false })
    expect(controlFor('null or one of "a", "b"')).toEqual({ kind: 'select', choices: ['a', 'b'], nullable: true })
    expect(controlFor('one of 1, 2, 3')).toMatchObject({ kind: 'select', choices: [1, 2, 3] })
    expect(controlFor('list of 16 bit unsigned integer; between 0 and 65535 (both inclusive)')).toEqual({ kind: 'list', item: { kind: 'number', min: 0, max: 65535 } })
    expect(controlFor('list of string')).toEqual({ kind: 'list', item: { kind: 'text', noSpaces: false } })
    expect(controlFor('list of one of "tcp", "udp"')).toEqual({ kind: 'list', item: { kind: 'select', choices: ['tcp', 'udp'] } })
  })

  it('falls back to a JSON editor for everything else', () => {
    for (const t of ['attribute set of string', 'list of (submodule)', 'package', 'string or signed integer', 'null or (list of string)', 'list of attribute set of string'])
      expect(controlFor(t)).toEqual({ kind: 'json' })
  })
})

describe('optionSegments', () => {
  it('splits on dots and marks placeholders', () => {
    const segs = optionSegments('services.nginx.virtualHosts.<name>.root')
    expect(segs.map((s) => s.text)).toEqual(['services', 'nginx', 'virtualHosts', '<name>', 'root'])
    expect(segs.map((s) => s.kind)).toEqual(['attr', 'attr', 'attr', 'name', 'attr'])
    expect(fillPath(segs, ['example.org'])).toEqual(['services', 'nginx', 'virtualHosts', 'example.org', 'root'])
  })

  it('never splits a quoted segment on its dots', () => {
    const segs = optionSegments('services.nginx.virtualHosts."a.b".root')
    expect(fillPath(segs, [])).toEqual(['services', 'nginx', 'virtualHosts', 'a.b', 'root'])
    expect(optionSegments('x."say \\"hi\\"".y').map((s) => s.text)).toEqual(['x', 'say "hi"', 'y'])
    expect(optionSegments('"<name>".a')[0]).toEqual({ text: '<name>', kind: 'attr' })
  })

  it('fills several placeholders in order and flags list elements', () => {
    const segs = optionSegments('users.users.<name>.openssh.<key>.x')
    expect(fillPath(segs, ['pablo', 'k'])).toEqual(['users', 'users', 'pablo', 'openssh', 'k', 'x'])
    expect(optionSegments('networking.interfaces.<name>.ipv4.addresses.*.address')[5].kind).toBe('list')
  })

  it('writes paths back as names, quoting where needed', () => {
    expect(pathName(['services', 'nginx', 'virtualHosts', 'a.b', 'root'])).toBe('services.nginx.virtualHosts."a.b".root')
    expect(fillPath(optionSegments(pathName(['a', 'b c', 'd'])), [])).toEqual(['a', 'b c', 'd'])
  })
})

describe('parseNix', () => {
  it('reads the literals option docs use', () => {
    expect(parseNix('false')).toBe(false)
    expect(parseNix('null')).toBe(null)
    expect(parseNix('42')).toBe(42)
    expect(parseNix('"prohibit-password"')).toBe('prohibit-password')
    expect(parseNix('"a\\"b\\n"')).toBe('a"b\n')
    expect(parseNix('[\n  22\n  2222\n]')).toEqual([22, 2222])
    expect(parseNix('[ ]')).toEqual([])
    expect(parseNix('[ "a" [ 1 ] ]')).toEqual(['a', [1]])
  })

  it('gives up on anything that is not a plain literal', () => {
    for (const s of ['pkgs.hello', '{ a = 1; }', '"${x}"', 'true false', '[ 1', null, undefined]) expect(parseNix(s)).toBeUndefined()
  })
})

describe('docsHTML', () => {
  it('escapes HTML before formatting', () => {
    expect(docsHTML('<script>alert(1)</script> & "x"')).toBe('<p>&lt;script&gt;alert(1)&lt;/script&gt; &amp; &quot;x&quot;</p>')
    expect(docsHTML('[x](javascript:alert(1))')).not.toContain('<a')
    expect(docsHTML('[x](https://a.b/"onmouseover=1)')).not.toMatch(/href="[^"]*"on/)
  })

  it('formats code, roles, emphasis, links, paragraphs and lists', () => {
    expect(docsHTML('Use `a <b>` or {option}`services.x.enable`.')).toBe('<p>Use <code>a &lt;b&gt;</code> or <code>services.x.enable</code>.</p>')
    expect(docsHTML('**bold** and *em*, see [docs](https://nixos.org).')).toBe(
      '<p><strong>bold</strong> and <em>em</em>, see <a href="https://nixos.org" target="_blank" rel="noreferrer">docs</a>.</p>',
    )
    expect(docsHTML('One\nline.\n\n- a\n- `b*c*`')).toBe('<p>One line.</p><ul><li>a</li><li><code>b*c*</code></li></ul>')
    expect(docsHTML(':::{.note}\nCareful.\n:::')).toBe('<p>Careful.</p>')
  })
})
