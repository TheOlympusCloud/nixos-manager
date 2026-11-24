// The bare screens: boot, the scroll story and login. The planet, its tiles, arcs, orbits, pulses
// and stars, and the blueprint, are all text, redrawn on every tick.
;(function () {
  'use strict'
  const { box, btn, big, field } = U
  const ASPECT = 2.02 // a cell is twice as tall as it is wide
  const hash = (a, b) => {
    const s = Math.sin(a * 127.1 + b * 311.7) * 43758.5453
    return s - Math.floor(s)
  }
  const dirChar = (dx, dy) => {
    const a = ((Math.atan2(dy, dx / ASPECT) * 180) / Math.PI + 360) % 180
    return a < 22.5 || a >= 157.5 ? '─' : a < 67.5 ? '╲' : a < 112.5 ? '│' : '╱'
  }
  const line = (buf, x1, y1, x2, y2, ch) => {
    const n = Math.max(Math.abs(x2 - x1), Math.abs(y2 - y1) * ASPECT, 1)
    for (let i = 0; i <= n; i++) buf.set(x1 + ((x2 - x1) * i) / n, y1 + ((y2 - y1) * i) / n, ch || dirChar(x2 - x1, y2 - y1))
  }

  // The snowflake: six lambdas about the centre, shaded by distance to their strokes and lit in
  // the logo's two blues (arms alternate between the `hot` and `globe` layers). It turns on the clock.
  function snowflake(L, cx, cy, R, angle) {
    const segs = []
    for (let k = 0; k < 6; k++) {
      const a = angle + (k * Math.PI) / 3
      const p = (r, t) => [Math.cos(t) * r, Math.sin(t) * r]
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
          if (d < best) (best = d), (tone = k)
        }
        const ch = best < 0.45 ? '█' : best < 0.75 ? '▓' : best < 1.05 ? '▒' : best < 1.35 ? '░' : ''
        if (ch) (tone ? L.hot : L.globe).set(x, y, ch)
      }
    }
  }

  // The planet: a lambert-shaded sphere whose surface is a dot matrix of packages. `scatter` lifts
  // the packages off it (drift); they come back as it returns to 0 (assemble).
  const RAMP = ' ··:;+=*#%@'
  function planet(L, cx, cy, R, rot, scatter = 0, dim = 1) {
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
        const lon = Math.atan2(dx / R, nz) + rot
        const lat = Math.asin(dy / R)
        const u = Math.floor(lon * 9)
        const v = Math.floor(lat * 9)
        const h = hash(u, v)
        const pkg = h > 0.45
        if (scatter > 0 && pkg && hash(v, u) < scatter * 0.8) {
          const far = 1 + scatter * (0.35 + hash(u + 3, v) * 1.5)
          warm.set(cx + dx * far * ASPECT + Math.sin(rot * 3 + u) * scatter * 3, cy + dy * far + Math.cos(rot * 2 + v) * scatter, h > 0.8 ? '▪' : '▫')
          continue
        }
        const b = Math.min(1, (lam * (pkg ? 0.5 + h * 0.6 : 0.22) + (1 - nz) ** 3 * 0.25) * dim)
        const ch = RAMP[Math.floor(b * (RAMP.length - 1))]
        ;(b > 0.62 ? hot : globe).set(x, y, ch)
      }
    }
  }
  function stars(buf, t) {
    for (let i = 0; i < 90; i++) {
      const x = Math.floor(hash(i, 1) * buf.cols)
      const y = Math.floor(hash(i, 2) * buf.rows)
      const tw = (t + i * 5) % 40
      buf.set(x, y, tw < 2 ? '✦' : tw < 4 ? '+' : hash(i, 3) > 0.6 ? '·' : '∙')
    }
    // A meteor now and then.
    const m = t % 90
    if (m < 14) for (let k = 0; k < 5; k++) buf.set(buf.cols * 0.2 + (m - k) * 4, 4 + (m - k), k ? '╲' : '●')
  }

  // ---- Boot ----------------------------------------------------------------------------------
  U.views.boot = {
    title: 'Boot',
    sweep: false,
    render: () => `<div class="stage nofx"><pre class="stars"></pre><pre class="globe" id="bootdark"></pre><pre class="hot" id="bootart"></pre><pre class="ink2" id="bootlog"></pre></div>`,
    mount(view) {
      const art = view.querySelector('#bootart')
      const log = view.querySelector('#bootlog')
      const cols = Math.floor(innerWidth / A.cw)
      const rows = Math.floor(innerHeight / A.lh)
      const POST = ['nixos-manager · tty0', '', '[ ok ] mount /nix/store', '[ ok ] open /var/lib/nixos-manager/fleet · main @ 4e1f9a2', '[ ok ] read flake.lock · 5 inputs', '[ .. ] nix eval .#nixosConfigurations']
      const hosts = ['homeserver', 'workstation', 'pi', 'oldbox']
      const hold = new URLSearchParams(location.search).has('hold')
      // With motion frozen the boot shows its last frame, then moves on like the animated one.
      if (A.still && !hold) setTimeout(() => location.hash === '#/boot' && U.go('#/fleet'), 1500)
      A.scene(art, (t) => {
        if (A.still) t = 69
        const b = new A.Buf(cols, rows)
        const dark = new A.Buf(cols, rows)
        const cx = Math.floor(cols / 2)
        const cy = Math.floor(rows * 0.4)
        // The mark turns; a halo of dots leaves it every 2 seconds.
        snowflake({ hot: b, globe: dark }, cx, cy, 10, t * 0.04)
        view.querySelector('#bootdark').textContent = dark.toString()
        const r = 12 + (t % 26) * 0.55
        const fade = '●•∙·· '[Math.min(5, Math.floor((t % 26) / 4.5))]
        for (let a = 0; a < 360; a += 4) {
          const y = cy + Math.sin((a * Math.PI) / 180) * r
          if (y < cy + 13) b.set(cx + Math.cos((a * Math.PI) / 180) * r * ASPECT, y, fade) // not through the log
        }
        const lines = POST.slice(0, Math.min(POST.length, 1 + Math.floor(t / 3)))
        const y0 = cy + 15
        const out = new A.Buf(cols, rows)
        lines.forEach((l, i) => out.text(cx - 30, y0 + i, l))
        if (t > 18)
          hosts.forEach((h, i) => {
            const p = Math.max(0, Math.min(1, (t - 18 - i * 5) / 14))
            const done = p >= 1
            out.text(cx - 23, y0 + lines.length + i, h.padEnd(12) + (done ? (i === 3 ? '✖ infinite recursion encountered' : '✓ ' + (3.2 + i * 0.5).toFixed(1) + ' s') : '█'.repeat(Math.floor(p * 24)) + '░'.repeat(24 - Math.floor(p * 24)) + ' ' + '┤┘┴└├┌┬┐'[t % 8]))
          })
        if (t > 56) out.text(cx - 30, y0 + lines.length + 5, 'Loading nixos-manager… 3/4 machines evaluated' + (t % 12 < 6 ? ' █' : ''))
        log.textContent = out.toString()
        if (t === 70 && !hold) U.go('#/fleet')
        return b.toString()
      })
      A.scene(view.querySelector('.stars'), (t) => {
        const b = new A.Buf(cols, rows)
        stars(b, t)
        // No stars behind the log.
        for (let y = Math.floor(rows * 0.4) + 14; y < rows; y++) for (let x = Math.floor(cols / 2) - 34; x < cols / 2 + 34; x++) b.ch[y][x] = ' '
        return b.toString()
      })
    },
  }

  // ---- The story -----------------------------------------------------------------------------
  const CH = [
    { id: 'hero', len: 1.5 }, { id: 'drift', len: 2.2, nav: 'The challenge' }, { id: 'assemble', len: 2.4, nav: 'The NixOS way' }, { id: 'planet', len: 1.8 },
    { id: 'orbit', len: 2, nav: 'The fleet' }, { id: 'blueprint', len: 4, nav: 'Blueprint' }, { id: 'console', len: 1.6, nav: 'Console' }, { id: 'deploy', len: 3, nav: 'Deploy' }, { id: 'outro', len: 1.6 },
  ]
  const mini = {
    fleet: '╭─────╮  ╭──╮\n│ ▣   ├─┬┤≋ │\n╰─────╯ │╰──╯\n╭─────╮ │╭──╮\n│ ▣   ├─┼┤≋ │\n╰─────╯ │╰──╯\n╭─────╮ │╭──╮\n│ ▣   ├─┴┤›_│\n╰─────╯  ╰──╯',
    machine: '█▀▀ █ █ ▀█▀\n● x86_64 ✎\n━━━━──────────\n╭──╮╭──╮╭──╮\n│22││14││ 1│\n╰──╯╰──╯╰──╯\n▌FEATURES\n ░░░ ░░░░ ░░',
    changes: ' [x] managed.json\n [ ] managed.json\n┌──────────────┐\n│+ "packages":[│\n│+   "neovim"  │\n│+ ]           │\n└──────────────┘\n ▐ Commit ▌',
    packages: 'λ › neo█\n──────────────\n[x] neovim  0.11\n[ ] neofetch 7.1\n[x] neovide 0.15\n░░░░░▒▓▒░░░░░░\n──────────────\n ▐ Add 2 ▌',
  }
  const COPY = {
    hero: () => `${big('Every machine.')}${big('One source', 'acc')}${big('of truth.', 'acc')}
      <p>A self-hosted console for every NixOS machine you own: evaluated from one flake, drawn as one graph, checked before anything ships.</p>
      <p class="row">${btn('Enter the console', 'primary', `onclick="location.hash='#/login'"`)}${btn('See how it works ↓', '', 'data-next')}</p>`,
    drift: () => `<p class="eyebrow warn">▌ The challenge</p>${big('Machines set up')}${big('by hand')}${big('drift apart.', 'warn')}
      <p>A version pinned here, a tweak forgotten there, an update that broke at two in the morning and cannot be undone.</p>
      <p>&nbsp;</p><pre class="art warn">openssl     3.0.13   ≠   1.1.1w\nkernel      6.12.58  ≠   6.1.0\npostgresql  16.4     ≠   13.2 · pinned by hand</pre>`,
    assemble: () => `<p class="eyebrow acc">▌ The NixOS way</p>${big('Declare it once.')}${big('Build it the same,', 'acc')}${big('everywhere.', 'acc')}
      <p>Every package is built from its inputs alone and stored under their hash: /nix/store/&lt;hash&gt;-name. Same inputs, same path, on every machine.</p>
      <p>&nbsp;</p><p class="mut">From 100 000+ packages in nixpkgs</p><pre class="big acc nofx" id="counter"></pre><p class="mut" id="counter-cap">packages</p>`,
    planet: () => `${big('Every package')}${big('is a node.')}${big('Every machine,', 'acc')}${big('a closure.', 'acc')}
      <p>Dependencies are edges in one graph. A machine is the set of everything it reaches: its closure.</p>`,
    orbit: () => `<p class="eyebrow acc">▌ The fleet</p>${big('Your machines,')}${big('in orbit.', 'acc')}
      <p>&nbsp;</p><pre class="art nofx"><b>▣ workstation</b>  x86_64-linux\n  desktop · plasma · firefox · steam\n\n<b>▣ homeserver</b>   x86_64-linux\n  forgejo · jellyfin · nextcloud\n\n<b>▣ pi</b>           aarch64-linux\n  home-assistant · mosquitto</pre>`,
    blueprint: () => `<p class="eyebrow acc">▌ The blueprint</p><h3>Drawn, connected, checked.</h3><p class="mut" style="margin-top:0">The configuration graph of the fleet, evaluated node by node.</p>`,
    console: () => `<p class="eyebrow acc">▌ The console</p>${big('Visual.')}${big('Reliable.')}${big('Yours.', 'acc')}
      <p>Click to change a machine; the console writes managed.json, checks the flake and shows what every machine will run before you commit.</p>
      <p>&nbsp;</p><pre class="art nofx">◈ Fleet graph            ✎ Edit by clicking\n▷ Checked before commit  ± Changes in NixOS terms\n↺ History and restore    λ Agents via MCP</pre>
      <p class="mut">1 repository · every machine evaluated · 0 unchecked pushes · 1 click restore</p>`,
    deploy: () => `<p class="eyebrow acc">▌ The round trip</p>${big('A change,')}${big('from click', 'acc')}${big('to switch.', 'acc')}
      <p>Commits land on your own Forgejo. Each machine pulls with comin and switches by itself.</p><p>&nbsp;</p><pre class="art nofx" id="rail"></pre>`,
    outro: () => `<div style="text-align:center">${big('Your fleet,')}${big('declared.', 'acc')}<p style="margin-inline:auto">One repository. Every machine evaluated. No unchecked push. The console is a login away.</p><p>&nbsp;</p><p>${btn('Enter the console', 'primary', `onclick="location.hash='#/login'"`)}</p></div>`,
  }
  const RAIL = [['Edit', 'homeserver: +jellyfin · port tcp/8096'], ['Check', 'nix flake check · built 3/3 · passed'], ['Commit', '4e1f9a2 · homeserver: enable jellyfin'], ['Push', 'main → forgejo · fleet/nixos'], ['Pull', 'comin · fetched 4e1f9a2 on 3 machines'], ['Switch', 'workstation ✓ · homeserver ✓ · pi ✓']]

  // The blueprint: flake → hosts → modules → services, drawn, evaluated, broken, fixed, stamped.
  function blueprint(L, p, t) {
    const { ink, hot, bad, good } = L
    const cols = ink.cols
    const ox = Math.max(4, Math.floor((cols - 150) / 2))
    const oy = 12
    const N = {
      flake: [ox, oy + 14, 'flake.nix'],
      ws: [ox + 26, oy + 4, 'hosts/workstation'], hs: [ox + 26, oy + 14, 'hosts/homeserver'], pi: [ox + 26, oy + 24, 'hosts/pi'],
      common: [ox + 60, oy + 1, 'modules/common.nix'], desktop: [ox + 60, oy + 7, 'modules/desktop.nix'], hsm: [ox + 60, oy + 15, 'homeserver/managed.json'], pim: [ox + 60, oy + 24, 'pi/managed.json'],
      openssh: [ox + 100, oy, 'openssh'], tailscale: [ox + 100, oy + 4, 'tailscale'], firefox: [ox + 100, oy + 8, 'firefox'], jellyfin: [ox + 100, oy + 12, 'jellyfin'], nextcloud: [ox + 100, oy + 16, 'nextcloud'], forgejo: [ox + 100, oy + 20, 'forgejo'], ha: [ox + 100, oy + 24, 'home-assistant'],
    }
    const E = [['flake', 'ws'], ['flake', 'hs'], ['flake', 'pi'], ['ws', 'common'], ['ws', 'desktop'], ['hs', 'common'], ['hs', 'hsm'], ['pi', 'common'], ['pi', 'pim'], ['common', 'openssh'], ['common', 'tailscale'], ['desktop', 'firefox'], ['hsm', 'jellyfin'], ['hsm', 'nextcloud'], ['hsm', 'forgejo'], ['pim', 'ha']]
    const act = p < 0.22 ? 0 : p < 0.48 ? 1 : p < 0.64 ? 2 : p < 0.84 ? 3 : 4
    const w = (n) => N[n][2].length + 6
    const draw = new A.Buf(cols, ink.rows)
    for (const [a, b] of E) {
      const [ax, ay] = N[a]
      const [bx, by] = N[b]
      const mid = bx - 4
      draw.path([[ax + w(a), ay + 1], [mid, ay + 1], [mid, by + 1], [bx - 1, by + 1]])
    }
    const piBroken = act === 2 || (act === 3 && p < 0.74)
    for (const k in N) {
      const [x, y, label] = N[k]
      const target = k === 'pi' && piBroken ? bad : draw
      target.box(x, y, w(k), 3, k === 'flake' ? 'double' : 'round')
      target.text(x + 2, y + 1, label)
    }
    // Act 1 draws the sheet left to right; from then on it is all there.
    const reveal = act === 0 ? Math.floor((p / 0.22) * cols) : cols
    const text = draw.toString().split('\n')
    text.forEach((l, y) => ink.text(0, y, l.slice(0, reveal)))
    // Act 2: a scan line crosses the sheet and ticks every node it passes.
    const scan = act === 1 ? ox + Math.floor(((p - 0.22) / 0.26) * 130) : act > 1 ? cols : -1
    if (act === 1) for (let y = oy - 1; y < oy + 28; y++) hot.set(scan, y, '┃'), hot.set(scan - 1, y, '▒'), hot.set(scan - 2, y, '░')
    for (const k in N) {
      const [x, y] = N[k]
      if (x + w(k) < scan && !(k === 'pi' && act >= 2 && p < 0.74) && !((k === 'pim' || k === 'ha') && act >= 2 && p < 0.74)) good.set(x + w(k) - 2, y + 1, '✓')
    }
    if (piBroken) {
      bad.set(N.pi[0] + w('pi') - 2, N.pi[1] + 1, '✖')
      const cx = N.pi[0] - 4
      const cy = N.pi[1] + 4
      bad.box(cx, cy, 48, 4, 'heavy')
      bad.text(cx + 2, cy + 1, 'error: infinite recursion encountered')
      bad.text(cx + 2, cy + 2, '  imports = [ ./default.nix ];' + (t % 8 < 4 ? ' ◂' : ''))
    }
    if (act === 3) {
      const cx = N.pi[0] + 48
      const cy = N.pi[1] + 4
      good.box(cx, cy, 46, 5, 'round')
      good.text(cx + 2, cy, ' patch · hosts/pi/default.nix ')
      bad.text(cx + 2, cy + 1, '- imports = [ ./default.nix ];')
      good.text(cx + 2, cy + 2, '+ imports = [ ./hardware-configuration.nix ];'.slice(0, Math.floor(((p - 0.64) / 0.08) * 44)))
      if (p > 0.74) good.text(cx + 2, cy + 3, 're-evaluating pi… ok')
    }
    if (act === 4) {
      const stamp = (x, y, s) => (good.box(x, y, s.length + 6, 3, 'double'), good.text(x + 3, y + 1, s))
      stamp(ox + 22, oy + 30, 'NIX FLAKE CHECK · PASSED')
      if (p > 0.9) stamp(ox + 58, oy + 30, 'BUILT · 3 MACHINES')
    }
    // Drafting furniture: the annotations and the title block.
    ink.text(ox, oy - 2, `${String(act + 1).padStart(2, '0')} / 05  ${['DRAW THE GRAPH', 'EVALUATE EVERY NODE', 'ONE MACHINE FAILS', 'FIX AND RE-EVALUATE', 'CHECKED'][act]}`)
    ink.text(ox + 60, oy - 2, 'evaluation · flake → hosts → modules → packages')
    const tb = [ink.cols - 52, ink.rows - 9]
    ink.box(tb[0], tb[1], 48, 6, 'square')
    ;['NIXOS-MANAGER · FLEET CONFIGURATION', 'DRAWN  nix eval      CHECK  nix flake check', 'REV    4e1f9a2       SHEET  1 of 1', `STATUS ${['DRAFT', 'EVALUATING', 'REJECTED', 'REVISED', 'APPROVED'][act]}`].forEach((s, i) => ink.text(tb[0] + 2, tb[1] + 1 + i, s))
  }

  U.views.intro = {
    title: 'Intro',
    sweep: false,
    render: () => `
      <nav class="introbar"><b>❄ nixos-manager</b><span class="grow"></span>${CH.filter((c) => c.nav).map((c) => `<a data-ch="${c.id}">${c.nav}</a>`).join('')}${btn('Enter the console', 'primary', `onclick="location.hash='#/login'"`)}</nav>
      <div id="story" style="height:${CH.reduce((s, c) => s + c.len, 0) * 100 + 100}vh">
        <div class="stage">
          <pre class="stars nofx"></pre><pre class="globe nofx"></pre><pre class="hot nofx"></pre><pre class="warm nofx"></pre><pre class="ink2 nofx"></pre><pre class="bad nofx"></pre><pre class="good nofx"></pre>
          <div class="copy" id="copy"></div><div id="show"></div>
        </div>
      </div>
      <div class="introfoot nofx"><span id="meta">fleet · 3 machines · 1 flake</span><span class="grow"></span><span id="prog"></span><span>/nix/store/8f3ka1c…-nixos-system-homeserver-26.05</span></div>`,
    mount(view) {
      const names = ['stars', 'globe', 'hot', 'warm', 'ink2', 'bad', 'good']
      const pres = Object.fromEntries(names.map((n) => [n, view.querySelector('.stage > .' + n)]))
      const copy = view.querySelector('#copy')
      const show = view.querySelector('#show')
      const cols = Math.floor(innerWidth / A.cw)
      const rows = Math.floor(innerHeight / A.lh)
      const vh = innerHeight
      let current = -1
      const where = () => {
        let y = view.scrollTop / vh
        for (let i = 0; i < CH.length; i++) {
          if (y < CH[i].len || i === CH.length - 1) return [i, Math.min(1, y / CH[i].len)]
          y -= CH[i].len
        }
      }
      const jump = (i, p = 0) => (view.scrollTop = (CH.slice(0, i).reduce((s, c) => s + c.len, 0) + CH[i].len * p) * vh)
      view.addEventListener('click', (e) => {
        const a = e.target.closest('[data-ch]')
        if (a) jump(CH.findIndex((c) => c.id === a.dataset.ch), 0.05)
        if (e.target.closest('[data-next]')) jump(1, 0.05)
      })
      const q = new URLSearchParams(location.search)
      if (q.get('ch')) jump(+q.get('ch'), +(q.get('p') || 0.4))
      U.introJump = jump

      const paint = A.scene(pres.ink2, (t) => {
        const [i, p] = where()
        const id = CH[i].id
        if (i !== current) {
          current = i
          copy.className = 'copy' + (id === 'outro' ? ' mid' : id === 'blueprint' ? ' bp' : '')
          copy.innerHTML = COPY[id]()
          show.innerHTML = id === 'console' ? `<div class="showcase">${[['fleet', 'fleet · services & programs'], ['machine', 'machines / workstation'], ['changes', 'changes · before commit'], ['packages', 'packages · search nixpkgs']].map(([k, cap]) => box({ shadow: true, foot: cap, cls: 'raised' }, `<pre class="art nofx">${mini[k]}</pre>`)).join('')}</div>` : ''
          A.scan(view, true)
          A.sweep(copy, 5)
          for (const a of view.querySelectorAll('[data-ch]')) a.classList.toggle('on', a.dataset.ch === id)
        }
        const L = Object.fromEntries(names.map((n) => [n, new A.Buf(cols, rows)]))
        L.ink = L.ink2
        if (id !== 'blueprint' && id !== 'console') stars(L.stars, t)
        const R0 = rows * 0.62
        const rot = t * 0.03
        // One keyframe per chapter: where the planet sits and how big it is.
        if (id === 'hero') planet(L, cols * 0.72, rows * 1.02 - p * 6, R0 * 1.25, rot)
        else if (id === 'drift') planet(L, cols * 0.75, rows * 0.56, R0 * 0.66, rot, Math.sin(Math.min(1, p * 1.25) * Math.PI * 0.5) * 0.9)
        else if (id === 'assemble') {
          const s = Math.max(0, 0.9 * (1 - p * 1.6))
          planet(L, cols * 0.75, rows * 0.56, R0 * 0.66, rot, s)
          const n = p > 0.92 ? 1 : Math.max(1, Math.round(100000 * Math.exp(-p * 12.5)))
          const c = copy.querySelector('#counter')
          if (c) (c.textContent = A.big(String(n).replace(/\B(?=(\d{3})+$)/g, ' '))), (copy.querySelector('#counter-cap').textContent = n === 1 ? 'graph' : 'packages')
          if (s === 0 && p < 0.75) for (let a = 0; a < 360; a += 3) L.hot.set(cols * 0.75 + Math.cos(a) * (R0 * 0.66 + (p - 0.56) * 60) * ASPECT, rows * 0.56 + Math.sin(a) * (R0 * 0.66 + (p - 0.56) * 60), '∙')
        } else if (id === 'planet' || id === 'orbit' || id === 'deploy' || id === 'outro') {
          const R = id === 'outro' ? R0 * (0.5 - p * 0.12) : id === 'planet' ? R0 * 0.7 : R0 * 0.46
          const cx = id === 'outro' ? cols * 0.5 : cols * 0.74
          const cy = id === 'outro' ? rows * 0.78 : rows * 0.52
          planet(L, cx, cy, R, rot, 0, id === 'outro' ? 0.7 : 1)
          if (id === 'planet') {
            // Dependency arcs: great circles on the near side, each carrying packets of light.
            ;[0.28, -0.5, 0.78].forEach((k, j) => {
              for (let a = 0; a < 180; a += 2) {
                const ph = (a * Math.PI) / 180
                const x = cx + Math.cos(ph) * R * ASPECT
                const y = cy + Math.sin(ph) * R * k
                const packet = (a + t * 4 + j * 50) % 60 < 4
                ;(packet ? L.ink2 : L.hot).set(x, y, packet ? '●' : '∙')
              }
            })
          } else if (id !== 'outro') {
            // Orbits, and a machine on each; a closure beam ties the machine to the planet.
            const M = [['workstation', 1.35, 0.4, 0], ['homeserver', 1.65, 0.5, 2.1], ['pi', 1.95, 0.6, 4.2]]
            const forge = [cols * 0.5, rows * 0.16]
            M.forEach(([name, a, b, ph], j) => {
              for (let d = 0; d < 360; d += 3) {
                const r = (d * Math.PI) / 180
                const x = cx + Math.cos(r) * R * a * ASPECT
                const y = cy + Math.sin(r) * R * a * b
                if (Math.hypot((x - cx) / ASPECT, y - cy) > R + 0.5 || Math.sin(r) > 0) L.globe.set(x, y, '·')
              }
              const th = ph + t * (0.02 - j * 0.004)
              const mx = cx + Math.cos(th) * R * a * ASPECT
              const my = cy + Math.sin(th) * R * a * b
              if (id === 'orbit') {
                const d0 = (R + 1.5) / Math.hypot((mx - cx) / ASPECT, my - cy)
                line(L.globe, cx + (mx - cx) * d0, cy + (my - cy) * d0, mx, my, '┄')
              }
              // A label clears the cells under it, so it reads when it crosses the planet.
              for (const n of ['globe', 'hot', 'stars']) L[n].text(Math.round(mx) - 2, Math.round(my), ' '.repeat(name.length + 6))
              L.ink2.text(Math.round(mx) - 1, Math.round(my), '▣ ' + name)
              if (id === 'deploy') {
                // Commit pulses leave Forgejo for each machine in turn once the push step is reached.
                const step = Math.floor(p * 6.2)
                if (step >= 4) {
                  const k = ((t * 0.06 + j * 0.33) % 1 + 1) % 1
                  const px = forge[0] + (mx - forge[0]) * k
                  const py = forge[1] + (my - forge[1]) * k - Math.sin(k * Math.PI) * 5
                  L.warm.set(px, py, '●'), L.warm.set(px - 2, py, '∙')
                }
                if (step >= 5) L.good.text(Math.round(mx) + name.length + 2, Math.round(my), '✓')
              }
            })
            if (id === 'deploy') L.ink2.text(Math.round(forge[0]) - 4, Math.round(forge[1]), '⌥ forge'), L.hot.box(Math.round(forge[0]) - 6, Math.round(forge[1]) - 1, 13, 3)
          }
        } else if (id === 'blueprint') blueprint(L, p, t)
        if (id === 'deploy') {
          const rail = copy.querySelector('#rail')
          const at = p * 6.2
          if (rail) rail.innerHTML = RAIL.map(([s, d], k) => (k < Math.floor(at) ? `<span class="ok">●</span> <b>${s.padEnd(7)}</b> <span class="mut">${d}</span>\n<span class="ok">┃</span>` : k === Math.floor(at) ? `<span class="acc">${'◐◓◑◒'[t % 4]}</span> <b>${s.padEnd(7)}</b> <span class="mut">${d.slice(0, Math.floor((at % 1) * d.length * 1.6))}</span>\n│` : `<span class="faint">○ ${s}</span>\n<span class="faint">┆</span>`)).join('\n')
        }
        // The scene stays clear of the two bars, and loose marks stay out from under the copy.
        for (const n of names) {
          const b = L[n]
          for (let y = 0; y < rows; y++) {
            const bar = y < 3 || y >= rows - 3
            const under = (n === 'stars' || n === 'warm') && id !== 'blueprint' && id !== 'outro' && y > 4 && y < rows * 0.7
            if (bar) b.ch[y].fill(' '), b.bits[y].fill(0)
            else if (under) for (let x = 0; x < Math.min(cols, 84); x++) b.ch[y][x] = ' '
          }
        }
        for (const n of names) if (n !== 'ink2') pres[n].textContent = L[n].toString()
        view.querySelector('#prog').textContent = `${String(i + 1).padStart(2, '0')}/${String(CH.length).padStart(2, '0')} ` + '━'.repeat(Math.round(((i + p) / CH.length) * 24)).padEnd(24, '─') + (i === 0 ? '  scroll ↓' : '')
        return L.ink2.toString()
      })
      // With motion frozen nothing ticks, so scrolling itself redraws the chapter.
      if (A.still) view.addEventListener('scroll', paint)
    },
  }

  // ---- Login ---------------------------------------------------------------------------------
  U.views.login = {
    title: 'Login',
    sweep: false,
    render: () => `<div class="stage nofx"><pre class="stars"></pre><pre class="globe"></pre><pre class="hot"></pre><pre class="warm"></pre></div>
      <div class="scrim" style="background:none">${box({ cls: 'modal', frame: 'double', shadow: true, title: '❄ nixos-manager', attrs: 'role="dialog" style="width:58ch"' }, `
        <pre class="art nofx" style="text-align:center">╲  λ  ╱\n ╲ │ ╱ \nλ ──❄── λ\n ╱ │ ╲ \n╱  λ  ╲</pre><div class="sp"></div>
        <p style="text-align:center" class="mut">Your fleet's NixOS configuration, in one place.</p><div class="sp"></div>
        <label class="field"><b class="prompt">Password ›</b><input type="password" value="correcthorse" autofocus></label><div class="sp"></div>
        <p class="err" hidden>✖ Wrong password. 4 tries left before a 30 s pause.</p>
        <div class="actions"><a href="#/intro" class="mut" style="margin-right:auto">‹ Back to the story</a>${btn('Log in', 'primary', `onclick="location.hash='#/boot'"`)}</div>`)}</div>`,
    mount(view) {
      const pres = Object.fromEntries(['stars', 'globe', 'hot', 'warm'].map((n) => [n, view.querySelector('.stage > .' + n)]))
      const cols = Math.floor(innerWidth / A.cw)
      const rows = Math.floor(innerHeight / A.lh)
      A.scene(pres.globe, (t) => {
        const L = Object.fromEntries(Object.keys(pres).map((n) => [n, new A.Buf(cols, rows)]))
        stars(L.stars, t)
        planet(L, cols * 0.5, rows * 1.25, rows * 0.75, t * 0.03, 0, 0.8)
        pres.stars.textContent = L.stars.toString()
        pres.hot.textContent = L.hot.toString()
        return L.globe.toString()
      })
      A.sweep(view.querySelector('.modal'), 5)
    },
  }
})()
