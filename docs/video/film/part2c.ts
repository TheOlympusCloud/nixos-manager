// Part 2, chapters 11 and 12: what nixos-manager is, and installing it.
import { scene } from './film'
import { caption, card, code, harrow, header, larrow, list, machine, mid, point, uarrow } from './kit'

scene('S11.1', async (s) => {
  header(s)
  await s.until('web console')
  const srv = s.box(6, 4, 52, 12, { title: 'nixos-manager server', tone: 'accent' })
  const parts = ['working copy (git)', 'Nix: evaluate, check, build', 'web console', 'MCP endpoint']
  const item = (i: number) => list(s, srv.inner.x, srv.inner.y + 1 + i * 2, [parts[i]], '', '▪', srv)
  await s.until('working copy')
  item(0)
  await s.until('runs')
  item(1)
  await s.until('visually')
  item(2)
  await s.until('checks')
  const fj = machine(s, 96, 4, 'forgejo', { lines: ['fleet.git · main'], arch: 'git', w: 30 })
  await s.until('pushes')
  harrow(s, 60, 5, 34, 'c-ok')
  s.text(70, 4, 'push', 'c-ok')
  larrow(s, 60, 7, 34, 'c-line')
  s.text(64, 8, 'webhook · pull · statuses', 'c-m')
  await s.until('deploys')
  const hosts = ['homeserver', 'pi', 'workstation'].map((n, i) => machine(s, 62 + i * 28, 24, n, { w: 27 }))
  s.buf({ x: 64, y: 9, w: 81, h: 15 }, { l: 'c-cy' }, (L, t) => {
    const p = Math.min(1, t / 6)
    const xs = [11, 39, 67] // the machines' middles
    L.l.path([[xs[0], 6], [xs[2], 6]])
    for (const x of xs) L.l.path([[x, 14], [x, Math.max(14 - Math.round(p * 8), 6)]])
    if (p >= 1) {
      L.l.path([[47, 6], [47, 0]])
      L.l.set(47, 0, '▲')
      L.l.text(49, 3, 'comin pulls')
    }
  })
  await s.until('talk back')
  // One path from the machines (row 25, their middle line) up to the server.
  uarrow(s, 30, 17, 8, 'c-warn')
  s.text(30, 25, '└' + '┄'.repeat(31), 'c-warn')
  await s.until('reports')
  s.text(33, 20, 'reports · heartbeats · closures', 'c-warn')
  await s.until('endpoint')
  item(3)
  card(s, 6, 30, ['AI assistant · script'], { tone: 'accent' })
  card(s, 6, 25, ['browser'])
  void [fj, hosts]
})

scene('S11.2', async (s) => {
  header(s)
  const step = (x: number, y: number, t: string, tone: 'accent' | 'warn' | 'ok' | undefined) => card(s, x, y, [t.padStart(Math.floor((18 + t.length) / 2)).padEnd(18)], { tone })
  const edit = step(10, 6, 'Edit', 'accent')
  const check = step(112, 6, 'Check', 'warn')
  const commit = step(112, 27, 'Commit', 'ok')
  const push = step(10, 27, 'Push = deploy', 'accent')
  harrow(s, 33, 7, 77, 'c-line')
  s.text(122, 10, '│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n▼', 'c-line')
  s.text(33, 28, '◀' + '─'.repeat(76), 'c-line')
  s.text(20, 10, '▲\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│\n│', 'c-line')
  void [edit, check, commit, push]
  await s.until('cannot commit')
  s.text(124, 17, '◼ locked', 'c-warn')
  await s.until('cannot push')
  s.text(66, 29, '◼ locked', 'c-warn')
  await s.until('exactly')
  s.type(mid(54), 13, 'a commit needs a passing check of exactly what is staged', 'c-warn')
  await s.until('person')
  s.type(mid(54), 16, '✓ a person in the web page can override it, explicitly', 'c-b')
  await s.until('automation')
  s.type(mid(54), 18, '✗ automation, through the API or MCP, never', 'c-err')
  await s.until('edit check', 1)
  for (const b of [edit, check, commit, push]) {
    b.node.dataset.tone = 'ok'
    await s.wait(3)
  }
})

const PCT = `echo 'ssh-ed25519 AAAA... you@laptop' > /root/me.pub
pct create 120 local:vztmpl/nixos-26.05-proxmox-lxc.tar.xz \\
  --hostname nixos-manager \\
  --ostype nixos --unprivileged 1 --features nesting=1 \\
  --cores 4 --memory 4096 --swap 2048 \\
  --rootfs local-lvm:30 \\
  --net0 name=eth0,bridge=vmbr0,ip=dhcp \\
  --ssh-public-keys /root/me.pub \\
  --onboot 1 --start 1`

