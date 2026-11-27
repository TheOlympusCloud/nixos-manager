// Part 2, chapters 13 to 17: first steps, finding your way, the fleet graph, machines, the machine page.
import { scene, type Pic, type Stage } from './film'
import { COLS, PIC, caption, card, header, len, list, mid, walk } from './kit'

const RAIL: [number, number, number, number] = [0.005, 0.055, 0.15, 0.29]
const WIZARD: [number, number, number, number] = [0.205, 0.085, 0.52, 0.76]
const PICKER: [number, number, number, number] = [0.22, 0.12, 0.56, 0.75]
const DRAWER: [number, number, number, number] = [0.56, 0.05, 0.44, 0.9]
const VIEW: [number, number, number, number] = [0.11, 0.05, 0.89, 0.9]

/** The capture currently on the stage. */
const current = (s: Stage) => [...s.els].find((e) => e.name.startsWith('pic:')) as Pic

/** Outline recorded boxes one after another, briefly. */
async function sweepMarks(s: Stage, keys: string[], ticks = 2) {
  for (const k of keys) {
    const m = current(s).mark(k, 'accent')
    await s.wait(ticks)
    m.remove()
  }
}

scene('S13.1', (s) =>
  walk(s, [
    ['show', 'intro-hero'],
    ['show', 'intro-drift', 'story'],
    ['show', 'settings-top', 'watch it again'],
    ['look', 'sec:ABOUT', undefined, 1.8],
    ['show', 'login', 'log in'],
    ['look', 'password', undefined, 3],
    ['cap', '10 attempts / 10 min per address · sessions last 7 days', 'rate limited', 'warn'],
  ]),
)

const STARTER = ['flake.nix', 'hosts/example/default.nix', 'hosts/example/hardware-configuration.nix', 'hosts/example/managed.json',
  'home/pablo.nix', 'modules/common.nix', 'modules/managed.nix', 'modules/report-deployments.nix', 'modules/secrets.nix',
  'modules/binary-cache.nix', '.forgejo/workflows/check.yml']

scene('S13.2', async (s) => {
  await walk(s, [['show', 'fresh-code'], ['look', [0.1, 0.06, 0.13, 0.55], 'starter fleet', 1.2]])
  await s.until('every folder', 2)
  s.clear()
  header(s)
  const box = s.box(mid(60), 6, 60, STARTER.length + 4, { title: 'the starter fleet', kind: 'square' })
  const rows = list(s, box.inner.x + 1, box.inner.y + 1, STARTER, '', '·', box)
  const lit = (i: number, cls: string) => (rows[i].node.className = cls)
  lit(1, 'c-warn')
  await s.until('host name')
  caption(s, 'folder name = host name = nixosConfigurations.<name>')
  await s.until('hand-written')
  lit(1, 'c-ok')
  for (const [cue, i, cls] of [['hardware', 2, 'c-acc'], ['where the console', 3, 'c-warn'], ['common', 5, 'c-acc'], ['applies', 6, 'c-acc'], ['example', 4, 'c-acc']] as const) {
    await s.until(cue)
    lit(i, cls)
  }
})

scene('S13.3', (s) =>
  walk(s, [
    ['show', 'fresh-machines'],
    ['look', [0.55, 0.84, 0.44, 0.1], 'no lock file', 1.5],
    ['show', 'fresh-inputs', 'click lock'],
    ['look', 'lock', undefined, 4],
    ['show', 'fresh-lock-log', 'records'],
    ['look', [0.11, 0.5, 0.55, 0.32], undefined, 1.15],
    ['show', 'fresh-changes', 'stage'],
    ['look', [0.11, 0.07, 0.29, 0.62], undefined, 1.15],
  ]),
)

scene('S13.4', (s) =>
  walk(s, [
    ['show', 'forgejo-1'],
    ['look', 'sec:FORGEJO', 'numbered', 1.08],
    // Each numbered step, from its heading to the next (the capture's sec: boxes are the whole card).
    ['look', [506, 290, 1374, 310], 'one', 1.15],
    ['look', [506, 596, 1374, 450], 'two', 1.1],
    ['look', [506, 1038, 1374, 180], 'three', 1.3],
    ['show', 'forgejo-2', 'four'],
    ['look', [506, 60, 1374, 500], undefined, 1.1],
    ['look', [506, 828, 1374, 320], 'five', 1.2],
  ]),
)

