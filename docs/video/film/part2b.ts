// Part 2, chapters 18 to 32: matrices, fleet modules, options, Health, changes, trials, deployments, inputs,
// history, secrets, the binary cache, homes, automation, recipes and the closing.
import { scene, snowflake, A, type El, type Stage } from './film'
import { COLS, caption, card, code, harrow, header, larrow, len, list, machine, mid, point, uarrow, varrow, walk } from './kit'

const VIEW: [number, number, number, number] = [0.11, 0.05, 0.89, 0.9]
const LEFT: [number, number, number, number] = [0.11, 0.06, 0.18, 0.6]
const RIGHT: [number, number, number, number] = [0.29, 0.05, 0.71, 0.9]

/** Clear the stage for a diagram, keeping the header. */
function fresh(s: Stage) {
  s.clear()
  header(s)
}

/** A line of commits: ● on a ─ rule, as the console draws history. */
function commits(s: Stage, x: number, y: number, n: number, gap = 8, label = 'main', cls = 'c-acc') {
  s.text(x, y, label, cls)
  const lx = x + len(label) + 2
  return s.text(lx, y, Array.from({ length: n }, () => '●').join('─'.repeat(gap - 1)), cls)
}

scene('S18.1', (s) =>
  walk(s, [
    ['show', 'matrix-packages'],
    ['look', [0.11, 0.28, 0.89, 0.65], 'matrices', 1.05],
    ['mark', 'htop', 'gaps'],
    ['show', 'matrix-services', 'health'],
    ['look', [0.11, 0.18, 0.89, 0.7], undefined, 1.05],
  ]),
)

scene('S19.1', async (s) => {
  await walk(s, [
    ['show', 'mod-new'],
    ['look', [0.11, 0.05, 0.89, 0.5], 'bundles', 1.1],
    ['show', 'mod-new-filled', 'create a module'],
    ['look', [0.17, 0.2, 0.45, 0.2], undefined, 1.3],
    ['show', 'mod-filled', 'give it an option'],
    ['look', VIEW, undefined, 1.02],
    ['show', 'mod-page-2', 'settings'],
    ['look', VIEW, undefined, 1.02],
    ['cap', "@option:port@ = this machine's value of the port option", 'means'],
  ])
  await s.until('behind the scenes', 2)
  fresh(s)
  const js = code(s, 6, 4, '{\n  "options": { "port": { "type": "port", "default": 8096 } },\n  "packages": [ "ffmpeg" ],\n  "settings": {\n    "services": { "jellyfin": { "enable": true } },\n    "networking": { "firewall": { "allowedTCPPorts": [ "@option:port@" ] } }\n  }\n}', 'modules/fleet/media/managed.json')
  await s.wait(16)
  const nx = code(s, 6, 15, 'options.fleet.modules.media = {\n  enable = lib.mkEnableOption "media";\n  port = lib.mkOption { type = types.port; default = 8096; };\n};\nconfig = lib.mkIf cfg.enable { … };', 'what NixOS sees')
  await s.until('hand-written')
  point(s, js, 5)
  await s.until('on each machine')
  caption(s, 'on each machine: fleet.modules.media.enable = true, in its managed.json')
  await s.until('without a default')
  point(s, nx, 2)
  await s.until('add nix', 2)
  fresh(s)
  await walk(s, [['show', 'mod-filled'], ['look', [0.75, 0.08, 0.25, 0.06], undefined, 1.3]])
})

scene('S20.1', (s) =>
  walk(s, [
    ['show', 'options'],
    ['look', [0.11, 0.06, 0.89, 0.6], 'browses', 1.05],
    ['show', 'options-search', 'select one'],
    ['look', 'search', undefined, 2.5],
    ['show', 'options-permit', 'type'],
    ['look', [0.3, 0.05, 0.7, 0.9], undefined, 1.05],
    ['show', 'options-permit-2', 'form'],
    ['look', [0.3, 0.05, 0.7, 0.9], undefined, 1.05],
    ['cap', 'NixOS computes it, or it sits inside a list of submodules: set it in Code', "can't be set", 'warn'],
  ]),
)

