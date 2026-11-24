// Cellwork: the clock, the frames and every glyph animation of the console. Everything on screen is text
// on one grid, and everything that moves steps on the one clock below.

import { countAt } from './motion'

const hasDOM = typeof document !== 'undefined'
const query = typeof location !== 'undefined' ? new URLSearchParams(location.search) : new URLSearchParams()

/** Motion is off: every animation shows its last frame at once (the reduced-motion setting, or `?still`). */
export const still = query.has('still') || (typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches)

// ---- The one clock ----------------------------------------------------------------------------------------

/** One beat of the clock, in ms: 12.5 steps a second for the whole screen. */
export const TICK = 80
type Sub = (t: number) => boolean | void
const subs = new Set<Sub>()
export const clock = { t: 0 }

/** Call fn on every tick until it returns false; returns the unsubscribe. */
export function on(fn: Sub): () => void {
  subs.add(fn)
  return () => subs.delete(fn)
}

/** Advance the clock one tick (the interval below does this; tests call it by hand). */
export function step() {
  clock.t++
  // The glyph spinners and dithers are CSS that reads this phase, so they need no subscription of their own.
  if (hasDOM) document.documentElement.dataset.t8 = String(clock.t % 8)
  for (const fn of [...subs]) {
    try {
      if (fn(clock.t) === false) subs.delete(fn)
    } catch (e) {
      subs.delete(fn)
      console.error(e)
    }
  }
}
if (hasDOM && !still && !query.has('manual')) setInterval(() => document.hidden || step(), TICK)

/** Run fn(progress 0..1) over a number of ticks, then done(). Without motion it jumps to 1. Returns a cancel. */
export function run(ticks: number, fn: (p: number) => void, done?: () => void): () => void {
  if (still || ticks < 1) {
    fn(1)
    done?.()
    return () => {}
  }
  let i = 0
  fn(0)
  return on(() => {
    fn(Math.min(1, ++i / ticks))
    if (i >= ticks) {
      done?.()
      return false
    }
  })
}

// ---- The cell ---------------------------------------------------------------------------------------------

/** The size of one cell in px: 1ch wide, one --lh row tall. */
export const cell = { cw: 8.43, lh: 17 }

export function measure() {
  if (!hasDOM || !document.body) return
  const probe = document.createElement('span')
  probe.textContent = '0'.repeat(100)
  probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre'
  document.body.append(probe)
  cell.cw = probe.getBoundingClientRect().width / 100 || cell.cw
  cell.lh = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--lh')) || cell.lh
  probe.remove()
}

export const esc = (s: unknown) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)

// ---- Frames -----------------------------------------------------------------------------------------------

/** The weight of a frame is its rank: round a card, square data, double a modal, heavy hovered or a drawer, dash pending. */
export type FrameKind = 'round' | 'square' | 'double' | 'heavy' | 'dash'
// tl, horizontal, tr, vertical, bl, br
export const SETS: Record<FrameKind, string> = { round: '╭─╮│╰╯', square: '┌─┐│└┘', double: '╔═╗║╚╝', heavy: '┏━┓┃┗┛', dash: '┌┄┐┆└┘' }

export type FrameLabels = { title?: string; tag?: string; foot?: string; shadow?: boolean }

/**
 * The glyphs of a frame cols x rows as HTML lines: title and tag sit on the top line, the foot on the bottom one.
 * p < 1 strokes it in from the top-left corner (a cell shows once the wave reaches it).
 */