scene('S14.1', async (s) => {
  await walk(s, [['show', 'fleet'], ['look', RAIL, 'rail', 1.2]])
  await s.until('each has', 2)
  await sweepMarks(s, ['Fleet', 'Machines', 'Packages', 'Services', 'Modules', 'Options', 'Health', 'Deployments', 'Dependencies', 'Inputs', 'Changes', 'History', 'Code'])
  caption(s, 'g then a letter: g f Fleet · g m Machines · g c Changes · g , Settings')
  for (const [cue, k] of [['g f', 'Fleet'], ['g m', 'Machines'], ['g c', 'Changes']]) {
    try {
      await s.until(cue, 1)
    } catch {
      continue
    }
    await sweepMarks(s, [k], 6)
  }
  await s.until('badges')
  for (const [k, tone] of [['Changes', 'accent'], ['Health', 'err'], ['Deployments', 'err']] as const) {
    const m = current(s).mark(k, tone)
    await s.wait(10)
    m.remove()
  }
})

scene('S14.2', (s) =>
  walk(s, [
    ['show', 'fleet'],
    ['look', [0.4, 0, 0.6, 0.045], 'top bar', 1.05],
    ['mark', 'status', 'state of the model'],
    ['mark', 'branch', 'current branch'],
    ['show', 'branch-menu', 'branch menu'],
    ['look', [0.4, 0.03, 0.35, 0.18], undefined, 1.3],
    ['show', 'fleet', 'ahead'],
    ['look', [0.7, 0, 0.3, 0.045], undefined, 1.1],
    ['mark', 'pull', 'pull and push'],
    ['mark', 'push'],
    ['show', 'palette', 'control k'],
    ['look', [0.25, 0.08, 0.62, 0.35], undefined, 1.2],
    ['cap', '/ searches inside the current view', 'slash'],
    ['nocap'],
    ['show', 'settings-top', 'lists all'],
    ['look', 'sec:KEYBOARD SHORTCUTS', undefined, 1.25],
    ['look', 'sec:THEME', 'two themes', 1.8],
    ['show', 'fleet-print', 'print'],
  ]),
)

scene('S15.1', (s) =>
  walk(s, [
    ['show', 'fleet'],
    ['look', [0.15, 0.08, 0.85, 0.85], 'home view', 1.0],
    ['mark', 'homeserver', 'each machine'],
    ['mark+', 'pi'],
    ['mark+', 'workstation'],
    ['look', [0.56, 0.17, 0.25, 0.65], 'around them', 1.15],
    ['show', 'fleet-files', 'files'],
    ['show', 'fleet-packages', 'package groups'],
    ['show', 'fleet-users', 'users'],
    ['show', 'fleet-inputs', 'flake inputs'],
    ['show', 'fleet', 'share a service'],
    ['look', [0.3, 0.17, 0.5, 0.65], undefined, 1.1],
    ['show', 'fleet-inspector', 'inspector'],
    ['look', [0.62, 0.05, 0.38, 0.9], undefined, 1.1],
    ['show', 'fleet', 'deployment state'],
    ['mark', 'homeserver', undefined, 'ok'],
    ['mark+', 'pi', undefined, 'err'],
    ['mark+', 'workstation', undefined, 'warn'],
    ['cap', 'deployed · behind · failed · silent'],
  ]),
)

scene('S16.1', (s) =>
  walk(s, [
    ['show', 'machines'],
    ['look', [0.11, 0.08, 0.89, 0.25], 'every', 1.1],
    ['show', 'machines-2', 'standalone'],
    ['look', [0.11, 0.1, 0.89, 0.6], undefined, 1.1],
    ['show', 'machines', 'open'],
    ['mark', 'new', 'create'],
    ['mark', 'sec:INSTALLER ISO', 'installer'],
  ]),
)

const ROLES = [
  ['Minimal', 'git, htop', 'nothing switched on'],
  ['Server', 'btop, git, htop, tmux', 'SSH keys only, no root login · fail2ban · firewall · weekly GC, 14 days · optimise'],
  ['Desktop', 'firefox, git, htop, vlc', 'Plasma 6 and SDDM · PipeWire · printing · NetworkManager · Bluetooth · zram · GC'],
]