scene('S20.2', (s) =>
  walk(s, [
    ['show', 'options-tz-2'],
    ['look', [0.3, 0.3, 0.7, 0.6], 'on every machine', 1.05],
    ['show', 'options-tz-copy', 'copy to'],
    ['look', [0.3, 0.2, 0.7, 0.7], undefined, 1.05],
    ['cap', 'lib.mkForce in hand-written Nix: a click cannot beat it', 'make force', 'warn'],
  ]),
)

const FINDINGS: [string, string, string][] = [
  ['✖', 'Machine does not evaluate', 'c-err'], ['▲', 'Firewall disabled → Turn the firewall on', 'c-warn'],
  ['▲', 'SSH accepts passwords → Allow SSH keys only', 'c-warn'], ['▲', 'SSH allows root logins → Forbid root logins', 'c-warn'],
  ['◆', 'SSH without fail2ban → Enable fail2ban', 'c-acc'], ['◆', 'No automatic garbage collection → Collect weekly, keep 14 days', 'c-acc'],
  ['◆', 'Store not optimised → Optimise the store automatically', 'c-acc'], ['▲', 'Input N days old → Update input', 'c-warn'],
  ['▲', 'Release at or near end of life · mixed releases', 'c-warn'], ['◆', 'Package on every machine except one', 'c-acc'],
  ['▲', 'No comin, or comin pulling another repository', 'c-warn'], ['▲', 'Last pushed commit not checked → Check', 'c-warn'],
  ['✖', 'Deployment failed · behind > 1 h · silent · changed by hand · reboot pending', 'c-err'],
]

scene('S21.1', async (s) => {
  await walk(s, [['show', 'health'], ['look', VIEW, 'checklist', 1.02]])
  await s.until('flags', 2)
  fresh(s)
  const b = s.box(mid(92), 4, 92, 29, { title: 'Health', tag: 'what it flags', kind: 'square' })
  const row = (i: number) => s.html(b.inner.x, b.inner.y + 1 + i * 2, `<span class="${FINDINGS[i][2]}">${FINDINGS[i][0]}</span> ${A.esc(FINDINGS[i][1])}`, '', b)
  const cues = ['evaluate', 'firewall', 'passwords', 'root', 'without', 'garbage', 'optimisation', 'old', 'releases', 'except', 'without co-min', 'not passed', 'deployment problems']
  for (let i = 0; i < FINDINGS.length; i++) {
    try {
      await s.until(cues[i])
    } catch {
      await s.wait(3)
    }
    row(i)
  }
  await s.until('fix button', 2)
  fresh(s)
  await walk(s, [['show', 'health-2'], ['look', VIEW, undefined, 1.02], ['cap', 'Before you press it, make sure each machine has an SSH key', 'before you press', 'warn']])
})

scene('S22.1', (s) =>
  walk(s, [
    ['show', 'changes'],
    ['look', VIEW, 'every click', 1.02],
    ['look', [0.29, 0.25, 0.71, 0.6], 'text diff', 1.1],
    ['show', 'changes-staged', 'stage'],
    ['look', LEFT, undefined, 1.2],
  ]),
)

const LEGEND: [string, string][] = [['+ packages, services, users, ports', 'c-ok'], ['units: restart · reload · start · stop · not restarted', 'c-warn'],
  ['/etc files added, changed, removed', 'c-acc'], ['reboot needed: kernel, initrd, kernel parameters', 'c-err'],
  ['after a check: the closure size, and every version that moves', 'c-pur'], ["machines that don't change: listed as unchanged", 'c-m']]

scene('S22.2', async (s) => {
  await walk(s, [
    ['show', 'changes'],
    ['look', [0.29, 0.08, 0.71, 0.2], 'below the diff', 1.1],
    ['cap', 'A machine changes when its system toplevel changes', 'top level'],
    ['nocap', 'for each changed machine'],
    ['show', 'changes-2'],
    ['look', RIGHT, undefined, 1.02],
  ])
  await s.until('decided', 2)
  fresh(s)
  const b = s.box(mid(70), 7, 70, 16, { title: 'On the machines', kind: 'square', tone: 'accent' })
  const row = (i: number) => s.type(b.inner.x, b.inner.y + 1 + i * 2, LEGEND[i][0], LEGEND[i][1], b)
  row(0)
  row(1)
  for (const [cue, i] of [['slash', 2], ['reboot', 3], ['closure size', 4], ['unchanged', 5]] as const) {
    await s.until(cue)
    row(i)
  }
})

