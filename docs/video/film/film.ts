// The stage the video's scenes are drawn on: the console's Cellwork (web/src/lib/ascii.ts) on one grid of
// COLS x ROWS cells. Every element sits on whole cells, so the stage can check that nothing leaves the grid and
// that no two elements cover the same cells: film.py fails a scene that breaks either rule.
import * as A from '../../../web/src/lib/ascii'
import { bufs, snowflake, stars, trace } from '../../../web/src/intro/scenes'

export { A, bufs, snowflake, stars, trace }

export const COLS = 145
export const ROWS = 40

export type Rect = { x: number; y: number; w: number; h: number }
type Word = { w: string; s: number; e: number }
export type Box = [number, number, number, number]
type ShotMeta = { size: [number, number]; boxes: Record<string, Box> }

declare global {
  interface Window {
    film: Film
    SHOTS: Record<string, ShotMeta>
  }
}

// Deterministic glyph noise (scramble and friends use Math.random): the same scene renders the same frames.
let seed = 1
Math.random = () => {
  seed = (seed * 16807) % 2147483647
  return (seed - 1) / 2147483646
}

const norm = (w: string) => w.toLowerCase().replace(/[^a-z0-9]/g, '')
/** Words too common to highlight when the narration says them. */
const STOP = new Set(
  ('this that with from have your they them then than what when each every into only also just more some here there which will would ' +
    'could does done make made like over same other their these those been were where about after before again once because while ' +
    'still even very much many most such both first next last itself always never another without within between through under until ' +
    'whole thing things something anything nothing everything really need needs want wants used uses using says show shows shown open ' +
    'opens click clicks button page pages machine machines nixos console change changes file files part line lines name names way ' +
    'time times keep keeps take takes give gives look looks right left back down said tells tell call called comes come goes set sets').split(' '),
)
const tokens = (s: string) => s.toLowerCase().split(/[^a-z0-9]+/)
const ease = (p: number) => 1 - (1 - p) ** 3

export class El {
  cancel: () => void = () => {}
  /** A text element's whole text, once typed: a line of it can be lit. */
  full = ''
  constructor(
    public stage: Stage,
    public node: HTMLElement,
    public rect: Rect,
    public name: string,
    public parent?: El,
  ) {}
  /** The cells inside a frame: one row and two columns in from each side (the console's rule). */
  get inner(): Rect {
    const { x, y, w, h } = this.rect
    return { x: x + 2, y: y + 1, w: w - 4, h: h - 2 }
  }
  remove() {
    this.cancel()
    this.node.remove()
    this.stage.els.delete(this)
    for (const e of [...this.stage.els]) if (e.parent === this) e.remove()
  }
}

export type FrameOpts = A.FrameLabels & { kind?: A.FrameKind; tone?: 'accent' | 'ok' | 'warn' | 'err'; animate?: boolean }