export function frameHTML(cols: number, rows: number, kind: FrameKind, o: FrameLabels = {}, p = 1): string {
  cols = Math.max(4, cols)
  rows = Math.max(2, rows)
  const [tl, h, tr, v, bl, br] = SETS[kind] ?? SETS.round
  const put = (line: string[], at: number, text: string): [number, number] => {
    for (let i = 0; i < text.length && at + i < cols - 2; i++) line[at + i] = text[i]
    return [at, Math.min(at + text.length, cols - 2)]
  }
  const top = [tl, ...h.repeat(cols - 2), tr]
  const bot = [bl, ...h.repeat(cols - 2), br]
  const marks: { top: [string, [number, number]][]; bot: [string, [number, number]][] } = { top: [], bot: [] }
  if (o.title) marks.top.push(['b', put(top, 2, ' ' + o.title + ' ')])
  if (o.tag) marks.top.push(['i', put(top, Math.max(2, cols - 4 - o.tag.length), ' ' + o.tag + ' ')])
  if (o.foot) marks.bot.push(['i', put(bot, Math.max(2, cols - 4 - o.foot.length), ' ' + o.foot + ' ')])
  const reach = p >= 1 ? Infinity : p * (cols + rows * 2)
  const html = (line: string[], y: number, ms: [string, [number, number]][]) => {
    let out = ''
    for (let x = 0; x < line.length; x++) {
      const m = ms.find(([, [a, b]]) => a === x && b > a)
      if (m && x + 2 * y <= reach) {
        out += `<${m[0]}>${esc(line.slice(m[1][0], m[1][1]).join(''))}</${m[0]}>`
        x = m[1][1] - 1
      } else out += x + 2 * y <= reach ? esc(line[x]) : ' '
    }
    return out
  }
  const shade = o.shadow && p >= 1 ? '<s>░</s>' : ''
  const lines = [html(top, 0, marks.top)]
  const gap = ' '.repeat(cols - 2)
  for (let y = 1; y < rows - 1; y++) lines.push((2 * y <= reach ? v : ' ') + gap + (cols - 1 + 2 * y <= reach ? v : ' ') + shade)
  lines.push(html(bot, rows - 1, marks.bot) + shade)
  if (shade) lines.push(' <s>' + '░'.repeat(cols) + '</s>')
  return lines.join('\n')
}

export type FrameOptions = FrameLabels & {
  kind?: FrameKind
  /** Stroke the frame in when it appears. */
  animate?: boolean
  /** Hover and keyboard focus lift it: the frame turns heavy. */
  lift?: boolean
}

type Framed = { pre: HTMLPreElement; o: FrameOptions; hover: boolean; busy: boolean }
const framed = new Map<HTMLElement, Framed>()
const resized =
  hasDOM && typeof ResizeObserver !== 'undefined' ? new ResizeObserver((es) => es.forEach((e) => framed.get(e.target as HTMLElement)?.busy || draw(e.target as HTMLElement))) : null

function draw(el: HTMLElement, p = 1) {
  const st = framed.get(el)
  if (!st) return
  // Svelte may empty the element (an each block going to zero): put the glyphs back.
  if (st.pre.parentNode !== el) el.append(st.pre)
  const w = el.offsetWidth
  const h = el.offsetHeight
  if (w < cell.cw * 4) {
    st.pre.textContent = '' // hidden: nothing to frame
    return
  }
  // offsetWidth is whole pixels: a box N cells wide can measure up to 1px short of N * cw.
  const cols = Math.floor(w / cell.cw + 0.15)
  const rows = Math.round(h / cell.lh)
  st.pre.innerHTML = frameHTML(cols, rows, st.hover ? 'heavy' : (st.o.kind ?? 'round'), st.o, p)
}

/** Redraw every frame on screen (after the font loads and the cell is measured again). */
export const redrawFrames = () => framed.forEach((_, el) => draw(el))

/** Svelte action: draw a glyph frame over the element. The element keeps one row and two columns free on each side. */
export function frame(node: HTMLElement, opts: FrameOptions | FrameKind = {}) {
  let o: FrameOptions = typeof opts === 'string' ? { kind: opts } : opts
  const pre = document.createElement('pre')
  pre.className = 'fr'
  pre.setAttribute('aria-hidden', 'true')
  node.append(pre)
  const st: Framed = { pre, o, hover: false, busy: false }
  framed.set(node, st)
  node.dataset.frame = o.kind ?? 'round'
  resized?.observe(node)
  const lift = (v: boolean) => () => {
    if (!st.o.lift || st.hover === v) return
    st.hover = v
    if (!st.busy) draw(node)
  }
  const enter = lift(true)
  const leave = lift(false)
  const out = (e: FocusEvent) => node.contains(e.relatedTarget as Node) || leave()
  node.addEventListener('mouseenter', enter)
  node.addEventListener('mouseleave', leave)
  node.addEventListener('focusin', enter)
  node.addEventListener('focusout', out)
  // Svelte can drop the glyphs when it empties the element; observe that cheaply.
  const kids = new MutationObserver(() => pre.parentNode !== node && draw(node))
  kids.observe(node, { childList: true })
  let cancel = () => {}
  if (o.animate && !still) {
    st.busy = true
    cancel = run(
      5,
      (p) => draw(node, p),
      () => {
        st.busy = false
        draw(node)
      },
    )
  } else queueMicrotask(() => draw(node))
  return {
    update(next: FrameOptions | FrameKind = {}) {
      o = typeof next === 'string' ? { kind: next } : next
      st.o = o
      node.dataset.frame = o.kind ?? 'round'
      if (!st.busy) draw(node)
    },
    destroy() {
      cancel()
      resized?.unobserve(node)
      kids.disconnect()
      framed.delete(node)
      node.removeEventListener('mouseenter', enter)
      node.removeEventListener('mouseleave', leave)
      node.removeEventListener('focusin', enter)
      node.removeEventListener('focusout', out)
      pre.remove()
    },
  }
}

