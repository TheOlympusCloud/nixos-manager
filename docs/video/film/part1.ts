// Part 1: how Nix and NixOS work (chapters 0 to 10).
import { scene, A, type El, type Stage } from './film'
import { countAt } from '../../../web/src/lib/motion'
import { COLS, PIC, caption, card, code, harrow, header, larrow, len, list, machine, mid, point, storePath, uarrow, uncaption, varrow } from './kit'

scene('S0.1', async (s) => {
  header(s)
  const xs = [12, 59, 106]
  const names = ['homeserver', 'pi', 'workstation']
  const say = ['home server', 'raspberry', 'desktop']
  const hosts = []
  for (let i = 0; i < 3; i++) {
    await s.until(say[i])
    hosts.push(machine(s, xs[i], 5, names[i], { lines: ['Debian 12', 'set up by hand'], w: 28 }))
  }
  const piles = [
    ['+ apt install nginx', '✎ /etc/ssh/sshd_config', '+ cron: fix-disk.sh'],
    ['+ pip install --user', '✎ /etc/hosts', '= libssl 1.1, pinned'],
    ['+ curl … | sh', '✎ ~/.bashrc', '+ driver 535 by hand'],
  ]
  await s.until('collects')
  const items = []
  for (let k = 0; k < 3; k++) {
    for (let i = 0; i < 3; i++) items.push(s.type(xs[i] + 2, 11 + k, piles[i][k], 'c-warn'))
    await s.wait(5)
  }
  await s.until('breaks')
  hosts[0].node.dataset.tone = 'err'
  hosts[0].rows[1]!.node.textContent = '✗ broken'
  hosts[0].rows[1]!.node.className = 'c-err'
  await s.until('rebuild')
  const q = s.big(xs[0] + 12, 15, '?', 'err')
  await s.until('different')
  for (const e of [...items, q]) e.remove()
  hosts.forEach((h) => {
    h.node.dataset.tone = 'accent'
    h.rows[0]!.node.textContent = '● NixOS 26.05'
    h.rows[1]!.node.textContent = ' '
    h.rows[1]!.node.className = ''
  })
  await s.until('described')
  const files = ['flake.nix', 'hosts/homeserver/default.nix', 'hosts/pi/default.nix', 'hosts/workstation/default.nix', 'modules/common.nix']
  const repo = s.box(mid(46), 20, 46, files.length + 2, { title: 'files', kind: 'square' })
  list(s, repo.inner.x, repo.inner.y, files, '', '·', repo)
  await s.until('repository')
  repo.remove()
  const git = s.box(mid(46), 20, 46, files.length + 2, { title: 'fleet.git', tag: 'main', tone: 'accent' })
  list(s, git.inner.x, git.inner.y, files, '', '·', git)
  await s.until('built')
  const cx = xs.map((x) => x + 14)
  s.buf({ x: 0, y: 9, w: COLS, h: 11 }, { l: 'c-acc' }, (L, t) => {
    const p = Math.min(1, t / 8)
    L.l.path([[cx[0], 10], [cx[2], 10]])
    for (const x of cx) {
      L.l.path([[x, 10], [x, 10 - Math.round(p * 9)]])
      if (p >= 1) L.l.set(x, 0, '▲')
    }
  })
  await s.until('same way')
  hosts.forEach((h) => {
    h.node.dataset.tone = 'ok'
    h.rows[1]!.node.textContent = '✓ built from fleet.git'
    h.rows[1]!.node.className = 'c-ok'
  })
})

scene('S0.2', async (s) => {
  header(s)
  const p1 = ['The Nix store', 'Derivations', 'Flakes', 'The module system', 'NixOS: build, switch, roll back', 'home-manager', 'Secrets with sops-nix', 'Deploying with comin']
  const p2 = ['Fleet', 'Machines', 'Packages', 'Services', 'Modules', 'Options', 'Health', 'Deployments', 'Dependencies', 'Inputs', 'Changes', 'History', 'Code', 'Settings']
  await s.until('two parts')
  const a = s.box(10, 6, 58, 20, { title: 'Part 1 · How Nix works' })
  const b = s.box(77, 6, 58, 20, { title: 'Part 2 · nixos-manager', tone: 'accent' })
  const cues = ['store', 'derivations', 'flakes', 'module system', 'running machine']
  for (let i = 0; i < 5; i++) {
    await s.until(cues[i])
    list(s, a.inner.x, a.inner.y + 1 + i * 2, [p1[i]], 'c-b', '›', a)
  }
  for (let i = 5; i < 8; i++) {
    await s.wait(3)
    list(s, a.inner.x, a.inner.y + 1 + i * 2, [p1[i]], 'c-b', '›', a)
  }
  await s.until('second')
  for (let i = 0; i < 14; i++) {
    s.type(b.inner.x + (i < 7 ? 0 : 26), b.inner.y + 1 + (i % 7) * 2, '◇ ' + p2[i], i < 7 ? '' : 'c-m', b)
    await s.wait(1)
  }
  await s.until('every view')
  s.clear()
  header(s)
  const pic = s.pic(PIC.x, PIC.y, PIC.w, PIC.h, 'fleet', { title: 'nixos-manager · Fleet' })
  await s.wait(20)
  pic.look([0.15, 0.08, 0.6, 0.6], 1.05, 10)
})

/** A folder of a classic system, its files listed. */
function folder(s: Parameters<typeof card>[0], x: number, y: number, path: string, files: string[], w = 26) {
  return card(s, x, y, files, { title: path, w, h: 9, kind: 'square' })
}

