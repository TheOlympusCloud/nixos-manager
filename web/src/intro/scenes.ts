// The intro's text scenes (design/ascii/prototype/js/intro.js): the snowflake, the planet of package dots, its
// arcs, orbits and commit pulses, the stars, and the blueprint. Each draws into character buffers, one per colour
// layer; the components repaint them on the one clock (lib/ascii.ts `scene`).
import { Buf, SPIN, cell, type FrameKind } from '../lib/ascii'
import { window01 } from './story'
import { EDGES, ERROR_LINES, FAILING, FIX_LINES, NODES, STAMPS, STEPS, blueprintAt } from './blueprint/timeline'
import type { Chapter } from './three/types'

/** A cell is about twice as tall as it is wide. */
export const ASPECT = 2.02

const hash = (a: number, b: number) => {
  const s = Math.sin(a * 127.1 + b * 311.7) * 43758.5453
  return s - Math.floor(s)
}

/** The window in cells. */
export const grid = () => ({ cols: Math.floor(innerWidth / cell.cw), rows: Math.floor(innerHeight / cell.lh) })

export const bufs = <K extends string>(names: readonly K[], cols: number, rows: number) =>
  Object.fromEntries(names.map((n) => [n, new Buf(cols, rows)])) as Record<K, Buf>

const clear = (b: Buf, x: number, y: number) => {
  if (y < 0 || y >= b.rows || x < 0 || x >= b.cols) return
  b.ch[y][x] = ' '
  b.bits[y][x] = 0
}

const dirChar = (dx: number, dy: number) => {
  const a = ((Math.atan2(dy, dx / ASPECT) * 180) / Math.PI + 360) % 180
  return a < 22.5 || a >= 157.5 ? '─' : a < 67.5 ? '╲' : a < 112.5 ? '│' : '╱'
}
function line(b: Buf, x1: number, y1: number, x2: number, y2: number, ch?: string) {
  const n = Math.max(Math.abs(x2 - x1), Math.abs(y2 - y1) * ASPECT, 1)
  for (let i = 0; i <= n; i++) b.set(x1 + ((x2 - x1) * i) / n, y1 + ((y2 - y1) * i) / n, ch || dirChar(x2 - x1, y2 - y1))
}

/** The first p (0..1) of an orthogonal polyline through whole cells, drawn with joining box lines. */
export function trace(b: Buf, points: [number, number][], p: number) {
  const len = (a: [number, number], c: [number, number]) => Math.abs(c[0] - a[0]) + Math.abs(c[1] - a[1])
  let left = Math.floor(points.slice(1).reduce((s, q, i) => s + len(points[i], q), 0) * Math.min(1, p))
  const out: [number, number][] = [points[0]]
  for (let i = 1; i < points.length && left > 0; i++) {
    const [a, c] = [points[i - 1], points[i]]
    const k = Math.min(left, len(a, c))
    out.push([a[0] + Math.sign(c[0] - a[0]) * k, a[1] + Math.sign(c[1] - a[1]) * k])
    left -= k
  }
  if (out.length > 1) b.path(out)
}

/** The snowflake: six lambdas about the centre, shaded by distance to their strokes; arms alternate between the two blues. */
export function snowflake(L: { hot: Buf; globe: Buf }, cx: number, cy: number, R: number, angle: number) {
  const segs: number[][] = []
  for (let k = 0; k < 6; k++) {
    const a = angle + (k * Math.PI) / 3
    const p = (r: number, t: number) => [Math.cos(t) * r, Math.sin(t) * r]
    const [mx, my] = p(R * 0.62, a)
    const b = a + Math.PI / 3
    segs.push([...p(R * 0.2, a), ...p(R, a), k % 2], [mx, my, mx + Math.cos(b) * R * 0.52, my + Math.sin(b) * R * 0.52, k % 2])
  }
  for (let y = Math.floor(cy - R - 1); y <= cy + R + 1; y++) {
    for (let x = Math.floor(cx - (R + 1) * ASPECT); x <= cx + (R + 1) * ASPECT; x++) {
      const px = (x - cx) / ASPECT
      const py = y - cy
      let best = 9
      let tone = 0
      for (const [x1, y1, x2, y2, k] of segs) {
        const vx = x2 - x1
        const vy = y2 - y1
        const u = Math.max(0, Math.min(1, ((px - x1) * vx + (py - y1) * vy) / (vx * vx + vy * vy)))
        const d = Math.hypot(px - x1 - vx * u, py - y1 - vy * u)
        if (d < best) [best, tone] = [d, k]
      }
      const ch = best < 0.45 ? '█' : best < 0.75 ? '▓' : best < 1.05 ? '▒' : best < 1.35 ? '░' : ''
      if (ch) (tone ? L.hot : L.globe).set(x, y, ch)
    }
  }
}

