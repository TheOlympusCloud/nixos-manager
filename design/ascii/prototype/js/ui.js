// Prototype shell: sample fleet, component helpers, router and overlays.
// Markup only; all drawing and motion is in ascii.js.
;(function () {
  'use strict'
  const esc = A.esc
  const U = (window.U = {})

  // ---- Sample fleet (the one the intro screenshots show) -------------------------------------
  const D = (window.D = {
    machines: [
      { name: 'homeserver', sys: 'x86_64', rel: '26.05', pk: 17, sv: 10, us: 1, state: 'edit', kernel: '6.12.58', deploy: ['ok', 'Up to date'], sha: '4e1f9a2',
        feats: [['forgejo', 'services'], ['jellyfin', 'services'], ['nextcloud', 'services'], ['podman', 'virtualisation']] },
      { name: 'workstation', sys: 'x86_64', rel: '26.05', pk: 22, sv: 14, us: 1, state: 'edit', hm: 'P', kernel: '6.12.58', deploy: ['warn', '1 behind'], sha: 'b07c3d1',
        feats: [['desktopManager.plasma6', 'services'], ['pipewire', 'services'], ['printing', 'services'], ['steam', 'programs'], ['docker', 'virtualisation'], ['bluetooth', 'hardware']] },
      { name: 'pi', sys: 'aarch64', rel: '26.05', pk: 13, sv: 7, us: 1, state: 'code', kernel: '6.12.47', deploy: ['err', 'Build failed'], sha: 'b07c3d1',
        feats: [['home-assistant', 'services'], ['mosquitto', 'services']] },
      { name: 'oldbox', sys: 'x86_64', state: 'broken', error: 'infinite recursion encountered', deploy: ['off', 'Not reporting'] },
    ],
    shared: [['git', 'programs', 3], ['openssh', 'services', 3], ['fstrim', 'services', 2], ['zsh', 'programs', 3], ['tailscale', 'services', 3], ['comin', 'services', 3]],
    packages: [['bat', '0.25.0'], ['btop', '1.4.5'], ['curl', '8.14.1'], ['firefox', '144.0'], ['git', '2.51.0'], ['htop', '3.4.1'], ['jq', '1.8.1'], ['neovim', '0.11.4'], ['ripgrep', '14.1.1'], ['spotify', '1.2.74'], ['tmux', '3.5a'], ['wget', '1.25.0']],
    inputs: [['nixpkgs', 'github:NixOS/nixpkgs/nixos-26.05', '8f3ka1c', '74 days ago', 'err'], ['home-manager', 'github:nix-community/home-manager/release-26.05', '2d91e07', '31 days ago', 'warn'], ['sops-nix', 'github:Mic92/sops-nix', 'c41b7f2', '9 days ago', 'ok'], ['disko', 'github:nix-community/disko', '77a0e35', '4 days ago', 'ok'], ['comin', 'github:nlewo/comin', 'e90d6b8', '12 days ago', 'ok']],
    commits: [['4e1f9a2', 'homeserver: enable jellyfin', 'you', '12 min ago'], ['b07c3d1', 'workstation: add neovim, spotify', 'you', '2 h ago'], ['a3391fe', 'flake.lock: update nixpkgs', 'scheduled update', 'yesterday'], ['9c0d4b7', 'pi: open tcp/1883 for mosquitto', 'you', '3 days ago'], ['51e8a20', 'modules/common: fail2ban everywhere', 'you', '6 days ago'], ['0f7b9d3', 'add machine pi', 'you', '2 weeks ago']],
  })
  D.m = (name) => D.machines.find((m) => m.name === name)

  // ---- Glyph icons: the 61 names of Icon.svelte, one or two cells each -----------------------
  U.icons = {
    file: '▤', folder: '▸', 'file-plus': '▤+', 'folder-plus': '▸+', branch: '⌥', commit: '◉', check: '▷', tick: '✓',
    history: '↺', plus: '+', minus: '−', refresh: '↻', undo: '↶', trash: '⌫', edit: '✎', x: '×', right: '›', left: '‹',
    down: '▾', collapse: '«', play: '▶', fleet: '◈', server: '▣', package: '◫', services: '≋', sliders: '≔', diff: '±',
    code: 'λ', settings: '⚙', search: '⌕', user: '♟', users: '♟♟', input: '↧', layers: '≋', copy: '❐', external: '↗',
    alert: '▲', info: '◆', up: '↑', 'arrow-down': '↓', pull: '⇣', push: '⇡', logout: '⏏', key: '¤', sidebar: '◧',
    fit: '⇲', 'dir-right': '▸', 'dir-down': '▾', cpu: '▦', terminal: '›_', box: '□', disk: '◔', shield: '☗', globe: '◍',
    filter: '▽', grid: '▦', more: '⋯', activity: '♥', merge: '↣', deploy: '⇉', mark: '❄',
  }
  const ic = (U.ic = (name, cls = '') => `<i class="ic ${cls}" aria-hidden="true">${U.icons[name] || '?'}</i>`)

  // ---- Components ----------------------------------------------------------------------------
  // Status dot: ● ok, ◐ working (spins), ✖ failed, ○ silent.
  U.dot = (s) => (s === 'busy' ? '<i class="spin busy" data-k="dot">◐</i>' : `<i class="${s}">${{ ok: '●', err: '✖', off: '○', warn: '▲', info: '◆' }[s] || '●'}</i>`)
  U.badge = (tone, text, sigil) => `<span class="badge ${tone}">${sigil ?? { ok: '●', err: '✖', off: '○', warn: '▲', info: '◆', busy: '◐', acc: '▪' }[tone] ?? '▪'} ${esc(text)}</span>`
  U.state = (m) => (m.state === 'edit' ? U.badge('ok', 'Editable here', '✎') : m.state === 'code' ? U.badge('mut', 'Code only', 'λ') : U.badge('err', 'Does not evaluate'))
  U.chip = (text, group = '') => `<span class="chip ${group ? 'g-' + group : ''}">${esc(text)}</span>`
  U.btn = (label, cls = '', attrs = '') => `<button class="btn ${cls}" ${attrs}>${label}</button>`
  U.box = (o, inner) =>
    `<${o.tag || 'section'} class="${o.cls || ''}" data-frame="${o.frame || 'round'}"${o.title ? ` data-title="${esc(o.title)}"` : ''}${o.note ? ` data-tag="${esc(o.note)}"` : ''}${o.foot ? ` data-foot="${esc(o.foot)}"` : ''}${o.tone ? ` data-tone="${o.tone}"` : ''}${o.shadow ? ' data-shadow' : ''} ${o.attrs || ''}>${inner}</${o.tag || 'section'}>`
  U.big = (text, cls = '') => `<pre class="big ${cls}" data-big="${esc(text)}"></pre>`
  U.field = (label, attrs = '', value = '') => `<label class="field"><span>${label}</span><input type="text" value="${esc(value)}" ${attrs}></label>`
  U.chk = (label, on = false, attrs = '') => `<label class="chk"><input type="checkbox" ${on ? 'checked' : ''} ${attrs}><i></i><span>${label}</span></label>`
  U.sw = (label, on = false, cls = '') => `<label class="sw ${cls}"><input type="checkbox" role="switch" ${on ? 'checked' : ''}><i></i><span>${label}</span></label>`
  U.seg = (items, on = 0) => `<span class="seg">${items.map((t, i) => `<a class="${i === on ? 'on' : ''}">${t}</a>`).join('')}</span>`
  U.tabs = (items, on = 0) => `<nav class="tabs">${items.map(([t, n, href], i) => `<a class="${i === on ? 'on' : ''}" ${href ? `href="${href}"` : ''}>${t}${n != null ? ` <small>${n}</small>` : ''}</a>`).join('')}</nav>`
  U.kv = (pairs) => `<dl class="kv">${pairs.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join('')}</dl>`
  U.table = (head, rows, cls = '') =>
    `<div class="tbl ${cls}"><table><thead><tr>${head.map((h) => `<th${/^#/.test(h) ? ' class="num"' : ''}>${h.replace(/^#/, '')}</th>`).join('')}</tr></thead><tbody>${rows
      .map((r) => `<tr>${r.map((c, i) => `<td${/^#/.test(head[i] || '') ? ' class="num"' : ''}>${c}</td>`).join('')}</tr>`)
      .join('')}</tbody></table></div>`
  U.bar = (v, w = 20, live = false) => `<span class="bar" data-v="${v}" data-w="${w}"${live ? ' data-live' : ''}></span>`
  U.skel = (w) => `<span class="skel" data-w="${w}"></span>`
  U.spin = (k = 'tee') => `<i class="spin" data-k="${k}">┤</i>`
  U.steps = (names, at) => `<div class="steps">${names.map((s, i) => `<span class="${i === at ? 'on' : i < at ? 'done' : ''}">${i < at ? '✓ ' : ''}${s}</span>`).join('<i> ──── </i>')}</div>`
  U.head = (title, sub = '', tools = '') => `<header class="ph"><div>${U.big(title)}<p class="mut">${sub}</p></div><div class="tools">${tools}</div></header>`
  // A stat tile: the number in the pixel face, rolled up on arrival.
  U.tile = (label, n, note = '', tone = '') => U.box({ cls: 'tile', title: label }, `<pre class="big ${tone}" data-big="${n}"></pre><p class="mut">${note}</p>`)

  // ---- Shell ---------------------------------------------------------------------------------
  const NAV = [
    ['fleet', 'Fleet', 'fleet', 'f'], ['machines', 'Machines', 'server', 'm'], ['packages', 'Packages', 'package', 'p'], ['services', 'Services', 'services', 's'],
    ['options', 'Options', 'sliders', 'o'], ['health', 'Health', 'activity', 'a', '3', 'err'], ['deployments', 'Deployments', 'deploy', 'd'], ['inputs', 'Inputs', 'input', 'i'],
    ['changes', 'Changes', 'diff', 'c', '2', ''], ['history', 'History', 'history', 'h'], ['code', 'Code', 'code', 'e'],
  ]
  U.views = {}
  U.shell = () => `
    <div class="shell">
      <nav class="rail" data-frame="round" data-title="❄ nixos-manager">
        <a class="search" data-act="palette">${ic('search')} <span class="grow">Search…</span><kbd>⌃K</kbd></a>
        <div class="rule"></div>
        <div class="rows nav">${NAV.map(([id, label, icon, key, n, tone]) => `<a href="#/${id}" data-nav="${id}">${ic(icon)}<span class="grow">${label}</span>${n ? `<span class="count ${tone}" data-count="${id}">${n}</span>` : `<span class="key">g${key}</span>`}</a>`).join('')}</div>
        <div class="rail-end">
          <pre class="mini nofx scene" id="railart" aria-hidden="true"></pre>
          <div class="rows nav"><a href="#/settings" data-nav="settings">${ic('settings')}<span class="grow">Settings</span><span class="key">g,</span></a><a href="#/system" data-nav="system">${ic('grid')}<span class="grow">Design system</span></a></div>
        </div>
      </nav>
      <div class="main">
        <header class="top" data-frame="round">
          <h1 id="crumb"></h1>
          <span class="grow"></span>
          <a class="chipstate warn" data-act="model"><i class="spin" data-k="dot">◐</i> 3/4 OK</a><i class="sep">│</i>
          <a data-act="branch">${ic('branch')} main ${ic('down')}</a>
          <span class="mut">↑0 ↓0</span>
          ${U.btn(ic('pull') + ' Pull', '', 'data-act="pull"')}${U.btn(ic('push') + ' Push', '', 'data-act="push"')}<i class="sep">│</i>
          <a data-act="user"><span class="count">P</span> ${ic('down')}</a>
        </header>
        <main id="view"></main>
        <footer class="modeline nofx">
          <b class="mode">λ fleet/nixos</b><span>${ic('branch')} main</span><span><i class="err">✖</i> 1 <i class="warn">▲</i> 2 <i class="info">◆</i> 4</span>
          <span class="grow ticker" id="ticker"></span>
          <span class="mut"><kbd>⌃K</kbd> palette <kbd>g</kbd>+letter go <kbd>/</kbd> search <kbd>t</kbd> theme</span><span id="clock"></span>
        </footer>
      </div>
    </div>`

  // The view title is typed into the top bar; the page repaints top to bottom.
  function route() {
    const [name = 'intro', ...parts] = location.hash.replace(/^#\/?/, '').split('/').filter(Boolean)
    const bare = { boot: 1, intro: 1, login: 1 }[name]
    const app = document.getElementById('app')
    U.closeAll()
    if (bare) {
      app.innerHTML = `<div id="view" class="bare"></div>`
    } else if (!app.querySelector('.shell')) {
      app.innerHTML = U.shell()
      A.scan(app, true)
      railArt()
      ticker()
    }
    const v = U.views[name] || U.views.fleet
    const view = document.getElementById('view')
    view.className = (bare ? 'bare ' : '') + 'v-' + name
    view.scrollTop = 0
    view.innerHTML = v.render(parts)
    if (!bare) {
      for (const a of document.querySelectorAll('[data-nav]')) a.classList.toggle('on', a.dataset.nav === name)
      const crumb = document.getElementById('crumb')
      const title = v.title + (parts[0] && v.crumb !== false ? ' › ' + parts[0] : '')
      if (crumb.dataset.t !== title) (crumb.dataset.t = title), A.type(crumb, title, 2)
    }
    A.scan(view, true)
    if (v.sweep !== false) A.sweep(view)
    v.mount && v.mount(view, parts)
    for (const el of view.querySelectorAll('.tile .big')) el.dataset.big.match(/^\d+$/) && A.run(8, (p) => (el.textContent = A.big(String(Math.round(+el.dataset.big * (1 - (1 - p) ** 3))).padStart(el.dataset.big.length, '0'))))
  }
  U.go = (hash) => (location.hash === hash ? route() : (location.hash = hash))

  // A small snowflake idles at the foot of the rail: six λ arms, lit in turn.
  function railArt() {
    const pre = document.getElementById('railart')
    if (!pre) return
    const art = ['  ╲  λ  ╱  ', '   ╲ │ ╱   ', 'λ ───❄─── λ', '   ╱ │ ╲   ', '  ╱  λ  ╲  ']
    A.scene(pre, (t) => {
      const k = Math.floor(t / 4) % 6
      const lit = [[0, 5], [0, 8], [2, 10], [4, 8], [4, 5], [2, 0]][k]
      return art.map((row, y) => [...row].map((c, x) => (c === 'λ' && !(y === lit[0] && Math.abs(x - lit[1]) < 3) ? '·' : c)).join('')).join('\n')
    })
  }
  // The mode line carries a ticker of what the server last did.
  function ticker() {
    const el = document.getElementById('ticker')
    const lines = ['evaluated workstation in 4.1 s', 'comin: homeserver switched to 4e1f9a2', 'check 4e1f9a2 passed · built 3/3', 'pi: build of b07c3d1 failed', 'nixd ready', 'webhook: push to main by pablo']
    let i = 0
    const next = () => el.isConnected && A.type(el, '› ' + lines[i++ % lines.length], 2)
    next()
    A.on((t) => (el.isConnected ? (t % 75 === 0 && next(), true) : false))
    const clock = document.getElementById('clock')
    A.on((t) => (clock.isConnected ? (t % 12 === 0 && (clock.textContent = new Date().toTimeString().slice(0, 5)), true) : false))
    clock.textContent = new Date().toTimeString().slice(0, 5)
  }

  // ---- Overlays ------------------------------------------------------------------------------
  const layer = () => document.getElementById('overlays')
  U.closeAll = () => layer() && (layer().innerHTML = '')
  function open(html, cls = 'scrim') {
    const wrap = document.createElement('div')
    wrap.className = cls
    wrap.innerHTML = (cls === 'scrim' ? '<div class="dither nofx" aria-hidden="true"></div>' : '') + html
    layer().append(wrap)
    if (cls === 'scrim') A.dither(wrap.firstChild)
    A.scan(wrap, true)
    A.sweep(wrap, 6)
    const f = wrap.querySelector('[autofocus], input, button')
    f && f.focus()
    wrap.addEventListener('click', (e) => (e.target === wrap || e.target.closest('[data-close]')) && wrap.remove())
    return wrap
  }
  U.open = open
  // A dialog is the only double frame on screen. Danger: red frame, focus starts on Cancel.
  U.dialog = (o) =>
    open(
      U.box(
        { cls: 'modal', frame: 'double', title: o.title, tone: o.danger ? 'err' : '', shadow: true, attrs: 'role="dialog" aria-modal="true"' },
        `<p>${o.body}</p>${o.pre ? `<pre class="pre">${esc(o.pre)}</pre>` : ''}${o.input ? `<p>&nbsp;</p>${U.field('<b class="prompt">λ ›</b>', 'autofocus', o.input)}` : ''}
         <div class="actions">${U.btn('Cancel', '', 'data-close' + (o.danger ? ' autofocus' : ''))}${U.btn(o.ok || 'OK', o.danger ? 'danger' : 'primary', 'data-close')}</div>`,
      ),
    )
  // A toast is typed in; its bottom edge burns down to the moment it leaves.
  U.toast = (o) => {
    const el = document.createElement('div')
    const sig = { ok: '<i class="ok">✓</i>', error: '<i class="err">✖</i>', info: '<i class="info">◆</i>', busy: U.spin() }[o.kind || 'info']
    el.innerHTML = U.box({ cls: 'toast', shadow: true, tone: { ok: 'ok', error: 'err' }[o.kind] || '', attrs: 'role="status"' }, `<a class="x" data-x>×</a>${sig} <b>${esc(o.title || '')}</b><p class="msg"></p>${o.action ? `<p><a>${o.action}</a></p>` : ''}<span class="bar fuse"></span>`)
    const box = el.firstChild
    document.querySelector('.toasts').append(box)
    A.scan(box, true)
    A.type(box.querySelector('.msg'), o.text || '', 3)
    const life = o.kind === 'busy' ? 0 : o.action ? 100 : 62
    const fuse = box.querySelector('.fuse')
    const w = 40
    let left = life
    const leave = () => A.run(4, (p) => (box.style.opacity = 1 - p), () => box.remove())
    box.querySelector('[data-x]').onclick = leave
    if (life) A.on(() => (box.isConnected ? ((fuse.innerHTML = A.bar(left / life, w)), --left < 0 ? (leave(), false) : true) : false))
    if (A.still) fuse.innerHTML = A.bar(0.6, w)
    return box
  }
  U.menu = (anchor, html) => {
    U.closeAll()
    const r = anchor.getBoundingClientRect()
    const wrap = open(U.box({ cls: 'menu', frame: 'square', shadow: true, attrs: 'role="menu"' }, html), 'pop')
    const m = wrap.firstElementChild
    m.style.top = Math.round((r.bottom + A.lh) / A.lh) * A.lh + 'px'
    m.style.right = Math.max(2 * A.cw, innerWidth - r.right - 2 * A.cw) + 'px'
    setTimeout(() => document.addEventListener('click', function off(e) { if (!m.contains(e.target)) wrap.remove(), document.removeEventListener('click', off) }), 0)
  }
  U.drawer = (title, html) => {
    const wrap = document.createElement('div')
    wrap.className = 'drawer-wrap'
    wrap.innerHTML = `<div class="scrim inview" data-close></div>` + U.box({ cls: 'drawer', frame: 'heavy', title, note: 'Esc', tag: 'aside' }, html)
    document.getElementById('view').append(wrap)
    A.scan(wrap, true)
    A.sweep(wrap, 5)
    wrap.addEventListener('click', (e) => e.target.closest('[data-close]') && wrap.remove())
    return wrap
  }

  // ---- Keys ----------------------------------------------------------------------------------
  let pendingG = 0
  addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') return e.preventDefault(), U.palette()
    if (e.key === 'Escape') {
      const last = layer().lastElementChild || document.querySelector('.drawer-wrap')
      return last && last.remove()
    }
    if (e.target.closest('input, textarea, select')) return
    const keys = Object.fromEntries(NAV.map((n) => [n[3], n[0]]))
    keys[','] = 'settings'
    if (pendingG && Date.now() - pendingG < 1200 && keys[e.key]) (pendingG = 0), U.go('#/' + keys[e.key])
    else if (e.key === 'g') pendingG = Date.now()
    else if (e.key === 't') U.theme()
    else if (e.key === '/') {
      const box = document.querySelector('#view [data-search]')
      box && (e.preventDefault(), box.focus())
    }
  })
  U.theme = (name) => {
    const root = document.documentElement
    name = name || (root.dataset.theme === 'print' ? 'night' : 'print')
    name === 'print' ? (root.dataset.theme = 'print') : delete root.dataset.theme
  }

  addEventListener('hashchange', route)
  addEventListener('DOMContentLoaded', () => {
    const q = new URLSearchParams(location.search)
    if (q.get('theme')) U.theme(q.get('theme'))
    document.fonts.ready.then(() => {
      A.measure()
      route()
      const show = q.get('show')
      show && U.demo[show] && U.demo[show]()
    })
  })
  // Clicks on data-act run the matching demo action (menus, dialogs, toasts).
  U.demo = {}
  addEventListener('click', (e) => {
    const a = e.target.closest('[data-act]')
    if (a && U.demo[a.dataset.act]) e.preventDefault(), U.demo[a.dataset.act](a)
    const tab = e.target.closest('.tabs > a:not([href])')
    if (tab) {
      for (const t of tab.parentElement.children) t.classList.toggle('on', t === tab)
      A.ink(tab.parentElement)
    }
    const seg = e.target.closest('.seg > a')
    if (seg) for (const t of seg.parentElement.children) t.classList.toggle('on', t === seg)
  })
})()