scene('S1.1', async (s) => {
  header(s)
  const bin = folder(s, 26, 14, '/usr/bin', ['bash', 'ssh', 'python3'])
  const lib = folder(s, 56, 14, '/usr/lib', ['libc.so', 'libz.so'])
  const etc = folder(s, 86, 14, '/etc', ['hosts', 'fstab', 'ssh/sshd_config'], 34)
  caption(s, 'One shared place, overwritten in place')
  const flash = async (b: typeof bin) => {
    b.node.dataset.tone = 'accent'
    await s.wait(6)
    delete b.node.dataset.tone
  }
  await s.until('libraries')
  await flash(lib)
  await s.until('programs')
  await flash(bin)
  await s.until('settings')
  await flash(etc)
  await s.until('two programs')
  const a = card(s, 52, 7, ['package A'], { tone: 'ok' })
  const b = card(s, 70, 7, ['package B'], { tone: 'ok' })
  varrow(s, 58, 10, 4, 'c-ok')
  const ssl = s.type(lib.inner.x, lib.inner.y + 2, 'libssl.so.3.0', 'c-ok', lib)
  await s.until('second')
  varrow(s, 76, 10, 4, 'c-warn')
  ssl.remove()
  s.real(lib.inner.x, lib.inner.y + 2, 'libssl.so.3.2', 'c-warn', lib)
  await s.until('breaks')
  a.node.dataset.tone = 'err'
  a.rows[0]!.node.className = 'c-err'
  s.text(a.rect.x - 10, a.rect.y + 1, '✗ broken', 'c-err')
  await s.until('edit')
  etc.rows[2]!.node.className = 'c-warn'
  s.text(etc.inner.x + 16, etc.inner.y + 2, '✎', 'c-warn', etc)
  await s.until('nothing')
  list(s, etc.inner.x, etc.inner.y + 4, ['who?', 'why?', 'what was it before?'], 'c-m', '?', etc)
  await s.until('two machines')
  s.clear()
  header(s)
  const pkgs = ['nginx 1.24', 'openssl 3.0', 'python 3.11', 'sshd: port 22']
  const left = machine(s, 16, 12, 'homeserver', { lines: [...pkgs, ''], w: 36 })
  const right = machine(s, 93, 12, 'workstation', { lines: [...pkgs, ''], w: 36 })
  await s.until('drift')
  const drift: [typeof left, number, string][] = [[right, 1, 'openssl 3.2'], [left, 3, 'sshd: port 2222'], [right, 0, 'nginx 1.26'], [left, 2, 'python 3.12']]
  for (const [m, i, t] of drift) {
    m.rows[i]!.cancel = A.run(5, (p) => (m.rows[i]!.node.textContent = A.scramble(t, p)))
    m.rows[i]!.node.className = 'c-warn'
    await s.wait(4)
  }
  right.rows[4]!.node.textContent = '+ hand-fixed cron job'
  right.rows[4]!.node.className = 'c-err'
  s.big(mid(len(A.big('DRIFT'))), 14, 'DRIFT', 'err')
  await s.until('works')
  caption(s, '“it works on my machine”', 'warn')
})

scene('S1.2', async (s) => {
  header(s)
  const lib = folder(s, 14, 12, '/usr/lib', ['libc.so', 'libz.so', 'libssl.so.3.2'])
  await s.until('never', 2)
  for (const r of lib.rows) r!.cancel = A.run(4, (p) => (r!.node.textContent = A.scramble(r!.node.textContent!, 1 - p)))
  await s.wait(4)
  lib.remove()
  const fill = s.text(14, 12, Array(9).fill('░'.repeat(26)).join('\n'), 'c-line')
  await s.wait(3)
  fill.remove()
  await s.until('every package')
  const store = s.box(50, 5, 64, 12, { title: '/nix/store', tag: 'read-only', tone: 'accent' })
  const paths: [string, string][] = [['openssl-3.0.15', '7x2kqw0m3c8hn1yr'], ['openssl-3.2.4', 'b9mzc2v7xq4lsd8a'], ['nginx-1.26.2', 'p1lwwz0g8d2f6qk1']]
  const els = []
  for (let i = 0; i < 3; i++) {
    els.push(storePath(s, store.inner.x, store.inner.y + 1 + i * 2, paths[i][0], paths[i][1], store))
    await s.wait(4)
  }
  await s.until('fingerprint')
  s.text(store.inner.x + 11, store.inner.y + 6, '└─ the hash: how it was built', 'c-warn', store)
  await s.until('each program')
  const a = card(s, 14, 6, ['package A'], { tone: 'ok' })
  const b = card(s, 14, 10, ['package B'], { tone: 'ok' })
  harrow(s, 29, 7, 21, 'c-ok')
  harrow(s, 29, 11, 21, 'c-ok')
  void a, b
  await s.until('because')
  caption(s, 'Every version in its own place. Nothing is overwritten.')
  await s.until('installing')
  card(s, 14, 14, ['package C'], { tone: 'accent' })
  storePath(s, store.inner.x, store.inner.y + 8, 'openssl-3.4.1', 'q8hn2a6dk5w1zb0c', store)
  harrow(s, 29, 15, 21, 'c-acc')
  await s.wait(6)
  s.text(10, 7, '✓', 'c-ok')
  s.text(10, 11, '✓', 'c-ok')
  s.text(10, 15, '✓', 'c-ok')
  await s.until('rebuild')
  uncaption(s)
  const there = s.box(50, 20, 64, 9, { title: 'another machine', tone: 'ok' })
  for (let i = 0; i < 3; i++) s.real(there.inner.x, there.inner.y + 1 + i * 2, `/nix/store/${paths[i][1]}-${paths[i][0]}`, 'c-b', there)
  await s.until('same result')
  s.text(116, 24, '= the same', 'c-ok')
})


function fresh(s: Stage) {
  s.clear()
  header(s)
}