scene('S22.3', async (s) => {
  await walk(s, [['show', 'checks-panel'], ['look', LEFT, 'run the check', 1.2], ['show', 'check-running', 'flake check'], ['look', RIGHT, undefined, 1.02]])
  await s.until('really builds', 2)
  fresh(s)
  commits(s, 12, 7, 4, 10)
  s.text(12 + 6 + 30, 9, '└ the newest pushed commit that passed a check', 'c-ok')
  const st = card(s, 100, 6, ['staged content'], { tone: 'warn' })
  void st
  s.text(56, 7, '┄'.repeat(40), 'c-warn')
  await s.until('so a package')
  const hosts = [['homeserver', 'toplevel differs → build', 'warn'], ['pi', 'toplevel differs → build', 'warn'], ['workstation', 'the same → skip', '']] as const
  hosts.forEach(([n, why, tone], i) => {
    machine(s, 14 + i * 42, 15, n, { lines: [why], tone: tone || undefined, w: 36 })
  })
  await s.until('log streams', 2)
  fresh(s)
  await walk(s, [
    ['show', 'check-done'],
    ['look', RIGHT, undefined, 1.02],
    ['show', 'checks-panel', 'tick other'],
    ['look', [0.11, 0.1, 0.18, 0.25], undefined, 1.4],
    ['cap', 'checks.build = false: only evaluate and dry-build', 'small server', 'warn'],
  ])
})

scene('S22.4', (s) =>
  walk(s, [
    ['show', 'commit-message'],
    ['look', 'message', 'write a message', 1.8],
    ['show', 'history-show', 'trailer'],
    ['look', RIGHT, undefined, 1.02],
    ['cap', 'Via: web session · or Via: token <name>', 'how it arrived'],
  ]),
)

scene('S22.5', async (s) => {
  await walk(s, [['show', 'push-dialog'], ['look', [0.24, 0.21, 0.52, 0.57], 'push opens', 1.1]])
  await s.until('goes to', 2)
  fresh(s)
  const srv = card(s, 8, 7, ['the commit', 'checked ✓'], { title: 'nixos-manager', tone: 'accent' })
  const fj = machine(s, 60, 7, 'forgejo', { lines: ['fleet.git · main'], arch: 'git' })
  harrow(s, srv.rect.x + srv.rect.w + 1, 8, fj.rect.x - srv.rect.x - srv.rect.w - 2, 'c-warn')
  const hs = ['homeserver', 'pi', 'workstation'].map((n, i) => machine(s, 20 + i * 40, 20, n, { w: 30 }))
  await s.until('comin takes over')
  // comin on homeserver and pi pulls from Forgejo: one bus from their middles up into it.
  const up = fj.rect.x + Math.floor(fj.rect.w / 2)
  uarrow(s, up, 10, 5, 'c-cy')
  s.text(up + 2, 12, 'comin pulls', 'c-cy')
  s.text(35, 15, '┌' + '─'.repeat(up - 36) + '┴' + '─'.repeat(74 - up) + '┐', 'c-cy')
  for (const i of [0, 1]) {
    s.text(35 + i * 40, 16, '│\n│\n│\n│', 'c-cy')
    hs[i].node.dataset.tone = 'ok'
    hs[i].rows[0].node.textContent = '✓ switched'
  }
  hs[2].rows[0].node.textContent = 'unchanged'
  hs[2].rows[0].node.className = 'c-m'
  await s.until('rebases')
  caption(s, 'Rebased by a pull = never checked as a whole: Push refuses. Check again, then push.', 'warn')
})

scene('S23.1', async (s) => {
  await walk(s, [
    ['show', 'trial-ready'],
    ['look', LEFT, 'try it on one machine', 1.2],
    ['mark', 'try', 'try on pi'],
    ['show', 'trial-confirm', 'testing pi branch'],
    ['look', [0.3, 0.4, 0.4, 0.2], undefined, 1.3],
    ['show', 'trial-view', 'watch it'],
    ['look', [0.29, 0.1, 0.71, 0.4], undefined, 1.1],
    ['show', 'trial-end-confirm', 'end the trial'],
    ['look', [0.15, 0.4, 0.4, 0.2], undefined, 1.3],
  ])
  await s.until('why does', 2)
  fresh(s)
  const pi = machine(s, 16, 12, 'pi', { lines: ['network: changed'], w: 26 })
  const fj = machine(s, 100, 12, 'forgejo', { lines: ['the fix'], arch: 'git', w: 26 })
  s.text(44, 13, '┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄ ✗ ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄', 'c-err')
  void fj
  await s.until('loses')
  pi.node.dataset.tone = 'err'
  await s.until('with a trial')
  caption(s, 'With a trial, a reboot is always enough', 'ok')
})

