// Cellwork engine: the clock, the frames and every glyph animation of the console.
// Plain script, no dependencies; each function here is what a Svelte action or helper would wrap.
;(function () {
  'use strict'
  const A = (window.A = {})
  const query = new URLSearchParams(location.search)
  // ?still (or the reduced-motion setting) freezes every animation on its last frame.
  A.still = query.has('still') || matchMedia('(prefers-reduced-motion: reduce)').matches
  // ?manual hands the clock to whoever calls A.step(): the capture script, frame by frame.
  A.manual = query.has('manual')

  // ---- The one clock -------------------------------------------------------------------------
  // Every animation steps on the same 80 ms beat, so the whole screen moves like one terminal.
  const subs = new Set()
  A.t = 0
  A.on = (fn) => (subs.add(fn), () => subs.delete(fn))
  A.step = () => {
    A.t++
    for (const fn of [...subs]) {
      try {
        if (fn(A.t) === false) subs.delete(fn)
      } catch (e) {
        subs.delete(fn)
        console.error(e)
      }
    }
  }
  if (!A.manual && !A.still) setInterval(() => document.hidden || A.step(), 80)
  // Run fn(progress 0..1) over a number of ticks, then done().
  A.run = (ticks, fn, done) => {
    if (A.still || ticks < 1) return fn(1), done && done()
    let i = 0
    fn(0)
    A.on(() => {
      fn(Math.min(1, ++i / ticks))
      if (i >= ticks) return done && done(), false
    })
  }

  // ---- The cell ------------------------------------------------------------------------------
  let cw = 8.43
  let lh = 17
  A.measure = () => {
    const probe = document.createElement('span')
    probe.textContent = '0'.repeat(100)
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:pre'
    document.body.append(probe)
    cw = probe.getBoundingClientRect().width / 100
    lh = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--lh')) || 17
    probe.remove()
    A.cw = cw
    A.lh = lh
  }
  const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' })[c])
  A.esc = esc
  const rnd = (n) => Math.floor(Math.random() * n)

  // ---- Frames --------------------------------------------------------------------------------
  // tl, horizontal, tr, vertical, bl, br. The weight of a frame is its rank:
  // round = a card, square = data, double = a modal, heavy = hovered or selected, dash = pending.
  const SETS = { round: '╭─╮│╰╯', square: '┌─┐│└┘', double: '╔═╗║╚╝', heavy: '┏━┓┃┗┛', dash: '┌┄┐┆└┘' }
  const framed = new WeakMap()
  const ro = new ResizeObserver((entries) => {
    for (const e of entries) if (framed.has(e.target) && !framed.get(e.target).busy) draw(e.target, 1)
  })

  function draw(el, p) {
    const st = framed.get(el)
    if (!st) return
    const r = el.getBoundingClientRect()
    if (!el.isConnected || r.width < cw * 4) return // removed or hidden: nothing to frame
    const cols = Math.max(4, Math.floor(r.width / cw + 0.05))
    const rows = Math.max(2, Math.round(r.height / lh))
    const kind = st.hover || el.dataset.frame || 'round'
    const [tl, h, tr, v, bl, br] = SETS[kind] || SETS.round
    const shadow = el.hasAttribute('data-shadow')
    const put = (line, at, text) => {
      for (let i = 0; i < text.length && at + i < cols - 2; i++) line[at + i] = text[i]
      return [at, Math.min(at + text.length, cols - 2)]
    }
    const top = [tl, ...h.repeat(cols - 2), tr]
    const bot = [bl, ...h.repeat(cols - 2), br]
    const marks = { top: [], bot: [] }
    if (el.dataset.title) marks.top.push(['b', put(top, 2, ' ' + el.dataset.title + ' ')])
    if (el.dataset.tag) marks.top.push(['i', put(top, Math.max(2, cols - 4 - el.dataset.tag.length), ' ' + el.dataset.tag + ' ')])
    if (el.dataset.foot) marks.bot.push(['i', put(bot, Math.max(2, cols - 4 - el.dataset.foot.length), ' ' + el.dataset.foot + ' ')])
    // The frame is stroked in from its top-left corner: a cell shows once the wave reaches it.
    const reach = p >= 1 ? Infinity : p * (cols + rows * 2)
    const html = (line, y, ms) => {
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
    const lines = [html(top, 0, marks.top)]
    const gap = ' '.repeat(cols - 2)
    for (let y = 1; y < rows - 1; y++) {
      const left = 2 * y <= reach ? v : ' '
      const right = cols - 1 + 2 * y <= reach ? v : ' '
      lines.push(left + gap + right + (shadow && p >= 1 ? '<s>░</s>' : ''))
    }
    lines.push(html(bot, rows - 1, marks.bot) + (shadow && p >= 1 ? '<s>░</s>' : ''))
    if (shadow && p >= 1) lines.push(' <s>' + '░'.repeat(cols) + '</s>')
    st.pre.innerHTML = lines.join('\n')
  }

  // Give every [data-frame] under root its glyphs; with animate, stroke the new ones in.
  A.scan = (root = document.body, animate = false) => {
    for (const el of root.querySelectorAll('[data-big]')) {
      if (el.dataset.done === el.dataset.big) continue
      el.dataset.done = el.dataset.big
      el.setAttribute('aria-label', el.dataset.big)
      const text = A.big(el.dataset.big, +el.dataset.scale || 1)
      if (animate) A.wipe(el, text)
      else el.textContent = text
    }
    const els = [...root.querySelectorAll('[data-frame]')]
    if (root.matches && root.matches('[data-frame]')) els.unshift(root)
    for (const el of els) {
      let st = framed.get(el)
      if (st && st.pre.isConnected) {
        draw(el, 1)
        continue
      }
      const pre = document.createElement('pre')
      pre.className = 'fr'
      pre.setAttribute('aria-hidden', 'true')
      el.append(pre)
      st = { pre, busy: false }
      framed.set(el, st)
      ro.observe(el)
      if (animate && !A.still) {
        st.busy = true
        A.run(5, (p) => draw(el, p), () => ((st.busy = false), draw(el, 1)))
      } else draw(el, 1)
    }
    for (const el of root.querySelectorAll('.tabs')) A.ink(el, false)
    for (const el of root.querySelectorAll('.bar[data-v]')) el.innerHTML = A.bar(+el.dataset.v, +el.dataset.w || 20)
    tickLive()
  }
  A.redraw = (el) => draw(el, 1)

  // Hover lifts a card: its frame turns heavy and casts a ░ shadow.
  let lifted = null
  document.addEventListener('mouseover', (e) => {
    const el = e.target.closest && e.target.closest('.lift')
    if (el === lifted) return
    for (const [node, kind] of [[lifted, null], [el, 'heavy']]) {
      const st = node && framed.get(node)
      if (st) (st.hover = kind), st.busy || draw(node, 1)
    }
    lifted = el
  })

  // ---- Text ----------------------------------------------------------------------------------
  // Text is "realised" the way Nix realises a store path: out of its hash. Unresolved characters
  // show in Nix's base-32 alphabet, and since every glyph is one cell wide nothing ever reflows.
  const B32 = '0123456789abcdfghijklmnpqrsvwxyz'
  const scramble = (s, p) => {
    const keep = Math.floor(s.length * p)
    let out = s.slice(0, keep)
    for (let i = keep; i < s.length; i++) out += /\s/.test(s[i]) ? s[i] : B32[rnd(32)]
    return out
  }
  A.scramble = scramble

  // Repaint everything under root, top to bottom, like a screen redrawn over a serial line.
  A.sweep = (root, rowsPerTick = 9) => {
    if (A.still) return
    const skip = '.fr,.nofx,pre,textarea,script,style,.big,.bar,.skel,.spin,.ink,.scene'
    const tw = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
    const top = root.getBoundingClientRect().top
    const range = document.createRange()
    const items = []
    for (let n = tw.nextNode(); n; n = tw.nextNode()) {
      if (!n.data.trim() || n.parentElement.closest(skip)) continue
      range.selectNodeContents(n)
      const y = range.getBoundingClientRect().top
      if (y > innerHeight + 100) continue
      // A node already mid-sweep keeps its real text, not the placeholder it shows right now.
      n._final = n._final ?? n.data
      items.push({ n, s: n._final, at: Math.max(0, Math.floor((y - top) / lh / rowsPerTick)) })
    }
    for (const it of items) it.n.data = it.s.replace(/\S/g, '·')
    let k = 0
    A.on(() => {
      k++
      let live = false
      for (const it of items) {
        if (it.done) continue
        const d = k - it.at
        if (d >= 3) (it.n.data = it.s), (it.n._final = null), (it.done = true)
        else if (d > 0) (it.n.data = scramble(it.s, d / 3)), (live = true)
        else live = true
      }
      return live
    })
  }

  // One element resolves to a new text (a value that just changed).
  A.realise = (el, text, ticks = 5) => A.run(ticks, (p) => (el.textContent = scramble(text, p)))
  // A number rolls up to its value like an odometer.
  A.count = (el, n, ticks = 9) =>
    A.run(ticks, (p) => (el.textContent = p >= 1 ? String(n) : String(Math.floor(n * (1 - (1 - p) ** 3))).padStart(String(n).length, ' ')))
  // Type text in, one burst of characters per tick.
  A.type = (el, text, perTick = 3, done) =>
    A.run(Math.ceil(text.length / perTick), (p) => (el.textContent = text.slice(0, Math.round(text.length * p))), done)
  // Multi-line art wipes in left to right behind a ▓▒░ edge.
  A.wipe = (el, text, ticks = 7) => {
    const lines = text.split('\n')
    const w = Math.max(...lines.map((l) => l.length))
    A.run(ticks, (p) => {
      const c = Math.round(w * p)
      el.textContent = p >= 1 ? text : lines.map((l) => l.slice(0, c) + (l.slice(c, c + 3).trim() ? '▓▒░'.slice(0, w - c) : '')).join('\n')
    })
  }

  // The pixel face. Five pixel rows per glyph, printed as three text rows of half blocks.
  const FONT = {
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
    ':': '0 1 0 1 0', '!': '1 1 1 0 1', '?': '110 001 010 000 010', '+': '000 010 111 010 000', '_': '000 000 000 000 111',
    '&': '010 101 010 101 011', "'": '1 1 0 0 0', '>': '100 010 001 010 100', '%': '101 001 010 100 101',
  }
  A.big = (text, scale = 1) => {
    const rows = ['', '', '', '', '']
    for (const ch of String(text).toUpperCase()) {
      const g = (FONT[ch] || FONT['?']).split(' ')
      for (let y = 0; y < 5; y++) rows[y] += g[y] + '0'
    }
    if (scale > 1) return rows.map((r) => [...r].map((b) => (b === '1' ? '██' : '  ')).join('')).join('\n')
    const out = []
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

  // A meter: full blocks, one eighth-block for the fraction, a faint track.
  A.bar = (v, w = 20, sheen = -1) => {
    v = Math.max(0, Math.min(1, v))
    const cells = v * w
    const full = Math.floor(cells)
    const part = ' ▏▎▍▌▋▊▉'[Math.floor((cells - full) * 8)]
    let fill = '█'.repeat(full)
    if (sheen >= 0 && full > 3) {
      const at = sheen % (full + 6)
      fill = [...fill].map((c, i) => ({ 0: '░', 1: '▒', 2: '▓' })[at - i] || c).join('')
    }
    const rest = w - full - (part.trim() ? 1 : 0)
    return fill + part.trim() + '<s>' + '░'.repeat(Math.max(0, rest)) + '</s>'
  }
  A.spark = (values) => {
    const max = Math.max(...values, 1)
    return values.map((v) => '▁▂▃▄▅▆▇█'[Math.min(7, Math.floor((v / max) * 8))]).join('')
  }

  // The active tab stands on a heavy stretch of the rule; it slides there cell by cell.
  A.ink = (tabs, animate = true) => {
    const on = tabs.querySelector('a.on')
    let ink = tabs.querySelector('.ink')
    if (!ink) (ink = document.createElement('pre')), (ink.className = 'ink'), ink.setAttribute('aria-hidden', 'true'), tabs.append(ink)
    if (!on) return
    const base = tabs.getBoundingClientRect()
    const r = on.getBoundingClientRect()
    const cols = Math.ceil(base.width / cw)
    const to = Math.round((r.left - base.left) / cw)
    const len = Math.round(r.width / cw)
    const from = animate && tabs._at != null ? tabs._at : to
    const fromLen = animate && tabs._len != null ? tabs._len : len
    tabs._at = to
    tabs._len = len
    A.run(from === to ? 0 : 4, (p) => {
      const e = 1 - (1 - p) ** 2
      const at = Math.round(from + (to - from) * e)
      const l = Math.round(fromLen + (len - fromLen) * e)
      ink.innerHTML = '─'.repeat(at) + '<b>' + '━'.repeat(l) + '</b>' + '─'.repeat(Math.max(0, cols - at - l))
    })
  }

  // Spinners, skeletons and working meters all read the clock.
  const SPIN = { tee: '┤┘┴└├┌┬┐', arc: '◜◝◞◟', dot: '◐◓◑◒', ping: '·∙•●•∙', bounce: '▖▘▝▗', eq: '▁▃▅▇▅▃' }
  function tickLive() {
    const t = A.t
    for (const el of document.querySelectorAll('.spin')) {
      const f = SPIN[el.dataset.k] || SPIN.tee
      el.textContent = f[t % f.length]
    }
    for (const el of document.querySelectorAll('.skel')) {
      const w = +el.dataset.w || 24
      let s = ''
      for (let i = 0; i < w; i++) s += '░░░░░░▒▓▒░'[(i - t + 1000) % 10]
      el.textContent = s
    }
    for (const el of document.querySelectorAll('.bar[data-live]')) el.innerHTML = A.bar(+el.dataset.v, +el.dataset.w || 20, t)
  }
  A.on(tickLive)

  // ---- Scenes --------------------------------------------------------------------------------
  // A character buffer with a line layer: segments add compass bits, and the bits pick the glyph,
  // so lines that meet or cross join themselves.
  const JOIN = ' │─╰││╭├─╯─┴╮┤┬┼'
  A.Buf = class {
    constructor(cols, rows) {
      this.cols = cols
      this.rows = rows
      this.ch = Array.from({ length: rows }, () => Array(cols).fill(' '))
      this.bits = Array.from({ length: rows }, () => new Uint8Array(cols))
    }
    set(x, y, c) {
      x = Math.round(x)
      y = Math.round(y)
      if (y >= 0 && y < this.rows && x >= 0 && x < this.cols) this.ch[y][x] = c
    }
    get(x, y) {
      return (this.ch[y] || [])[x]
    }
    text(x, y, s) {
      for (let i = 0; i < s.length; i++) this.set(x + i, y, s[i])
    }
    bit(x, y, b) {
      if (y >= 0 && y < this.rows && x >= 0 && x < this.cols) this.bits[y][x] |= b
    }
    // An orthogonal polyline through [x, y] points; returns the cells it passes, in order.
    path(points) {
      const cells = []
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
    box(x, y, w, h, kind = 'round') {
      const [tl, hz, tr, v, bl, br] = SETS[kind]
      this.text(x, y, tl + hz.repeat(w - 2) + tr)
      for (let i = 1; i < h - 1; i++) this.set(x, y + i, v), this.set(x + w - 1, y + i, v)
      this.text(x, y + h - 1, bl + hz.repeat(w - 2) + br)
    }
    toString() {
      return this.ch.map((row, y) => row.map((c, x) => (c === ' ' && this.bits[y][x] ? JOIN[this.bits[y][x]] : c)).join('')).join('\n')
    }
  }

  // Redraw a <pre> on every tick until it leaves the page. fn(t) returns the text (or writes it).
  // Returns the painter, for the scenes that must also redraw when motion is frozen.
  A.scene = (pre, fn) => {
    let t = 0
    const paint = () => {
      const out = fn(t++)
      if (typeof out === 'string') pre.textContent = out
    }
    paint()
    if (!A.still) A.on(() => (pre.isConnected ? paint() : false))
    return paint
  }
  A.dither = (el) => (el.textContent = '░ '.repeat(12000))

  A.measure()
  document.fonts && document.fonts.ready.then(() => (A.measure(), A.scan()))
})()