// The planet: a lambert-shaded sphere whose surface is a dot matrix of packages on a luminance ramp. `scatter`
// lifts the packages off it (drift); they lock back as it returns to 0 (assemble).
const RAMP = ' ··:;+=*#%@'
export function planet(L: { globe: Buf; hot: Buf; warm: Buf }, cx: number, cy: number, R: number, rot: number, scatter = 0, dim = 1) {
  const { globe, hot, warm } = L
  const x0 = Math.max(0, Math.floor(cx - R * ASPECT - 2))
  const x1 = Math.min(globe.cols - 1, Math.ceil(cx + R * ASPECT + 2))
  for (let y = Math.max(0, Math.floor(cy - R - 1)); y <= Math.min(globe.rows - 1, Math.ceil(cy + R + 1)); y++) {
    for (let x = x0; x <= x1; x++) {
      const dx = (x - cx) / ASPECT
      const dy = y - cy
      const d = Math.hypot(dx, dy)
      if (d > R) {
        // A thin lit atmosphere on the day side.
        if (d < R + 0.9 && dx * -0.5 + dy * -0.62 > R * 0.25) hot.set(x, y, '·')
        continue
      }
      const nz = Math.sqrt(1 - (d * d) / (R * R))
      const lam = Math.max(0, (dx / R) * -0.5 + (dy / R) * -0.62 + nz * 0.6)
      const u = Math.floor((Math.atan2(dx / R, nz) + rot) * 9)
      const v = Math.floor(Math.asin(dy / R) * 9)
      const h = hash(u, v)
      const pkg = h > 0.45
      if (scatter > 0 && pkg && hash(v, u) < scatter * 0.8) {
        const far = 1 + scatter * (0.35 + hash(u + 3, v) * 1.5)
        warm.set(cx + dx * far * ASPECT + Math.sin(rot * 3 + u) * scatter * 3, cy + dy * far + Math.cos(rot * 2 + v) * scatter, h > 0.8 ? '▪' : '▫')
        continue
      }
      const b = Math.min(1, (lam * (pkg ? 0.5 + h * 0.6 : 0.22) + (1 - nz) ** 3 * 0.25) * dim)
      ;(b > 0.62 ? hot : globe).set(x, y, RAMP[Math.floor(b * (RAMP.length - 1))])
    }
  }
}

/** Twinkling stars, and a meteor now and then. */
export function stars(b: Buf, t: number) {
  for (let i = 0; i < 90; i++) {
    const tw = (t + i * 5) % 40
    b.set(Math.floor(hash(i, 1) * b.cols), Math.floor(hash(i, 2) * b.rows), tw < 2 ? '✦' : tw < 4 ? '+' : hash(i, 3) > 0.6 ? '·' : '∙')
  }
  const m = t % 90
  if (m < 14) for (let k = 0; k < 5; k++) b.set(b.cols * 0.2 + (m - k) * 4, 4 + (m - k), k ? '╲' : '●')
}

/** The deploy rail fills over the middle of its chapter, so the heading has time to land first. */
export const deployFill = (t: number) => Math.min(1, Math.max(0, (t - 0.12) / 0.78))

// ---- The story: one keyframe per chapter, where the planet sits and what surrounds it ------------------------

export const LAYERS = ['stars', 'globe', 'hot', 'warm', 'fg', 'good'] as const
export type Layers = Record<(typeof LAYERS)[number], Buf>