/** A framed console capture whose view can move to a part of the screenshot. */
export class Pic extends El {
  img!: HTMLImageElement
  win!: HTMLElement
  overlays: { node: HTMLElement; box: Box; pad: number }[] = []
  lit = new Map<string, number>() // box key -> tick of its last narration highlight
  punched: Box | null = null // the view before an idle punch-in
  /** When the picture has stood still a while: step in a little towards what is in focus, or back out. */
  punch() {
    if (this.punched) {
      const back = this.punched
      this.punched = null
      return this.look(back, 1, 8, true)
    }
    const v = this.view
    const o = this.overlays[this.overlays.length - 1]
    const [fx, fy] = o ? [o.box[0] + o.box[2] / 2, o.box[1] + o.box[3] / 2] : [v[0] + v[2] / 2, v[1] + v[3] / 2]
    const w = v[2] * 0.88
    const h = v[3] * 0.88
    const x = Math.min(Math.max(fx - w / 2, v[0]), v[0] + v[2] - w)
    const y = Math.min(Math.max(fy - h / 2, v[1]), v[1] + v[3] - h)
    this.look([x, y, w, h], 1, 8, true)
    this.punched = v
  }
  anim: () => void = () => {}
  shot = ''
  view: Box = [0, 0, 1, 1]
  meta(): ShotMeta {
    return window.SHOTS[this.shot]
  }
  /** A named box of the screenshot, [x, y, w, h] in its pixels, or fractions of it. */
  box(key: string | Box): Box {
    const m = this.meta()
    if (typeof key === 'string') {
      const b = m.boxes[key]
      if (!b) throw new Error(`${this.stage.sid}: ${this.shot} has no box ${key}`)
      // A box of a few pixels (an anchor, an empty node) would zoom into a blur.
      if (b[2] < 16 || b[3] < 16) this.stage.problems.add(`box too small: ${this.shot} ${key} ${b[2]}x${b[3]}`)
      return b
    }
    const [x, y, w, h] = key
    return Math.max(x, y, w, h) <= 1 ? [x * m.size[0], y * m.size[1], w * m.size[0], h * m.size[1]] : key
  }
  /** The view, in screenshot pixels, that shows region with a margin at the frame's aspect. */
  fit(region: Box, margin = 1.15): Box {
    const [W, H] = this.meta().size
    const r = this.inner
    const aspect = (r.w * A.cell.cw) / (r.h * A.cell.lh)
    let [x, y, w, h] = region
    let cw = w * margin
    let ch = h * margin
    if (cw / ch > aspect) ch = cw / aspect
    else cw = ch * aspect
    if (cw > W) (cw = W), (ch = W / aspect)
    if (ch > H) (ch = H), (cw = H * aspect)
    const cx = Math.min(Math.max(x + w / 2, cw / 2), W - cw / 2)
    const cy = Math.min(Math.max(y + h / 2, ch / 2), H - ch / 2)
    let [vx, vy] = [cx - cw / 2, cy - ch / 2]
    // Never slice the navigation rail or the top bar in half: a view that would cut through one starts just
    // past it instead, as long as the region still fits.
    const boxes = Object.entries(this.meta().boxes)
    const rail = Math.max(0, ...boxes.filter(([k]) => k.startsWith('nav:')).map(([, b]) => b[0] + b[2]))
    const bar = Math.max(0, ...boxes.filter(([k, b]) => k.startsWith('btn:') && b[1] < 80 && b[0] > rail).map(([, b]) => b[1] + b[3]))
    if (rail && vx > 4 && vx < rail + 8 && x >= rail && x + w <= rail + 8 + cw) vx = Math.min(rail + 8, W - cw)
    if (bar && vy > 4 && vy < bar + 6 && y >= bar && y + h <= bar + 6 + ch) vy = Math.min(bar + 6, H - ch)
    return [vx, vy, cw, ch]
  }
  paint(v: Box) {
    this.view = v
    const r = this.inner
    const s = (r.w * A.cell.cw) / v[2]
    this.img.style.width = `${this.meta().size[0] * s}px`
    this.img.style.transform = `translate(${-v[0] * s}px, ${-v[1] * s}px)`
    for (const o of this.overlays) this.put(o)
  }
  /** Place an overlay on its region in the current view, in pixels, kept inside the window. */
  private put(o: { node: HTMLElement; box: Box; pad: number }) {
    const r = this.inner
    const v = this.view
    const s = (r.w * A.cell.cw) / v[2]
    const [x, y, w, h] = o.box
    const l = Math.max(0, (x - v[0]) * s - o.pad)
    const t = Math.max(0, (y - v[1]) * s - o.pad)
    const rr = Math.min(r.w * A.cell.cw, (x + w - v[0]) * s + o.pad)
    const b = Math.min(r.h * A.cell.lh, (y + h - v[1]) * s + o.pad)
    Object.assign(o.node.style, { left: `${l}px`, top: `${t}px`, width: `${Math.max(0, rr - l)}px`, height: `${Math.max(0, b - t)}px` })
  }
  /**
   * An outline round a region of the screenshot, exact to the pixel so it never crosses the text beside it,
   * drawn in from its top-left corner over a few ticks. Not a grid element: it lives inside the capture.
   */
  overlay(key: string | Box, cls: string, pad = 6, ticks = 4): El {
    const node = document.createElement('div')
    node.className = 'hl ' + cls
    this.win.append(node)
    const o = { node, box: this.box(key), pad }
    this.overlays.push(o)
    this.put(o)
    const el = new El(this.stage, node, this.rect, `hl:${key}`, this)
    const stop = A.run(ticks, (p) => (node.style.clipPath = p >= 1 ? '' : `inset(0 ${100 - p * 100}% ${100 - p * 100}% 0)`))
    el.cancel = () => (stop(), (this.overlays = this.overlays.filter((x) => x !== o)))
    return el
  }
  /** Step the view to region over a few ticks (the console's motion: stepped, never smooth). */
  look(key: string | Box, margin = 1.15, ticks = 6, quiet = false) {
    const to = this.fit(this.box(key), margin)
    const from = this.view
    if (!quiet) (this.stage.sfx('zoom'), (this.punched = null))
    this.anim()
    this.anim = A.run(ticks, (p) => {
      const e = ease(p)
      this.paint(from.map((v, i) => v + (to[i] - v) * e) as Box)
    })
  }
  whole() {
    this.look([0, 0, 1, 1], 1)
  }
  /** The cells inside the frame that cover a region of the screenshot in the current view. */
  cells(key: string | Box, pad = 1): Rect {
    const [x, y, w, h] = this.box(key)
    const v = this.view
    const r = this.inner
    const s = (r.w * A.cell.cw) / v[2]
    const x0 = Math.floor(((x - v[0]) * s) / A.cell.cw) - pad
    const y0 = Math.floor(((y - v[1]) * s) / A.cell.lh) - pad
    const x1 = Math.ceil(((x + w - v[0]) * s) / A.cell.cw) + pad
    const y1 = Math.ceil(((y + h - v[1]) * s) / A.cell.lh) + pad
    const cx = Math.max(0, Math.min(r.w - 4, x0))
    const cy = Math.max(0, Math.min(r.h - 2, y0))
    return { x: r.x + cx, y: r.y + cy, w: Math.max(4, Math.min(r.w, x1) - cx), h: Math.max(2, Math.min(r.h, y1) - cy) }
  }
  marks: El[] = []
  /** The console's highlight on a region: a heavy outline, exact to the pixel. */
  mark(key: string | Box, tone: FrameOpts['tone'] = 'warn'): El {
    this.stage.sfx('mark')
    const m = this.overlay(key, `hl-${tone}`)
    this.marks.push(m)
    return m
  }
  /** Remove every mark on this capture (the narration's own highlights go by themselves). */
  unmark() {
    this.marks.forEach((m) => m.remove())
    this.marks = []
  }
  /** The screenshot's named boxes as words, to light the one the narration names (buttons, headings, rows). */
  labels(): { key: string; toks: string[] }[] {
    const seen = new Set<string>()
    const out: { key: string; toks: string[] }[] = []
    for (const [key, b] of Object.entries(this.meta().boxes)) {
      if (b[2] < 16 || b[3] < 16) continue
      let toks = key.replace(/^(btn|nav|h|row|sec):/, '').toLowerCase().split(/[^a-z0-9]+/)
      toks = toks.filter((t) => t.length > 1 && !/^\d+$/.test(t) && !/^g[a-z]$/.test(t))
      if (toks.length > 3) toks = toks[0].length >= 4 ? [toks[0]] : []
      if (!toks.length || toks.join('').length < 4 || (toks.length === 1 && STOP.has(toks[0]))) continue
      const id = toks.join(' ')
      if (seen.has(id)) continue
      seen.add(id)
      out.push({ key, toks })
    }
    return out
  }
  /** Light the box whose label the narration just finished saying, if it is in view and not the whole page. */
  follow(words: Word[], i: number, t: number) {
    const v = this.view
    for (const { key, toks } of this.labels()) {
      const n = toks.length
      const whole = i - n + 1 >= 0 && toks.every((tk, k) => words[i - n + 1 + k].w === tk)
      const word = words[i].w.length >= 5 && !STOP.has(words[i].w) && toks.includes(words[i].w)
      if (!whole && !word) continue
      if (t - (this.lit.get(key) ?? -999) < 50) continue
      const [x, y, w, h] = this.box(key)
      const iw = Math.min(x + w, v[0] + v[2]) - Math.max(x, v[0])
      const ih = Math.min(y + h, v[1] + v[3]) - Math.max(y, v[1])
      if (iw <= 0 || ih <= 0 || iw * ih < 0.5 * w * h || w * h > 0.4 * v[2] * v[3]) continue
      this.lit.set(key, t)
      const hl = this.overlay(key, 'hl-follow', 4, 2)
      A.run(16, () => {}, () => hl.remove())
      return true
    }
    return false
  }
}