// ---- Text -------------------------------------------------------------------------------------------------

// Text is realised the way Nix realises a store path, out of its hash: unresolved characters show in Nix's
// base-32 alphabet. Every glyph is one cell wide, so nothing reflows while it happens.
export const B32 = '0123456789abcdfghijklmnpqrsvwxyz'
const rnd = (n: number) => Math.floor(Math.random() * n)

/** s with its first p (0..1) resolved and the rest in base-32; whitespace stays put. */
export function scramble(s: string, p: number): string {
  const keep = Math.floor(s.length * Math.max(0, Math.min(1, p)))
  let out = s.slice(0, keep)
  for (let i = keep; i < s.length; i++) out += /\s/.test(s[i]) ? s[i] : B32[rnd(32)]
  return out
}

const SWEEP_SKIP = '.fr,.nofx,pre,textarea,script,style,.big,.bar,.skel,.cm-editor,.svelte-flow__edges,input,select,[contenteditable]'
type Swept = Text & { _final?: string | null; _shown?: string }

/**
 * Repaint everything under root, top to bottom, like a screen redrawn over a serial line: dots, then base-32,
 * then the text. Text that changes underneath (Svelte updating it) wins at once.
 */
export function sweep(root: HTMLElement, rowsPerTick = 9) {
  if (still || !hasDOM || !root.isConnected) return
  const tw = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const top = root.getBoundingClientRect().top
  const range = document.createRange()
  const items: { n: Swept; s: string; at: number; done?: boolean }[] = []
  for (let n = tw.nextNode() as Swept | null; n; n = tw.nextNode() as Swept | null) {
    if (!n.data.trim() || !n.parentElement?.getClientRects().length || n.parentElement.closest(SWEEP_SKIP)) continue
    range.selectNodeContents(n)
    const y = range.getBoundingClientRect().top
    if (y > innerHeight + 100) continue
    // A node already mid-sweep keeps its real text, not the placeholder it shows right now.
    if (n._final == null || n.data !== n._shown) n._final = n.data
    items.push({ n, s: n._final, at: Math.max(0, Math.floor((y - top) / cell.lh / rowsPerTick)) })
  }
  const show = (n: Swept, s: string) => {
    n.data = s
    n._shown = s
  }
  for (const it of items) show(it.n, it.s.replace(/\S/g, '·'))
  let k = 0
  on(() => {
    k++
    let live = false
    for (const it of items) {
      if (it.done) continue
      const d = k - it.at
      if (it.n.data !== it.n._shown || d >= 3) {
        // Someone else wrote the node meanwhile: theirs is the real text.
        if (it.n.data === it.n._shown) it.n.data = it.s
        it.n._final = null
        it.done = true
      } else if (d > 0) {
        show(it.n, scramble(it.s, d / 3))
        live = true
      } else live = true
    }
    return live
  })
}

/** Type text into el, a burst of characters per tick. */
export const type = (el: HTMLElement, text: string, perTick = 3, done?: () => void) =>
  run(Math.ceil(text.length / perTick), (p) => (el.textContent = text.slice(0, Math.round(text.length * p))), done)