/** The scene behind the copy of a chapter at progress p (0..1), on tick t. The copy is the left 84 columns. */
export function story(L: Layers, id: Chapter, p: number, t: number, machines: string[]) {
  const { cols, rows } = L.fg
  // On a narrow window the copy spans it: the planet becomes a small dome rising over the bottom bar.
  const side = (x: number, y: number, R: number) => (cols >= 120 ? [x, y, R] : [cols * 0.5, rows - 3, Math.min(R, cols / ASPECT / 2 - 1)])
  stars(L.stars, t)
  const R0 = rows * 0.62
  const rot = t * 0.03
  if (id === 'hero') {
    const [cx, cy, R] = side(cols * 0.72, rows * 1.02 - p * 6, R0 * 1.25)
    planet(L, cx, cy, R, rot)
  } else if (id === 'drift') {
    const [cx, cy, R] = side(cols * 0.75, rows * 0.56, R0 * 0.66)
    planet(L, cx, cy, R, rot, Math.sin(Math.min(1, p * 1.25) * Math.PI * 0.5) * 0.9)
  } else if (id === 'assemble') {
    const s = Math.max(0, 0.9 * (1 - p * 1.6))
    const [cx, cy, R] = side(cols * 0.75, rows * 0.56, R0 * 0.66)
    planet(L, cx, cy, R, rot, s)
    // The last package locks in and a ring leaves the planet.
    const r = R + (p - 0.56) * 60
    if (s === 0 && p < 0.75) for (let a = 0; a < 360; a += 3) L.hot.set(cx + Math.cos(a) * r * ASPECT, cy + Math.sin(a) * r, '∙')
  } else {
    const [cx, cy, R] =
      id === 'outro' ? [cols * 0.5, rows * 0.78, R0 * (0.5 - p * 0.12)] : side(cols * 0.74, rows * 0.52, id === 'planet' ? R0 * 0.7 : R0 * 0.46)
    planet(L, cx, cy, R, rot, 0, id === 'outro' ? 0.7 : 1)
    if (id === 'planet') {
      // Dependency arcs: great circles on the near side, each carrying packets of light.
      ;[0.28, -0.5, 0.78].forEach((k, j) => {
        for (let a = 0; a < 180; a += 2) {
          const ph = (a * Math.PI) / 180
          const packet = (a + t * 4 + j * 50) % 60 < 4
          ;(packet ? L.fg : L.hot).set(cx + Math.cos(ph) * R * ASPECT, cy + Math.sin(ph) * R * k, packet ? '●' : '∙')
        }
      })
    } else if (id === 'orbit' || id === 'deploy') {
      // Orbits, and a machine on each; in the orbit chapter a closure beam ties the machine to the planet.
      const [fx, fy] = [Math.round(cols * 0.5), Math.round(rows * 0.16)]
      const step = Math.min(5, Math.floor(deployFill(p) * 6 + 1e-6))
      machines.forEach((name, j) => {
        const [a, b] = [1.35 + j * 0.3, 0.4 + j * 0.1]
        for (let d = 0; d < 360; d += 3) {
          const r = (d * Math.PI) / 180
          const x = cx + Math.cos(r) * R * a * ASPECT
          const y = cy + Math.sin(r) * R * a * b
          if (Math.hypot((x - cx) / ASPECT, y - cy) > R + 0.5 || Math.sin(r) > 0) L.globe.set(x, y, '·')
        }
        const th = j * 2.1 + t * (0.02 - j * 0.004)
        const mx = cx + Math.cos(th) * R * a * ASPECT
        const my = cy + Math.sin(th) * R * a * b
        if (id === 'orbit') {
          const d0 = (R + 1.5) / Math.hypot((mx - cx) / ASPECT, my - cy)
          line(L.globe, cx + (mx - cx) * d0, cy + (my - cy) * d0, mx, my, '┄')
        }
        // A label clears the cells under it, so it reads when it crosses the planet.
        for (const n of ['globe', 'hot', 'stars'] as const) L[n].text(Math.round(mx) - 2, Math.round(my), ' '.repeat(name.length + 6))
        L.fg.text(Math.round(mx) - 1, Math.round(my), '▣ ' + name)
        if (id === 'deploy') {
          // Commit pulses leave the forge for each machine once they pull; a tick once they have switched.
          if (step >= 4) {
            const k = (((t * 0.06 + j * 0.33) % 1) + 1) % 1
            const px = fx + (mx - fx) * k
            const py = fy + (my - fy) * k - Math.sin(k * Math.PI) * 5
            L.warm.set(px, py, '●')
            L.warm.set(px - 2, py, '∙')
          }
          if (step >= 5) L.good.text(Math.round(mx) + name.length + 2, Math.round(my), '✓')
        }
      })
      if (id === 'deploy') {
        L.fg.text(fx - 4, fy, '⌥ forge')
        L.hot.box(fx - 6, fy - 1, 13, 3)
      }
    }
  }
  // The scene stays clear of the two bars, and loose marks stay out from under the copy.
  for (const n of LAYERS) {
    const b = L[n]
    for (let y = 0; y < rows; y++) {
      if (y < 3 || y >= rows - 3) {
        b.ch[y].fill(' ')
        b.bits[y].fill(0)
      } else if ((n === 'stars' || n === 'warm') && id !== 'outro' && y > 4 && y < rows * 0.7) for (let x = 0; x < Math.min(cols, 84); x++) b.ch[y][x] = ' '
    }
  }
}

