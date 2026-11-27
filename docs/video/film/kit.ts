// The video's recurring pieces, made of the stage's elements: the header, chapter cards, captions, machine
// cards, store paths, code cards and arrows. Sizes are in cells; the stage is COLS x ROWS.
import { A, COLS, ROWS, El, Pic, Stage, bufs, snowflake, stars, type Box, type Intro } from './film'

export const TOP = 3 // first row under the header
export const BOTTOM = 33 // last row above the caption
export const mid = (w: number) => Math.floor((COLS - w) / 2)
export const len = (s: string) => Math.max(...s.split('\n').map((l) => [...l].length))

/** Words of a title in the pixel face, as lines no wider than max cells. */
export function bigLines(text: string, max = COLS - 10): string[] {
  const out: string[] = []
  for (const word of text.toUpperCase().split(' ')) {
    const last = out[out.length - 1]
    if (last !== undefined && len(A.big(last + ' ' + word)) <= max) out[out.length - 1] = last + ' ' + word
    else out.push(word)
  }
  return out
}

/** The console's top bar for the video: where we are, on the first two rows. */
export function header(s: Stage) {
  const { n, chapter, title } = s.info
  s.html(2, 0, `<span class="c-acc">λ</span> <span class="c-m">${String(n).padStart(2, '0')}</span> <b>${A.esc(chapter)}</b>`)
  s.text(COLS - 2 - len(title), 0, title, 'c-m')
  s.text(2, 1, '─'.repeat(COLS - 4), 'c-line')
}

/** A line of text at the bottom, in a frame, typed in; the next one replaces it. */
const captions = new WeakMap<Stage, El>()
export function caption(s: Stage, text: string, tone: 'accent' | 'ok' | 'warn' | 'err' = 'accent'): El {
  captions.get(s)?.remove()
  const w = len(text) + 6
  const b = s.box(mid(w), ROWS - 4, w, 3, { tone })
  s.type(b.inner.x + 1, b.inner.y, text, 'c-b', b)
  captions.set(s, b)
  return b
}
export const uncaption = (s: Stage) => (captions.get(s)?.remove(), captions.delete(s))

/** A framed box with lines of text inside, sized to them unless w or h say otherwise. */
export function card(s: Stage, x: number, y: number, lines: string[], o: { title?: string; tag?: string; tone?: 'accent' | 'ok' | 'warn' | 'err'; cls?: string; w?: number; h?: number; kind?: A.FrameKind } = {}) {
  const w = o.w ?? Math.max(len(lines.join('\n') || ' ') + 4, (o.title?.length ?? 0) + (o.tag?.length ?? 0) + 8)
  const b = s.box(x, y, w, o.h ?? lines.length + 2, { title: o.title, tag: o.tag, tone: o.tone, kind: o.kind })
  const rows = lines.map((l, i) => s.text(b.inner.x, b.inner.y + i, l, o.cls ?? '', b))
  return Object.assign(b, { rows })
}

/** A machine, as the console's fleet graph draws it. */
export function machine(s: Stage, x: number, y: number, name: string, o: { arch?: string; lines?: string[]; tone?: 'accent' | 'ok' | 'warn' | 'err'; w?: number } = {}) {
  const lines = o.lines ?? ['● NixOS 26.05']
  return card(s, x, y, lines, { title: '▣ ' + name, tag: o.arch ?? (name === 'pi' ? 'aarch64' : 'x86_64'), tone: o.tone, w: o.w })
}

/** A store path, /nix/store/<hash>-name, its hash realised out of base-32. */
export function storePath(s: Stage, x: number, y: number, name: string, hash: string, parent?: El): El {
  const el = s.text(x, y, `/nix/store/${hash}-${name}`, '', parent)
  const paint = (h: string) => (el.node.innerHTML = `<span class="c-m">/nix/store/</span><span class="c-f">${h}</span><b>-${A.esc(name)}</b>`)
  el.cancel = A.run(6, (p) => paint(A.scramble(hash, p)))
  return el
}

/** Code in a frame, typed in a burst per tick. */
export function code(s: Stage, x: number, y: number, src: string, title?: string, perTick = 8) {
  const lines = src.split('\n')
  const b = s.box(x, y, Math.max(len(src), (title?.length ?? 0) + 2) + 4, lines.length + 2, { title, kind: 'square' })
  const t = s.type(b.inner.x, b.inner.y, src, '', b, perTick)
  return Object.assign(b, { body: t, lines })
}

/** A ▌ in a code card's margin, by line (0-based): the console's way of pointing at a line. */
export function point(s: Stage, c: ReturnType<typeof code>, line: number, tone = 'c-warn') {
  return s.text(c.rect.x + 1, c.inner.y + line, '▌', tone, c)
}