/** Svelte action: the text is typed in whenever it changes. */
export function typed(el: HTMLElement, text: string) {
  let cancel = type(el, text, 2)
  return {
    update(next: string) {
      cancel()
      cancel = type(el, next, 2)
    },
    destroy: () => cancel(),
  }
}

/** Svelte action: the text resolves out of base-32 whenever it changes. */
export function realised(el: HTMLElement, text: string) {
  let cancel = run(5, (p) => (el.textContent = scramble(text, p)))
  return {
    update(next: string) {
      cancel()
      cancel = run(5, (p) => (el.textContent = scramble(next, p)))
    },
    destroy: () => cancel(),
  }
}

/** Multi-line art wipes in left to right behind a ▓▒░ edge. */
export function wipe(el: HTMLElement, text: string, ticks = 7) {
  const lines = text.split('\n')
  const w = Math.max(...lines.map((l) => l.length))
  return run(ticks, (p) => {
    const c = Math.round(w * p)
    el.textContent = p >= 1 ? text : lines.map((l) => l.slice(0, c) + (l.slice(c, c + 3).trim() ? '▓▒░'.slice(0, w - c) : '')).join('\n')
  })
}

// ---- The pixel face ---------------------------------------------------------------------------------------

// Five pixel rows per glyph, printed as three text rows of half blocks.
const FONT: Record<string, string> = {
  A: '010 101 111 101 101', B: '110 101 110 101 110', C: '011 100 100 100 011', D: '110 101 101 101 110',
  E: '111 100 110 100 111', F: '111 100 110 100 100', G: '011 100 101 101 011', H: '101 101 111 101 101',
  I: '111 010 010 010 111', J: '001 001 001 101 010', K: '101 101 110 101 101', L: '100 100 100 100 111',
  M: '10001 11011 10101 10001 10001', N: '1001 1101 1011 1001 1001', O: '010 101 101 101 010',
  P: '110 101 110 100 100', Q: '010 101 101 111 011', R: '110 101 110 101 101', S: '011 100 010 001 110',
  T: '111 010 010 010 010', U: '101 101 101 101 111', V: '101 101 101 101 010',
  W: '10001 10001 10101 11011 10001', X: '101 101 010 101 101', Y: '101 101 010 010 010',
  Z: '111 001 010 100 111', 0: '111 101 101 101 111', 1: '010 110 010 010 111', 2: '110 001 010 100 111',
  3: '110 001 010 001 110', 4: '101 101 111 001 001', 5: '111 100 110 001 110', 6: '011 100 111 101 111',
  7: '111 001 010 010 010', 8: '111 101 111 101 111', 9: '111 101 111 001 110', ' ': '00 00 00 00 00',
  '.': '0 0 0 0 1', ',': '00 00 00 01 10', '-': '000 000 111 000 000', '/': '001 001 010 100 100',
  ':': '0 1 0 1 0', '!': '1 1 1 0 1', '?': '110 001 010 000 010', '+': '000 010 111 010 000', _: '000 000 000 000 111',
  '&': '010 101 010 101 011', "'": '1 1 0 0 0', '>': '100 010 001 010 100', '%': '101 001 010 100 101',
  '@': '111 101 101 100 011',
}

/** text in the pixel face: three rows of ▀▄█ (scale 2: five rows of ██, twice as wide). */
export function big(text: string, scale = 1): string {
  const rows = ['', '', '', '', '']
  for (const ch of String(text).toUpperCase()) {
    const g = (FONT[ch] ?? FONT['?']).split(' ')
    for (let y = 0; y < 5; y++) rows[y] += g[y] + '0'
  }
  if (scale > 1) return rows.map((r) => [...r].map((b) => (b === '1' ? '██' : '  ')).join('')).join('\n')
  const out: string[] = []
  for (let y = 0; y < 5; y += 2) {
    let line = ''
    for (let x = 0; x < rows[0].length; x++) {
      const a = rows[y][x] === '1'
      const b = y + 1 < 5 && rows[y + 1][x] === '1'
      line += a && b ? '█' : a ? '▀' : b ? '▄' : ' '
    }
    out.push(line)
  }
  return out.join('\n')
}