/** Parts of a long text marked from below: └──┘ under each span, a label under that. */
function brace(s: Stage, x: number, y: number, n: number, label: string, cls = 'c-m') {
  s.text(x, y, '└' + '─'.repeat(n - 2) + '┘', 'c-line')
  return s.text(x + Math.max(0, Math.floor((n - len(label)) / 2)), y + 1, label, cls)
}

scene('S2.1', async (s) => {
  header(s)
  const store = s.box(mid(70), 4, 70, 11, { title: '/nix/store', tone: 'accent' })
  const ps: [string, string][] = [['openssl-3.0.15', '7x2kqw0m3c8hn1yr'], ['openssl-3.2.4', 'b9mzc2v7xq4lsd8a'], ['nginx-1.26.2', 'p1lwwz0g8d2f6qk1'], ['bash-5.2p37', '0dh4a9rx2kq7c1mz']]
  await s.until('store')
  for (let i = 0; i < 4; i++) {
    storePath(s, store.inner.x + 2, store.inner.y + 1 + i * 2, ps[i][0], ps[i][1], store)
    await s.wait(3)
  }
  await s.until('store path')
  s.text(store.inner.x, store.inner.y + 5, '▶', 'c-warn', store)
  await s.until('three parts', 2)
  fresh(s)
  const hash = 'p1lwwz0g8d2f6qk1y9mxcbn3hrj4sv5a'
  const x = mid(56)
  const path = storePath(s, x, 18, 'nginx-1.26.2', hash)
  caption(s, '/nix/store/<hash>-<name>-<version>')
  const marks: El[] = []
  await s.until('store itself')
  marks.push(brace(s, x, 19, 11, 'the store'))
  await s.until('hash')
  marks.push(brace(s, x + 11, 19, 32, 'the hash: 32 characters', 'c-warn'))
  await s.until('readable')
  marks.push(brace(s, x + 43, 19, 13, 'name, version'))
  await s.until('important', 2)
  s.clear(path, ...s.els.values().filter((e) => e.name.startsWith('λ') || e.name.startsWith('─')))
  const ins = ['source code', 'compiler', 'build script', 'libraries']
  const cards: ReturnType<typeof card>[] = []
  for (let i = 0; i < 4; i++) {
    await s.until(ins[i])
    cards.push(card(s, 22 + i * 26, 4, [ins[i]], { w: 22, kind: 'square' }))
  }
  s.buf({ x: 22, y: 7, w: 104, h: 3 }, { l: 'c-line' }, (L) => {
    for (const cx of [11, 37, 63, 89]) L.l.path([[cx, 0], [cx, 1]])
    L.l.path([[11, 1], [89, 1]])
    L.l.path([[50, 1], [50, 2]])
  })
  const fn = card(s, mid(24), 10, ['   hash function'], { w: 24, tone: 'accent' })
  varrow(s, 72, 13, 5, 'c-acc')
  void fn
  await s.until('change any')
  cards[1].node.dataset.tone = 'warn'
  cards[1].rows[0].node.textContent = 'compiler -O3'
  cards[1].rows[0].node.className = 'c-warn'
  await s.until('different hash')
  path.cancel = A.run(6, (p) => (path.node.innerHTML = `<span class="c-m">/nix/store/</span><span class="c-warn">${A.scramble('9fkq2w7xv0m3c8hn1yrd5sb4lza6tgpe', p)}</span><b>-nginx-1.26.2</b>`))
  await s.until('nothing')
  s.text(x + 58, 18, '◼ read-only', 'c-warn')
})

scene('S2.2', async (s) => {
  header(s)
  const nginx = card(s, 34, 5, ['nginx-1.26'], { tone: 'accent', kind: 'square' })
  const libs = ['openssl-3.0', 'zlib-1.3', 'pcre2-10.44']
  await s.until('points')
  const cards = libs.map((l, i) => card(s, 12 + i * 22, 13, [l], { w: 18, kind: 'square' }))
  const lines = s.buf({ x: 12, y: 8, w: 62, h: 5 }, { l: 'c-acc' }, (L) => {
    L.l.path([[29, 0], [29, 2]])
    L.l.path([[8, 2], [52, 2]])
    for (const cx of [8, 30, 52]) L.l.path([[cx, 2], [cx, 4]])
  })
  await s.until('c library')
  const glibc = card(s, 34, 21, ['glibc-2.40'], { kind: 'square' })
  await s.until('follow')
  const down = s.buf({ x: 12, y: 16, w: 62, h: 5 }, { l: 'c-line' }, (L) => {
    for (const cx of [8, 30, 52]) L.l.path([[cx, 0], [cx, 2]])
    L.l.path([[8, 2], [52, 2]])
    L.l.path([[29, 2], [29, 4]])
  })
  await s.until('closure')
  const cl = s.box(8, 3, 70, 23, { kind: 'dash', tone: 'warn', title: 'the closure of nginx' })
  for (const e of [nginx, ...cards, glibc, lines, down]) e.parent = cl
  caption(s, 'Closure: a path plus everything it references')
  await s.until('copy')
  const pi = machine(s, 100, 6, 'pi', { lines: ['● NixOS 26.05', ''], w: 30 })
  await s.wait(4)
  pi.rows[1].node.textContent = '✓ the closure runs here'
  pi.rows[1].node.className = 'c-ok'
  await s.until('binary caches')
  card(s, 100, 22, ['binary cache'], { tone: 'accent', w: 30 })
  uarrow(s, 115, 11, 11, 'c-cy')
  await s.until('download')
  s.text(117, 16, 'same hash:', 'c-cy')
  s.text(117, 17, 'download, no build', 'c-cy')
})

const HELLO = `stdenv.mkDerivation {
  pname = "hello";
  version = "2.12.1";
  src = fetchurl {
    url = "mirror://gnu/hello/hello-2.12.1.tar.gz";
    hash = "sha256-jZkUKv2SV28wsM18tCqNxoCZmLxdYH2Idh9RLibH2yA=";
  };
}`