scene('S24.1', (s) =>
  walk(s, [
    ['show', 'deploy-setup'],
    ['look', 'step1', 'report back', 1.25],
    ['look', 'step2', 'report token', 1.2],
    ['show', 'deploy-setup-2', 'one click'],
    ['look', VIEW, undefined, 1.02],
    ['cap', 'A token reports for its own machine only', 'own machine'],
  ]),
)

scene('S24.2', (s) =>
  walk(s, [
    ['show', 'deployments'],
    ['look', VIEW, 'latest commits', 1.02],
    ['mark', 'pi', 'error', 'err'],
    ['show', 'deployments-2', 'heartbeat'],
    ['look', VIEW, undefined, 1.02],
    ['cap', 'Silent after 2 hours without a heartbeat · for a laptop: silentAfter = null', 'silent', 'warn'],
    ['cap', 'changed by hand · switch unfinished · reboot pending', 'changed by hand'],
  ]),
)

scene('S24.3', async (s) => {
  await walk(s, [['show', 'revert-dialog'], ['look', VIEW, 'revert the commit', 1.02], ['show', 'revert-changes', 'stages a revert'], ['look', VIEW, undefined, 1.02]])
  await s.until('no', 2)
  fresh(s)
  commits(s, 20, 12, 4, 12)
  s.text(26 + 36, 13, '▲ broke pi', 'c-err')
  await s.until('forward')
  s.text(26 + 36 + 1, 12, '───────────●', 'c-ok')
  s.text(26 + 36 + 13, 13, '▲ revert: a new commit with the old configuration', 'c-ok')
  s.text(26, 16, '◀── never back to an old commit: comin only moves forward', 'c-m')
  await s.until('cut a machine')
  caption(s, 'Cut off? Push the fix, then boot the previous generation: comin picks the fix up', 'warn')
})

scene('S24.4', (s) =>
  walk(s, [
    ['show', 'notifications'],
    ['look', 'sec:NOTIFICATIONS', 'notifications', 1.1],
    ['cap', 'ntfy · nixos-manager › pi failed to deploy a3f19c2', 'fails to deploy', 'err'],
  ]),
)

scene('S25.1', (s) =>
  walk(s, [['show', 'dependencies'], ['look', VIEW, 'side by side', 1.02], ['cap', 'Versions differ: pi still runs openssl 3.0.15', 'older', 'warn']]),
)

scene('S26.1', (s) =>
  walk(s, [
    ['show', 'inputs'],
    ['look', [0.11, 0.05, 0.89, 0.6], 'inputs lists', 1.05],
    ['cap', 'amber after 30 days · red after 60', 'amber', 'warn'],
    ['mark', 'update', 'update one'],
  ]),
)

scene('S26.2', async (s) => {
  await walk(s, [['show', 'scheduled'], ['look', 'sec:SCHEDULED UPDATE', 'schedule', 1.05]])
  await s.until('commits the new lock', 2)
  fresh(s)
  commits(s, 16, 18, 4, 12)
  s.text(16 + 6 + 36, 15, '╭───────────●  flake-update', 'c-warn')
  s.text(16 + 6 + 36, 16, '│', 'c-warn')
  s.text(16 + 6 + 36, 17, '│', 'c-warn')
  s.text(16 + 6 + 36 + 15, 16, 'no machine follows it', 'c-m')
  await s.until('checks it')
  s.text(16 + 6 + 36 + 15, 13, '✓ checked · a notification: ready', 'c-ok')
  await s.until('inputs then shows', 2)
  fresh(s)
  await walk(s, [['show', 'scheduled-2'], ['look', VIEW, undefined, 1.02], ['cap', 'Merge into main: a fast-forward · or Discard', 'merge into main', 'ok']])
})

