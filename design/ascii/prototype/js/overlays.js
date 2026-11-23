// Everything that opens over the console: palette, package search, the new-machine wizard, menus,
// dialogs, toasts and the service catalog.
;(function () {
  'use strict'
  const { ic, box, chip, btn, seg, dot, kv, field, chk, spin, bar, skel } = U

  // ---- Command palette: a λ prompt in the only other double frame ----------------------------
  const GROUPS = [
    ['Recent', [['▣', 'workstation', 'Machine'], ['▤', 'hosts/homeserver/default.nix', 'File']]],
    ['Actions', [['+', 'New machine…', ''], ['▷', 'Run checks', 'On the staged changes'], ['◫', 'Add packages…', ''], ['↧', 'Update all flake inputs', ''], ['⇣', 'Pull', ''], ['⇡', 'Push', ''], ['♥', 'Open health', ''], ['◉', 'Commit…', ''], ['⏏', 'Log out', '']]],
    ['Views', ['Fleet', 'Machines', 'Packages', 'Services', 'Options', 'Health', 'Deployments', 'Inputs', 'Changes', 'History', 'Code', 'Settings'].map((v) => ['›', v, 'g ' + (v === 'Settings' ? ',' : v === 'Health' ? 'a' : v === 'Code' ? 'e' : v[0].toLowerCase())])],
    ['Machines', D.machines.map((m) => ['▣', m.name, m.sys])],
    ['Packages', [['◫', 'neovim', '0.11.4'], ['◫', 'firefox', '144.0']]],
    ['Options', [['≔', 'services.openssh.enable', 'boolean'], ['≔', 'services.jellyfin.enable', 'boolean']]],
  ]
  U.palette = (q = '') => {
    U.closeAll()
    const wrap = U.open(
      box(
        { cls: 'modal palette', frame: 'double', shadow: true, title: 'Go anywhere', note: 'Esc', foot: '↑↓ move · Enter open · Options from workstation', attrs: 'role="dialog" style="width:84ch;align-self:start;margin-top:calc(var(--lh)*6)"' },
        `<label class="field"><b class="prompt">λ ›</b><input type="text" placeholder="Search machines, packages, options, files, actions…" value="${q}" autofocus></label><div class="rule"></div><div class="results" style="height:calc(var(--lh)*22);overflow:hidden"></div>`,
      ),
    )
    const input = wrap.querySelector('input')
    const out = wrap.querySelector('.results')
    let sel = 0
    const paint = () => {
      const needle = input.value.trim().toLowerCase()
      let i = 0
      out.innerHTML = GROUPS.map(([g, items]) => {
        const hits = items.filter((it) => !needle || it[1].toLowerCase().includes(needle)).slice(0, needle ? 6 : 4)
        return hits.length ? `<p class="mut">${g}</p><div class="rows">${hits.map(([icn, label, detail]) => `<a class="${i++ === sel ? 'on' : ''}" data-go="${g === 'Views' ? '#/' + label.toLowerCase() : g === 'Machines' ? '#/machines/' + label : ''}"><i class="acc">${icn}</i><span class="grow">${label}</span><span class="mut">${detail}</span></a>`).join('')}</div>` : ''
      }).join('') || `<p class="mut">Nothing matches “${A.esc(input.value)}”.</p>`
      return i
    }
    paint()
    input.addEventListener('input', () => ((sel = 0), paint()))
    input.addEventListener('keydown', (e) => {
      const n = paint()
      if (e.key === 'ArrowDown') (sel = (sel + 1) % n), paint(), e.preventDefault()
      if (e.key === 'ArrowUp') (sel = (sel + n - 1) % n), paint(), e.preventDefault()
      if (e.key === 'Enter') {
        const go = out.querySelector('.on')?.dataset.go
        wrap.remove()
        go && U.go(go)
      }
    })
    out.addEventListener('click', (e) => {
      const a = e.target.closest('[data-go]')
      if (a) wrap.remove(), a.dataset.go && U.go(a.dataset.go)
    })
  }

  // ---- Package search ------------------------------------------------------------------------
  U.pkgsearch = () =>
    U.open(
      box(
        { cls: 'modal', frame: 'double', shadow: true, title: 'Add packages', note: 'from the nixpkgs pinned in flake.lock', attrs: 'role="dialog" style="width:96ch"' },
        `<label class="field"><b class="prompt">λ ›</b><input type="text" value="neo" placeholder="Search packages by name or description" autofocus></label><div class="rule"></div>
         <div class="rows">${[['neovim', '0.11.4', 'Vim text editor fork focused on extensibility and agility', true, ''], ['neofetch', '7.1.0', 'Fast, highly customizable system info script', false, ''], ['neomutt', '20250905', 'Small but very powerful text-based mail client', false, ''], ['neovide', '0.15.2', 'Simple graphical user interface for Neovim', true, ''], ['neo4j', '5.26.1', 'Highly scalable, robust native graph database', false, 'Already here'], ['neo-cowsay', '2.0.4', 'Cowsay reborn, written in Go', false, '']].map(([n, v, d, on, b]) => `<a>${chk('', on)}<b style="width:14ch">${n}</b><span class="mut" style="width:10ch">${v}</span><span class="grow mut">${d}</span><span class="ok">${b}</span></a>`).join('')}
         <a>${skel(70)}</a></div>
         <div class="rule"></div>
         <p class="row"><span class="mut">Selected</span>${chip('neovim ×', 'programs')}${chip('neovide ×', 'programs')}</p>
         <p class="row"><span class="mut">Add to  </span>${chk('homeserver', true)}${chk('workstation', true)}${chk('pi <span class="faint">λ Code only</span>', false, 'disabled')}</p>
         <div class="actions">${btn('Cancel', '', 'data-close')}${btn('Add 2 packages to 2 machines', 'primary', 'data-close data-act="saved"')}</div>`,
      ),
    )

  // ---- New machine wizard --------------------------------------------------------------------
  const STEPS = ['Name', 'Role', 'Platform', 'Review']
  const PAGES = [
    () => `<p class="mut">What is the machine called?</p><div class="sp"></div>${U.big('nas', 'acc')}<label class="field"><b class="prompt">λ ›</b><input type="text" value="nas" autofocus></label><div class="sp"></div><p class="mut">Architecture</p><p>${seg(['x86_64 <span class="faint">Intel, AMD</span>', 'aarch64 <span class="faint">Raspberry Pi, ARM</span>'], 0)}</p>`,
    () => `<p class="mut">What is nas for?</p><div class="sp"></div><div class="grid" style="grid-template-columns:repeat(3,1fr)">${[['Server', '▣', 'SSH, firewall, no desktop', 'sel'], ['Desktop', '◧', 'Plasma, audio, printing', ''], ['Minimal', '□', 'Just boots. You add the rest', '']].map(([t, g, d, s]) => box({ cls: 'lift ' + s, frame: s ? 'heavy' : 'round', title: t }, `<pre class="art nofx" style="text-align:center">${g}</pre><p class="mut">${d}</p>`)).join('')}</div>`,
    () => `<p class="mut">Where does nas run?</p><div class="sp"></div><p>${seg(['Physical machine', 'Proxmox VM', 'Proxmox LXC'], 0)}</p><div class="sp"></div><p>${chk('Installing NixOS on it: write a disk layout too', true)}</p><div class="sp"></div>${kv([['Disk layout', seg(['ext4', 'Btrfs'], 1)], ['Disk to erase', '<input type="text" value="/dev/nvme0n1" style="width:24ch">']])}<div class="sp"></div><p class="mut">hardware-configuration.nix (paste from nixos-generate-config, or leave empty)</p><textarea placeholder="{ config, lib, modulesPath, ... }: …"></textarea>`,
    () => `<div class="edtabs"><span class="on">hosts/nas/default.nix</span><span>hardware-configuration.nix</span><span>disko.nix</span><span>managed.json</span></div><pre class="pre nofx">{ ... }:\n{\n  imports = [ ./hardware-configuration.nix ./disko.nix ../../modules/common.nix ];\n  networking.hostName = "nas";\n  services.openssh.enable = true;\n  system.stateVersion = "26.05";\n}</pre><div class="sp"></div><p class="note">Installing it: the machine's page gives the nixos-anywhere command. The console never runs the install itself.</p>`,
  ]
  U.wizard = (step = 0) => {
    U.closeAll()
    const wrap = U.open(
      box(
        { cls: 'modal', frame: 'double', shadow: true, title: 'New machine', note: `${step + 1} / 4`, attrs: 'role="dialog" style="width:90ch"' },
        `${U.steps(STEPS, step)}<div class="sp"></div>
         <div style="min-height:calc(var(--lh)*15)">${PAGES[step]()}</div>
         <div class="actions">${step ? btn('‹ Back', '', 'data-step="-1"') : ''}${btn('Cancel', '', 'data-close')}${step < 3 ? btn('Next ›', 'primary', 'data-step="1"') : btn('Create nas', 'primary', 'data-close data-act="created"')}</div>`,
      ),
    )
    wrap.addEventListener('click', (e) => {
      const b = e.target.closest('[data-step]')
      b && U.wizard(step + +b.dataset.step)
    })
  }

  // ---- Demo actions wired to data-act --------------------------------------------------------
  Object.assign(U.demo, {
    palette: () => U.palette(),
    pkgsearch: () => U.pkgsearch(),
    wizard: () => U.wizard(0),
    wizard2: () => U.wizard(1),
    branch: (a) =>
      U.menu(a || document.querySelector('[data-act=branch]'), `<p class="mut">Local branches</p><div class="rows"><a><span class="acc">✓</span><span class="grow">main</span><span class="mut">↑0 ↓0</span></a><a><span> </span><span class="grow">testing-pi</span><span class="mut">Merge into main</span></a><a><span> </span><span class="grow">update/nixpkgs</span><span class="mut">Merge into main</span></a></div><div class="rule dash"></div><p class="mut">New branch from HEAD</p><div class="row">${field('<b class="prompt">›</b>', 'placeholder="branch-name" style="width:20ch"')}${btn('Create')}</div>`),
    user: (a) => U.menu(a || document.querySelector('[data-act=user]'), `<p><b>Pablo</b> <span class="mut">pablo@home.arpa</span></p><div class="rule dash"></div><div class="rows"><a href="#/settings"><i class="acc">¤</i><span class="grow">Tokens and MCP</span></a><a href="#/login"><i class="acc">⏏</i><span class="grow">Log out</span></a></div>`),
    model: (a) => U.menu(a || document.querySelector('[data-act=model]'), `<p class="mut">Evaluation</p><div class="rows">${D.machines.map((m) => `<a href="#/machines/${m.name}">${dot(m.state === 'broken' ? 'err' : 'ok')}<span class="grow">${m.name}</span><span class="mut">${m.state === 'broken' ? 'does not evaluate' : '4.1 s'}</span></a>`).join('')}</div><div class="rule dash"></div>${btn('↻ Evaluate again')}`),
    confirm: () => U.dialog({ title: 'Discard the update?', body: 'flake.lock goes back to the revisions main has. Nothing was committed, so nothing is lost on the machines.', ok: 'Discard' }),
    danger: () => U.dialog({ title: 'Revert 4e1f9a2?', danger: true, body: 'A new commit undoes “homeserver: enable jellyfin”. It is checked like any other change and nothing is pushed until you push.', pre: 'hosts/homeserver/managed.json   +1 −4', ok: 'Revert the commit' }),
    prompt: () => U.dialog({ title: 'New branch', body: 'Created from HEAD (4e1f9a2).', input: 'try-jellyfin', ok: 'Create' }),
    saved: () => U.toast({ kind: 'ok', title: 'Saved', text: 'managed.json updated on workstation.', action: 'Review in Changes ›' }),
    committed: () => U.toast({ kind: 'ok', title: 'Committed 7d20c4e', text: 'homeserver, workstation: add neovim' }),
    created: () => U.toast({ kind: 'ok', title: 'nas was created', text: '4 files in hosts/nas, staged for review.', action: 'Review in Changes ›' }),
    info: () => U.toast({ kind: 'info', title: 'Test message sent', text: 'ntfy answered 200 in 84 ms.' }),
    error: () => U.toast({ kind: 'error', title: 'Push rejected', text: 'main moved on Forgejo. Pull first, then push again.', action: 'Pull ›' }),
    busy: () => U.toast({ kind: 'busy', title: 'Updating nixpkgs…', text: 'nix flake update nixpkgs' }),
    pull: () => U.toast({ kind: 'ok', title: 'Pulled', text: 'Already up to date with origin/main.' }),
    push: () => U.demo.error(),
    checks: () => U.toast({ kind: 'busy', title: 'Running checks…', text: 'nix flake check on the staged changes' }),
    toasts: () => (U.demo.busy(), U.demo.error(), U.demo.saved()),
    catalog: () =>
      U.drawer('Popular services', `<label class="field"><span>${ic('filter')}</span><input type="search" placeholder="media, backup, containers…"></label>${[['Media', [['Jellyfin', 'Your films and music, streamed', 'on'], ['Immich', 'Photo backup from your phone', ''], ['Navidrome', 'Music server', '']]], ['Files and sync', [['Nextcloud', 'Files, calendar, contacts', 'on'], ['Syncthing', 'Folders in sync between machines', 'adding']]], ['Home', [['Home Assistant', 'Home automation hub', 'missing'], ['Mosquitto', 'MQTT broker', '']]], ['Security', [['Vaultwarden', 'Password manager server', ''], ['AdGuard Home', 'Network-wide ad blocking', '']]]].map(([cat, items]) => `<div class="sp"></div><h2>${cat}</h2>${items.map(([n, d, s]) => `<p class="row" style="flex-wrap:nowrap"><b style="width:16ch">${n}</b><span class="grow mut">${d}</span>${s === 'on' ? '<span class="ok">● On</span>' : s === 'adding' ? `<span class="acc">${spin()} Adding…</span>` : s === 'missing' ? '<span class="faint">Not in this NixOS</span>' : btn('Add')}</p>`).join('')}`).join('')}`),
  })
})()