scene('S3.1', async (s) => {
  header(s)
  await s.until('derivation')
  const c = code(s, mid(70), 6, HELLO, 'hello.nix')
  await s.until('source')
  for (const i of [3, 4, 5, 6]) point(s, c, i)
  await s.until('two steps', 2)
  fresh(s)
  await s.until('evaluates')
  card(s, 6, 8, ['Nix expression'], { w: 18 })
  harrow(s, 25, 9, 5, 'c-line')
  s.text(23, 7, 'evaluate', 'c-m')
  await s.until('produces')
  card(s, 31, 8, ['…/a8x0f…-hello-2.12.1.drv'], { w: 32, kind: 'square' })
  await s.until('nothing')
  s.text(33, 12, 'nothing built yet', 'c-m')
  await s.until('realises')
  harrow(s, 64, 9, 5, 'c-line')
  const sb = s.box(69, 7, 22, 5, { kind: 'dash', tone: 'warn' })
  card(s, 70, 8, ['build (realise)'], { w: 20, tone: 'accent' }).parent = sb
  await s.until('sandbox')
  s.text(60, 13, 'sandbox: no network, only the declared inputs', 'c-warn')
  await s.until('output')
  harrow(s, 91, 9, 5, 'c-line')
  card(s, 97, 8, ['…/4lk9c…-hello-2.12.1'], { w: 29, tone: 'ok', kind: 'square' })
  await s.until('binary cache')
  card(s, 38, 21, ['binary cache'], { w: 22, tone: 'accent' })
  uarrow(s, 49, 15, 6, 'c-cy')
  s.text(51, 17, 'already built?', 'c-cy')
  await s.until('download')
  harrow(s, 61, 22, 49, 'c-cy')
  uarrow(s, 110, 11, 11, 'c-cy')
  s.text(72, 23, 'download instead', 'c-cy')
})

scene('S3.2', async (s) => {
  header(s)
  await s.until('nix packages is')
  card(s, 8, 10, ['nixpkgs'], { tone: 'accent', w: 16 })
  const leaves = ['firefox', 'jellyfin', 'python3', 'linuxPackages', 'nixos/modules']
  s.buf({ x: 24, y: 5, w: 14, h: 13 }, { l: 'c-line' }, (L) => {
    L.l.path([[0, 6], [6, 6]])
    L.l.path([[6, 0], [6, 12]])
    for (let i = 0; i < 5; i++) L.l.path([[6, i * 3], [13, i * 3]])
  })
  const lv = leaves.map((l, i) => s.type(40, 5 + i * 3, l, i === 4 ? 'c-m' : ''))
  await s.until('hundred thousand')
  const n = s.text(66, 9, A.big('120 000'), 'big warn')
  n.cancel = A.run(10, (p) => (n.node.textContent = A.big(String(countAt(0, 120000, p)).replace(/\B(?=(\d{3})+(?!\d))/g, ' '))))
  s.text(66, 13, 'packages and more', 'c-m')
  await s.until('modules')
  lv[4].node.className = 'c-acc'
  await s.until('comes in branches', 2)
  fresh(s)
  await s.until('stable')
  s.text(8, 8, 'nixos-unstable', 'c-acc')
  const un = s.text(24, 8, '●' + '──●'.repeat(36), 'c-acc')
  s.text(8, 16, 'nixos-26.05', 'c-ok')
  s.text(24, 16, '●' + '────────●'.repeat(5), 'c-ok')
  void un
  await s.until('security')
  s.text(24, 18, 'security fixes for about 7 months', 'c-ok')
  await s.until('unstable')
  s.text(60, 6, 'moves every day', 'c-acc')
  await s.until('follows')
  machine(s, 90, 24, 'homeserver', { w: 30 })
  uarrow(s, 98, 17, 7, 'c-ok')
  await s.until('single package')
  s.text(112, 9, '│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n▼', 'c-acc')
  s.text(114, 14, 'one package', 'c-acc')
  await s.until('a word', 2)
  fresh(s)
  const shapes: [string, string, string][] = [['attribute sets', '{ name = "hello"; version = "2.12"; }', 'an attribute set'],
    ['functions', 'x: x + 1', 'a function'], ['let bindings', 'let\n  port = 8096;\nin\n  { allowedTCPPorts = [ port ]; }', 'a let binding']]
  let y = 9
  for (const [cue, src, title] of shapes) {
    await s.until(cue)
    const c = code(s, mid(48), y, src, title)
    y += c.rect.h + 1
  }
})

const FLAKE = `{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    home-manager.url = "github:nix-community/home-manager/release-26.05";
    comin.url = "github:nlewo/comin";
    sops-nix.url = "github:Mic92/sops-nix";
    disko.url = "github:nix-community/disko";
  };
  outputs = { nixpkgs, ... }@inputs: {
    nixosConfigurations.homeserver = nixpkgs.lib.nixosSystem { … };
  };
}`