scene('S27.1', (s) =>
  walk(s, [
    ['show', 'history-commit'],
    ['look', VIEW, 'every commit', 1.02],
    ['show', 'history-show', 'what it did'],
    ['look', RIGHT, undefined, 1.02],
    ['show', 'file-history', 'single files'],
    ['look', VIEW, undefined, 1.02],
  ]),
)

scene('S27.2', (s) =>
  walk(s, [
    ['show', 'code-file'],
    ['look', VIEW, 'full editor', 1.02],
    ['look', [0.25, 0.08, 0.6, 0.6], 'completion', 1.1],
    ['cap', 'Ctrl+S saves · Shift+Alt+F formats with nixfmt', 'control'],
    ['cap', 'Clicks go to managed.json: your hand-written files stay yours', 'never rewrites', 'ok'],
  ]),
)

const LAYOUT = [['.sops.yaml', 'who can open each file'], ['secrets/common.json', 'every machine with a key'],
  ['secrets/hosts/<name>.pub', "a machine's SSH host key (public)"], ['secrets/hosts/<name>.json', 'that machine alone']]

scene('S28.1', async (s) => {
  header(s)
  const b = s.box(mid(76), 9, 76, 11, { title: 'the secrets in the repository', kind: 'square' })
  const row = (i: number) => s.html(b.inner.x, b.inner.y + 1 + i * 2, `<span class="c-acc">${LAYOUT[i][0].padEnd(28)}</span><span class="c-m">${LAYOUT[i][1]}</span>`, '', b)
  await s.until('keeps')
  row(0)
  await s.until('common')
  row(1)
  await s.until('named after')
  row(2)
  row(3)
  await s.until('age key', 2)
  fresh(s)
  await walk(s, [
    ['show', 'secrets'],
    ['look', 'sec:SECRETS', undefined, 1.1],
    ['cap', 'Back up $NIXMGR_DATA/sops-age-key', 'back it up', 'warn'],
    ['nocap', 'approved'],
    ['show', 'secrets-2'],
    ['look', VIEW, undefined, 1.02],
    ['cap', "A key from a pull or a token waits for a person's approval", "person's"],
  ])
})

scene('S28.2', async (s) => {
  await walk(s, [
    ['show', 'secrets'],
    ['look', 'sec:SECRETS', 'add a secret', 1.1],
    ['cap', 'Only the encrypted form is written · Show decrypts it for you alone', 'encrypted', 'ok'],
    ['cap', 'No MCP tool can ever read a value', 'no', 'err'],
  ])
  await s.until('on the machine', 2)
  fresh(s)
  card(s, mid(30), 7, ['/run/secrets/wifi'], { tone: 'ok' })
  await s.until('point')
  const c = code(s, mid(74), 13, 'sops.secrets.wifi.owner = "wpa_supplicant";\nnetworking.wireless.secretsFile = config.sops.secrets.wifi.path;')
  await s.wait(12)
  point(s, c, 1)
  await s.until('two setup', 2)
  fresh(s)
  await walk(s, [['show', 'secrets-2'], ['look', VIEW, undefined, 1.02], ['cap', 'Every change here is a change in the working copy: check, commit, push', 'every change']])
})

scene('S29.1', async (s) => {
  header(s)
  await s.until('already builds')
  const srv = card(s, 8, 6, ['the check built', "pi's system"], { title: 'nixos-manager', tone: 'accent' })
  const pi = machine(s, 96, 6, 'pi', { lines: ['aarch64'], w: 30 })
  await s.until('turn on')
  harrow(s, srv.rect.x + srv.rect.w + 1, 7, pi.rect.x - srv.rect.x - srv.rect.w - 2, 'c-cy')
  s.text(40, 8, ':5000 · signed with its key', 'c-cy')
  await s.until('downloads')
  pi.rows[0].node.textContent = '✓ downloaded, not compiled'
  pi.rows[0].node.className = 'c-ok'
  await s.until('enable')
  code(s, 8, 14, 'services.nixos-manager.binaryCache = { enable = true; openFirewall = true; };', 'the server')
  await s.until('address')
  code(s, 8, 19, 'fleet.binaryCache = {\n  url = "http://nixos-manager.home.lan:5000";\n  publicKey = "nixos-manager-1:…";\n};', 'modules/common.nix')
  await s.until('away from home')
  s.text(96, 10, 'away from home: builds after 5 s', 'c-m')
  await s.until('emulation')
  caption(s, 'ARM machine, x86 server: emulation or a remote ARM builder (see the README)', 'warn')
})

