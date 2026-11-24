// The living style guide (#/system): palette with measured contrast, type, frames, glyphs, every
// component in every state, and each animation with a replay button.
;(function () {
  'use strict'
  const { ic, box, chip, btn, seg, dot, kv, badge, table, tabs, field, chk, sw, bar, skel, spin, big } = U

  const lum = (rgb) => {
    const [r, g, b] = rgb.map((v) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
    return 0.2126 * r + 0.7152 * g + 0.0722 * b
  }
  const rgbOf = (name) => {
    const probe = document.createElement('i')
    probe.style.color = `var(${name})`
    document.body.append(probe)
    const m = getComputedStyle(probe).color.match(/[\d.]+/g).slice(0, 3).map(Number)
    probe.remove()
    return m
  }
  const contrast = (a, b) => {
    const [x, y] = [lum(rgbOf(a)), lum(rgbOf(b))].sort((p, q) => q - p)
    return (x + 0.05) / (y + 0.05)
  }
  U.contrast = contrast
  const hex = (name) => '#' + rgbOf(name).map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')
  const swatch = (name, use, on = '--bg') => {
    const text = /^--(fg|accent$|accent-strong|green|red|yellow|blue|purple|cyan|g-|danger$|line-strong)/.test(name)
    const c = text ? contrast(name, on) : 0
    return `<div class="swatch"><i style="background:var(${name})"></i><b>${name.slice(2)}</b> <span class="mut">${hex(name)}</span><br><span class="mut">${use}</span>${text ? `<br><span class="${c >= 4.5 ? 'ok' : c >= 3 ? 'warn' : 'err'}">${c.toFixed(1)}:1 ${c >= 4.5 ? 'AA text' : c >= 3 ? 'glyphs only' : 'fails'}</span>` : ''}</div>`
  }
  const row = (label, html) => `<div class="demo-row"><span class="lab">${label}</span><div class="row">${html}</div></div>`

  U.views.system = {
    title: 'Design system',
    render: () => `
      ${U.head('Cellwork', 'The visual language of nixos-manager: one cell, one font, one clock.', `<span class="seg"><a class="on" onclick="U.theme('night');U.go('#/system')">Night</a><a onclick="U.theme('print');U.go('#/system')">Print</a></span>`)}
      <h2>The three rules</h2><div class="sp"></div>
      <div class="grid" style="grid-template-columns:repeat(3,1fr)">
        ${box({ title: '1 · The cell' }, `<pre class="art nofx">┌─┐  1ch wide, 1 row (17 px) tall.\n│█│  Every size, gap and position\n└─┘  is a whole number of cells.</pre>`)}
        ${box({ title: '2 · Glyphs only' }, `<pre class="art nofx">╭─╮ ░▒▓█ ┤┘  Lines, corners, icons, bars\n│ │ ▁▃▅▇ ●◐○  and shadows are characters.\n╰─╯ [x] ──●  No border, radius or SVG.</pre>`)}
        ${box({ title: '3 · One clock' }, `<pre class="art nofx">│▏▎▍▌▋▊▉█  Every animation steps on the\n├─┼─┼─┼─┤  same 80 ms beat (12.5 fps),\n0  80 160  so the screen moves as one.</pre>`)}
      </div>

      <div class="sp"></div><h2>Palette</h2><div class="sp"></div>
      <p class="mut">Screen</p><div class="sw-grid">${[['--bg-deep', 'behind everything'], ['--bg', 'the screen'], ['--bg-raised', 'dialogs, menus, toasts'], ['--bg-input', 'fields'], ['--bg-hover', 'hovered row, mode line'], ['--bg-active', 'focused field, pressed'], ['--border', 'rules, tracks'], ['--line', 'frames at rest'], ['--line-strong', 'brackets, modal frames'], ['--shade', '░ shadows']].map(([n, u]) => swatch(n, u)).join('')}</div>
      <div class="sp"></div><p class="mut">Text and signal</p><div class="sw-grid">${[['--fg', 'body'], ['--fg-strong', 'headings, values'], ['--fg-muted', 'labels'], ['--fg-faint', 'hints, placeholders'], ['--accent', 'focus, links, selection'], ['--accent-strong', 'hovered accent'], ['--green', 'ok · set here · added'], ['--yellow', 'warning · behind'], ['--red', 'error · failed · removed'], ['--purple', 'programs · keywords'], ['--cyan', 'services'], ['--g-virtualisation', 'virtualisation'], ['--g-hardware', 'hardware']].map(([n, u]) => swatch(n, u)).join('')}</div>

      <div class="sp"></div><h2>Type</h2><div class="sp"></div>
      <div class="grid cols11">
        ${box({ title: 'One size, four voices' }, `${big('Pixel face')}<p class="mut">Page titles, machine names, big numbers. Uppercase, 3 rows.</p><div class="sp"></div><h2>Section heading</h2><p class="mut">Bold caps behind an accent bar.</p><div class="sp"></div><h3>Strong text, values and names</h3><p>Body text in bone white, 14/17 px. <span class="mut">Muted for labels,</span> <span class="faint">faint for hints.</span></p><p><a>Links are accent</a> and turn reverse-video under the pointer.</p>`)}
        ${box({ title: 'Coverage test', note: 'must stay aligned' }, `<pre class="art nofx">╭──┬──╮ ┌──┬──┐ ╔══╦══╗ ┏━━┳━━┓ ┌┄┄┬┄┄┐\n│  │  │ │  │  │ ║  ║  ║ ┃  ┃  ┃ ┆  ┆  ┆\n├──┼──┤ ├──┼──┤ ╠══╬══╣ ┣━━╋━━┫ ├┄┄┼┄┄┤\n╰──┴──╯ └──┴──┘ ╚══╩══╝ ┗━━┻━━┛ └┄┄┴┄┄┘\n░▒▓█ ▏▎▍▌▋▊▉ ▁▂▃▄▅▆▇ ▀▄▌▐ ▖▘▝▗ ▚▞\n┤┘┴└├┌┬┐ ◐◓◑◒ ●○◉◌ ·∙• ■□▪▫ ▣◫◧◈\n← ↑ → ↓ ↗ ↧ ⇡ ⇣ ⇉ ↶ ↺ ↻ ⇲ ↣ ✓ ✖ ✎ ⌫ ⌕ ⌥ ⚙ ¤ ☗ ⏏ ❄ λ</pre><p class="mut">If any line here is ragged, the font lacks a glyph and a fallback of another width was used.</p>`)}
      </div>

      <div class="sp"></div><h2>Frames</h2><div class="sp"></div>
      <p class="mut">The weight of a frame is its rank. Hover the first one.</p><div class="sp"></div>
      <div class="grid" style="grid-template-columns:repeat(5,1fr)">
        ${box({ title: 'round', cls: 'lift' }, '<p>A card or a panel.</p><p class="mut">Hover: heavy, accent.</p>')}
        ${box({ title: 'square', frame: 'square' }, '<p>Data: tables, diffs,</p><p class="mut">menus, fields.</p>')}
        ${box({ title: 'double', frame: 'double', shadow: true }, '<p>A modal. One on</p><p class="mut">screen, with ░ shadow.</p>')}
        ${box({ title: 'heavy', frame: 'heavy', tone: 'accent' }, '<p>Selected, or a</p><p class="mut">drawer that slid in.</p>')}
        ${box({ title: 'dash', frame: 'dash' }, '<p>Not there yet: a form</p><p class="mut">to add one, an empty slot.</p>')}
      </div><div class="sp"></div>
      <div class="grid" style="grid-template-columns:repeat(4,1fr)">${[['', 'neutral'], ['ok', 'done, passed'], ['warn', 'needs a look'], ['err', 'failed']].map(([t, d]) => box({ title: t || 'tone', tone: t, note: 'tag', foot: 'foot' }, `<p>${d}</p>`)).join('')}</div>

      <div class="sp"></div><h2>Components and states</h2><div class="sp"></div>
      ${box({ title: 'Buttons' }, row('default', btn('Push') + btn('↻ Refresh')) + row('hover / focus', `<button class="btn" style="color:var(--on-accent);background:var(--accent)">Push</button><span class="mut">reverse video; focus swaps the brackets for ▸ ◂</span>`) + row('primary', btn('Commit', 'primary') + btn('Add 2 packages', 'primary')) + row('danger', btn('Revert', 'danger')) + row('busy', `<button class="btn is-busy">${spin()} Pushing…</button>`) + row('disabled', btn('Commit', '', 'disabled')) + row('ghost', btn('×', 'ghost') + btn('⇲', 'ghost')))}
      <div class="sp"></div>
      <div class="grid cols11">
      ${box({ title: 'Fields' }, row('text', field('Name', 'placeholder="alice"')) + row('filled', field('Name', '', 'pablo')) + row('prompt', `<label class="field"><b class="prompt">λ ›</b><input type="text" placeholder="Search…"></label>`) + row('invalid', `<label class="field invalid"><span>Port</span><input type="text" value="99999"></label>`) + '<p class="err">✖ A port is a number from 1 to 65535.</p>' + row('choice', seg(['ext4', 'Btrfs'], 0) + seg(['true', 'false', 'not set'], 2)))}
      ${box({ title: 'Toggles' }, row('checkbox', chk('Hide defaults', true) + chk('Only where machines differ')) + row('radio', `<label class="rad"><input type="radio" name="r" checked><i></i><span>weekly</span></label><label class="rad"><input type="radio" name="r"><i></i><span>daily</span></label>`) + row('switch', sw('Firewall', true) + sw('Store optimisation', false)) + row('saving', sw('fail2ban', true, 'is-busy')) + row('disabled', chk('pi <span class="faint">λ Code only</span>', false, 'disabled')))}
      ${box({ title: 'Status' }, row('dots', `${dot('ok')} ok ${dot('busy')} working ${dot('warn')} warning ${dot('err')} failed ${dot('off')} silent ${dot('info')} note`) + row('machine', U.state(D.machines[0]) + ' ' + U.state(D.machines[2]) + ' ' + U.state(D.machines[3])) + row('deployment', ['ok:Up to date', 'warn:1 behind', 'err:Build failed', 'off:Not reporting'].map((s) => badge(...s.split(':'))).join(' ')) + row('source', '<span class="ok">✎ Set here</span> <span class="mut">λ In code · common.nix</span> <span class="faint">· Default</span>') + row('chips', chip('jellyfin', 'services') + chip('zsh', 'programs') + chip('docker', 'virtualisation') + chip('bluetooth', 'hardware') + chip('x86_64')) + row('counts, keys', '<span class="count">2</span> <span class="count err">3</span> <kbd>⌃K</kbd> <kbd>Esc</kbd>'))}
      ${box({ title: 'Progress' }, row('meter', bar(0.37, 30) + ' 37%') + row('working', bar(0.62, 30, true)) + row('spinners', `${spin()} tee ${spin('arc')} arc ${spin('dot')} dot ${spin('ping')} ping ${spin('eq')} log tail ${spin('bounce')} bounce`) + row('skeleton', skel(36)) + row('spark', `<span class="acc">${A.spark([1, 3, 2, 5, 4, 7, 6, 8, 5, 3, 4, 6])}</span> <span class="mut">evaluations per day</span>`) + row('steps', U.steps(['Name', 'Role', 'Platform', 'Review'], 1)))}
      </div><div class="sp"></div>
      ${box({ title: 'Tabs, tables, rows' }, tabs([['Overview'], ['Essentials'], ['Packages', 22], ['Users', 1]], 0) + table(['Package', 'Version', 'From', '#Machines'], [['<b>neovim</b>', '0.11.4', '<span class="ok">✎ Set here</span>', '3/4'], ['<b>git</b>', '2.51.0', '<span class="mut">λ In code</span>', '4/4']]) + `<div class="sp"></div><div class="rows" style="width:50ch"><a><i class="acc">▣</i><span class="grow">a row at rest</span><span class="mut">detail</span></a><a class="on"><i>▣</i><span class="grow">the selected row</span><span>Enter</span></a></div>`)}
      <div class="sp"></div>
      ${box({ title: 'Overlays', note: 'click to open' }, `<div class="row">${btn('Command palette', '', 'data-act="palette"')}${btn('Dialog', '', 'data-act="confirm"')}${btn('Danger dialog', '', 'data-act="danger"')}${btn('Prompt', '', 'data-act="prompt"')}${btn('Package search', '', 'data-act="pkgsearch"')}${btn('Wizard', '', 'data-act="wizard"')}${btn('Drawer', '', 'data-act="catalog"')}${btn('Branch menu', '', 'data-act="branch"')}${btn('Toasts', '', 'data-act="toasts"')}<a href="#/states">Loading, error and empty states ›</a></div>`)}

      <div class="sp"></div><h2>Glyph icons</h2><div class="sp"></div>
      <div class="glyphs">${Object.entries(U.icons).map(([n, g]) => `<span><b>${g}</b>${n}</span>`).join('')}</div>

      <div class="sp"></div><h2>Motion</h2><div class="sp"></div>
      <p class="mut">Each animation of the paper console, and the text animation that replaces it. All of them stop on their last frame under “reduce motion”.</p><div class="sp"></div>
      ${table(['Was', 'Now', 'Live', ''], [
        ['view fade-in, rise, stagger', '<b>Repaint sweep</b>: text resolves out of Nix base-32, top to bottom', '<span id="m-sweep">/nix/store/8f3ka1c-nixos-system-homeserver</span>', btn('Replay', '', 'data-m="sweep"')],
        ['pop-in, dialog-in, pal-in, drop', '<b>Stroke-in</b>: the frame is drawn from its top-left corner', `<span id="m-frame" data-frame="round" data-title="frame" style="display:inline-block;width:24ch;height:calc(var(--lh)*3)"></span>`, btn('Replay', '', 'data-m="frame"')],
        ['slide-in crumb, toast fly', '<b>Typed</b>: characters arrive three per tick', '<span id="m-type">Machines › workstation</span>', btn('Replay', '', 'data-m="type"')],
        ['countUp', '<b>Odometer</b>: the number rolls up in the pixel face', '<pre class="big acc nofx" id="m-count" style="display:inline-block"></pre>', btn('Replay', '', 'data-m="count"')],
        ['spin, turn', '<b>Tee spinner</b> ┤┘┴└├┌┬┐, one frame per tick', `${spin()} ${spin('dot')} ${spin('eq')}`, ''],
        ['ping, pulse', '<b>Dot cycle</b> ·∙•●•∙ on the working dot', spin('ping'), ''],
        ['shimmer', '<b>Dither wave</b> ░▒▓▒░ crossing the placeholder', skel(28), ''],
        ['sheen', '<b>Leading edge</b> ▓▒░ running through the meter', bar(0.7, 28, true), ''],
        ['tab ink', '<b>Heavy rule</b> ━━ slides under the active tab, cell by cell', '<span class="mut">click the tabs above</span>', ''],
        ['lift on hover', '<b>Frame weight</b>: round → heavy, in accent', '<span class="mut">hover any card</span>', ''],
        ['edge-flow', '<b>Packets</b> •∙∙∙ run along the wires of the fleet graph', '<a href="#/fleet">Fleet ›</a>', ''],
        ['bump', '<b>Flash</b>: a count that changes shows in reverse video for 2 ticks', '<span class="count" id="m-bump">2</span>', btn('Replay', '', 'data-m="bump"')],
        ['intro WebGL + SVG blueprint', '<b>Text scenes</b>: a shaded sphere of package dots, orbits, pulses, a drafted graph', '<a href="#/intro">Intro ›</a>', ''],
        ['boot turn + halo', '<b>Snowflake</b> turning in line glyphs, a ring of dots leaving it', '<a href="#/boot">Boot ›</a>', ''],
      ])}<div class="sp"></div>`,
    mount(view) {
      const m = {
        sweep: () => A.sweep(view.querySelector('#m-sweep').parentElement),
        frame: () => {
          const el = view.querySelector('#m-frame')
          el.querySelector('.fr')?.remove()
          A.scan(el.parentElement, true)
        },
        type: () => A.type(view.querySelector('#m-type'), 'Machines › workstation', 2),
        count: () => A.run(10, (p) => (view.querySelector('#m-count').textContent = A.big(String(Math.round(112 * (1 - (1 - p) ** 3))).padStart(3, '0')))),
        bump: () => {
          const el = view.querySelector('#m-bump')
          el.textContent = +el.textContent + 1
          el.classList.add('flash')
          A.run(2, () => {}, () => el.classList.remove('flash'))
        },
      }
      m.count()
      view.addEventListener('click', (e) => {
        const b = e.target.closest('[data-m]')
        b && m[b.dataset.m]()
      })
    },
  }
})()