export const harrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => s.text(x, y, '─'.repeat(n - 1) + '▶', cls)
export const varrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => s.text(x, y, '│\n'.repeat(n - 1) + '▼', cls)
export const larrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => s.text(x, y, '◀' + '─'.repeat(n - 1), cls)
export const uarrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => s.text(x, y, '▲' + '\n│'.repeat(n - 1), cls)

/** A list, one item per row, each after a glyph. */
export function list(s: Stage, x: number, y: number, items: string[], cls = '', mark = '›', parent?: El) {
  return items.map((it, i) => s.html(x, y + i, `<span class="c-acc">${mark}</span> ${A.esc(it)}`, cls, parent))
}

// ---- Console captures ---------------------------------------------------------------------------------------

/** Where a capture sits: a frame whose inside has the screenshots' 16:9 shape, leaving room for a caption. */
export const PIC = { x: 16, y: 3, w: 113, h: 32 }

type Tone = 'accent' | 'ok' | 'warn' | 'err'
export type Step =
  | ['show', string, string?] // a capture, whole; on a cue
  | ['look', string | Box, string?, number?] // zoom to a recorded box (outlined) or an area (fractions); cue; margin
  | ['mark', string, string?, Tone?] // outline a recorded box, instead of the outline before
  | ['mark+', string, string?, Tone?] // one more outline, beside the others (boxes far apart only)
  | ['cap', string, string?, Tone?] // a caption
  | ['nocap', string?]
  | ['whole', string?]

/** Walk through console captures, step by step on the narration's cues. */
export async function walk(s: Stage, steps: Step[], title = 'nixos-manager') {
  if (![...s.els].some((e) => e.name.startsWith('λ'))) header(s)
  let pic: Pic | null = ([...s.els].find((e) => e.name.startsWith('pic:')) as Pic) ?? null
  let marks: El[] = []
  const unmark = () => (marks.forEach((m) => m.remove()), (marks = []))
  for (const st of steps) {
    const cue = st[2] as string | undefined
    if (cue) await s.until(cue, 2)
    if (st[0] === 'show') {
      unmark()
      pic?.remove()
      pic = s.pic(PIC.x, PIC.y, PIC.w, PIC.h, st[1], { title })
      await s.wait(2)
    } else if (st[0] === 'look') {
      unmark()
      pic!.look(st[1], st[3] ?? 1.25)
      await s.wait(7)
      if (typeof st[1] === 'string') marks.push(pic!.mark(st[1]))
    } else if (st[0] === 'mark' || st[0] === 'mark+') {
      if (st[0] === 'mark') unmark()
      marks.push(pic!.mark(st[1], st[3] ?? 'warn'))
    } else if (st[0] === 'cap') {
      caption(s, st[1], (st[3] as Tone) ?? 'accent')
    } else if (st[0] === 'nocap') {
      uncaption(s)
    } else {
      unmark()
      pic!.whole()
    }
  }
}

// ---- What plays before a scene's voice ------------------------------------------------------------------------

/** A chapter card: the part, the number, the title in the pixel face; it holds until the voice starts. */
async function chapterCard(s: Stage, i: Intro) {
  const lines = bigLines(i.title!)
  const h = lines.length * 4 - 1
  let y = Math.floor((ROWS - h) / 2) - 1
  if (i.part) s.type(mid(len(i.part)), y - 4, i.part, 'c-m')
  s.real(mid(2), y - 2, String(i.n).padStart(2, '0'), 'c-acc')
  for (const l of lines) {
    const art = A.big(l)
    s.big(mid(len(art)), y, l)
    y += 4
  }
  s.text(mid(40), y, '━'.repeat(40), 'c-acc')
  await s.tick(Math.round((s.t0 * 1000) / A.TICK) - 3)
  s.clear()
}

/** The title of the video: the snowflake turning on the clock, then the name in the pixel face. */
async function opening(s: Stage) {
  s.buf({ x: mid(41), y: 3, w: 41, h: 18 }, { hot: 'c-acc', globe: 'c-acc2' }, (L, t) => snowflake(L, 20, 8.5, 9, t * 0.06))
  await s.wait(6)
  const a = A.big('NIX, NIXOS')
  const b = A.big('NIXOS-MANAGER')
  s.big(mid(len(a)), 23, 'NIX, NIXOS')
  await s.wait(4)
  s.big(mid(len(b)), 27, 'NIXOS-MANAGER', 'acc')
  await s.wait(5)
  const sub = 'How Nix works, and how to run a fleet of machines with it'
  s.type(mid(sub.length), 32, sub, 'c-m')
  await s.tick(Math.round((s.t0 * 1000) / A.TICK) - 2)
  s.clear()
}

window.film.intros = { chapter: chapterCard, opening }

export { A, COLS, ROWS, El, Pic, Stage, bufs, stars }