scene('S16.2', async (s) => {
  await walk(s, [['show', 'nm-name'], ['look', WIZARD, undefined, 1.05], ['show', 'nm-role', 'pick a role'], ['look', WIZARD, undefined, 1.05]])
  await s.until('minimal', 2)
  s.clear()
  header(s)
  const w = 2 + 10 + 26 + len(ROLES[1][2]) + 4
  const t = s.box(mid(w), 9, w, 10, { title: 'roles', tag: 'what lands in managed.json', kind: 'square' })
  s.text(t.inner.x, t.inner.y, 'Role'.padEnd(10) + 'Packages'.padEnd(26) + 'Settings', 'c-acc', t)
  const rows = ROLES.map((r, i) => s.text(t.inner.x, t.inner.y + 2 + i * 2, r[0].padEnd(10) + r[1].padEnd(26) + r[2], 'c-m', t))
  rows[0].node.className = 'c-b'
  for (const [cue, i] of [['server turns', 1], ['desktop adds', 2]] as const) {
    await s.until(cue)
    rows.forEach((r) => (r.node.className = 'c-m'))
    rows[i].node.className = 'c-b'
  }
  await s.until('everything')
  rows.forEach((r) => (r.node.className = ''))
  await s.until('platform', 2)
  s.clear()
  await walk(s, [
    ['show', 'nm-platform'],
    ['look', WIZARD, undefined, 1.05],
    ['show', 'nm-user-filled', 'no role'],
    ['look', WIZARD, undefined, 1.05],
    ['show', 'nm-disk', 'physical machine'],
    ['look', WIZARD, undefined, 1.05],
    ['show', 'nm-review', 'nothing is committed'],
    ['look', WIZARD, undefined, 1.05],
  ])
})

scene('S16.3', (s) =>
  walk(s, [
    ['show', 'install-card'],
    ['look', 'card', 'install card', 1.3],
    ['show', 'install-hostkey', 'fetch its host key'],
    ['look', 'card', undefined, 1.1],
    ['cap', 'No password sent, nothing run on the machine', 'only reads', 'ok'],
    ['nocap', 'command'],
    ['show', 'install-command'],
    ['look', 'card', undefined, 1.1],
  ]),
)

const BOOT = ['boot the USB stick', 'sudo fleet-install: addresses and the host key', 'the console: fetch, compare, approve the key',
  'Enter: fetch the repository, show the disks', "type the machine's name", 'partition, nixos-install --flake',
  "set the users' passwords", 'push the hardware configuration', 'reboot: comin pulls, secrets open']

scene('S16.4', async (s) => {
  await walk(s, [
    ['show', 'iso-1'],
    ['look', 'sec:INSTALLER ISO', 'installer', 1.5],
    ['show', 'iso-building', 'pinned'],
    ['look', VIEW, undefined, 1.0],
    ['show', 'iso-done', 'usb stick'],
    ['look', VIEW, undefined, 1.0],
  ])
  await s.until('sudo', 2)
  s.clear()
  header(s)
  const b = s.box(mid(64), 6, 64, BOOT.length * 2 + 3, { title: 'sudo fleet-install', kind: 'square', tone: 'accent' })
  const step = (i: number) => s.type(b.inner.x, b.inner.y + 1 + i * 2, `${String(i + 1).padStart(2)}  ${BOOT[i]}`, i === 8 ? 'c-ok' : '', b)
  step(0)
  for (const [cue, i] of [['shows', 1], ['approve', 2], ['enter', 3], ['type', 4], ['partitions', 5], ['passwords', 6], ['hardware', 7], ['reboots', 8]] as const) {
    await s.until(cue)
    step(i)
  }
  await s.until('nothing secret')
  caption(s, 'Nothing secret is on the ISO', 'warn')
})

const TABS: [number, number, number, number] = [0.105, 0.245, 0.62, 0.045]

scene('S17.1', (s) =>
  walk(s, [
    ['show', 'm-overview'],
    ['look', TABS, 'nine tabs', 1.05],
    ['look', [0.11, 0.3, 0.89, 0.15], 'summarises', 1.1],
    ['mark', 'sec:FEATURES', 'services'],
    ['mark', 'sec:USERS AND ACCESS', 'users'],
    ['show', 'm-overview-2', 'secrets'],
    ['look', [0.6, 0.2, 0.4, 0.6], undefined, 1.1],
    ['show', 'm-overview', 'deployment'],
    ['look', 'sec:DEPLOYMENT', undefined, 1.3],
  ]),
)