/** Svelte action: an element showing big(text); it wipes in when it appears. */
export function bigText(el: HTMLElement, text: string) {
  el.setAttribute('aria-label', text)
  let cancel = wipe(el, big(text))
  return {
    update(next: string) {
      el.setAttribute('aria-label', next)
      cancel()
      el.textContent = big(next)
    },
    destroy: () => cancel(),
  }
}

/** Svelte action: a whole number in the pixel face that rolls up like an odometer, from 0 and then from the old value. */
// The digits keep their width while they roll, so nothing beside them moves.
export function odometer(el: HTMLElement, n: number) {
  let shown = 0
  let cancel = () => {}
  const roll = (to: number) => {
    cancel()
    el.setAttribute('aria-label', String(to))
    const from = shown
    const width = String(Math.max(Math.abs(from), Math.abs(to))).length
    cancel = run(from === to || !Number.isFinite(to) ? 0 : 8, (p) => {
      shown = countAt(from, to, p)
      el.textContent = big(p >= 1 ? String(to) : String(shown).padStart(width, '0'))
    })
  }
  roll(n)
  return { update: roll, destroy: () => cancel() }
}

/** Svelte action: reverse video for two ticks whenever key changes (a count that moved, a badge that appeared). */
export function flash(el: HTMLElement, key: unknown) {
  let last = key
  let cancel = () => {}
  const go = () => {
    cancel()
    el.classList.add('flash')
    cancel = run(2, () => {}, () => el.classList.remove('flash'))
  }
  go()
  return {
    update(next: unknown) {
      if (next !== last) (last = next), go()
    },
    destroy: () => cancel(),
  }
}

// ---- Meters -----------------------------------------------------------------------------------------------

/** A meter w cells wide: full blocks, one eighth block for the fraction, a faint ░ track (in <s>). sheen >= 0 runs a ▓▒░ edge through it. */
export function bar(v: number, w = 20, sheen = -1): string {
  v = Math.max(0, Math.min(1, Number.isFinite(v) ? v : 0))
  const cells = v * w
  const full = Math.floor(cells)
  const part = ' ▏▎▍▌▋▊▉'[Math.floor((cells - full) * 8)].trim()
  let fill = '█'.repeat(full)
  if (sheen >= 0 && full > 3) {
    const at = sheen % (full + 6)
    fill = [...fill].map((c, i) => ({ 0: '░', 1: '▒', 2: '▓' })[at - i] ?? c).join('')
  }
  const rest = w - full - (part ? 1 : 0)
  return fill + part + '<s>' + '░'.repeat(Math.max(0, rest)) + '</s>'
}

/** A sparkline: one ▁..█ per value. */
export const spark = (values: number[]) => {
  const max = Math.max(...values, 1)
  return values.map((v) => '▁▂▃▄▅▆▇█'[Math.min(7, Math.floor((v / max) * 8))]).join('')
}

/** Spinner frames; the CSS classes .spin and .dot.busy step through the same ones on the clock's phase. */
export const SPIN = { tee: '┤┘┴└├┌┬┐', arc: '◜◝◞◟', dot: '◐◓◑◒', ping: '·∙•●•∙' }

/**
 * Svelte action on a tab strip: the active tab (aria-current, aria-selected or .on) stands on a heavy stretch of
 * the ─ rule under the strip, and the stretch slides to a newly picked tab cell by cell.
 */
export function ink(tabs: HTMLElement, _key?: unknown) {
  const pre = document.createElement('pre')
  pre.className = 'ink'
  pre.setAttribute('aria-hidden', 'true')
  tabs.append(pre)
  let at: number | null = null
  let len: number | null = null
  let cancel = () => {}
  const paint = (animate: boolean) => {
    if (pre.parentNode !== tabs) tabs.append(pre)
    const cur = tabs.querySelector<HTMLElement>('[aria-current="page"],[aria-selected="true"],.on')
    const cols = Math.ceil(tabs.offsetWidth / cell.cw)
    if (!cur) {
      pre.textContent = '─'.repeat(cols)
      return
    }
    const base = tabs.getBoundingClientRect()
    const r = cur.getBoundingClientRect()
    const to = Math.round((r.left - base.left) / cell.cw)
    const l2 = Math.round(r.width / cell.cw)
    const from = animate && at != null ? at : to
    const fromLen = animate && len != null ? len : l2
    at = to
    len = l2
    cancel()
    cancel = run(from === to ? 0 : 4, (p) => {
      const e = 1 - (1 - p) ** 2
      const a = Math.round(from + (to - from) * e)
      const l = Math.round(fromLen + (l2 - fromLen) * e)
      pre.innerHTML = '─'.repeat(Math.max(0, a)) + '<b>' + '━'.repeat(Math.max(0, l)) + '</b>' + '─'.repeat(Math.max(0, cols - a - l))
    })
  }
  const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(() => paint(false)) : null
  ro?.observe(tabs)
  queueMicrotask(() => paint(false))
  return {
    update: () => queueMicrotask(() => paint(true)),
    destroy() {
      cancel()
      ro?.disconnect()
      pre.remove()
    },
  }
}