export class Stage {
  root = document.getElementById('stage')!
  els = new Set<El>()
  problems = new Set<string>()
  sid = ''
  /** The scene's chapter (number, title) and its own title, for the header. */
  info = { n: 0, chapter: '', title: '' }
  words: Word[] = []
  t0 = 0
  cursor = 0
  done = false

  // ---- placing elements ------------------------------------------------------------------------------------
  place(r: Rect, tag: string, name: string, parent?: El, cls = ''): El {
    const node = document.createElement(tag)
    node.className = cls
    node.style.left = `${r.x}ch`
    node.style.top = `calc(${r.y} * var(--lh))`
    node.style.width = `${r.w}ch`
    node.style.height = `calc(${r.h} * var(--lh))`
    this.root.append(node)
    const el = new El(this, node, r, name, parent)
    this.els.add(el)
    return el
  }
  /** Text at a cell, one or more lines; colour class c-acc, c-b, c-m… */
  events: { t: number; k: string }[] = []
  /** A sound cue at this tick; audio.py turns the cues into the video's effects, sparsely. */
  sfx(k: string) {
    this.events.push({ t: A.clock.t, k })
  }
  /** A frame's tone, changed with its cue: ok and err sound, the others only show. */
  tone(el: El, t: 'accent' | 'ok' | 'warn' | 'err') {
    el.node.dataset.tone = t
    if (t === 'ok' || t === 'err') this.sfx(t === 'ok' ? 'ok' : 'fail')
  }
  text(x: number, y: number, s: string, cls = '', parent?: El): El {
    const lines = s.split('\n')
    const el = this.place({ x, y, w: Math.max(...lines.map((l) => [...l].length)), h: lines.length }, 'pre', s.slice(0, 24), parent, cls)
    el.node.textContent = s
    el.full = s
    return el
  }
  /** Text typed in, a burst of characters per tick (ascii.ts `type`). */
  type(x: number, y: number, s: string, cls = '', parent?: El, perTick = 4): El {
    const el = this.text(x, y, s, cls, parent)
    el.cancel = A.type(el.node, s, perTick)
    this.sfx(s.startsWith('✓') ? 'ok' : s.startsWith('✗') ? 'fail' : 'type')
    return el
  }
  /** Text realised out of Nix's base-32 alphabet, as a store path out of its hash (ascii.ts `scramble`). */
  real(x: number, y: number, s: string, cls = '', parent?: El, ticks = 5): El {
    const el = this.text(x, y, s, cls, parent)
    el.cancel = A.run(ticks, (p) => (el.node.textContent = A.scramble(s, p)))
    this.sfx('real')
    return el
  }
  /** HTML text (b, i, s and colour spans) the grid measures by its plain text. */
  html(x: number, y: number, html: string, cls = '', parent?: El): El {
    const plain = html.replace(/<[^>]+>/g, '').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&')
    const el = this.text(x, y, plain, cls, parent)
    el.node.innerHTML = html
    return el
  }
  /** A glyph frame (ascii.ts `frame`), stroked in from its top-left corner. */
  box(x: number, y: number, w: number, h: number, o: FrameOpts = {}, parent?: El): El {
    const el = this.place({ x, y, w, h }, 'div', `box:${o.title ?? ''}`, parent)
    // frameHTML cuts labels at the corner: a title (and tag) must fit on the top line whole.
    const label = (o.title ? o.title.length + 4 : 2) + (o.tag ? o.tag.length + 2 : 0)
    if (label > w - 2) this.problems.add(`label cut: box ${o.title ?? ''} ${o.tag ?? ''} needs ${label + 2} cols, has ${w}`)
    if (o.tone) el.node.dataset.tone = o.tone
    const act = A.frame(el.node, { kind: o.kind ?? 'round', title: o.title, tag: o.tag, foot: o.foot, animate: o.animate ?? true })
    if (o.animate ?? true) this.sfx('frame')
    el.cancel = () => act.destroy()
    return el
  }
  /** The pixel face (ascii.ts `big`), wiped in. */
  big(x: number, y: number, s: string, cls = '', parent?: El): El {
    const art = A.big(s)
    const el = this.text(x, y, art, 'big ' + cls, parent)
    el.name = `big:${s}`
    el.cancel = A.wipe(el.node, art)
    this.sfx('big')
    return el
  }
  /** Lines of Buf layers repainted every tick (ascii.ts `scene`), one colour per layer. */
  buf<K extends string>(r: Rect, layers: Record<K, string>, paint: (L: Record<K, A.Buf>, t: number) => void, parent?: El): El {
    const el = this.place(r, 'div', 'buf', parent)
    const names = Object.keys(layers) as K[]
    const pres = names.map((n) => {
      const p = document.createElement('pre')
      p.className = 'in ' + layers[n]
      el.node.append(p)
      return p
    })
    const painter = A.scene(pres[0], (t) => {
      const L = bufs(names, r.w, r.h)
      paint(L, t)
      names.forEach((n, i) => i && (pres[i].textContent = L[n].toString()))
      return L[names[0]].toString()
    })
    el.cancel = painter.stop
    return el
  }
  /** A framed console capture, showing the whole screenshot. */
  pic(x: number, y: number, w: number, h: number, shot: string, o: FrameOpts = {}): Pic {
    const b = this.box(x, y, w, h, { title: o.title, kind: o.kind ?? 'round', tone: o.tone, animate: o.animate })
    const p = new Pic(this, b.node, b.rect, `pic:${shot}`)
    p.cancel = () => (p.anim(), b.cancel())
    this.els.delete(b)
    this.els.add(p)
    const r = p.inner
    const win = document.createElement('div')
    win.className = 'in pic'
    win.style.left = '2ch'
    win.style.top = 'var(--lh)'
    win.style.width = `${r.w}ch`
    win.style.height = `calc(${r.h} * var(--lh))`
    p.img = document.createElement('img')
    p.img.src = `../build/shots/${shot}.png`
    win.append(p.img)
    b.node.append(win)
    p.win = win
    p.shot = shot
    this.sfx('shot')
    p.paint(p.fit(p.box([0, 0, 1, 1]), 1))
    return p
  }
  /** Remove everything but keep. */
  clear(...keep: El[]) {
    for (const e of [...this.els]) if (!keep.includes(e) && !keep.includes(e.parent!)) e.remove()
  }
  /** Repaint the stage top to bottom like a serial line (ascii.ts `sweep`), the console's change of view. */
  sweep() {
    A.sweep(this.root, 6)
  }