scene('S30.1', (s) =>
  walk(s, [
    ['show', 'laptop'],
    ['look', VIEW, 'standalone', 1.02],
    ['show', 'laptop-2', 'kept in'],
    ['look', VIEW, undefined, 1.02],
    ['cap', 'home-manager switch --flake .#pablo@laptop', 'deploy it'],
    ['nocap', 'health'],
    ['show', 'health-managed'],
    ['look', [506, 790, 2324, 310], undefined, 1.2], // the managed.nix finding
    ['mark', 'button', 'update'],
    ['show', 'managed-diff', 'review'],
    ['look', [0.29, 0.25, 0.71, 0.3], undefined, 1.1],
  ]),
)

const SCOPES: [string, string, string][] = [['Read only', 'files, history, the fleet, deployments', 'accent'],
  ['Edit, no push', 'also: edit, stage, check, commit, write secrets', 'warn'], ['Full', 'also: push, merge an update, read a secret, approve keys', 'err']]

scene('S31.1', async (s) => {
  await walk(s, [['show', 'tokens'], ['look', 'sec:API TOKENS', 'api tokens', 1.1]])
  await s.until('scope', 2)
  fresh(s)
  const boxes = [
    { x: 30, y: 5, w: 86, h: 27 },
    { x: 34, y: 8, w: 78, h: 20 },
    { x: 38, y: 11, w: 70, h: 13 },
  ]
  let inside: El | null = null
  for (const [cue, i] of [['read only', 2], ['edit', 1], ['full', 0]] as const) {
    await s.until(cue)
    const [name, what, tone] = SCOPES[2 - i]
    const ring = s.box(boxes[i].x, boxes[i].y, boxes[i].w, boxes[i].h, { title: name, tone: tone as 'accent' })
    if (inside) inside.parent = ring // each scope holds the one before
    s.type(boxes[i].x + 2, boxes[i].y + boxes[i].h - 2, what, 'c-m', ring)
    inside = ring
  }
  await s.until('smallest')
  caption(s, 'The smallest scope it needs · an expiry if it is temporary', 'ok')
})

const TOOLS: [string, string[]][] = [
  ['Fleet', ['fleet_overview', 'fleet_insights', 'host_details', 'model_diff', 'compare_option', 'search_packages', 'search_options']],
  ['Packages and options', ['add_packages', 'remove_packages', 'edit_packages', 'set_package_override', 'set_option', 'unset_option', 'copy_to_machines']],
  ['Homes', ['set_home_manager', 'link_home_secret', 'set_home_config', 'set_keyring_entry']],
  ['Modules', ['list_modules', 'create_module', 'edit_module', 'delete_module', 'update_managed_module']],
  ['Machines and files', ['create_machine', 'list_files', 'read_file', 'write_file', 'format_nix']],
  ['Git', ['status', 'diff', 'stage', 'commit', 'log', 'revert_commit', 'merge_branch', 'push', 'pull']],
  ['Checks and inputs', ['list_targets', 'run_check', 'flake_lock', 'flake_update']],
  ['Deployments', ['deployments', 'try_on_machine', 'end_trial', 'machine_dependencies', 'fleet_dependencies']],
  ['Secrets', ['list_secrets', 'set_secret', 'delete_secret']],
]

