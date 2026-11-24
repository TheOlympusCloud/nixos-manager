// Fleet: the graph on the character grid. Nodes are framed elements placed by cell; the wires are
// text, routed at right angles, and packets run along them from each machine to what it enables.
;(function () {
  'use strict'
  const { ic, box, chip, btn, seg, dot, kv, badge } = U

  const MW = 38
  const PX = 74
  const PW = 24

  function layout() {
    let y = 1
    const machines = D.machines.map((m, i) => {
      const h = m.state === 'broken' ? 5 : 6 + Math.ceil(m.feats.length / 3)
      const n = { m, i, x: 4, y, w: MW, h }
      y += h + 1
      return n
    })
    const pills = D.shared.map(([name, group, n], j) => ({ name, group, n, x: PX, y: 3 + j * 4, w: PW, h: 3 }))
    const edges = []
    for (const a of machines) {
      if (a.m.state === 'broken') continue
      for (const p of pills) {
        if (p.name === 'fstrim' && a.m.name === 'pi') continue
        const cx = a.x + a.w + 5 + a.i * 4
        edges.push({ a, p, pts: [[a.x + a.w, a.y + 2], [cx, a.y + 2], [cx, p.y + 1], [p.x - 1, p.y + 1]] })
      }
    }
    return { machines, pills, edges }
  }

  const machineNode = (n) => {
    const m = n.m
    const pos = `style="left:${n.x}ch;top:calc(var(--lh)*${n.y});width:${n.w}ch;height:calc(var(--lh)*${n.h})"`
    if (m.state === 'broken')
      return box({ cls: 'node lift', title: m.name, tone: 'err', attrs: `${pos} data-node="${m.name}" tabindex="0"` }, `<p class="err">${m.error}</p><p>&nbsp;</p><p>${U.state(m)}</p>`)
    return box(
      { cls: 'node lift', title: m.name, note: m.sys, attrs: `${pos} data-node="${m.name}" tabindex="0"` },
      `<p>${dot('ok')} NixOS ${m.rel} <span class="right">${m.hm ? `<span class="count">${m.hm}</span>` : ''}</span></p>
       <p>${ic('package', 'mut')} ${m.pk}  ${ic('services', 'mut')} ${m.sv}  ${ic('user', 'mut')} ${m.us}   ${U.state(m)}</p>
       <div class="rule dash"></div>
       <div class="chips">${m.feats.slice(0, 5).map(([f, g]) => chip(f.replace('desktopManager.', ''), g)).join('')}${m.feats.length > 5 ? chip('+' + (m.feats.length - 5)) : ''}</div>`,
    )
  }
  const pillNode = (p) =>
    box(
      { cls: 'node lift', frame: 'square', attrs: `style="left:${p.x}ch;top:calc(var(--lh)*${p.y});width:${p.w}ch;height:calc(var(--lh)*3)" data-node="${p.name}" tabindex="0"` },
      `<span class="row" style="flex-wrap:nowrap"><i class="g-${p.group}">${p.group === 'programs' ? '›_' : '≋'}</i><b class="grow">${p.name}</b><span class="mut">${p.n}</span></span>`,
    )

  U.views.fleet = {
    title: 'Fleet',
    render: () => `
      <div class="fleet-bar">
        <label class="field" style="width:30ch"><span>${ic('search')}</span><input type="search" placeholder="Search the fleet…" data-search></label>
        ${seg(['≋ Services', '▤ Files', '◫ Packages', '♟ Users', '↧ Inputs'])}
        <span class="grow"></span>
        <a href="#/health"><i class="err">✖</i> 1 error <i class="warn">▲</i> 2 warnings <i class="info">◆</i> 4 suggestions</a>
        ${btn('⇣', 'ghost', 'title="Download"')}${btn('⇲', 'ghost', 'title="Full screen"')}
      </div>
      <div class="canvas nofx" id="canvas">
        <pre class="dots" aria-hidden="true"></pre><pre class="wires" aria-hidden="true"></pre><pre class="lit" aria-hidden="true"></pre><pre class="packets" aria-hidden="true"></pre>
      </div>`,
    mount(view) {
      const canvas = view.querySelector('#canvas')
      const [dots, wires, lit, packets] = canvas.querySelectorAll('pre')
      const g = layout()
      canvas.insertAdjacentHTML(
        'beforeend',
        g.machines.map(machineNode).join('') +
          g.pills.map(pillNode).join('') +
          box({ cls: 'zoom', frame: 'square' }, `<a>+</a> <a>−</a> <a>⇲</a>`) +
          box({ cls: 'minimap', frame: 'square', title: 'map' }, `▛▀▀▜    ▄▄\n▌  ▐ ╶──▄▄\n▛▀▀▜ ╶──▄▄\n▌  ▐ ╶──▄▄\n▛▀▜  ╶──▄▄\n▙▄▟     ▄▄`),
      )
      A.scan(canvas, true)
      A.sweep(canvas, 12)

      const cols = Math.ceil(canvas.clientWidth / A.cw)
      const rows = Math.ceil(canvas.clientHeight / A.lh)
      // The dot grid: a mark every 4 columns and 2 rows, a cross every 16 and 8.
      const grid = new A.Buf(cols, rows)
      for (let y = 0; y < rows; y += 2) for (let x = 0; x < cols; x += 4) grid.set(x, y, y % 8 === 0 && x % 16 === 0 ? '+' : '·')
      dots.textContent = grid.toString()

      const base = new A.Buf(cols, rows)
      for (const e of g.edges) e.cells = base.path(e.pts)
      // Wires are drawn in one after the other, then stay.
      const full = base.toString().split('\n')
      A.run(10, (p) => (wires.textContent = full.map((l) => l.slice(0, Math.round(cols * p))).join('\n')))

      let hot = null
      const paint = (t) => {
        const on = g.edges.filter((e) => hot && (e.a.m.name === hot || e.p.name === hot))
        const hi = new A.Buf(cols, rows)
        for (const e of on) hi.path(e.pts)
        lit.textContent = hi.toString()
        const pk = new A.Buf(cols, rows)
        // One packet per wire at rest; lit wires carry a train of them, twice as fast.
        g.edges.forEach((e, i) => {
          const active = on.includes(e)
          if (hot && !active) return
          const n = e.cells.length
          const trains = active ? [0, 4, 8, 12] : [0]
          for (const off of trains) {
            const at = (t * (active ? 2 : 1) + i * 7 + off) % (n + (active ? 0 : 30))
            if (at < n) pk.set(e.cells[at][0], e.cells[at][1], off ? '∙' : '•')
          }
        })
        packets.textContent = pk.toString()
      }
      A.scene(packets, paint)

      const setHot = (name) => {
        hot = name
        for (const el of canvas.querySelectorAll('.node')) {
          const id = el.dataset.node
          const linked = !hot || id === hot || g.edges.some((e) => (e.a.m.name === hot && e.p.name === id) || (e.p.name === hot && e.a.m.name === id))
          el.classList.toggle('dim', !linked)
        }
      }
      canvas.addEventListener('mouseover', (e) => {
        const n = e.target.closest('.node')
        setHot(n ? n.dataset.node : null)
      })
      canvas.addEventListener('click', (e) => {
        const n = e.target.closest('.node')
        const old = canvas.querySelector('.inspector')
        old && old.remove()
        if (!n) return
        const m = D.m(n.dataset.node)
        const pill = g.pills.find((p) => p.name === n.dataset.node)
        canvas.insertAdjacentHTML(
          'beforeend',
          box(
            { cls: 'inspector', frame: 'heavy', title: 'Inspector', note: '× Esc', tag: 'aside' },
            m
              ? `${U.big(m.name)}<p>${U.state(m)}</p><div class="sp"></div>${m.state === 'broken' ? `<p class="err">error: ${m.error}</p>` : kv([['System', m.sys + '-linux'], ['NixOS', m.rel], ['Kernel', m.kernel], ['Packages', m.pk], ['Services', m.sv], ['Deployment', badge(m.deploy[0], m.deploy[1])]])}
                 <div class="sp"></div><div class="row">${btn('Open machine', 'primary', `onclick="location.hash='#/machines/${m.name}'"`)}${btn('Options')}</div>`
              : `${U.big(pill.name)}<p class="g-${pill.group}">▪ ${pill.group}</p><div class="sp"></div><p>Enabled on ${pill.n} of 4 machines.</p><div class="sp"></div>
                 <div class="rows">${g.edges.filter((e) => e.p === pill).map((e) => `<a href="#/machines/${e.a.m.name}">${dot('ok')}<span class="grow">${e.a.m.name}</span><span class="mut">modules/common.nix</span></a>`).join('')}</div>
                 <div class="sp"></div><div class="row">${btn('Open the options')}</div>`,
          ),
        )
        const ins = canvas.querySelector('.inspector')
        A.scan(ins, true)
        A.sweep(ins, 5)
      })
      view.querySelector('[data-search]').addEventListener('input', (e) => {
        const q = e.target.value.trim().toLowerCase()
        for (const el of canvas.querySelectorAll('.node')) el.classList.toggle('dim', !!q && !el.dataset.node.includes(q))
      })
      if (new URLSearchParams(location.search).get('hot')) setHot(new URLSearchParams(location.search).get('hot'))
    },
  }
})()