  // ---- timing ----------------------------------------------------------------------------------------------
  /** The tick at which the next spoken occurrence of cue starts. */
  at(cue: string): number {
    const want = cue.split(/\s+/).map(norm)
    for (let i = this.cursor; i <= this.words.length - want.length; i++) {
      if (want.every((w, k) => this.words[i + k].w === w)) {
        this.cursor = i + 1
        return Math.round(((this.t0 + this.words[i].s) * 1000) / A.TICK)
      }
    }
    throw new Error(`${this.sid}: cue "${cue}" not found after word ${this.cursor}`)
  }
  /** Wait until a tick. */
  tick(n: number): Promise<void> {
    return new Promise((res) => {
      if (A.clock.t >= n) return res()
      A.on((t) => {
        if (t >= n) {
          res()
          return false
        }
      })
    })
  }
  /** Wait until the cue is spoken (lead ticks early). Each one is a step of the scene, kept for review. */
  async until(cue: string, lead = 1) {
    await this.tick(this.at(cue) - lead)
    this.steps.push(A.clock.t)
  }
  steps: number[] = []
  /** Words by the tick they start on, to follow the narration. */
  wordsAt = new Map<number, number[]>()
  cool = new Map<El, number>()
  litAt = -99
  /**
   * Highlights that follow the narration: when a word is spoken, a label on screen that holds it flashes in
   * reverse video (the console's flash) and keeps the selected-row colour for a second; on a capture, the
   * button or heading the narration names gets a soft outline. Never on the header, the captions or big text.
   */
  follow(t: number) {
    const now = this.wordsAt.get(t)
    if (!now || t - this.litAt < 6) return
    for (const i of now) {
      const w = this.words[i].w
      const pic = [...this.els].reverse().find((e) => e instanceof Pic) as Pic | undefined
      if (pic?.follow(this.words, i, t)) {
        this.litAt = t
        return
      }
      if (w.length < 4 || STOP.has(w)) continue
      const hit = [...this.els].find(
        (e) =>
          e.node.tagName === 'PRE' &&
          e.rect.y > 1 &&
          e.rect.y < ROWS - 4 &&
          !e.name.startsWith('big:') &&
          t - (this.cool.get(e) ?? -99) > 25 &&
          tokens(e.node.textContent ?? '').includes(w),
      )
      if (!hit) continue
      this.cool.set(hit, t)
      this.litAt = t
      if (hit.rect.h > 1) {
        // One line of a code card: only once it is typed, and only plain text (arrows repaint themselves).
        if (hit.node.textContent !== hit.full || hit.node.children.length) continue
        const lines = hit.full.split('\n')
        const k = lines.findIndex((l) => tokens(l).includes(w))
        hit.node.innerHTML = lines.map((l, j) => (j === k ? `<span class="lit">${A.esc(l)}</span>` : A.esc(l))).join('\n')
        A.run(14, () => {}, () => hit.node.isConnected && hit.node.textContent === hit.full && (hit.node.textContent = hit.full))
        return
      }
      hit.node.classList.add('flash')
      A.run(2, () => {}, () => {
        hit.node.classList.remove('flash')
        hit.node.classList.add('lit')
        A.run(12, () => {}, () => hit.node.classList.remove('lit'))
      })
      return
    }
  }
  wait(ticks: number) {
    return this.tick(A.clock.t + ticks)
  }
  /**
   * Motion for a picture that has stood still too long, never over text: a capture steps in towards what is in
   * focus (or back out); otherwise a bright run goes once round the newest frame's border, skipping its labels.
   */
  idle() {
    const live = [...this.els].filter((e) => e.node.isConnected).reverse()
    const pic = live.find((e) => e instanceof Pic) as Pic | undefined
    if (pic) return pic.punch()
    const box = live.find((e) => e.name.startsWith('box:') && e.rect.y > 1 && e.rect.y < ROWS - 4 && e.rect.w > 3)
    if (box) this.sheen(box)
  }
  sheen(box: El) {
    const { x, y, w, h } = box.rect
    const rows = (box.node.textContent ?? '').split('\n')
    const ring: [number, number, string][] = []
    for (let i = 0; i < w; i++) ring.push([x + i, y, i === 0 ? '┏' : i === w - 1 ? '┓' : '━'])
    for (let j = 1; j < h; j++) ring.push([x + w - 1, y + j, j === h - 1 ? '┛' : '┃'])
    for (let i = w - 2; i >= 0; i--) ring.push([x + i, y + h - 1, i === 0 ? '┗' : '━'])
    for (let j = h - 2; j > 0; j--) ring.push([x, y + j, '┃'])
    const border = (cx: number, cy: number) => '─│╭╮╰╯┌┐└┘═║╔╗╚╝━┃┏┓┗┛┄┆'.includes([...(rows[cy - y] ?? '')][cx - x] ?? ' ')
    const tone = box.node.dataset.tone
    const cls = tone === 'ok' ? 'c-ok' : tone === 'warn' ? 'c-warn' : tone === 'err' ? 'c-err' : 'c-acc'
    const seg = [0, 1, 2, 3].map(() => {
      const n = document.createElement('pre')
      n.className = 'sheen ' + cls
      this.root.append(n)
      return n
    })
    A.run(Math.ceil(ring.length / 3) + 4, (p) => {
      const head = Math.floor(p * (ring.length + 4))
      seg.forEach((n, k) => {
        const c = ring[head - k]
        const on = c && border(c[0], c[1])
        n.textContent = on ? c[2] : ''
        if (on) (n.style.left = `${c[0]}ch`), (n.style.top = `calc(${c[1]} * var(--lh))`)
      })
    }, () => seg.forEach((n) => n.remove()))
  }