scene('S4.1', async (s) => {
  header(s)
  await s.until('folder')
  const c = code(s, mid(78), 4, FLAKE, 'flake.nix')
  await s.until('inputs')
  for (let i = 1; i <= 7; i++) point(s, c, i, 'c-acc')
  await s.until('outputs')
  for (let i = 8; i <= 10; i++) point(s, c, i, 'c-ok')
  await s.until('fleet', 2)
  fresh(s)
  const ins = ['nixpkgs', 'home-manager', 'comin', 'sops-nix', 'disko']
  ins.forEach((n, i) => card(s, 6, 5 + i * 5, [n], { w: 18, tone: 'accent', kind: 'square' }))
  s.buf({ x: 24, y: 6, w: 14, h: 21 }, { l: 'c-acc' }, (L) => {
    for (let i = 0; i < 5; i++) L.l.path([[0, i * 5], [6, i * 5]])
    L.l.path([[6, 0], [6, 20]])
    L.l.path([[6, 10], [12, 10]])
    L.l.set(13, 10, '▶')
  })
  card(s, 38, 15, ['outputs = { … }: { … }'], { w: 28 })
  await s.until('configurations')
  const outs = ['homeserver', 'pi', 'workstation'].map((n, i) => card(s, 82, 8 + i * 7, [`nixosConfigurations.${n}`], { w: 40, tone: 'ok', kind: 'square' }))
  s.buf({ x: 66, y: 9, w: 16, h: 15 }, { l: 'c-ok' }, (L) => {
    L.l.path([[0, 7], [6, 7]])
    L.l.path([[6, 0], [6, 14]])
    for (const y of [0, 7, 14]) L.l.path([[6, y], [14, y]])
    for (const y of [0, 7, 14]) L.l.set(15, y, '▶')
  })
  await s.until('named')
  outs.forEach((o) => (o.rows[0].node.innerHTML = o.rows[0].node.textContent!.replace(/\.(\w+)$/, '.<span class="c-warn">$1</span>')))
})

const LOCK = `"nixpkgs": {
  "locked": {
    "owner": "NixOS",
    "repo": "nixpkgs",
    "rev": "a3f19c2e7b1d…",
    "narHash": "sha256-Qm3v…",
    "lastModified": 1764000000
  }
}`

scene('S4.2', async (s) => {
  header(s)
  await s.until('next to')
  const c = code(s, mid(40), 5, LOCK, 'flake.lock')
  await s.until('revision')
  point(s, c, 4)
  await s.until('hash')
  point(s, c, 5)
  caption(s, 'The lock: the exact revision of every input')
  await s.until('whoever', 2)
  fresh(s)
  const ins = ['nixpkgs', 'home-manager', 'comin']
  ins.forEach((n, i) => {
    card(s, 10, 7 + i * 6, [n], { w: 18, tone: 'accent', kind: 'square' })
    harrow(s, 29, 8 + i * 6, 40, 'c-acc')
  })
  card(s, 70, 13, ['flake.nix'], { w: 16 })
  s.buf({ x: 69, y: 8, w: 1, h: 13 }, { l: 'c-acc' }, (L) => L.l.path([[0, 0], [0, 12]]))
  for (let i = 0; i < 3; i++) s.text(45, 7 + i * 6, '◼ locked', 'c-warn')
  await s.until('today')
  s.type(30, 26, 'today, in a year, on any machine: the same nixpkgs', 'c-ok')
  await s.until('update')
  s.real(31, 9, 'rev a3f19c2 → 5d0e8b1', 'c-warn')
  await s.until('pure mode', 2)
  fresh(s)
  const pure = s.box(20, 6, 50, 20, { kind: 'heavy', tone: 'accent', title: 'pure evaluation' })
  card(s, 36, 14, ['flake.nix'], { w: 16 }).parent = pure
  const outside: [string, string, number][] = [['random files', '/var/lib/…', 8], ['environment', '$HOME, $USER', 15], ['network', 'https://…', 22]]
  for (const [cue, what, y] of outside) {
    await s.until(cue)
    s.text(96, y, what, 'c-m')
    s.text(72, y, '◀─ ✗ ─────────────────', 'c-err')
  }
  await s.until('reproducible')
  s.type(8, 29, "error: access to absolute path '/var/lib/…' is forbidden in pure evaluation mode", 'c-err')
  await s.until('keep this')
  caption(s, 'A new input needs a Lock before anything evaluates', 'warn')
})

scene('S5.1', async (s) => {
  header(s)
  await s.until('declare')
  const a = card(s, 6, 5, ['options.services.openssh.enable', '  : bool, default false'], { title: 'module A · NixOS declares', tone: 'accent' })
  await s.until('set values')
  const b = card(s, 6, 13, ['services.openssh.enable = true;'], { title: 'module B · you define', tone: 'ok' })
  await s.until('thousands')
  s.text(6, 3, 'NixOS ships thousands of these modules', 'c-m')
  await s.until('collects')
  s.buf({ x: 44, y: 6, w: 16, h: 9 }, { l: 'c-line' }, (L) => {
    L.l.path([[0, 1], [8, 1]])
    L.l.path([[0, 8], [8, 8]])
    L.l.path([[8, 1], [8, 8]])
    L.l.path([[8, 4], [15, 4]])
  })
  card(s, 62, 8, ['module system', 'merges every module'], { tone: 'warn' })
  void b
  await s.until('merges')
  varrow(s, 72, 12, 3, 'c-warn')
  card(s, 66, 15, ['config'], { w: 14, tone: 'warn' })
  await s.until('reacts')
  varrow(s, 72, 18, 3, 'c-acc')
  card(s, 52, 21, ['systemd.services.sshd'], { tone: 'accent', kind: 'square' })
  await s.until('writes')
  card(s, 82, 21, ['/etc/ssh/sshd_config'], { tone: 'accent', kind: 'square' })
  await s.until('describe')
  caption(s, 'You describe what you want; the module works out how.')
})

const LADDER: [string, string, string][] = [['50', 'lib.mkForce', 'c-pur'], ['90', 'set on one machine in nixos-manager', 'c-acc'],
  ['95', 'set for every machine in nixos-manager', 'c-acc'], ['100', 'a plain hand-written value', ''], ['1000', 'lib.mkDefault', 'c-m'], ['1500', "the option's own default", 'c-f']]

