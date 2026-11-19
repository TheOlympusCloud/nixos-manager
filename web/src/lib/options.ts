// NixOS option helpers: attribute paths, form controls from option types, Nix literals and docs.

export type Seg = { text: string; kind: 'attr' | 'name' | 'list' }

/**
 * Split an option name into its attribute path: dots separate segments, a quoted
 * segment keeps its dots (`virtualHosts."a.b"`), `<name>` is a placeholder the user
 * fills in and `*` stands for list elements (not addressable by a path).
 */
export function optionSegments(name: string): Seg[] {
  const segs: Seg[] = []
  let i = 0
  for (;;) {
    let text = ''
    let quoted = false
    if (name[i] === '"') {
      quoted = true
      for (i++; i < name.length && name[i] !== '"'; i++) {
        if (name[i] === '\\') i++
        text += name[i] ?? ''
      }
      i++ // closing quote
    } else {
      const dot = name.indexOf('.', i)
      const end = dot < 0 ? name.length : dot
      text = name.slice(i, end)
      i = end
    }
    const kind = quoted ? 'attr' : /^<.+>$/.test(text) ? 'name' : text === '*' ? 'list' : 'attr'
    segs.push({ text, kind })
    if (name[i] !== '.') return segs
    i++
  }
}

/** The settings path for an option, with the user's names in place of the `<name>` placeholders, in order. */
export function fillPath(segs: Seg[], names: string[]): string[] {
  let n = 0
  return segs.map((s) => (s.kind === 'name' ? (names[n++] ?? '') : s.text))
}

/**
 * Does a documented option (with `<name>` placeholders) declare this concrete path?
 * Returns the names that fill its placeholders, or null: users.users.<name>.shell vs users.users.pablo.shell → ["pablo"].
 */
export function matchOption(generic: string, path: string[]): string[] | null {
  const segs = optionSegments(generic)
  if (segs.length !== path.length) return null
  const names: string[] = []
  for (const [i, s] of segs.entries()) {
    if (s.kind === 'name') names.push(path[i])
    else if (s.kind === 'list' || s.text !== path[i]) return null
  }
  return names
}

