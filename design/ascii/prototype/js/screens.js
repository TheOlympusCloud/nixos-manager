// Every console view, with the sample fleet. Labels and states are the console's own.
;(function () {
  'use strict'
  const { ic, box, chip, btn, seg, dot, kv, badge, table, tabs, head, tile, field, chk, sw, bar, skel, spin, big } = U
  const esc = A.esc
  const V = U.views
  const good = D.machines.filter((m) => m.state !== 'broken')
  const src = (kind, file) => ({ here: '<span class="ok">✎ Set here</span>', code: `<span class="mut">λ In code · ${file || 'default.nix'}</span>`, def: '<span class="faint">· Default</span>' })[kind]
  // Each line is one element, so its spaces survive inside the flex row that holds the gutter.
  const codeLines = (lines) => `<div class="code nofx">${lines.map((l) => (typeof l === 'string' ? `<div><span>${l || ' '}</span></div>` : `<div class="${l[0]}"><span>${l[1] || ' '}</span></div>`)).join('')}</div>`
  // One pass over the raw line, so a token is never matched again inside another one's markup.
  const nix = (line) => {
    let out = ''
    let at = 0
    const re = /^(\s*)([a-zA-Z][\w.-]*)(?=\s*=)|(#.*$)|("[^"]*")|\b(true|false|null|\d+)\b|\b(inherit|with|let|in|imports|if|then|else)\b/g
    for (let m = re.exec(line); m; m = re.exec(line)) {
      out += esc(line.slice(at, m.index))
      at = m.index + m[0].length
      if (m[2]) out += m[1] + `<span class="tok-a">${esc(m[2])}</span>`
      else out += `<span class="tok-${m[3] ? 'c' : m[4] ? 's' : m[5] ? 'n' : 'k'}">${esc(m[0])}</span>`
    }
    return out + esc(line.slice(at))
  }

  // ---- Machines ------------------------------------------------------------------------------
  const machineCard = (m) =>
    m.state === 'broken'
      ? box({ cls: 'lift', title: m.name, note: m.sys, tone: 'err', attrs: `onclick="location.hash='#/machines/${m.name}'" tabindex="0"` }, `<p class="err">${m.error}</p><p>&nbsp;</p><p>&nbsp;</p><p>${U.state(m)}</p>`)
      : box(
          { cls: 'lift', title: m.name, note: m.sys, attrs: `onclick="location.hash='#/machines/${m.name}'" tabindex="0"` },
          `<p>${dot('ok')} NixOS ${m.rel} · ${m.name} <span class="right">${m.hm ? `<span class="count">${m.hm}</span>` : ''}</span></p>
           <p>${ic('package', 'mut')} ${m.pk} packages  ${ic('services', 'mut')} ${m.sv} services  ${ic('user', 'mut')} ${m.us} user</p>
           <p>&nbsp;</p><p class="row">${U.state(m)}<span class="right">${badge(m.deploy[0], m.deploy[1])}</span></p>`,
        )
  V.machines = {
    title: 'Machines',
    render: (parts) =>
      parts[0]
        ? machinePage(parts[0], parts[1] || 'overview')
        : head('Machines', '4 machines in nixosConfigurations', `<label class="field" style="width:44ch"><span>${ic('filter')}</span><input type="search" placeholder="Filter by name, system or service" data-search></label>${btn('+ New machine', 'primary', 'data-act="wizard"')}`) +
          `<div class="grid cards">${D.machines.map(machineCard).join('')}</div>
           <div class="sp"></div><h2>Home configurations</h2><div class="sp"></div>
           <div class="grid cards">${box({ cls: 'lift', title: 'pablo@workstation' }, `<p>${ic('user', 'acc')} 9 packages · 6 programs</p><div class="chips">${['git', 'zsh', 'starship', 'direnv', 'neovim', 'firefox'].map((p) => chip(p, 'programs')).join('')}</div>`)}</div>
           <div class="sp"></div><h2>Flake inputs</h2><div class="sp"></div>
           ${table(['Input', 'Source', 'Revision', 'Updated', ''], D.inputs.slice(0, 3).map(([n, s, r, a, t]) => [`<b>${n}</b>`, `<span class="mut">${s}</span>`, r, `${dot(t)} ${a}`, btn('Update')]))}
           <p>${btn('Update all inputs')}</p><div class="sp"></div>`,
    mount: (view, parts) => parts[0] && machineMount(view, parts[0], parts[1] || 'overview'),
  }

  const TABS = [['overview', 'Overview'], ['essentials', 'Essentials'], ['packages', 'Packages'], ['services', 'Services & programs'], ['users', 'Users'], ['network', 'Network & storage']]
  function machinePage(name, tab) {
    const m = D.m(name)
    if (!m) return `<div class="empty"><pre class="art">  ╭───╮\n  │ ? │\n  ╰───╯</pre><h3>No machine called “${esc(name)}”</h3>${btn('All machines', '', `onclick="location.hash='#/machines'"`)}</div>`
    if (m.state === 'broken')
      return `<div class="row">${big(m.name, 'err')}</div><p class="row">${dot('err')} ${m.sys} ${U.state(m)}</p><div class="sp"></div>
        ${box({ title: 'Evaluation failed', tone: 'err' }, `<pre class="pre">error: infinite recursion encountered\n       at /fleet/hosts/oldbox/default.nix:4:13:\n            3|   {\n            4|     imports = [ ./default.nix ];\n             |             ^</pre><div class="sp"></div><div class="row">${btn('Open file', 'primary')}${btn('Run checks')}${btn('Evaluate again')}</div>`)}`
    const n = { packages: m.pk, services: m.sv, users: m.us }
    return `<div class="ph"><div>${big(m.name)}<p class="row">${dot('ok')} <span class="chip">${m.sys}</span> ${U.state(m)} ${m.state === 'code' ? '<a>Turn on editing</a>' : ''} ${badge(m.deploy[0], m.deploy[1])}</p></div><div class="tools">${btn(ic('sliders') + ' Options', '', `onclick="location.hash='#/options'"`)}</div></div>
      <div class="row" style="gap:4ch">${[['Host name', m.name], ['System', m.sys + '-linux'], ['NixOS', m.rel], ['State version', m.rel], ['Kernel', m.kernel], ['Time zone', 'Europe/Madrid']].map(([k, v]) => `<span><span class="mut">${k}</span><br><b>${v}</b></span>`).join('')}</div>
      <div class="sp"></div>${tabs(TABS.map(([id, t]) => [t, n[id], `#/machines/${name}/${id}`]), TABS.findIndex((t) => t[0] === tab))}
      <div class="sp"></div><div id="tab">${machineTab[tab] ? machineTab[tab](m) : ''}</div><div class="sp"></div>`
  }
  let lastTab = null
  function machineMount(view, name, tab) {
    const t = view.querySelector('.tabs')
    if (!t) return
    // The heavy stretch of the rule slides from the tab you came from.
    const idx = TABS.findIndex((x) => x[0] === lastTab)
    if (lastTab && idx >= 0 && lastTab !== tab) {
      const prev = t.children[idx].getBoundingClientRect()
      t._at = Math.round((prev.left - t.getBoundingClientRect().left) / A.cw)
      t._len = Math.round(prev.width / A.cw)
      A.ink(t, true)
    }
    lastTab = tab
  }
  const machineTab = {
    overview: (m) => `
      <div class="grid tiles">${tile('Packages', m.pk, `${m.pk - 4} from this repository`)}${tile('Services & programs', m.sv, `${Math.ceil(m.sv / 2)} services`)}${tile('Users', m.us, '1 with home-manager')}${tile('Open ports', 2, 'firewall on')}${tile('File systems', 3, 'ext4, vfat, btrfs')}</div>
      <div class="sp"></div>
      <div class="grid cols2"><div class="stack">
        <h2>Features</h2>
        ${kv(['services', 'programs', 'virtualisation', 'hardware'].map((g) => [g[0].toUpperCase() + g.slice(1), `<div class="chips">${m.feats.filter((f) => f[1] === g).map((f) => chip(f[0], g)).join('') || (g === 'programs' ? ['git', 'zsh'].map((p) => chip(p, g)).join('') : '<span class="faint">none</span>')}</div>`]))}
        <h2>Configured by</h2>
        <div class="rows">${['flake.nix', `hosts/${m.name}/default.nix`, `hosts/${m.name}/hardware-configuration.nix`, `hosts/${m.name}/managed.json`, 'modules/common.nix'].map((f, i) => `<a href="#/code">${ic('file', 'mut')}<span class="grow">${f}</span>${i === 3 ? src('here') : ''}</a>`).join('')}</div>
      </div><div class="stack">
        ${box({ title: 'Deployment', note: 'All deployments ›' }, `${kv([['State', badge(m.deploy[0], m.deploy[1])], ['Runs', `<b>${m.sha}</b> ${m.name}: latest change`], ['Deployed', '12 min ago'], ['Follows', '⌥ main']])}<div class="sp"></div><p class="mut">Recent reports</p><p>${dot('ok')} deployed 4e1f9a2 <span class="mut">12 min ago</span></p><p>${dot('ok')} deployed b07c3d1 <span class="mut">2 h ago</span></p><p>${dot('err')} build failed a3391fe <span class="mut">yesterday</span></p>`)}
        ${box({ title: 'Users and access' }, kv([['Users', chip('pablo · home-manager')], ['TCP ports', chip('22') + ' ' + chip('8096')], ['UDP ports', chip('41641')], ['File systems', '<b>/</b> ext4  <b>/boot</b> vfat  <b>/home</b> btrfs']]))}
      </div></div>`,
    essentials: (m) => {
      const row = (label, control, from) => `<tr><td>${label}</td><td>${control}</td><td>${from}</td><td><a class="mut" title="Reset">↶</a></td></tr>`
      const val = (v) => `<a class="strong">${v} <span class="mut">✎</span></a>`
      const group = (title, note, rows) => box({ title, note }, `<div class="tbl"><table><tbody>${rows.join('')}</tbody></table></div>`)
      return `${m.state === 'code' ? `<p class="note">${m.name} is Code only: these are read from the evaluation. <a>Turn on editing</a> to change them here.</p><div class="sp"></div>` : ''}<div class="grid cols11">
        ${group('Region', 'where and how it speaks', [row('Time zone', val('Europe/Madrid'), src('here')), row('Language and formats', val('en_GB.UTF-8'), src('code', 'modules/common.nix')), row('Console keyboard', val('es'), src('def'))])}
        ${group('Maintenance', 'keeping the store small', [row('Automatic garbage collection', sw('', true), src('here')), row('Schedule', val('weekly'), src('here')), row('Keep generations for', val('14 days'), src('here')), row('Store optimisation', sw('', false), src('def'))])}
        ${group('Memory and network', '', [row('zram swap', sw('', true), src('code', 'modules/common.nix')), row('Firewall', sw('', true), src('def'))])}
        ${group('SSH', 'who gets in, and how', [row('SSH server', sw('', true), src('code', 'modules/common.nix')), row('Password logins', sw('', false), src('here')), row('Root login', '<input type="text" value="prohibit-password" style="width:20ch">', btn('Save', 'primary') + btn('Cancel'))])}
        ${group('Protection and VPN', '', [row('fail2ban', sw('Saving…', true, 'is-busy'), src('here')), row('Tailscale', sw('', true), src('code', 'modules/common.nix'))])}
      </div>`
    },
    packages: (m) => `
      <div class="row"><label class="field" style="width:36ch"><span>${ic('filter')}</span><input type="search" placeholder="Filter packages" data-search></label>${chk('Only set here')}<span class="right">${btn('+ Add packages', 'primary', 'data-act="pkgsearch"')}</span></div><div class="sp"></div>
      ${table(['Package', 'Version', 'From', ''], [[`<b>ffmpeg</b>`, `${spin()} <span class="acc">Adding…</span>`, src('here'), ''], ...D.packages.map(([p, v], i) => [`<b>${p}</b>`, v, i % 3 === 0 ? src('here') : i % 3 === 1 ? src('code', 'modules/common.nix') : src('code', `hosts/${m.name}/default.nix`), i % 3 === 0 ? '<a class="mut" title="Remove">×</a>' : '<a class="mut" title="Open the file">↗</a>'])])}`,
    services: (m) => `
      <div class="row"><label class="field" style="width:36ch"><span>${ic('filter')}</span><input type="search" placeholder="Filter services and programs" data-search></label><span class="right row">${btn('Popular services', '', 'data-act="catalog"')}${btn('+ Add service or program', 'primary')}</span></div>
      ${['services', 'programs', 'virtualisation', 'hardware'].map((g) => {
          const fs = [...m.feats.filter((f) => f[1] === g).map((f) => f[0]), ...(g === 'services' ? ['openssh', 'tailscale', 'comin'] : g === 'programs' ? ['git', 'zsh'] : [])]
          return fs.length ? `<div class="sp"></div><h2 class="g-${g}">${g}</h2><div class="sp"></div><div class="grid cards">${fs.map((f, i) => box({ title: f, cls: 'lift' }, `<p class="row">${sw(i === 1 && g === 'services' ? 'on · set here' : 'on', true)}<span class="right row"><a class="mut">Reset</a><a>Configure ›</a></span></p>`)).join('')}</div>` : ''
        }).join('')}`,
    users: (m) => `
      <div class="grid cols11">
      ${box({ title: 'pablo', note: 'Created here' }, `${kv([['Full name', 'Pablo'], ['Groups', `<div class="chips">${['wheel', 'networkmanager', 'docker', 'video'].map((g) => chip(g + ' ×')).join('')}<a>+ Group</a></div>`], ['SSH keys', `<p>${ic('key', 'acc')} ssh-ed25519 AAAAC3Nz…f3Kq pablo@workstation <a class="mut">×</a></p><p>${ic('key', 'acc')} ssh-ed25519 AAAAC3Nz…9xPd pablo@phone <a class="mut">×</a></p><a>+ Add key</a>`], ['Home', 'home-manager · 9 packages, 6 programs']])}<div class="sp"></div>${btn('Remove user', 'danger', 'data-act="danger"')}`)}
      ${box({ title: 'Add user', frame: 'dash' }, `<div class="stack">${field('User name', 'placeholder="alice"')}${field('Full name (optional)')}<div><p class="mut">Groups</p>${chk('wheel <span class="mut">administrator: can use sudo</span>', true)}<br>${chk('networkmanager')}<br>${chk('docker')}<br>${chk('video')} ${chk('audio')} ${chk('dialout')} ${chk('input')}</div>${field('SSH key', 'placeholder="ssh-ed25519 AAAA…"')}<div class="row">${btn('Add user', 'primary')}</div></div>`)}
      </div>`,
    network: (m) => `
      <div class="grid cols11">
      ${box({ title: 'Firewall' }, `<p class="row">${sw('Firewall on', true)} <span class="right">${src('def')}</span></p><div class="sp"></div><p class="mut">Allowed TCP ports</p><div class="chips">${['22', '8096', '443'].map((p) => chip(p + ' ×', 'services')).join('')}</div><div class="sp"></div><div class="row">${field('<b class="prompt">›</b>', 'placeholder="Port, e.g. 8080" style="width:20ch"')}${btn('Open port')}${btn('Reset')}</div>`)}
      ${box({ title: 'File systems' }, table(['Mount point', 'Type', 'Device'], [['<b>/</b>', 'ext4', '<span class="mut">/dev/disk/by-label/nixos</span>'], ['<b>/boot</b>', 'vfat', '<span class="mut">/dev/disk/by-label/boot</span>'], ['<b>/home</b>', 'btrfs', '<span class="mut">/dev/disk/by-label/home</span>']]))}
      </div>`,
  }

  // ---- Matrices ------------------------------------------------------------------------------
  const matrix = (kind) => ({
    title: kind === 'packages' ? 'Packages' : 'Services',
    render() {
      const cols = D.machines.map((m) => (m.state === 'code' ? `${m.name} <span class="faint">λ</span>` : m.state === 'broken' ? `<span class="err">${m.name} ✖</span>` : m.name))
      let rows
      if (kind === 'packages')
        rows = D.packages.map(([p, v], i) => `<tr><td><b>${p}</b></td>${D.machines.map((m, j) => (m.state === 'broken' ? '<td class="faint">┄</td>' : (i + j) % 4 === 0 ? `<td class="cell-add">+</td>` : i === 3 && j === 1 ? `<td>${spin()}</td>` : `<td class="${(i + j) % 3 ? 'cell-code' : 'cell-on'}">${v}${(i + j) % 3 ? '' : ' <a class="mut">×</a>'}</td>`)).join('')}<td class="mut">${3 - (i % 2)}/4 ${A.bar((3 - (i % 2)) / 4, 4)}</td></tr>`)
      else
        rows = ['services', 'programs', 'virtualisation', 'hardware'].flatMap((g) => {
          const names = [...new Set([...good.flatMap((m) => m.feats.filter((f) => f[1] === g).map((f) => f[0])), ...D.shared.filter((s) => s[1] === g).map((s) => s[0])])]
          return [`<tr class="grp"><td colspan="6" class="g-${g}">▪ ${g}</td></tr>`, ...names.map((f) => {
            const on = D.machines.map((m) => m.state !== 'broken' && (m.feats.some((x) => x[0] === f) || D.shared.some((s) => s[0] === f)))
            return `<tr><td><b>${f}</b> <a class="faint">≔</a></td>${D.machines.map((m, j) => (m.state === 'broken' ? '<td class="faint">┄</td>' : on[j] ? '<td class="cell-on">✓</td>' : '<td class="cell-add">+</td>')).join('')}<td class="mut">${on.filter(Boolean).length}/4 ${A.bar(on.filter(Boolean).length / 4, 4)}</td></tr>`
          })]
        })
      return head(this.title, kind === 'packages' ? '112 packages across 4 machines · 1 not evaluated' : '31 services and programs across 4 machines · 1 not evaluated',
          `<label class="field" style="width:30ch"><span>${ic('filter')}</span><input type="search" placeholder="Filter ${kind === 'packages' ? 'packages' : 'services and programs'}" data-search></label>${kind === 'packages' ? chk('Hide defaults', true) : ''}${chk('Only where machines differ')}${kind === 'packages' ? btn('+ Add packages', 'primary', 'data-act="pkgsearch"') : ''}`) +
        `<p class="row mut"><span class="ok">■ Set here</span><span>■ In code</span>${kind === 'packages' ? '<span class="faint">■ Default</span>' : ''}<span class="faint">┄ not evaluated</span></p><div class="sp"></div>
         <div class="tbl matrix"><table><thead><tr><th>${kind === 'packages' ? 'Package' : 'Feature'}</th>${cols.map((c) => `<th>${c}</th>`).join('')}<th>Machines</th></tr></thead><tbody>${rows.join('')}</tbody></table></div>
         <div class="sp"></div><p class="row">${btn('Show 50 more')}<span class="mut">${kind === 'packages' ? '12 of 112 shown' : 'all shown'}</span></p><div class="sp"></div>`
    },
  })
  V.packages = matrix('packages')
  V.services = matrix('services')

  // ---- Options -------------------------------------------------------------------------------
  V.options = {
    title: 'Options',
    render: () => `<div class="split">
      ${box({ cls: 'side', title: 'Options', note: 'workstation ▾' }, `
        <label class="field"><b class="prompt">λ ›</b><input type="search" value="services.openssh" placeholder="Search options, e.g. services.openssh" data-search></label><div class="sp"></div>
        <p class="mut">services.openssh</p>
        <div class="rows">${[['enable', 'On'], ['ports', ''], ['settings.PasswordAuthentication', 'Set here'], ['settings.PermitRootLogin', 'Set here'], ['settings.KbdInteractiveAuthentication', ''], ['openFirewall', ''], ['banner', ''], ['authorizedKeysFiles', ''], ['startWhenNeeded', '']].map(([o, s], i) => `<a class="${i === 2 ? 'on' : ''}"><span class="grow">${o}</span><span class="ok">${s}</span></a>`).join('')}</div>
        <div class="sp"></div><p class="mut">Set here on workstation <b>7</b></p><div class="rows">${['time.timeZone', 'nix.gc.automatic', 'nix.gc.dates', 'services.fail2ban.enable'].map((o) => `<a><span class="grow">${o}</span></a>`).join('')}</div>
        <div class="sp"></div><p class="mut">Popular</p><div class="chips">${['jellyfin', 'immich', 'nextcloud', 'home-assistant', 'syncthing', 'vaultwarden', 'adguardhome', 'grafana', 'docker', 'caddy'].map((s) => chip(s, 'services')).join('')}</div>`)}
      ${box({ cls: 'body', title: 'services.openssh.settings.PasswordAuthentication', note: 'boolean' }, `
        <p>Specifies whether password authentication is allowed.</p><div class="sp"></div>
        ${kv([['Default', '<code class="tok-n">true</code>'], ['Example', '<code class="tok-n">false</code>'], ['Declared in', '<span class="mut">nixos/modules/services/networking/ssh/sshd.nix</span>']])}
        <div class="sp"></div><h2>On workstation</h2><div class="sp"></div>
        <p>${src('here')} <span class="mut">so it is workstation's final value. It wins over hand-written Nix, except <code>mkForce</code>.</span></p><div class="sp"></div>
        <div class="row"><span class="mut">Value</span>${seg(['true', 'false', 'not set'], 1)}</div><div class="sp"></div>
        <div class="row">${btn('Save', 'primary', 'data-act="saved"')}${btn('Reset')}${btn('Undo edits', '', 'disabled')}</div>
        <div class="sp"></div><h2>On every machine</h2><div class="sp"></div>
        ${table(['Machine', 'Value', 'From'], [['<b>homeserver</b>', '<span class="tok-n">false</span>', src('code', 'modules/common.nix')], ['<b>workstation</b>', '<span class="tok-n">false</span>', src('here')], ['<b>pi</b>', '<span class="tok-n">true</span>', src('def')], ['<b>oldbox</b>', '<span class="err">✖ Does not evaluate</span>', '']])}
        <div class="sp"></div>${box({ title: 'Copy to…', frame: 'dash' }, `<div class="row">${chk('homeserver <span class="faint">already</span>', true, 'disabled')}${chk('pi <span class="faint">λ Code only</span>', false, 'disabled')}${btn('Copy')}</div>`)}`)}
    </div>`,
  }

  // ---- Health --------------------------------------------------------------------------------
  const finding = (tone, title, detail, hosts, fix) =>
    box({ tone: tone === 'info' ? '' : tone, title: ({ err: '✖ ', warn: '▲ ', info: '◆ ' })[tone] + title }, `<p>${detail}</p><p class="row">${hosts.map((h) => `<a href="#/machines/${h}" class="chip">${dot(tone === 'err' ? 'err' : 'ok')} ${h}</a>`).join('')}<span class="right row">${fix}</span></p>`)
  V.health = {
    title: 'Health',
    render: () =>
      head('Health', '7 findings on 4 machines, from the last evaluation', `<span class="busy">${spin('dot')} Evaluating…</span>${btn('↻ Check again')}`) +
      `<div class="grid tiles">${tile('Needs attention', 1, 'errors', 'err')}${tile('Warnings', 2, 'worth fixing', 'warn')}${tile('Suggestions', 4, 'good practice', 'acc')}${box({ title: 'Fleet', cls: 'tile' }, `<pre class="art nofx">homeserver  ${A.spark([3, 4, 4, 5, 6, 6, 7, 8])} <span class="ok">ok</span>\nworkstation ${A.spark([5, 5, 6, 4, 6, 7, 7, 8])} <span class="ok">ok</span>\npi          ${A.spark([6, 6, 5, 5, 4, 3, 2, 1])} <span class="warn">2</span>\noldbox      ${A.spark([4, 3, 0, 0, 0, 0, 0, 0])} <span class="err">✖</span></pre>`)}</div>
       <div class="sp"></div><h2 class="err">Needs attention</h2><div class="sp"></div><div class="stack">
       ${finding('err', 'oldbox does not evaluate', 'error: infinite recursion encountered, at hosts/oldbox/default.nix:4:13', ['oldbox'], btn('Open file', 'primary'))}</div>
       <div class="sp"></div><h2 class="warn">Warnings</h2><div class="sp"></div><div class="stack">
       ${finding('warn', 'SSH accepts passwords', 'Anyone who can reach port 22 may guess a password. Keys cannot be guessed.', ['pi'], `<span class="mut">Writing 1 of 2 · pi</span> ${bar(0.5, 18, true)}`)}
       ${finding('warn', 'nixpkgs is 74 days old', 'Security fixes land in nixos-26.05 every few days; this lock predates 31 of them.', ['homeserver', 'workstation', 'pi'], btn('Update nixpkgs', 'primary', 'data-act="busy"'))}</div>
       <div class="sp"></div><h2 class="info">Suggestions</h2><div class="sp"></div><div class="stack">
       ${finding('info', 'No automatic garbage collection', 'Old generations pile up until the disk is full.', ['pi'], btn('Collect weekly, keep 14 days'))}
       ${finding('info', 'htop is on every machine except pi', 'Probably meant to be everywhere.', ['pi'], '<span class="ok">✓ Applied on pi</span> <a href="#/changes">Review and commit in Changes ›</a>')}
       ${finding('info', 'SSH without fail2ban', 'Repeated failed logins are not banned.', ['homeserver'], btn('Enable fail2ban'))}
       ${finding('info', 'pi is Code only', 'Turn on visual editing to fix findings on pi with one click.', ['pi'], btn('How to turn it on'))}</div><div class="sp"></div>`,
  }

  // ---- Deployments ---------------------------------------------------------------------------
  V.deployments = {
    title: 'Deployments',
    render: () =>
      head('Deployments', 'What comin deployed on each machine, as the machines report it', `<span class="busy">${spin('dot')} Rolling out</span>${btn('↻ Refresh')}`) +
      `<div class="grid tiles">${tile('Up to date', 1, 'on the last commit of their branch', 'ok')}${tile('Behind', 1, 'catching up now', 'warn')}${tile('Failed', 1, 'the last deployment failed', 'err')}${tile('Silent or unknown', 1, 'not reporting')}</div><div class="sp"></div>
       ${box({ title: 'Rollout on main', note: '1 of 3 machines on 4e1f9a2' }, `<p>${bar(0.42, 60, true)} <span class="mut">1 / 3</span></p><div class="sp"></div><pre class="rollout nofx scene" id="rollout"></pre>`)}
       <div class="sp"></div><h2>Machines</h2><div class="sp"></div>
       ${table(['Machine', 'State', 'Runs', 'Deployed', 'Branch', 'Last report', ''], [
         ['<b>homeserver</b>', badge('ok', 'Up to date'), '4e1f9a2', '12 min ago', '⌥ main', '40 s ago', ''],
         ['<b>workstation</b>', badge('warn', '1 behind') + ' ' + spin(), 'b07c3d1', '2 h ago', '⌥ main', '12 s ago', ''],
         ['<b>pi</b>', badge('err', 'Build failed'), 'b07c3d1', '2 h ago', '⌥ main', '3 min ago', btn('Revert the commit that broke pi', 'danger', 'data-act="danger"')],
         ['<b>oldbox</b>', badge('off', 'Not reporting'), '<span class="faint">┄</span>', '<span class="faint">┄</span>', '<span class="faint">┄</span>', '19 days ago', btn('Forget')],
       ])}
       <div class="sp"></div>${box({ title: 'pi · Build of 4e1f9a2 failed 3 min ago', tone: 'err' }, `<pre class="log nofx">error: builder for '/nix/store/8f3ka1c…-jellyfin-web-10.11.2.drv' failed with exit code 1\n       last 2 log lines:\n       > FATAL ERROR: Reached heap limit Allocation failed - JavaScript heap out of memory\n       > npm error code 134</pre>`)}
       <div class="sp"></div><p class="mut">▸ Setup: the address, the report token and how to install it</p><div class="sp"></div>`,
    mount(view) {
      // Commits run down a rail; each machine is parked on the commit it runs, and the one
      // catching up climbs the rail toward the tip.
      const pre = view.querySelector('#rollout')
      A.scene(pre, (t) => {
        const climb = '⇡↑⇡ '[t % 4]
        const dots = '∙•●•'[t % 4]
        pre.innerHTML = [
          ` <b class="acc">◉</b> <b>4e1f9a2</b>  homeserver: enable jellyfin          <span class="mut">you · 12 min ago</span>    <span class="ok">▣ homeserver</span>`,
          ` │${climb.trim() ? ` <span class="warn">${climb}</span>` : '  '}                                                                    <span class="warn">${dots} workstation is switching…</span>`,
          ` ◉ <b>b07c3d1</b>  workstation: add neovim, spotify     <span class="mut">you · 2 h ago</span>       <span class="warn">▣ workstation</span>  <span class="err">▣ pi ✖ build failed</span>`,
          ` │`,
          ` ○ <b>a3391fe</b>  flake.lock: update nixpkgs           <span class="mut">scheduled update · yesterday</span>   <span class="faint">no machine</span>`,
          ` ┆`,
          ` ├─ <span class="acc">⌥ testing-pi</span> ── ◉ <b>77c1a0e</b> try: mosquitto 2.1      <span class="mut">trial · reboot undoes it</span>   <span class="faint">▣ pi (ended)</span>`,
          ` ┆`,
          ` ○ <span class="faint">Older or unknown</span>   <span class="faint">○ Not reporting: oldbox</span>`,
        ].join('\n')
      })
    },
  }

  // ---- Inputs --------------------------------------------------------------------------------
  V.inputs = {
    title: 'Inputs',
    render: () =>
      head('Inputs', '5 flake inputs every machine is built from, pinned in flake.lock', `${btn('Lock')}${btn('Update all', 'primary', 'data-act="busy"')}`) +
      table(['Input', 'Source', 'Revision', 'Updated', '', ''], D.inputs.map(([n, s, r, a, t]) => [`<b>${n}</b>`, `<span class="mut">${s}</span>`, `${r} <a class="mut" title="Copy">❐</a>`, `${dot(t === 'ok' ? 'off' : t)} ${a}`, t === 'err' ? badge('err', 'Outdated', '▲') : '', btn('Update')])) +
      `<p class="mut">● older than 30 days amber, 60 red</p><div class="sp"></div>
       <div class="grid cols11">
       ${box({ title: 'Update nixpkgs', note: 'running…', tone: 'accent' }, `<p>${spin()} <span class="acc">Updating…</span> ${bar(0.64, 30, true)} <span class="mut">evaluating 3 of 4</span></p><div class="sp"></div><pre class="log nofx" id="joblog"></pre>`)}
       ${box({ title: 'Scheduled update' }, `<p class="row">${sw('Update the inputs on a schedule', true)}<span class="right">${btn('Update now')}</span></p><div class="sp"></div><p><span class="mut">Every </span>${seg(['day', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'], 6)}</p><p><span class="mut">at    </span>${seg(['03:00 ▾'], -1)} <span class="mut">Europe/Madrid, the server's clock; next Sat 28 Nov 03:00</span></p><div class="sp"></div><p>${dot('ok')} <b>Ready to merge into main</b> <span class="mut">· checked, built 3/3</span></p><div class="sp"></div><div class="row">${btn('Merge into main', 'primary')}${btn('Discard')}</div>`)}
       </div>
       <div class="sp"></div><h2>What the update changes</h2><div class="sp"></div>
       ${table(['Package', 'homeserver', 'workstation', 'pi'], [['<b>firefox</b>', '<span class="faint">┄</span>', '143.0 <span class="acc">→ 144.0</span>', '<span class="faint">┄</span>'], ['<b>openssl</b>', '3.5.2 <span class="acc">→ 3.5.4</span>', '3.5.2 <span class="acc">→ 3.5.4</span>', '3.5.2 <span class="acc">→ 3.5.4</span>'], ['<b>linux</b>', '6.12.47 <span class="acc">→ 6.12.58</span>', '6.12.47 <span class="acc">→ 6.12.58</span>', '<span class="faint">unchanged</span>'], ['<b>jellyfin</b>', '10.11.1 <span class="acc">→ 10.11.2</span>', '<span class="faint">┄</span>', '<span class="faint">┄</span>']])}
       <div class="sp"></div><div class="row">${btn('Review and commit', 'primary')}${btn('Discard the update', 'danger')}</div><div class="sp"></div>`,
    mount(view) {
      // The job log scrolls like a terminal: new lines are typed at the bottom.
      const pre = view.querySelector('#joblog')
      const lines = ['$ nix flake update nixpkgs', "• Updated input 'nixpkgs':", "    'github:NixOS/nixpkgs/8f3ka1c' (2026-09-10)", "  → 'github:NixOS/nixpkgs/d02b6e4' (2026-11-22)", '$ nix eval .#nixosConfigurations.homeserver', 'evaluating derivation…  ok 3.8 s', '$ nix eval .#nixosConfigurations.workstation', 'evaluating derivation…  ok 4.1 s', '$ nix eval .#nixosConfigurations.pi', 'evaluating derivation…']
      let shown = A.still ? lines.length : 3
      A.scene(pre, (t) => {
        if (t % 9 === 8 && shown < lines.length) shown++
        return lines.slice(Math.max(0, shown - 7), shown).join('\n') + ' ' + '▁▃▅▇▅▃'[t % 6]
      })
    },
  }

  // ---- Changes -------------------------------------------------------------------------------
  const diffFile = (path, lines, stat) => box({ frame: 'square', title: path, note: stat }, codeLines(lines))
  V.changes = {
    title: 'Changes',
    render: () => `<div class="split">
      ${box({ cls: 'side', title: 'Commit' }, `
        ${tabs([['Commit', 2], ['Checks']], 0)}
        <p class="mut">Commit message</p><textarea placeholder="What changes, and why (Ctrl+Enter commits)">homeserver, workstation: add neovim</textarea><div class="sp"></div>
        <div class="row">${btn('▷ Run checks', '', 'data-act="checks"')}${btn('Commit', 'primary', 'data-act="committed"')}</div><div class="sp"></div>
        <p class="row">${chk('<b>Files</b> <span class="count">2</span> <span class="mut">1 staged</span>', false)}<span class="right mut">↶ ↻</span></p>
        <div class="rows"><a>${chk('', true)}<span class="grow">managed.json <span class="mut">hosts/homeserver</span></span><span class="ok">New</span></a><a>${chk('', false)}<span class="grow">managed.json <span class="mut">hosts/workstation</span></span><span class="ok">New</span></a></div>
        <div class="sp"></div><div class="rule dash"></div>
        <p class="mut">Checks</p><p>${dot('ok')} nix flake check <span class="mut">passed · 41 s</span></p><p>${dot('ok')} build homeserver <span class="mut">cached</span></p><p>${dot('busy')} build workstation ${bar(0.7, 12, true)}</p><p>${dot('off')} build pi <span class="mut">queued</span></p>`)}
      <div class="body">
        <div class="row">${big('Changes')}<span class="mut">2 changed files, 1 staged · <span class="sig-add">+13</span> <span class="sig-del">−0</span> lines</span></div><div class="sp"></div>
        <h2>On the machines</h2><div class="sp"></div>
        ${table(['Machine', 'Packages', 'Services', 'Rebuild'], [['<b>homeserver</b>', '<span class="sig-add">+neovim +spotify</span>', '<span class="faint">unchanged</span>', 'switch, no reboot'], ['<b>workstation</b>', '<span class="sig-add">+neovim</span>', '<span class="faint">unchanged</span>', 'switch, no reboot'], ['<b>pi</b>', '<span class="faint">unchanged</span>', '<span class="faint">unchanged</span>', '<span class="faint">nothing to do</span>']])}
        <p class="note"><a href="#/machines/oldbox">oldbox</a> does not evaluate, so it is not compared.</p>
        <div class="sp"></div><h2>Files</h2><div class="sp"></div><div class="stack">
        ${diffFile('hosts/homeserver/managed.json', [['hunk', ' Line 1'], ['add', '+ {'], ['add', '+   "packages": ['], ['add', '+     "neovim",'], ['add', '+     "spotify"'], ['add', '+   ],'], ['add', '+   "settings": {}'], ['add', '+ }']], 'New  +7 −0')}
        ${diffFile('hosts/workstation/managed.json', [['hunk', ' Line 1'], ['add', '+ {'], ['add', '+   "packages": ['], ['add', '+     "neovim"'], ['add', '+   ],'], ['add', '+   "settings": {}'], ['add', '+ }']], 'New  +6 −0')}
        ${diffFile('modules/common.nix', [['hunk', ' Line 12'], '    services.openssh.enable = true;', ['del', '-   services.openssh.settings.PasswordAuthentication = true;'], ['add', '+   services.openssh.settings.PasswordAuthentication = false;'], '    services.fail2ban.enable = true;'], 'Modified  +1 −1')}
        </div><div class="sp"></div>
      </div></div>`,
  }

  // ---- History -------------------------------------------------------------------------------
  V.history = {
    title: 'History',
    crumb: false,
    render: () => `<div class="split">
      ${box({ cls: 'side', title: 'History', note: '↻' }, `
        <label class="field"><span>File history</span><input type="text" placeholder="Any file, e.g. flake.nix"></label><p><a class="mut">Only the repository history</a></p><div class="sp"></div>
        <p class="mut">Repository</p>
        <div class="rows graph">${D.commits.map(([sha, subject, who, ago], i) => `<a class="${i === 0 ? 'on' : ''}"><span>${i === 0 ? '◉' : '○'}</span><span class="grow"><b>${subject}</b></span></a><a class="sub ${i === 0 ? 'on' : ''}"><span>${i === D.commits.length - 1 ? ' ' : '│'}</span><span class="grow mut">${sha} · ${who} · ${ago}</span></a>`).join('')}</div>`)}
      <div class="body">
        <div class="row">${big('4e1f9a2')}</div><h3>homeserver: enable jellyfin</h3>
        <p class="row mut">you · 12 min ago <a>❐ copy sha</a><a>↗ Forgejo</a><span class="right">${btn('↶ Revert', 'danger', 'data-act="danger"')}</span></p><div class="sp"></div>
        <h2>On the machines</h2><div class="sp"></div>
        ${table(['Machine', 'Services', 'Ports', 'Deployed'], [['<b>homeserver</b>', '<span class="sig-add">+jellyfin</span>', '<span class="sig-add">+tcp/8096</span>', badge('ok', '12 min ago')], ['<b>workstation</b>', '<span class="faint">unchanged</span>', '<span class="faint">unchanged</span>', badge('warn', 'switching', '◐')], ['<b>pi</b>', '<span class="faint">unchanged</span>', '<span class="faint">unchanged</span>', badge('err', 'build failed')]])}
        <div class="sp"></div><h2>Files <span class="count">1</span></h2><div class="sp"></div>
        ${diffFile('hosts/homeserver/managed.json', [['hunk', ' Line 2'], '    "packages": [ "neovim", "spotify" ],', ['del', '-   "settings": {}'], ['add', '+   "settings": {'], ['add', '+     "services.jellyfin.enable": true,'], ['add', '+     "services.jellyfin.openFirewall": true'], ['add', '+   }'], '  }'], 'Modified  +4 −1')}
      </div></div>`,
  }

  // ---- Code ----------------------------------------------------------------------------------
  const FILE = `{ config, pkgs, ... }:\n{\n  imports = [ ./hardware-configuration.nix ../../modules/common.nix ];\n\n  networking.hostName = "homeserver";\n  time.timeZone = "Europe/Madrid";\n\n  # Media and files, behind tailscale.\n  services.jellyfin.enable = true;\n  services.nextcloud = {\n    enable = true;\n    hostName = "cloud.home.arpa";\n    config.adminpassFile = config.sops.secrets."nextcloud/admin".path;\n  };\n  services.forgejo.enable = true;\n\n  virtualisation.podman.enable = true;\n  networking.firewall.allowedTCPPorts = [ 80 443 8096 ];\n\n  system.stateVersion = "26.05";\n}`
  V.code = {
    title: 'Code',
    render: () => `<div class="split">
      ${box({ cls: 'side', title: 'Files', note: '▤+ ▸+ ↻ «', attrs: 'style="width:34ch"' }, `<div class="rows tree">${[[0, '▾ hosts/'], [1, '▾ homeserver/'], [2, 'default.nix', 'M', 'on'], [2, 'hardware-configuration.nix'], [2, 'managed.json', 'A'], [1, '▸ workstation/'], [1, '▸ pi/'], [1, '▸ oldbox/'], [0, '▾ modules/'], [1, 'common.nix', 'M'], [1, 'managed.nix'], [1, 'secrets.nix'], [0, '▸ home/'], [0, '▸ secrets/'], [0, '.sops.yaml'], [0, 'flake.lock'], [0, 'flake.nix'], [0, 'README.md', '?']].map(([d, n, g, on]) => `<a class="${on || ''}" style="--d:${d}"><span class="grow">${n}</span><span class="${{ M: 'warn', A: 'ok', '?': 'mut' }[g] || ''}">${g || ''}</span></a>`).join('')}</div>`)}
      ${box({ cls: 'body', frame: 'square', attrs: 'style="display:flex;flex-direction:column;overflow:hidden"' }, `
        <div class="edtabs"><span class="on">default.nix ●</span><span>common.nix ×</span><span>flake.nix ×</span></div><div class="rule"></div>
        <div class="code nofx" style="flex:1">${FILE.split('\n').map((l, i) => `<div><span>${i === 8 ? nix(l).replace('>true<', '><span class="cur">t</span>rue<') : nix(l) || ' '}</span></div>`).join('')}</div>
        <div class="rule"></div><div class="statusbar"><span class="mode">Ln 9, Col 31</span><span>Ctrl S save · Shift Alt F format · F12 definition</span><span class="grow"></span><a>↺ History of this file</a><a>↗ Open in Forgejo</a><span>${dot('ok')} nixd ready</span></div>`)}
    </div>`,
  }

  // ---- Settings ------------------------------------------------------------------------------
  V.settings = {
    title: 'Settings',
    render: () => `<div class="grid cols2"><div class="stack">
      ${box({ title: 'Repository' }, kv([['Working copy', '/var/lib/nixos-manager/fleet <span class="mut">NIXMGR_REPO</span>'], ['Branch', '⌥ main'], ['Upstream', 'origin/main'], ['Commit author', 'Pablo &lt;pablo@home.arpa&gt;']]) + `<p class="note">comin deploys main; a trial goes to testing-&lt;machine&gt;.</p>`)}
      ${box({ title: 'Forgejo', note: '● connected' }, `${field('Repository', '', 'ssh://git@forge.home.arpa/fleet/nixos.git')}<div class="sp"></div>${kv([['Deploy key', 'ssh-ed25519 AAAAC3Nz…Qm4t <a class="mut">❐</a>'], ['Connection', `${dot('ok')} reachable · can push`], ['Push webhook', `${dot('ok')} last delivery 12 min ago`], ['Commit statuses', `${dot('off')} no access token`]])}<div class="sp"></div><div class="row">${btn('Test connection')}${btn('Push')}${btn('What git said')}</div>`)}
      ${box({ title: 'Notifications' }, `${field('Console address', '', 'https://nix.home.arpa')}<div class="sp"></div><p>${sw('ntfy', true)}</p>${field('  Topic address', '', 'https://ntfy.home.arpa/fleet')}<p>${sw('Webhook', false)}</p><p>${sw('Email', false)}</p><div class="sp"></div><div class="row">${btn('Save', 'primary', 'data-act="saved"')}${btn('Send a test message', '', 'data-act="info"')}</div>`)}
      ${box({ title: 'Secrets', note: 'sops-nix' }, `${table(['Machine', 'SSH host key', ''], [['<b>homeserver</b>', 'ssh-ed25519 …h2Vd', '<a>Change</a>'], ['<b>workstation</b>', 'ssh-ed25519 …9KqL', '<a>Change</a>'], ['<b>pi</b>', '<span class="warn">▲ No key: gets no secrets</span>', '<a>Add key</a>']])}<div class="sp"></div>${table(['Secret', 'For', 'Value', ''], [['<b>nextcloud/admin</b>', 'homeserver', '••••••••••••', '<a>Show</a> <a>Change</a>'], ['<b>wifi/home</b>', 'workstation, pi', 'hunter2-not-really', '<a>Hide</a> <a>Change</a>']])}<div class="sp"></div><div class="row">${btn('Save secret', 'primary')}${btn('Re-encrypt')}</div>`)}
      ${box({ title: 'API tokens', note: 'MCP' }, `<div class="row">${field('New token name', 'style="width:24ch"', 'desktop-mcp')}${seg(['Read only', 'Edit, no push', 'Full'], 1)}${btn('Create', 'primary')}</div><div class="sp"></div>${table(['Name', 'Access', 'Created', 'Expires', 'Last used', ''], [['<b>ci</b>', 'Read only', '3 Oct', 'Never', '2 h ago', btn('Revoke', 'danger')], ['<b>laptop-mcp</b>', 'Full', '1 Nov', '30 days', 'yesterday', btn('Revoke', 'danger')]])}<div class="sp"></div>${kv([['MCP endpoint', 'https://nix.home.arpa/mcp <a class="mut">❐</a>']])}`)}
      </div><div class="stack">
      ${box({ title: 'Keyboard shortcuts' }, kv([['<kbd>⌃K</kbd>', 'Command palette'], ['<kbd>/</kbd>', 'Search this view'], ['<kbd>g</kbd> <kbd>f</kbd>', 'Fleet'], ['<kbd>g</kbd> <kbd>m</kbd>', 'Machines'], ['<kbd>g</kbd> <kbd>p</kbd> <kbd>s</kbd> <kbd>o</kbd>', 'Packages, Services, Options'], ['<kbd>g</kbd> <kbd>a</kbd> <kbd>d</kbd> <kbd>i</kbd>', 'Health, Deployments, Inputs'], ['<kbd>g</kbd> <kbd>c</kbd> <kbd>h</kbd> <kbd>e</kbd>', 'Changes, History, Code'], ['<kbd>Esc</kbd>', 'Close'], ['<kbd>⌃S</kbd>', 'Save the file'], ['<kbd>⌃⏎</kbd>', 'Commit']]))}
      ${box({ title: 'Appearance', frame: 'dash' }, `<p class="row"><span class="mut">Theme</span><span class="seg"><a class="on" onclick="U.theme('night')">Night</a><a onclick="U.theme('print')">Print</a></span></p><p class="mut">Print is the same console on greenbar paper, for daylight.</p>`)}
      ${box({ title: 'About' }, `<pre class="art nofx">  ╲  λ  ╱\n   ╲ │ ╱     nixos-manager\n λ ──❄── λ   Your fleet's NixOS configuration,\n   ╱ │ ╲     in one place.\n  ╱  λ  ╲</pre><div class="sp"></div>${btn('Watch the intro again', '', `onclick="location.hash='#/intro'"`)}`)}
      </div></div><div class="sp"></div>`,
  }

  // ---- The gate every view waits behind ------------------------------------------------------
  V.states = {
    title: 'States',
    render: () => `<div class="grid cols11">
      ${box({ title: 'Loading' }, `<div class="center" style="height:calc(var(--lh)*14)"><h3>${spin()} Evaluating the flake…</h3><p class="mut">Nix is working out what every machine runs.</p>${skel(40)}${skel(40)}${skel(28)}</div>`)}
      ${box({ title: 'Error', tone: 'err' }, `<div class="center" style="height:calc(var(--lh)*14)"><pre class="art err nofx">  ╭──────────╮\n  │ ✖  ✖     │\n  │   ━━━    │\n  ╰──────────╯</pre><h3>The flake does not evaluate</h3><pre class="pre">error: undefined variable 'pkgz' at flake.nix:14:9</pre><div class="row">${btn('Open flake.nix in Code', 'primary')}${btn('Evaluate again')}</div></div>`)}
      ${box({ title: 'Empty', frame: 'dash' }, `<div class="center" style="height:calc(var(--lh)*14)"><pre class="art nofx">  ┌┄┄┄┄┄┄┐ ┌┄┄┄┄┄┄┐\n  ┆  +   ┆ ┆      ┆\n  └┄┄┄┄┄┄┘ └┄┄┄┄┄┄┘</pre><h3>No machines yet</h3><p class="mut">Add a folder to hosts/ and it shows up here.</p><div class="row">${btn('+ New machine', 'primary', 'data-act="wizard"')}${btn('Open flake.nix')}</div></div>`)}
      ${box({ title: 'All clear', tone: 'ok' }, `<div class="center" style="height:calc(var(--lh)*14)">${big('OK', 'ok')}<h3>Nothing to fix</h3><p class="mut">Every machine evaluates, keeps its firewall on<br>and accepts SSH keys only.</p></div>`)}
    </div>`,
  }
})()