scene('S31.2', async (s) => {
  header(s)
  await s.until('point any')
  code(s, mid(62), 8, '{\n  "mcpServers": {\n    "nixos-manager": {\n      "type": "http",\n      "url": "http://<server>:8080/mcp",\n      "headers": { "Authorization": "Bearer <token>" }\n    }\n  }\n}', 'an MCP client')
  await s.until('sixty three', 2)
  fresh(s)
  const groups = []
  for (let i = 0; i < TOOLS.length; i++) {
    const [g, tools] = TOOLS[i]
    const x = 3 + (i % 3) * 47
    const y = 3 + Math.floor(i / 3) * 11
    const b = s.box(x, y, 45, 10, { title: g, kind: 'square', tone: i < 5 ? undefined : 'accent' })
    tools.forEach((t, k) => s.text(b.inner.x + (k % 2) * 21, b.inner.y + Math.floor(k / 2), t, 'c-m', b))
    groups.push(b)
    await s.wait(2)
  }
  await s.until('scope allows')
  for (const b of groups.slice(5)) b.node.dataset.tone = 'err'
  await s.wait(12)
  for (const b of groups.slice(5)) b.node.dataset.tone = 'accent'
  await s.until('same code', 2)
  fresh(s)
  const rules = [['same code as the web page, same rules', 'c-b'], ['a file is read before it is written', 'c-acc'],
    ['no commit, no push without a passing check', 'c-warn'], ['every commit says so: Via: token <name>', 'c-ok']]
  s.type(mid(60), 9, rules[0][0], rules[0][1])
  for (const [cue, i] of [['read a file', 1], ['never commit', 2], ['trailer', 3]] as const) {
    await s.until(cue)
    s.type(mid(60), 9 + i * 4, rules[i][0], rules[i][1])
  }
})

const RECIPES: [string, string, string[], string][] = [
  ['install a tool', 'Install a tool on one machine', ['pkg-add', 'changes-staged', 'check-done', 'deployments'], 'Packages › Add packages › Changes › Check › Commit › Push › Deployments'],
  ['run a service', 'Run a service', ['catalog', 'svc-configure', 'check-done', 'push-dialog'], 'Services › Add service › Configure › Check › Commit › Push'],
  ['risky', 'A risky network change', ['check-done', 'trial-ready', 'trial-view', 'trial-end-confirm'], 'Stage › Check › Try on <machine> › watch › Promote to main'],
  ['breaks', 'Something broke', ['deployments', 'revert-dialog', 'revert-changes', 'push-dialog'], 'Deployments › Revert the commit that broke it › Check › Commit › Push'],
  ['stay up to date', 'Stay up to date', ['scheduled', 'scheduled-2', 'inputs', 'deployments'], 'Inputs › Scheduled update › notification › review › Merge into main'],
]

scene('S32.1', async (s) => {
  header(s)
  const t = A.big('FIVE RECIPES')
  s.big(mid(len(t)), 14, 'FIVE RECIPES')
  for (let i = 0; i < RECIPES.length; i++) {
    const [cue, name, shots, steps] = RECIPES[i]
    await s.until(cue, 3)
    fresh(s)
    s.html(4, 4, `<span class="c-acc">${i + 1}</span>  <b>${A.esc(name)}</b>`)
    shots.forEach((sh, k) => s.pic(3 + k * 35, 9, 34, 11, sh))
    caption(s, steps)
  }
})

scene('S32.2', async (s) => {
  header(s)
  await s.until('every machine')
  const hosts = ['homeserver', 'pi', 'workstation'].map((n, i) => machine(s, 14 + i * 42, 22, n, { w: 32 }))
  const fj = machine(s, mid(32), 4, 'forgejo', { lines: ['fleet.git · main'], arch: 'git', w: 32 })
  const con = card(s, mid(32), 12, ['❄ nixos-manager'], { tone: 'accent', w: 32 })
  varrow(s, 72, 8, 4, 'c-acc')
  void hosts, fj, con
  const words: [string, string, string][] = [['every change', 'shown in NixOS terms', 'c-acc'], ['checked', 'checked by a real build', 'c-warn'],
    ['deployed', 'deployed by a push', 'c-ok'], ['reversible', 'every step reversible', 'c-pur']]
  for (let i = 0; i < words.length; i++) {
    await s.until(words[i][0])
    s.type(4 + i * 36, 30, '› ' + words[i][1], words[i][2])
  }
  await s.until("that's", 2)
  s.clear()
  s.buf({ x: mid(41), y: 3, w: 41, h: 18 }, { hot: 'c-acc', globe: 'c-acc2' }, (L, t) => snowflake(L, 20, 8.5, 9, t * 0.06))
  const a = A.big('NIX, NIXOS')
  const b = A.big('NIXOS-MANAGER')
  s.big(mid(len(a)), 23, 'NIX, NIXOS')
  s.big(mid(len(b)), 27, 'NIXOS-MANAGER', 'acc')
})

void [COLS, larrow, list]