/** Write a path back as an option name, quoting segments that need it. */
export function pathName(path: string[]): string {
  return path.map((p) => (/^[A-Za-z_][A-Za-z0-9_'-]*$/.test(p) ? p : JSON.stringify(p))).join('.')
}

export type Scalar =
  | { kind: 'number'; min?: number; max?: number }
  | { kind: 'text'; noSpaces: boolean }
  | { kind: 'select'; choices: (string | number)[] }
export type Control =
  | { kind: 'switch' }
  | { kind: 'tristate' }
  | ((Scalar & { nullable: boolean }) | { kind: 'list'; item: Scalar })
  | { kind: 'json' }

function scalar(type: string): Scalar | null {
  let m: RegExpMatchArray | null
  if (/^(signed )?integer$/.test(type)) return { kind: 'number' }
  // "16 bit unsigned integer; between 0 and 65535 (both inclusive)", "integer between 1 and 10 (both inclusive)"
  if ((m = type.match(/^(?:\d+ bit (?:unsigned |signed )?integer;|integer) between (-?\d+) and (-?\d+)/)))
    return { kind: 'number', min: Number(m[1]), max: Number(m[2]) }
  if (/^unsigned integer, meaning >=0/.test(type)) return { kind: 'number', min: 0 }
  if (/^positive integer, meaning >0/.test(type)) return { kind: 'number', min: 1 }
  if (/^(non-empty |single-line )?string( without spaces)?$/.test(type)) return { kind: 'text', noSpaces: type.endsWith('without spaces') }
  if (/^(absolute )?path$|^string matching the pattern |^strings concatenated with /.test(type)) return { kind: 'text', noSpaces: false }
  if (type.startsWith('one of ')) {
    const choices = [...type.slice(7).matchAll(/"((?:[^"\\]|\\.)*)"|(-?\d+)/g)].map((c) => (c[1] !== undefined ? JSON.parse(`"${c[1]}"`) : Number(c[2])))
    return choices.length ? { kind: 'select', choices } : null
  }
  return null
}

/** Which form control edits an option of this type (the type string NixOS documents). */
export function controlFor(type: string): Control {
  const t = type.trim()
  if (t === 'boolean') return { kind: 'switch' }
  if (t === 'null or boolean') return { kind: 'tristate' }
  const nullable = t.startsWith('null or ')
  const s = scalar(nullable ? t.slice(8) : t)
  if (s) return { ...s, nullable }
  if (!nullable && t.startsWith('list of ')) {
    const item = scalar(t.slice(8))
    if (item) return { kind: 'list', item }
  }
  return { kind: 'json' }
}

// ---- Nix literals (option defaults and examples come as Nix expressions) ----

/** Parse a simple Nix literal (bool, null, number, string, list of those) into JSON; undefined when it is anything else. */
export function parseNix(src: string | null | undefined): unknown {
  if (src == null) return undefined
  let i = 0
  const ws = () => {
    while (i < src.length && /\s/.test(src[i])) i++
  }
  const fail = Symbol()
  const value = (): unknown => {
    ws()
    if (src[i] === '"') {
      let out = ''
      for (i++; i < src.length && src[i] !== '"'; i++) {
        if (src[i] === '$' && src[i + 1] === '{') return fail // interpolation
        if (src[i] === '\\') {
          i++
          out += ({ n: '\n', t: '\t', r: '\r' } as Record<string, string>)[src[i]] ?? src[i]
        } else out += src[i]
      }
      if (src[i] !== '"') return fail
      i++
      return out
    }
    if (src[i] === '[') {
      i++
      const items: unknown[] = []
      for (;;) {
        ws()
        if (src[i] === ']') return i++, items
        if (i >= src.length) return fail
        const v = value()
        if (v === fail) return fail
        items.push(v)
      }
    }
    const m = /^(-?\d+(?:\.\d+)?|true|false|null)(?![\w.-])/.exec(src.slice(i))
    if (!m) return fail
    i += m[0].length
    return m[1] === 'true' ? true : m[1] === 'false' ? false : m[1] === 'null' ? null : Number(m[1])
  }
  const v = value()
  ws()
  return v === fail || i < src.length ? undefined : v
}

// ---- option descriptions (Markdown) ----

const escapeHTML = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')

function inline(text: string): string {
  // Code spans first, so emphasis and links never apply inside them.
  return text
    .split(/(\{[a-z]+\}`[^`]*`|`[^`]*`)/)
    .map((part, i) => {
      if (i % 2) return `<code>${escapeHTML(part.replace(/^\{[a-z]+\}/, '').slice(1, -1))}</code>`
      return escapeHTML(part)
        .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
        .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
        .replace(/(^|[\s(])\*([^*\s][^*]*)\*/g, '$1<em>$2</em>')
    })
    .join('')
}

/** Option docs as HTML: everything escaped first, then paragraphs, lists, code, emphasis and http(s) links. */
export function docsHTML(md: string): string {
  const blocks = md
    .replace(/\r/g, '')
    .replace(/^:::.*$/gm, '') // admonition fences ({.note}); their text stays
    .split(/\n\s*\n/)
    .map((b) => b.trim())
    .filter(Boolean)
  return blocks
    .map((b) => {
      if (b.startsWith('```')) return `<pre><code>${escapeHTML(b.replace(/^```[^\n]*\n?|```$/g, ''))}</code></pre>`
      const lines = b.split('\n')
      if (lines.every((l) => /^\s*[-*] /.test(l))) return `<ul>${lines.map((l) => `<li>${inline(l.replace(/^\s*[-*] /, ''))}</li>`).join('')}</ul>`
      return `<p>${inline(lines.join(' '))}</p>`
    })
    .join('')
}