scene('S17.2', (s) =>
  walk(s, [
    ['show', 'm-essentials'],
    ['look', [0.11, 0.28, 0.89, 0.7], 'essentials', 1.05],
    ['mark', 'sec:REGION', 'time zone'],
    ['mark', 'sec:MAINTENANCE', 'garbage'],
    ['show', 'm-essentials-2', 'compressed'],
    ['look', VIEW, undefined, 1.05],
    ['show', 'ess-zram', 'flip a switch'],
    // The capture's 'row' box is the label only: frame the whole row, label to switch.
    ['look', [522, 1216, 1124, 48], undefined, 1.6],
    ['mark', [522, 1216, 1124, 48]],
    ['cap', '→ hosts/homeserver/managed.json · nothing reaches the machine yet', 'nothing reaches', 'warn'],
  ]),
)

scene('S17.3', (s) =>
  walk(s, [
    ['show', 'pkg-list'],
    ['look', [0.11, 0.38, 0.89, 0.5], 'packages lists', 1.05],
    ['show', 'pkg-add', 'add packages'],
    ['look', PICKER, undefined, 1.1],
    ['show', 'pkg-add-picked', 'what the machine'],
    ['look', PICKER, undefined, 1.1],
    ['show', 'pkg-remove-confirm', 'remove works'],
    ['show', 'pkg-removed', 'records'],
    ['look', 'nano', undefined, 1.8],
    ['cap', 'Removed here · the Nix that adds it stays untouched', undefined, 'warn'],
    ['nocap', 'replace'],
    ['show', 'pkg-replace'],
    ['look', [0.43, 0.04, 0.57, 0.85], undefined, 1.05],
  ]),
)

scene('S17.4', async (s) => {
  await walk(s, [
    ['show', 'pkg-customise'],
    ['look', [0.47, 0.05, 0.53, 0.9], 'customise', 1.05],
  ])
  const unmark = () => [...s.els].filter((e) => e.parent === current(s)).forEach((m) => m.remove())
  // One outline at a time: the sections sit close, and outlines must not cross the headings beside them.
  for (const [cue, key] of [['another nix', 'sec:From another nixpkgs'], ['arguments', 'sec:Arguments .override'],
    ['build attributes', 'sec:Attributes .overrideAttrs'], ['pin a version', 'sec:Version and source'],
    ['patches', "sec:Patches after the package's own"], ['nix file', 'sec:Nix']]) {
    await s.until(cue)
    unmark()
    current(s).mark(key)
  }
  await s.until('overlay', 2)
  s.clear()
  header(s)
  const pkgs = ['btop', 'htop', 'git', 'curl']
  const ov = card(s, mid(46), 7, ['btop = btop.override { … }'], { title: 'overlay', tone: 'warn' })
  const row = pkgs.map((p, i) => card(s, 38 + i * 18, 17, [p], { w: 14, kind: 'square' }))
  s.text(38, 21, "homeserver's packages", 'c-m')
  await s.wait(8)
  s.text(44, 11, '│\n│\n│\n│\n│\n▼', 'c-warn')
  row[0].node.dataset.tone = 'warn'
  row[0].rows[0].node.textContent = 'btop ✎'
  await s.until('everything')
  caption(s, 'everything that uses the package gets the customised one', 'warn')
  await s.until('reset')
  delete row[0].node.dataset.tone
  row[0].rows[0].node.textContent = 'btop'
  caption(s, 'Reset to default')
})

scene('S17.5', (s) =>
  walk(s, [
    ['show', 'pkg-every'],
    ['look', 'every', 'every machine', 1.8],
    ['look', [0.11, 0.44, 0.89, 0.05], 'modules slash', 1.1],
    ['cap', "A machine's own value wins where both set something", 'machine sets'],
    ['nocap', 'copy to'],
    ['show', 'options-tz-copy'],
    ['look', [0.3, 0.2, 0.7, 0.7], undefined, 1.05],
    ['show', 'nix-for', 'anything the console'],
    ['look', [0.15, 0.05, 0.85, 0.9], undefined, 1.05],
  ]),
)

scene('S17.6', (s) =>
  walk(s, [
    ['show', 'm-dependencies'],
    ['look', [0.11, 0.3, 0.5, 0.6], 'closure', 1.1],
    ['show', 'deps-search', 'search'],
    ['look', 'search', undefined, 2.5],
    ['show', 'deps-openssl', 'version'],
    ['look', [0.58, 0.39, 0.4, 0.12], undefined, 1.4],
    ['look', [0.58, 0.5, 0.4, 0.08], 'why it is here', 1.4],
  ]),
)