scene('S5.2', async (s) => {
  header(s)
  const rules: [string, string, string, string, string][] = [
    ['attribute sets', 'Attribute sets merge key by key', '{ PermitRootLogin = "no"; }', '{ X11Forwarding = false; }', 'both keys'],
    ['lists', 'Lists are joined', '[ 22 ]', '[ 80 443 ]', '[ 22 80 443 ]'],
    ['single value', 'Two single values conflict', '"Europe/Madrid"', '"UTC"', 'error: conflicting definitions'],
  ]
  for (let i = 0; i < 3; i++) {
    const [cue, title, l, r, out] = rules[i]
    await s.until(cue)
    const y = 4 + i * 9
    s.text(8, y, title, 'c-b')
    const ca = card(s, 8, y + 2, [l], { kind: 'square' })
    s.text(ca.rect.x + ca.rect.w + 2, y + 3, '+', 'c-m')
    const cb = card(s, ca.rect.x + ca.rect.w + 5, y + 2, [r], { kind: 'square' })
    s.text(cb.rect.x + cb.rect.w + 2, y + 3, '→', 'c-m')
    card(s, cb.rect.x + cb.rect.w + 5, y + 2, [out], { tone: i === 2 ? 'err' : 'ok' })
  }
  await s.until('to resolve', 2)
  fresh(s)
  const lad = s.box(30, 5, 70, 16, { title: 'priority', tag: 'lower number wins', kind: 'square' })
  const row = (i: number) => s.html(lad.inner.x + 2, lad.inner.y + 1 + i * 2, `<span class="${LADDER[i][2] || 'c-b'}">${LADDER[i][0].padStart(5)}</span>   ${A.esc(LADDER[i][1])}`, LADDER[i][2], lad)
  s.text(22, 6, '▲\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│', 'c-warn')
  s.text(12, 20, 'it wins', 'c-warn')
  await s.until('one hundred')
  row(3)
  await s.until('make default')
  row(4)
  row(5)
  await s.until('make force')
  row(0)
  await s.until('ninety')
  row(1)
  row(2)
  await s.until('clicking')
  s.text(lad.rect.x + 1, lad.inner.y + 3, '▌\n \n▌', 'c-acc', lad)
  await s.until('one detail', 2)
  const demo = card(s, 30, 24, ['hand-written:  [ 22 80 ]', 'clicked (90):  [ 22 443 ]', 'the result:    [ 22 443 ]'], { title: 'a list at a winning priority', tone: 'warn' })
  demo.rows[2].node.className = 'c-warn'
  await s.until('finally', 2)
  fresh(s)
  const mk = code(s, mid(72), 6, '{ config, lib, ... }: {\n  options.fleet.modules.media.enable = lib.mkEnableOption "media";\n  config = lib.mkIf config.fleet.modules.media.enable {\n    services.jellyfin.enable = true;\n  };\n}', 'a module of your own')
  await s.until('enable option')
  point(s, mk, 1, 'c-acc')
  await s.until('only apply')
  for (const i of [2, 3, 4]) point(s, mk, i, 'c-ok')
})

scene('S6.1', async (s) => {
  header(s)
  card(s, 4, 5, ['nixosConfigurations.homeserver'], { tone: 'accent', kind: 'square' })
  await s.until('evaluated')
  harrow(s, 40, 6, 6)
  card(s, 47, 5, ['evaluate all modules'], {})
  await s.until('top level')
  harrow(s, 73, 6, 6)
  card(s, 80, 5, ['system.build.toplevel'], { tone: 'warn', kind: 'square' })
  await s.until('store path')
  varrow(s, 92, 8, 3, 'c-warn')
  storePath(s, 60, 12, 'nixos-system-homeserver-26.05', '8f3kxq2m7c1v4z9a')
  const items: [string, string][] = [['kernel', 'kernel'], ['ramdisk', 'initrd'], ['slash', 'etc/'], ['program', 'sw/bin/'], ['activates', 'activate, bin/switch-to-configuration']]
  for (let i = 0; i < items.length; i++) {
    await s.until(items[i][0])
    s.type(64, 14 + i * 2, (i === items.length - 1 ? '└─ ' : '├─ ') + items[i][1])
  }
  await s.until('if two', 2)
  fresh(s)
  card(s, 10, 8, ['configuration A'], { tone: 'accent' })
  card(s, 10, 16, ['configuration B'], { tone: 'accent' })
  await s.until('same top level')
  s.buf({ x: 30, y: 9, w: 20, h: 9 }, { l: 'c-acc' }, (L) => {
    L.l.path([[0, 0], [8, 0], [8, 8], [0, 8]])
    L.l.path([[8, 4], [18, 4]])
    L.l.set(19, 4, '▶')
  })
  const p = storePath(s, 51, 13, 'nixos-system-pi', 'k2v9dq7m1x0c4z8a')
  await s.until('same system')
  const note = s.type(51, 16, 'the same toplevel: the same system', 'c-ok')
  await s.until('top level changes')
  p.cancel = A.run(6, (q) => (p.node.innerHTML = `<span class="c-m">/nix/store/</span><span class="c-warn">${A.scramble('x7c1mq2k9v0d4z8a', q)}</span><b>-nixos-system-pi</b>`))
  note.node.textContent = 'the toplevel changed: the machine changes'
  note.node.className = 'c-warn'
})