// ---- Scenes -----------------------------------------------------------------------------------------------

// Line bits: 1 up, 2 right, 4 down, 8 left. Lines that meet or cross pick the glyph that joins them.
const JOIN = ' │─╰││╭├─╯─┴╮┤┬┼'

/** A character buffer with a line layer: path segments add compass bits, so lines that meet join themselves. */
export class Buf {
  ch: string[][]
  bits: Uint8Array[]
  constructor(
    public cols: number,
    public rows: number,
  ) {
    this.ch = Array.from({ length: rows }, () => Array<string>(cols).fill(' '))
    this.bits = Array.from({ length: rows }, () => new Uint8Array(cols))
  }
  set(x: number, y: number, c: string) {
    x = Math.round(x)
    y = Math.round(y)
    if (y >= 0 && y < this.rows && x >= 0 && x < this.cols) this.ch[y][x] = c
  }
  get(x: number, y: number) {
    return this.ch[y]?.[x]
  }
  text(x: number, y: number, s: string) {
    for (let i = 0; i < s.length; i++) this.set(x + i, y, s[i])
  }
  bit(x: number, y: number, b: number) {
    if (y >= 0 && y < this.rows && x >= 0 && x < this.cols) this.bits[y][x] |= b
  }
  /** An orthogonal polyline through [x, y] points; returns the cells it passes, in order. */
  path(points: [number, number][]): [number, number][] {
    const cells: [number, number][] = []
    for (let i = 0; i < points.length - 1; i++) {
      let [x, y] = points[i]
      const [x2, y2] = points[i + 1]
      const dx = Math.sign(x2 - x)
      const dy = Math.sign(y2 - y)
      while (x !== x2 || y !== y2) {
        this.bit(x, y, dx > 0 ? 2 : dx < 0 ? 8 : dy > 0 ? 4 : 1)
        cells.push([x, y])
        x += dx
        y += dy
        this.bit(x, y, dx > 0 ? 8 : dx < 0 ? 2 : dy > 0 ? 1 : 4)
      }
    }
    cells.push(points[points.length - 1])
    return cells
  }
  box(x: number, y: number, w: number, h: number, kind: FrameKind = 'round') {
    const [tl, hz, tr, v, bl, br] = SETS[kind]
    this.text(x, y, tl + hz.repeat(w - 2) + tr)
    for (let i = 1; i < h - 1; i++) {
      this.set(x, y + i, v)
      this.set(x + w - 1, y + i, v)
    }
    this.text(x, y + h - 1, bl + hz.repeat(w - 2) + br)
  }
  toString() {
    return this.ch.map((row, y) => row.map((c, x) => (c === ' ' && this.bits[y][x] ? JOIN[this.bits[y][x]] : c)).join('')).join('\n')
  }
}

/** Repaint a <pre> on every tick until it leaves the page. fn(t) returns its text. Returns the painter. */
export function scene(pre: HTMLElement, fn: (t: number) => string | void) {
  let t = 0
  const paint = () => {
    const out = fn(t++)
    if (typeof out === 'string') pre.textContent = out
  }
  paint()
  const stop = still ? () => {} : on(() => (pre.isConnected ? paint() : false))
  return Object.assign(paint, { stop })
}

/** Fill a scrim with the ░ dither it shows instead of a blur. */
export const dither = (el: HTMLElement) => (el.textContent = '░ '.repeat(12000))

if (hasDOM) {
  measure()
  document.fonts?.ready.then(() => {
    measure()
    redrawFrames()
  })
}
