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
const ease = (p: number) => 1 - (1 - p) ** 3

export class El {
  cancel: () => void = () => {}
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
    return [cx - cw / 2, cy - ch / 2, cw, ch]
  }
  paint(v: Box) {
    this.view = v
    const r = this.inner
    const s = (r.w * A.cell.cw) / v[2]
    this.img.style.width = `${this.meta().size[0] * s}px`
    this.img.style.transform = `translate(${-v[0] * s}px, ${-v[1] * s}px)`
  }
  /** Step the view to region over a few ticks (the console's motion: stepped, never smooth). */
  look(key: string | Box, margin = 1.15, ticks = 6) {
    const to = this.fit(this.box(key), margin)
    const from = this.view
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
  /** A heavy frame around a region: the console's highlight. */
  mark(key: string | Box, tone: FrameOpts['tone'] = 'warn', label?: string): El {
    const c = this.cells(key)
    return this.stage.box(c.x, c.y, c.w, c.h, { kind: 'heavy', tone, title: label }, this)
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
  text(x: number, y: number, s: string, cls = '', parent?: El): El {
    const lines = s.split('\n')
    const el = this.place({ x, y, w: Math.max(...lines.map((l) => [...l].length)), h: lines.length }, 'pre', s.slice(0, 24), parent, cls)
    el.node.textContent = s
    return el
  }
  /** Text typed in, a burst of characters per tick (ascii.ts `type`). */
  type(x: number, y: number, s: string, cls = '', parent?: El, perTick = 4): El {
    const el = this.text(x, y, s, cls, parent)
    el.cancel = A.type(el.node, s, perTick)
    return el
  }
  /** Text realised out of Nix's base-32 alphabet, as a store path out of its hash (ascii.ts `scramble`). */
  real(x: number, y: number, s: string, cls = '', parent?: El, ticks = 5): El {
    const el = this.text(x, y, s, cls, parent)
    el.cancel = A.run(ticks, (p) => (el.node.textContent = A.scramble(s, p)))
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
    el.cancel = () => act.destroy()
    return el
  }
  /** The pixel face (ascii.ts `big`), wiped in. */
  big(x: number, y: number, s: string, cls = '', parent?: El): El {
    const art = A.big(s)
    const el = this.text(x, y, art, 'big ' + cls, parent)
    el.name = `big:${s}`
    el.cancel = A.wipe(el.node, art)
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
    p.shot = shot
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
  wait(ticks: number) {
    return this.tick(A.clock.t + ticks)
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

export class Film {
  stage = new Stage()
  /** What can play before a scene's voice (kit.ts registers the title and the chapter card). */
  intros: Record<string, (s: Stage, i: Intro) => Promise<void>> = {}
  /** Start a scene at tick 0; an intro plays during the first t0 seconds. */
  start(sid: string, words: Word[], t0: number, intro: Intro | null, info: Stage['info']) {
    A.clock.t = 0
    seed = 1
    const s = this.stage
    Object.assign(s, { sid, words, t0, info, cursor: 0, done: false, steps: [] })
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
  async step(): Promise<number> {
    A.step()
    await new Promise((r) => setTimeout(r, 0))
    const imgs = [...this.stage.root.querySelectorAll('img')].filter((i) => !i.complete)
    await Promise.all(imgs.map((i) => i.decode().catch(() => {})))
    this.stage.check()
    // A hash of what is on screen: the renderer keeps one image for a run of identical ticks.
    const html = this.stage.root.innerHTML
    let h = 0
    for (let i = 0; i < html.length; i++) h = (Math.imul(h, 31) + html.charCodeAt(i)) | 0
    return h
  }
  problems() {
    return [...this.stage.problems]
  }
  steps() {
    return this.stage.steps
  }
}

window.film = new Film()