  // ---- the layout check ------------------------------------------------------------------------------------
  check() {
    const live = [...this.els].filter((e) => e.node.isConnected)
    const within = (a: El, b: El) => {
      for (let p = b.parent; p; p = p.parent) if (p === a) return true
      return false
    }
    for (const e of live) {
      const { x, y, w, h } = e.rect
      if (x < 0 || y < 0 || x + w > COLS || y + h > ROWS) this.problems.add(`off the grid: ${e.name} at ${x},${y} ${w}x${h}`)
      if (y + h > ROWS - 1 && A.clock.t * A.TICK >= this.t0 * 1000) this.problems.add(`on the status line: ${e.name} at ${x},${y} ${w}x${h}`)
      // Inside a frame means inside its border.
      const p = e.parent?.rect
      if (p && (x < p.x + 1 || y < p.y + 1 || x + w > p.x + p.w - 1 || y + h > p.y + p.h - 1))
        this.problems.add(`over the frame: ${e.name} (${x},${y} ${w}x${h}) in ${e.parent!.name} (${p.x},${p.y} ${p.w}x${p.h})`)
      // A capture must be drawn at the size its view asks for (page CSS could cap it).
      if (e instanceof Pic && e.img.complete) {
        const want = (e.meta().size[0] * e.inner.w * A.cell.cw) / e.view[2]
        if (Math.abs(e.img.getBoundingClientRect().width - want) > 2) this.problems.add(`capture drawn at the wrong size: ${e.name} ${e.shot}`)
      }
    }
    for (let i = 0; i < live.length; i++) {
      for (let j = i + 1; j < live.length; j++) {
        const a = live[i]
        const b = live[j]
        if (within(a, b) || within(b, a)) continue
        const r = a.rect
        const s = b.rect
        if (r.x < s.x + s.w && s.x < r.x + r.w && r.y < s.y + s.h && s.y < r.y + r.h)
          this.problems.add(`overlap: ${a.name} (${r.x},${r.y} ${r.w}x${r.h}) and ${b.name} (${s.x},${s.y} ${s.w}x${s.h})`)
      }
    }
  }
}