scene('S6.2', async (s) => {
  header(s)
  await s.until('switch to configuration')
  s.type(mid(26), 4, 'switch-to-configuration', 'c-acc')
  const doors: [string, string[], 'ok' | 'warn' | 'accent'][] = [['switch', ['activate now', 'and make it the boot default'], 'ok'], ['test', ['activate now', 'boot entry untouched'], 'warn'], ['boot', ['nothing now', 'the next reboot'], 'accent']]
  const cues = ['switch activates', 'test activates', 'boot does']
  for (let i = 0; i < 3; i++) {
    await s.until(i ? cues[i] : 'three modes')
    card(s, 14 + i * 40, 7, doors[i][1], { title: doors[i][0], tone: doors[i][2], w: 36 })
    if (!i) await s.until(cues[0])
  }
  await s.until('activating', 2)
  const units: [string, string, string][] = [['sshd.service', 'restart', 'c-warn'], ['nginx.service', 'reload', 'c-cy'], ['jellyfin.service', 'start', 'c-ok'], ['old.service', 'stop', 'c-err']]
  const box = s.box(14, 13, 56, 12, { title: 'the units', kind: 'square' })
  for (let i = 0; i < 4; i++) {
    await s.until(['restarted', 'reloaded', 'started', 'stopped'][i])
    s.html(box.inner.x, box.inner.y + 1 + i * 2, `${units[i][0].padEnd(22)}<span class="${units[i][2]}">${units[i][1]}</span>`, '', box)
  }
  await s.until('kernel')
  card(s, 84, 15, ['new kernel', 'reboot needed'], { tone: 'warn', w: 30 })
  await s.until('shows')
  caption(s, 'nixos-manager shows all of this before you deploy')
})

scene('S7.1', async (s) => {
  header(s)
  await s.until('generation')
  const gens = [41, 42, 43].map((n, i) => card(s, 10, 5 + i * 6, [`generation ${n}`, `…-nixos-system-26.05.${i + 1}`], { kind: 'square' }))
  gens[2].node.dataset.tone = 'ok'
  s.text(50, 18, '◀── /nix/var/nix/profiles/system', 'c-acc')
  await s.until('boot menu')
  card(s, 90, 5, ['NixOS - Configuration 43', 'NixOS - Configuration 42', 'NixOS - Configuration 41'], { title: 'boot menu' })
  await s.until('going back')
  caption(s, 'nixos-rebuild switch --rollback')
  await s.until('rollback')
  delete gens[2].node.dataset.tone
  gens[1].node.dataset.tone = 'warn'
  s.clear(...[...s.els].filter((e) => !e.name.startsWith('◀──')))
  s.text(50, 12, '◀── /nix/var/nix/profiles/system', 'c-warn')
  await s.until('nothing')
  s.type(50, 14, 'instant: nothing is rebuilt', 'c-ok')
})

scene('S7.2', async (s) => {
  header(s)
  const names = ['nixos-system-43', 'nixos-system-42', 'firefox-145', 'nixos-system-38', 'firefox-139', 'gcc-13']
  await s.until('grows')
  const ps = names.map((n, i) => storePath(s, 60, 5 + i * 2, n, ['a1', 'b2', 'c3', 'd4', 'e5', 'f6'][i] + 'kq7m2x9c0z4v'))
  await s.until('root')
  const roots = ['generation 43', 'generation 42', 'a result link']
  roots.forEach((r, i) => card(s, 8, 4 + i * 4, [r], { w: 20, tone: 'ok', kind: 'square' }))
  s.text(29, 5, '─────────────────────────────▶', 'c-ok')
  s.text(29, 9, '─────────────────────────────▶', 'c-ok')
  s.text(29, 13, '────────────────────╮', 'c-ok')
  s.text(49, 14, '╰──────────▶', 'c-ok')
  s.text(8, 17, 'roots', 'c-ok')
  void ps
  await s.until('deleted')
  s.text(60, 18, 'nix-collect-garbage', 'c-warn')
  for (const i of [3, 4, 5]) ps[i].node.className = 'c-f'
  for (const i of [3, 4, 5]) ps[i].node.innerHTML = ps[i].node.textContent!.replace(/./g, '░')
  await s.until('schedule')
  s.html(60, 20, `keep  ${A.bar(14 / 30, 30)}  14 days`, 'c-acc')
  await s.until('health')
  s.type(60, 22, 'Health: “No automatic garbage collection”', 'c-warn')
  await s.until('warning', 2)
  fresh(s)
  const sv = code(s, mid(36), 10, 'system.stateVersion = "26.05";', 'once, at install')
  await s.until('not the version')
  s.type(mid(48), 15, 'not the NixOS release the machine runs', 'c-err')
  await s.until('records')
  s.type(mid(78), 17, 'the release it was first installed with: keeps the data of its services readable', 'c-m')
  await s.until('never')
  s.text(sv.rect.x + sv.rect.w + 2, 11, '◼ never change it', 'c-warn')
})

scene('S8.1', async (s) => {
  header(s)
  await s.until('describes')
  const sys = s.box(20, 22, 104, 5, { title: 'the system: NixOS', tone: 'accent' })
  s.text(sys.inner.x, sys.inner.y + 1, 'packages · services · users · /etc', 'c-m', sys)
  await s.until('same')
  const user = s.box(20, 5, 104, 15, { title: "pablo's home: home-manager", tone: 'ok' })
  const chips: [string, string][] = [['packages', 'home.packages'], ['shell', 'programs.zsh · programs.git'], ['dotfiles', '~/.config/kitty/kitty.conf']]
  for (let i = 0; i < 3; i++) {
    await s.until(chips[i][0])
    s.type(user.inner.x + 2, user.inner.y + 1 + i * 2, '› ' + chips[i][1], '', user)
  }
  await s.until('link')
  s.type(user.inner.x + 2, user.inner.y + 8, '~/.config/kitty/kitty.conf ──▶ /nix/store/c71bq…-hm_kitty.conf  (read-only, versioned)', 'c-warn', user)
  await s.until('two ways', 2)
  fresh(s)
  await s.until('module')
  card(s, 10, 8, ['home-manager.users.pablo =', '  import ../../home/pablo.nix;', '', 'built and switched with the system'], { title: 'as a NixOS module', tone: 'accent' })
  await s.until('standalone')
  card(s, 76, 8, ['home-manager switch --flake .#pablo@laptop', '', 'macOS, another Linux…'], { title: 'standalone', tone: 'ok' })
  await s.until('both')
  caption(s, 'nixos-manager handles both')
})