// ---- The blueprint: flake → hosts → modules → packages, drawn, evaluated, broken, patched, stamped ---------

export const SHEET = ['mut', 'fg', 'hot', 'bad', 'good'] as const
export type Sheet = Record<(typeof SHEET)[number], Buf>

// Where each node sits: its layer's column and its row in that layer, in cells from the sheet's origin.
const COL = [0, 22, 52, 86]
const ROW = [[14], [4, 14, 24], [1, 7, 15, 24], [0, 4, 8, 12, 16, 20, 24]]
const STATUS = { draw: 'draw', evaluate: 'evaluate', fail: 'failing', fix: 'fix', stamp: 'passed' }

/** The sheet at timeline position t (0..1) on tick k; the head of the chapter sits above row 10. */
export function blueprint(L: Sheet, t: number, k: number) {
  const { mut, fg, hot, bad, good } = L
  const s = blueprintAt(t)
  const { cols, rows } = fg
  const ox = Math.max(4, Math.floor((cols - 150) / 2))
  const oy = 12
  const at = Object.fromEntries(NODES.map((n) => [n.id, { x: ox + COL[n.col], y: oy + ROW[n.col][n.row], w: n.label.length + 6, n }]))
  EDGES.forEach(([a, b], i) => {
    const [A, B] = [at[a], at[b]]
    const mid = B.x - 4
    trace(fg, [[A.x + A.w, A.y + 1], [mid, A.y + 1], [mid, B.y + 1], [B.x - 1, B.y + 1]], s.edges[i])
  })
  for (const { x, y, w, n } of Object.values(at)) {
    const st = s.nodes[n.id]
    if (st.draw <= 0) continue
    const busy = st.status === 'scanning' || st.status === 'fixing'
    const tone = st.status === 'fail' ? bad : busy ? hot : fg
    if (st.draw < 1)
      trace(tone, [[x, y], [x + w - 1, y], [x + w - 1, y + 2], [x, y + 2], [x, y]], st.draw)
    else tone.box(x, y, w, 3, n.kind === 'flake' ? 'double' : 'round')
    if (st.draw > 0.4) (st.status === 'fail' ? bad : fg).text(x + 2, y + 1, n.label)
    if (st.status === 'ok') good.set(x + w - 2, y + 1, '✓')
    else if (st.status === 'fail') bad.set(x + w - 2, y + 1, '✖')
    else if (busy) hot.set(x + w - 2, y + 1, SPIN.tee[k % 8])
  }
  // Evaluation: a scan line crosses the sheet while the nodes are ticked off.
  if (s.step === 'evaluate') {
    const sx = ox + Math.round(window01(t, STEPS[1].from, STEPS[1].to) * 112)
    for (let y = oy - 1; y < oy + 28; y++)
      ['░', '▒', '┃'].forEach((c, i) => {
        for (const b of [mut, fg, good]) clear(b, sx - 2 + i, y)
        hot.set(sx - 2 + i, y, c)
      })
  }
  // The callouts go under the graph when the window is tall enough (the prototype's 900 px); on a shorter one they
  // are laid on the sheet itself, over the empty middle above hosts/pi, and the patch takes the error's place.
  const below = rows >= oy + 40
  const pi = at[FAILING]
  const [cx, cy] = below ? [pi.x - 8, oy + 28] : [pi.x, oy + 18]
  const callout = (b: Buf, x: number, y: number, w: number, h: number, kind: FrameKind) => {
    if (!below) for (let j = y; j < y + h; j++) for (let i = x; i < x + w; i++) for (const l of Object.values(L)) clear(l, i, j)
    b.box(x, y, w, h, kind)
  }
  const failing = s.nodes[FAILING].status === 'fail' || s.nodes[FAILING].status === 'fixing'
  if (s.error > 0 && failing && (below || s.step !== 'fix')) {
    callout(bad, cx, cy, Math.max(...ERROR_LINES.map((l) => l.length)) + 4, ERROR_LINES.length + 2, 'heavy')
    ERROR_LINES.forEach((l, i) => (i ? fg : bad).text(cx + 2, cy + 1 + i, l))
    if (k % 8 < 4) bad.text(cx + 3 + ERROR_LINES[2].length, cy + 3, '◂')
  }
  if (s.step === 'fix') {
    const px = below ? cx + 52 : cx
    callout(good, px, cy, Math.max(...FIX_LINES.map((l) => l.length)) + 4, 4, 'round')
    good.text(px + 2, cy, ' patch · hosts/pi/default.nix ')
    bad.text(px + 2, cy + 1, FIX_LINES[0])
    const typed = window01(t, STEPS[3].from, STEPS[3].from + 0.4 * (STEPS[3].to - STEPS[3].from))
    good.text(px + 2, cy + 2, FIX_LINES[1].slice(0, Math.ceil(typed * FIX_LINES[1].length)))
  }
  if (s.stamp > 0) {
    const stamp = (x: number, text: string) => (callout(good, x, cy + 1, text.length + 6, 3, 'double'), good.text(x + 3, cy + 2, text))
    stamp(pi.x, STAMPS[0].toUpperCase())
    if (s.stamp >= 1) stamp(pi.x + STAMPS[0].length + 10, STAMPS[1].toUpperCase())
  }
  if (s.paper <= 0) return
  // Drafting furniture: the act, the dimensions, the notes and the title block.
  const step = STEPS.findIndex((x) => x.id === s.step)
  hot.text(ox, oy - 2, `${String(step + 1).padStart(2, '0')} / ${String(STEPS.length).padStart(2, '0')}`)
  fg.text(ox + 9, oy - 2, STEPS[step].label.toUpperCase())
  mut.text(ox + COL[2], oy - 2, 'evaluation · flake → hosts → modules → packages')
  mut.text(ox + COL[1], oy + 27, '3 machines')
  mut.text(ox + COL[3], oy + 27, '7 of 100 000+ packages · reached by hash')
  // The title block sits bottom right; a short window moves it up beside the head, where the notes give way.
  const low = rows >= oy + 36
  if (low)
    ['inputs.nixpkgs = github:NixOS/nixpkgs/nixos-25.05', 'flake.lock · pinned · reproducible', '', 'managed.json · edited by clicking', 'hand-written nix is never rewritten'].forEach((l, i) =>
      mut.text(cols - 56, 4 + i, l),
    )
  const [tx, ty] = [cols - 62, low ? rows - 8 : 3]
  fg.box(tx, ty, 58, 5, 'square')
  fg.text(tx + 2, ty + 1, 'NIXOS-MANAGER' + 'FLEET · CONFIGURATION GRAPH'.padStart(41))
  fg.text(tx + 2, ty + 2, 'DRAWN nix eval'.padEnd(22) + 'CHECK nix flake check')
  fg.text(tx + 2, ty + 3, 'REV 4e1f9a2'.padEnd(22) + `SHEET 1 of 1 · STATUS ${STATUS[s.step]}`)
}