scene('S17.7', (s) =>
  walk(s, [
    ['show', 'm-services'],
    ['look', [0.11, 0.3, 0.89, 0.6], 'services and programs', 1.05],
    ['show', 'catalog', 'catalog'],
    ['look', DRAWER, undefined, 1.05],
    ['show', 'catalog-search', 'jellyfin and immich'],
    ['look', DRAWER, undefined, 1.05],
    ['show', 'svc-pi-after', 'adding'],
    ['look', [0.11, 0.3, 0.89, 0.5], undefined, 1.1],
    ['show', 'svc-configure', 'configure'],
    ['look', DRAWER, undefined, 1.05],
  ]),
)

scene('S17.8', (s) => walk(s, [['show', 'm-modules-media'], ['look', [0.11, 0.28, 0.89, 0.5], 'turns', 1.1]]))

scene('S17.9', (s) =>
  walk(s, [
    ['show', 'm-users'],
    ['look', [0.11, 0.28, 0.6, 0.5], 'users lists', 1.1],
    ['show', 'users-add', 'add a user'],
    ['look', [0.11, 0.2, 0.89, 0.7], undefined, 1.05],
    ['show', 'm-users', 'whole lists'],
    ['look', [0.11, 0.3, 0.6, 0.05], undefined, 1.3],
    ['cap', 'Before turning password logins off, make sure a key is here', 'before removing', 'warn'],
  ]),
)

scene('S17.10', (s) =>
  walk(s, [
    ['show', 'm-home'],
    ['look', [0.11, 0.3, 0.89, 0.25], 'turn it on', 1.1],
    ['show', 'home-on', 'add packages'],
    ['look', [0.11, 0.38, 0.89, 0.3], undefined, 1.1],
    ['show', 'home-2', 'dotfiles'],
    ['look', VIEW, undefined, 1.05],
    ['show', 'home-3', 'never put a token'],
    ['look', VIEW, undefined, 1.05],
    ['cap', '@secret:name@ → filled in by sops-nix, readable only by you', 'template'],
    ['nocap', 'own'],
    ['show', 'home-4'],
    ['look', VIEW, undefined, 1.05],
  ]),
)

const APPS = [
  ['A file the app never writes', 'kitty, Waybar', 'Files in ~, linked read-only'],
  ['A file the app also writes', 'Supersonic config.toml', 'App settings file: only the keys set here'],
  ['A password in the keyring', "Supersonic's server password", 'Keyring entry'],
  ['Opaque or rotating storage', 'Discord login, OAuth tokens', 'Kept in the app only'],
]

scene('S17.11', async (s) => {
  await walk(s, [['show', 'home-4'], ['look', [0.11, 0.3, 0.89, 0.6], undefined, 1.05]])
  await s.until('handles', 2)
  s.clear()
  header(s)
  const w = 2 + 30 + 32 + 44 + 2
  const t = s.box(mid(w), 8, w, 13, { title: 'Home › Apps', tag: 'four kinds of app data', kind: 'square' })
  const line = (r: string[]) => r[0].padEnd(30) + r[1].padEnd(32) + r[2]
  s.text(t.inner.x, t.inner.y, line(['Kind', 'Example', 'Managed as']), 'c-acc', t)
  s.type(t.inner.x, t.inner.y + 2, line(APPS[0]), '', t)
  for (const [cue, i] of [['also writes', 1], ['keyring', 2], ['keep to themselves', 3]] as const) {
    await s.until(cue)
    s.type(t.inner.x, t.inner.y + 2 + i * 2, line(APPS[i]), '', t)
    if (i === 1) {
      await s.until('wait while')
      s.text(t.inner.x, t.inner.y + 10, 'files the app writes: JSON · TOML · YAML · INI · properties · it can wait while the app runs', 'c-m', t)
    }
  }
})

scene('S17.12', (s) =>
  walk(s, [
    ['show', 'm-network'],
    ['look', [0.115, 0.3, 0.38, 0.22], 'every open port', 1.2],
    ['look', [0.5, 0.3, 0.45, 0.22], 'opening a port', 1.2],
    ['look', [0.11, 0.55, 0.89, 0.2], 'file systems', 1.1],
  ]),
)

void [COLS, PIC]