scene('S9.1', async (s) => {
  header(s)
  await s.until('everything')
  card(s, 8, 8, ['password = "hunter2";'], { kind: 'square' })
  harrow(s, 34, 9, 10)
  storePath(s, 45, 9, 'etc-wifi.conf', 'q1mz8kx2c7v0d4a9')
  await s.until('every user')
  s.type(45, 11, '◉ every user of the machine can read it', 'c-err')
  await s.until('password')
  s.text(8, 13, '✗ never a secret in Nix', 'c-err')
  await s.until('solves', 2)
  fresh(s)
  await s.until('encrypted')
  card(s, 6, 6, ['secrets/hosts/homeserver.json', '"wifi": "ENC[AES256_GCM,data:…]"', '', '◼ the server’s key', '◼ homeserver’s SSH host key'], { title: 'in the repository', tone: 'accent' })
  await s.until('decrypt')
  harrow(s, 48, 9, 12, 'c-ok')
  machine(s, 62, 7, 'homeserver', { w: 30 })
  await s.until('activation')
  card(s, 62, 14, ['/run/secrets/wifi', '-r-------- wpa_supplicant'], { tone: 'ok', w: 30 })
  await s.until('points')
  code(s, 6, 22, 'networking.wireless.secretsFile = config.sops.secrets.wifi.path;', 'your configuration: the path, never the value')
})

scene('S10.1', async (s) => {
  header(s)
  await s.until('many machines')
  const hs = ['homeserver', 'pi', 'workstation'].map((n, i) => machine(s, 14 + i * 42, 24, n, { w: 32 }))
  await s.until('repository')
  const fj = machine(s, mid(32), 5, 'forgejo', { lines: ['fleet.git · main'], arch: 'git', w: 32 })
  await s.until('each machine')
  s.buf({ x: 14, y: 9, w: 116, h: 15 }, { l: 'c-cy' }, (L, t) => {
    const p = Math.min(1, t / 6)
    for (const cx of [16, 58, 100]) L.l.path([[cx, 14], [cx, Math.max(14 - Math.round(p * 7), 7)]])
    L.l.path([[16, 7], [100, 7]])
    L.l.path([[58, 7], [58, 0]])
    if (p >= 1) L.l.set(58, 0, '▲')
    L.l.text(60, 3, 'comin, every few minutes', )
  })
  await s.until('new commit')
  s.real(fj.rect.x + fj.rect.w + 2, 6, '● a3f19c2 new on main', 'c-warn')
  await s.until('builds')
  hs.forEach((h, i) => (h.rows[0].node.textContent = `building .#${['homeserver', 'pi', 'workstation'][i]}`))
  await s.until('switches')
  hs.forEach((h) => ((h.node.dataset.tone = 'ok'), (h.rows[0].node.textContent = '✓ switched to a3f19c2')))
  await s.until('push')
  caption(s, 'A push to main is a deploy')
})

scene('S10.2', async (s) => {
  header(s)
  s.text(8, 14, 'main', 'c-acc')
  s.text(14, 14, '●' + '──────────●'.repeat(4), 'c-acc')
  await s.until('second branch')
  s.text(58, 10, '╭──────────● testing-pi', 'c-warn')
  s.text(58, 11, '│\n│\n│', 'c-warn')
  await s.until('on top')
  const pi = machine(s, 100, 5, 'pi', { lines: ['follows testing-pi'], w: 30 })
  await s.until('test mode')
  pi.rows[0].node.textContent = 'test mode: no boot entry'
  pi.node.dataset.tone = 'warn'
  await s.until('reboot')
  s.type(100, 9, 'a reboot: back to main', 'c-ok')
  await s.until('forward', 2)
  fresh(s)
  s.text(8, 14, 'main', 'c-acc')
  s.text(14, 14, '●' + '──────────●'.repeat(4), 'c-acc')
  await s.until('refuses')
  s.text(26, 16, '◀──────────────────────── ✗ never back to an old commit', 'c-err')
  await s.until('reverts')
  s.text(59, 14, '──────────●', 'c-ok')
  s.text(68, 12, 'revert: a new commit', 'c-ok')
  await s.until('two ideas')
  card(s, 30, 22, ['Trials'], { tone: 'warn', w: 30 })
  card(s, 80, 22, ['Reverts'], { tone: 'ok', w: 30 })
})

scene('S10.3', async (s) => {
  header(s)
  const chain: [string, string][] = [['packages', 'store paths'], ['derivations', 'derivations'], ['flake', 'flake + lock'], ['modules', 'modules'],
    ['system', 'toplevel'], ['switching', 'switch'], ['generations', 'generations']]
  let x = 4
  for (const [cue, label] of chain) {
    await s.until(cue)
    const c = card(s, x, 6, [label], { tone: 'accent', kind: 'square' })
    x += c.rect.w
    if (cue !== 'generations') {
      harrow(s, x, 7, 3, 'c-line')
      x += 3
    }
  }
  await s.until('deploys')
  card(s, 20, 14, ['repository'], { tone: 'ok' })
  harrow(s, 36, 15, 8)
  card(s, 45, 14, ['comin'], { tone: 'ok' })
  harrow(s, 56, 15, 8)
  card(s, 65, 14, ['machines'], { tone: 'ok' })
  card(s, 20, 20, ['sops-nix: secrets'], { tone: 'warn' })
  card(s, 45, 20, ['home-manager: users'], { tone: 'warn' })
  await s.until('console', 2)
  fresh(s)
  const t = A.big('NIXOS-MANAGER')
  s.big(mid(len(t)), 16, 'NIXOS-MANAGER', 'acc')
})

void [COLS, larrow, list]