type SceneFn = (s: Stage) => Promise<void>
const SCENES: Record<string, SceneFn> = {}
export const scene = (sid: string, fn: SceneFn) => (SCENES[sid] = fn)

export type Intro = { kind: string; n?: number; title?: string; part?: string }
/** Where the scene sits in its chapter (seconds), its last tick, and how it leaves: the body or everything. */
export type Live = { at: number; len: number; end: number; outro: 'body' | 'all' | null }

/** Ticks the picture may stand still before the stage moves something itself (film.py fails at 5 s). */
const IDLE = 45

export class Film {
  stage = new Stage()
  last = 0
  since = 0
  /** What can play before a scene's voice (kit.ts registers the title and the chapter card). */
  intros: Record<string, (s: Stage, i: Intro) => Promise<void>> = {}
  /** Start a scene at tick 0; an intro plays during the first t0 seconds. */
  /** kit.ts adds what runs beside every scene: the status line, the scene's dither out. */
  onStart: (s: Stage, live: Live) => void = () => {}
  start(sid: string, words: Word[], t0: number, intro: Intro | null, info: Stage['info'], live: Live) {
    A.clock.t = 0
    seed = 1
    const s = this.stage
    Object.assign(s, { sid, words, t0, info, cursor: 0, done: false, steps: [], events: [] })
    s.wordsAt = new Map()
    words.forEach((w, i) => {
      const k = Math.round(((t0 + w.s) * 1000) / A.TICK)
      s.wordsAt.set(k, [...(s.wordsAt.get(k) ?? []), i])
    })
    A.on((t) => void s.follow(t))
    this.onStart(s, live)
    const run = async () => {
      if (intro) await this.intros[intro.kind](s, intro)
      if (!SCENES[sid]) throw new Error(`no scene ${sid}`)
      await SCENES[sid](s)
      s.done = true
    }
    run().catch((e) => s.problems.add(`error: ${e?.message ?? e}`))
  }
  has(sid: string) {
    return sid in SCENES
  }
  /** One tick: step the clock, let the scene act, check the layout. Returns what is on screen, to skip repeats. */
  async step(): Promise<string> {
    A.step()
    await new Promise((r) => setTimeout(r, 0))
    const imgs = [...this.stage.root.querySelectorAll('img')].filter((i) => !i.complete)
    await Promise.all(imgs.map((i) => i.decode().catch(() => {})))
    this.stage.check()
    // Hashes of what is on screen: everything (the renderer keeps one image for a run of identical ticks),
    // and the scene alone, without the status line (film.py's check that the picture never stands still).
    const hash = (html: string) => {
      let h = 0
      for (let i = 0; i < html.length; i++) h = (Math.imul(h, 31) + html.charCodeAt(i)) | 0
      return h
    }
    const scene = hash(this.stage.root.innerHTML)
    const t = A.clock.t
    if (scene !== this.last) (this.last = scene), (this.since = t)
    else if (t - this.since >= IDLE && t * A.TICK >= this.stage.t0 * 1000) (this.stage.idle(), (this.since = t))
    return `${hash(document.getElementById('amb')!.innerHTML)}:${scene}`
  }
  events() {
    return this.stage.events
  }
  problems() {
    return [...this.stage.problems]
  }
  steps() {
    return this.stage.steps
  }
}

window.film = new Film()
