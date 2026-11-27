// The video's recurring pieces, made of the stage's elements: the header, chapter cards, captions, machine
// cards, store paths, code cards and arrows. Sizes are in cells; the stage is COLS x ROWS.
import { A, COLS, ROWS, El, Pic, Stage, bufs, snowflake, stars, type Box, type Intro, type Live } from './film'

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
  // A block cursor after the line once it is typed, blinking on the clock like a terminal's.
  const cur = s.text(b.inner.x + 1 + len(text), b.inner.y, ' ', 'c-acc', b)
  const from = A.clock.t + Math.ceil(len(text) / 4)
  cur.cancel = A.on((t) => void (t >= from && (cur.node.textContent = Math.floor((t - from) / 6) % 2 ? ' ' : '▌')))
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

/**
 * An arrow that draws itself in the way it points, then now and then sends a bright packet along that way:
 * something flows there. idx lists the arrow's cells (indexes into its text) in the direction of flow.
 */
function flow(el: El, idx: number[]) {
  const base = [...(el.node.textContent ?? '')]
  const n = idx.length
  const draw = Math.max(2, Math.ceil(n / 4))
  const period = 42 + ((n * 7) % 17)
  const start = A.clock.t
  const lit: Record<string, string> = { '─': '━', '│': '┃' }
  let shown = ''
  const paint = (html: string) => html !== shown && ((shown = html), (el.node.innerHTML = html))
  el.cancel = A.on((t) => {
    if (!el.node.isConnected) return false
    const k = t - start
    if (k <= draw) {
      const on = new Set(idx.slice(0, Math.round((k / draw) * n)))
      return void paint(base.map((c, i) => (c === '\n' || on.has(i) ? A.esc(c) : ' ')).join(''))
    }
    const at = Math.floor(((k - draw) % period) * 3)
    if (at >= n + 2) return void paint(base.map(A.esc).join(''))
    const hot = new Map([[idx[at], 'c-b'], [idx[at - 1], 'c-acc'], [idx[at - 2], 'c-acc']])
    paint(base.map((c, i) => (hot.has(i) ? `<b class="${hot.get(i)}">${A.esc(lit[c] ?? c)}</b>` : A.esc(c))).join(''))
  })
  return el
}
const cells = (txt: string) => [...txt].flatMap((c, i) => (c === '\n' ? [] : [i]))
export const harrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => {
  const el = s.text(x, y, '─'.repeat(n - 1) + '▶', cls)
  return flow(el, cells(el.node.textContent!))
}
export const varrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => {
  const el = s.text(x, y, '│\n'.repeat(n - 1) + '▼', cls)
  return flow(el, cells(el.node.textContent!))
}
export const larrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => {
  const el = s.text(x, y, '◀' + '─'.repeat(n - 1), cls)
  return flow(el, cells(el.node.textContent!).reverse())
}
export const uarrow = (s: Stage, x: number, y: number, n: number, cls = 'c-line') => {
  const el = s.text(x, y, '▲' + '\n│'.repeat(n - 1), cls)
  return flow(el, cells(el.node.textContent!).reverse())
}

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
  | ['mark', string | Box, string?, Tone?] // outline a recorded box (or a region), instead of the outline before
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
    else s.steps.push(A.clock.t)
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
  s.sfx('transition')
  const reveal = dither('in', 0, ROWS, 9, 'c-acc')
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
  s.type(mid(40), y, '━'.repeat(40), 'c-acc', undefined, 3)
  await reveal.done
  await s.tick(Math.round((s.t0 * 1000) / A.TICK) - 10)
  const hide = dither('out', 0, ROWS, 8, 'c-acc')
  await hide.done
  s.clear()
  hide.remove()
}

/** The title of the video: the snowflake turning on the clock, then the name in the pixel face. */
async function opening(s: Stage) {
  s.sfx('title')
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
  await s.tick(Math.round((s.t0 * 1000) / A.TICK) - 10)
  const hide = dither('out', 0, ROWS, 8)
  await hide.done
  s.clear()
  hide.remove()
}

// ---- Beside every scene: the status line and the way out ------------------------------------------------------

const amb = () => document.getElementById('amb')!

/**
 * A dither band sweeping across rows y0 to y1 in n ticks, left to right: 'out' leaves the background behind it
 * (what was there is gone), 'in' leaves what is there. It sits above the stage, outside the grid.
 */
export function dither(dir: 'in' | 'out', y0: number, y1: number, n = 8, cls = 'c-line') {
  const B = 6
  const box = document.createElement('div')
  box.className = 'dz'
  box.style.top = `calc(${y0} * var(--lh))`
  box.style.height = `calc(${y1 - y0} * var(--lh))`
  const mask = document.createElement('div')
  mask.className = 'dz-mask'
  const band = document.createElement('pre')
  band.className = 'dz-band ' + cls
  band.textContent = Array(y1 - y0).fill(dir === 'out' ? '█▓▓▒░░' : '░░▒▓▓█').join('\n')
  box.append(mask, band)
  amb().append(box)
  const done = new Promise<void>((res) =>
    A.run(n, (p) => {
      const x = Math.round(p * (COLS + B)) - B
      band.style.left = `${x}ch`
      if (dir === 'out') (mask.style.left = '0'), (mask.style.width = `${Math.max(0, x)}ch`)
      else (mask.style.left = `${Math.max(0, x + B)}ch`), (mask.style.width = `${Math.max(0, COLS - x - B)}ch`)
    }, () => {
      if (dir === 'in') box.remove()
      else band.remove()
      res()
    }),
  )
  return { done, remove: () => box.remove() }
}

const clock = (sec: number) => `${String(Math.floor(sec / 60)).padStart(2, '0')}:${String(Math.floor(sec % 60)).padStart(2, '0')}`

/**
 * The status line on the last row, like the console's: the chapter's progress and time, and a meter that moves
 * while the narrator speaks. Outside the grid and the still check: it is the frame, not the picture.
 */
function statusLine(s: Stage, o: Live) {
  const pre = document.createElement('pre')
  pre.className = 'status'
  pre.style.left = '2ch'
  pre.style.top = `calc(${ROWS - 1} * var(--lh))`
  amb().append(pre)
  const from = Math.round((s.t0 * 1000) / A.TICK)
  const meter: number[] = Array(8).fill(0)
  A.on((t) => {
    if (t < from) return void (pre.innerHTML = '')
    const sec = t * A.TICK / 1000
    const speaking = s.words.some((w) => sec >= s.t0 + w.s && sec < s.t0 + w.e)
    meter.shift()
    meter.push(speaking ? 2 + Math.floor(Math.random() * 7) : 0)
    const at = o.at + sec
    const W = 60
    const left = `<span class="c-acc">▶</span> <span class="c-m">${clock(at)} / ${clock(o.len)}</span>  ${A.bar(at / o.len, W)}`
    const right = `<span class="c-acc">${meter.map((v) => '▁▂▃▄▅▆▇█'[Math.min(7, v)]).join('')}</span>`
    pre.innerHTML = left + ' '.repeat(COLS - 4 - 16 - W - 8) + right
  })
}

window.film.onStart = (s, o) => {
  statusLine(s, o)
  // The scene leaves in a dither once its narration is over: the body only while the chapter goes on (the
  // header stays), everything before a new chapter.
  if (o.outro) {
    const y0 = o.outro === 'body' ? 2 : 0
    A.on((t) => {
      if (t !== o.end - 9) return
      dither('out', y0, ROWS, 8)
      return false
    })
  }
}

window.film.intros = { chapter: chapterCard, opening }

export { A, COLS, ROWS, El, Pic, Stage, bufs, stars }