const MODULE = `services.nixos-manager = {
  enable = true;
  openFirewall = true; # port 8080
  author = {
    name = "Your Name";
    email = "you@example.com";
  };
  passwordHashFile = "/var/lib/secrets/nixos-manager-password";
};`

scene('S12.1', async (s) => {
  header(s)
  await s.until('recommended')
  card(s, mid(40), 8, ['  ▣ NixOS container', '  └ nixos-manager'], { title: 'Proxmox', tone: 'accent', w: 40 })
  await s.until('builds for real')
  s.type(mid(30), 14, 'it evaluates and builds for real', 'c-ok')
  await s.until('download', 2)
  s.clear()
  header(s)
  const pct = code(s, 6, 4, PCT, 'on the Proxmox host')
  await s.until('nesting')
  point(s, pct, 3)
  await s.until('memory')
  point(s, pct, 4, 'c-acc')
  caption(s, '4 GB of RAM to start · 8 GB past a dozen machines · 30 GB of disk', 'warn')
  await s.until('import', 2)
  s.clear()
  header(s)
  const mod = code(s, 6, 4, MODULE, '/etc/nixos/configuration.nix')
  await s.until('author')
  for (const i of [3, 4, 5, 6]) point(s, mod, i)
  await s.until("password's")
  point(s, mod, 7, 'c-acc')
  await s.until('pipe')
  code(s, 6, 17, `printf '%s\\n' "$pw" | nixos-manager hash-password > /var/lib/secrets/nixos-manager-password\nsystemctl restart nixos-manager`, 'the password')
  await s.until('port')
  caption(s, '→ http://<container>:8080', 'ok')
})

const OPTIONS = [
  ['enable', 'Run the console'], ['package', 'The nixos-manager build (the flake sets it)'], ['listen', 'Address and port; default 0.0.0.0:8080'],
  ['openFirewall', 'Open that port'], ['dataDir', 'Working copy, keys, tokens; /var/lib/nixos-manager'],
  ['author.name, author.email', 'Identity of every commit'], ['passwordHashFile', 'bcrypt hash of the login password'],
  ['remote', 'Repository to clone and push (or set it in Settings)'], ['environmentFile', 'Extra secrets as KEY=value'],
  ['extraEnvironment', 'Extra variables, e.g. NIXMGR_EVAL_JOBS = "1"'], ['checks.build', 'false: checks only evaluate; each machine builds itself'],
  ['binaryCache.enable, .openFirewall', 'Serve the store to the machines (port 5000)'],
]

scene('S12.2', async (s) => {
  header(s)
  await s.until('all the')
  const t = s.box(mid(104), 4, 104, OPTIONS.length * 2 + 3, { title: 'services.nixos-manager.*', kind: 'square' })
  const rows = OPTIONS.map((o, i) => s.html(t.inner.x, t.inner.y + 1 + i * 2, `<b>${o[0].padEnd(36)}</b><span class="c-m">${o[1]}</span>`, '', t))
  await s.until('checks dot build')
  rows[10].node.className = 'c-warn'
  s.text(t.rect.x + 1, rows[10].rect.y, '▌', 'c-warn', t)
  await s.until('binary cache')
  s.text(t.rect.x + 1, rows[11].rect.y, '▌', 'c-cy', t)
})

const COMPOSE = `services:
  nixos-manager:
    build: .
    ports: ["8080:8080"]
    environment:
      NIXMGR_AUTHOR_NAME: "Your Name"
      NIXMGR_AUTHOR_EMAIL: "you@example.com"
      NIXMGR_PASSWORD_HASH_FILE: /run/secrets/nixmgr/password-hash
      # NIXMGR_CHECK_BUILD: "0"
  data-owner:   # gives the data volume to the image's user
    user: root`

scene('S12.3', async (s) => {
  header(s)
  await s.until('docker image')
  const c = code(s, 4, 4, COMPOSE, 'compose.yaml')
  await s.until('cannot sandbox')
  card(s, 80, 4, ["builds can read the server's data,", 'including the key to the secrets.', '', 'Only when everyone who can push', 'may see the secrets.'], { title: 'Docker', tone: 'warn' })
  await s.until('otherwise')
  s.type(82, 12, 'Otherwise: the NixOS container', 'c-ok')
  await s.until('plain')
  caption(s, 'Plain HTTP: keep it on a trusted network, Tailscale or a TLS proxy')
  void c
})
