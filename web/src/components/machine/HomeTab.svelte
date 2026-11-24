<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type Host } from '../../lib/api'
  import { frame } from '../../lib/ascii'
  import { addHomePackages, excludePackages, fleet, loadManaged, openOrCreate, removeHomePackages, restorePackages, setHome, targetName } from '../../lib/fleet.svelte'
  import { pathName } from '../../lib/options'
  import { SHARED, SHARED_FILE, appliesShared, dotfilesDir, homeNixFile, homeNixTemplate, homeRows, homeStateVersion, type PackageRow } from '../../lib/packages'
  import { codeLink, link, machineLink } from '../../lib/router.svelte'
  import { app, refresh } from '../../lib/state.svelte'
  import { confirmBox, report } from '../../lib/ui.svelte'
  import { managedOptions, showValue } from '../../lib/values'
  import Drawer from '../Drawer.svelte'
  import Icon from '../Icon.svelte'
  import OptionBrowser from '../OptionBrowser.svelte'
  import PackagePicker from '../PackagePicker.svelte'
  import ManagedModuleNotice from './ManagedModuleNotice.svelte'

  /** Each user's home-manager configuration on the machine, as home-manager's NixOS module builds it. */
  let { host }: { host: Host } = $props()

  const managed = $derived(fleet.managed[host.name])
  const full = $derived(!!host.managedFile && !!managed && (host.managedVersion ?? 1) >= 2)
  let user = $state(untrack(() => (host.users.find((u) => u.home) ?? host.users[0])?.name ?? ''))
  const u = $derived(host.users.find((x) => x.name === user))
  const mine = $derived(managed?.home?.[user])
  const hasHome = $derived(!!u?.home || !!mine)
  /** A user's packages in modules/managed.json reach every machine whose managed.json has their home (managed.nix 3). */
  const canShare = $derived(full && appliesShared(host) && !!mine)
  $effect(() => {
    if (canShare && !fleet.managed[SHARED]) loadManaged(SHARED)
  })
  const shared = $derived(canShare ? fleet.managed[SHARED] : undefined)
  let everywhere = $state(false)
  const target = $derived(everywhere && shared ? SHARED : host.name)
  const edited = $derived(fleet.managed[target]?.home?.[user])
  const rows = $derived(homeRows(host, user, managed, shared))
  const attrIn = (p: PackageRow) => (target === SHARED ? p.sharedAttr : p.attr)

  const filesDir = $derived(dotfilesDir(host, user))
  const dotfiles = $derived(app.entries.filter((e) => !e.dir && e.path.startsWith(filesDir)).map((e) => e.path.slice(filesDir.length)))
  /** What home-manager links into ~ from modules and programs, not from the files here. */
  const linked = $derived((u?.home?.files ?? []).filter((f) => !dotfiles.includes(f)))
  const nixFile = $derived(homeNixFile(host, user))
  const nixExists = $derived(app.entries.some((e) => e.path === nixFile))

  const prefix = 'home-manager.users.<name>.'
  const setHere = $derived(managedOptions(managed?.settings).filter((o) => o.path[0] === 'home-manager' && o.path[1] === 'users' && o.path[2] === user && o.path.length > 3))

  let busy = $state('')
  let drawer = $state<null | { kind: 'add' } | { kind: 'options'; query: string; selected: string }>(null)
  let newFile = $state('')
  const fileProblem = $derived.by(() => {
    const f = newFile.trim()
    if (!f) return ''
    if (f.startsWith('/') || f.startsWith('~') || f.split('/').some((s) => s === '..' || s === '' || s === '.')) return 'A path inside the home folder, such as .config/starship.toml.'
    if (f.split('/').some((s) => s.toLowerCase() === '.git')) return 'A .git folder cannot be kept in the repository.'
    return dotfiles.includes(f) ? 'That file is already here.' : ''
  })

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }
  const turnOn = () => run('on', () => setHome(host.name, user, true, u?.home ? undefined : homeStateVersion(host)))
  async function turnOff() {
    const ok = await confirmBox({
      title: `Stop managing ${user}'s home here?`,
      message: `What is set here for ${user} on ${host.name} goes: packages, removed packages and options.${u?.home ? ` A home-manager configuration written by hand for ${user} stays.` : ''} The files in ${filesDir} and ${nixFile} stay in the repository, unused: delete them in Code.`,
      ok: 'Stop managing',
      danger: true,
    })
    if (ok) await run('on', () => setHome(host.name, user, false))
  }

  async function remove(p: PackageRow) {
    const t = target
    const attr = attrIn(p)
    if (attr) return run(p.name, () => removeHomePackages(t, user, [attr]))
    const ok = await confirmBox({
      title: `Remove ${p.name} from ${user}'s home${t === SHARED ? ' on every machine' : ''}?`,
      message:
        `${p.name} comes from ${p.sharedAttr ? SHARED_FILE : `${user}'s home-manager configuration (a module or a program)`}, which stays as it is: it is taken out of ${user}'s packages by name, ` +
        (t === SHARED ? `in ${SHARED_FILE}, on every machine whose managed.json has ${user}'s home.` : `in ${host.managedFile}.`) +
        ' Restore puts it back.',
      ok: 'Remove',
    })
    if (ok) await run(p.name, () => excludePackages(t, [p.name], user))
  }

  async function addFile(e: SubmitEvent) {
    e.preventDefault()
    const f = newFile.trim()
    if (!f || fileProblem) return
    // The user must be in managed.json's home for the files next to it to apply.
    if (!mine && !(await setHome(host.name, user, true))) return
    if (await openOrCreate(filesDir + f, '')) newFile = ''
  }
  async function deleteFile(f: string) {
    const ok = await confirmBox({ title: `Delete ~/${f}?`, message: `${filesDir}${f} is deleted from the repository, so home-manager no longer links it into ${user}'s home.`, ok: 'Delete', danger: true })
    if (!ok) return
    await run('file:' + f, async () => {
      await api.remove(filesDir + f).catch((e) => report(e, `Could not delete ${f}`))
      await refresh()
    })
  }
</script>

{#if !host.homeManager}
  <div class="empty-state">
    <h2>home-manager is not loaded on {host.name}</h2>
    <p>
      Users get a home-manager configuration through home-manager's NixOS module, as the starter flake loads it for every machine. Add it to {host.name}'s modules in
      <span class="mono">flake.nix</span>, with the <span class="mono">home-manager</span> input, then run Lock:
    </p>
    <pre class="mono snippet">inputs.home-manager.nixosModules.home-manager</pre>
    <div class="row"><a class="btn primary" href={codeLink('flake.nix')}><Icon name="code" /> Open flake.nix</a></div>
  </div>
{:else if !host.users.length}
  <div class="empty-state">
    <h2>No users on {host.name}</h2>
    <p>A home belongs to a normal user: add one in <a href={machineLink(host.name, 'users')}>Users</a> first.</p>
  </div>
{:else}
  <ManagedModuleNotice {host} />
  <div class="toolbar">
    <div class="seg" role="radiogroup" aria-label="User">
      {#each host.users as x (x.name)}
        <button role="radio" aria-checked={user === x.name} class:on={user === x.name} onclick={() => (user = x.name)}>{x.name}</button>
      {/each}
    </div>
    <span class="spacer"></span>
    {#if hasHome && full}
      <button onclick={() => openOrCreate(nixFile, homeNixTemplate(user, host.name))} title="{user}'s own home-manager module, {nixFile}"><Icon name="code" /> {nixExists ? 'Edit' : 'Create'} Nix for {user}</button>
      <button onclick={() => (drawer = { kind: 'options', query: '', selected: '' })}><Icon name="sliders" /> Options</button>
    {/if}
  </div>

  {#if !hasHome}
    <section class="card" use:frame={{ title: `${user} has no home here` }}>
      <p>home-manager can manage {user}'s packages, programs and dotfiles on {host.name}, from {host.managedFile ?? 'managed.json'} and the files next to it.</p>
      {#if full}
        <button class="primary" disabled={busy === 'on'} onclick={turnOn}><Icon name="plus" /> Turn on home-manager for {user}</button>
        <p class="muted small">It starts at release {homeStateVersion(host)} (<span class="mono">home.stateVersion</span>).</p>
      {:else if !host.managedFile}
        <p class="muted small">{host.name} is Code only: <a href={machineLink(host.name)}>turn on editing</a> first.</p>
      {/if}
    </section>
  {:else}
    <p class="src">
      {#if mine}<span class="chip ok" title="{user} is in managed.json's home">✎ Managed here</span>{/if}
      {#if u?.home && !mine}<span class="chip">λ Written by hand</span>{/if}
      {#if !u?.home}<span class="badge busy">{fleet.computing ? 'Evaluating…' : 'Not in the evaluation yet'}</span>{/if}
      {#if mine && full}<button class="link small" disabled={busy === 'on'} onclick={turnOff}>Stop managing here</button>{/if}
    </p>

    <section class="block" aria-labelledby="hp-title">
      <div class="head">
        <h3 id="hp-title">Packages <span class="num">{rows.filter((r) => !r.removed).length}</span></h3>
        <span class="spacer"></span>
        {#if canShare}
          <div class="seg" role="radiogroup" aria-label="Where changes to {user}'s packages go">
            <button role="radio" aria-checked={!everywhere} class:on={!everywhere} onclick={() => (everywhere = false)} title="Changes go to {host.managedFile}">{host.name}</button>
            <button role="radio" aria-checked={everywhere} class:on={everywhere} disabled={!shared} onclick={() => (everywhere = true)} title="Changes go to {SHARED_FILE}">Every machine</button>
          </div>
        {/if}
        {#if full}<button class="small primary" onclick={() => (drawer = { kind: 'add' })}><Icon name="plus" /> Add packages</button>{/if}
      </div>
      {#if target === SHARED}
        <p class="muted small scope">Changes go to <a class="mono" href={codeLink(SHARED_FILE)}>{SHARED_FILE}</a>, for {user}'s home on every machine whose managed.json has it; a machine's own wins where both set something.</p>
      {/if}
      <table class="data">
        <thead><tr><th>Package</th><th>From</th><th><span class="sr-only">Actions</span></th></tr></thead>
        <tbody>
          {#each rows as p (p.name)}
            <tr class:pending={p.pending} class:gone={p.removed}>
              <td class="mono name">{p.name}</td>
              <td>
                {#if p.pending}<span class="badge busy">Adding{fleet.computing ? '…' : ''}</span>{/if}
                {#if p.removed}<span class="chip warn">⌫ Removed {p.removedShared ? 'on every machine' : 'here'}</span>{:else if p.attr}<span class="chip ok">✎ Set here</span>{:else if p.sharedAttr}<span
                    class="chip ok"
                    title="Set for every machine, in {SHARED_FILE}">⇉ Every machine</span
                  >{:else}<span class="def">· home-manager configuration</span>{/if}
              </td>
              <td class="act">
                {#if full && p.removed}
                  <button class="small" disabled={busy === p.name || !edited?.removedPackages.includes(p.name)} onclick={() => run(p.name, () => restorePackages(target, [p.name], user))}><Icon name="undo" /> Restore</button>
                {:else if full}
                  <button class="icon" aria-label="Remove {p.name} from {user}'s home" title="Remove (you can undo)" disabled={busy === p.name} onclick={() => remove(p)}><Icon name="trash" /></button>
                {/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="3" class="muted none">No packages in {user}'s home.</td></tr>
          {/each}
        </tbody>
      </table>
    </section>

    <section class="block" aria-labelledby="ho-title">
      <div class="head">
        <h3 id="ho-title">Programs and options</h3>
      </div>
      {#if u?.home?.programs.length}
        <div class="chips">
          {#each u.home.programs as p (p)}<button class="chip g-programs" onclick={() => (drawer = { kind: 'options', query: `programs.${p}.`, selected: '' })}>{p}</button>{/each}
        </div>
      {/if}
      {#if setHere.length}
        <ul class="set">
          {#each setHere as o (o.name)}
            <li>
              <button class="bare o" onclick={() => (drawer = { kind: 'options', query: '', selected: o.name })}><span class="mono">{pathName(o.path.slice(3))}</span> <code class="val">{showValue(o.value)}</code></button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="muted small">Nothing set here for {user} yet: <b>Options</b> searches every home-manager option.</p>
      {/if}
    </section>

    <section class="block" aria-labelledby="hf-title">
      <div class="head">
        <h3 id="hf-title">Files in ~</h3>
      </div>
      <p class="muted small">Kept in <span class="mono">{filesDir}</span> and linked into {user}'s home (<span class="mono">home.file</span>; <span class="mono">.config/…</span> is <span class="mono">xdg.configFile</span>). They are copied to the Nix store, which every user of the machine can read: never keep a token or password here (<span class="mono">.netrc</span>, <span class="mono">.config/gh/hosts.yml</span>…); keep it in <a href={link('settings')}>Secrets</a>.</p>
      <ul class="files">
        {#each dotfiles as f (f)}
          <li>
            <a class="mono" href={codeLink(filesDir + f)}><Icon name="file" /> ~/{f}</a>
            {#if full}<button class="icon" aria-label="Delete ~/{f}" disabled={busy === 'file:' + f} onclick={() => deleteFile(f)}><Icon name="trash" /></button>{/if}
          </li>
        {/each}
        {#each linked as f (f)}
          <li class="muted"><span class="mono">~/{f}</span> <span class="small">· from home-manager modules</span></li>
        {/each}
      </ul>
      {#if full}
        <form class="add" onsubmit={addFile}>
          <label class="field"><span class="label">New file, relative to ~</span><input class="mono" bind:value={newFile} placeholder=".config/starship.toml" spellcheck="false" aria-invalid={!!fileProblem} /></label>
          <button type="submit" disabled={!newFile.trim() || !!fileProblem}><Icon name="file-plus" /> Create and edit</button>
        </form>
        {#if fileProblem}<p class="problem small">{fileProblem}</p>{/if}
      {/if}
    </section>
  {/if}
{/if}

{#if drawer?.kind === 'add'}
  <Drawer title="Add packages to {user}'s home on {targetName(target)}" onclose={() => (drawer = null)} width={640}>
    <PackagePicker label="Add" onpick={(hit) => addHomePackages(target, user, [hit.attr])} />
  </Drawer>
{:else if drawer?.kind === 'options'}
  <Drawer title="{user}'s home-manager options on {host.name}" onclose={() => (drawer = null)} width={760}>
    <OptionBrowser host={host.name} scope={{ prefix, names: [user] }} bind:query={drawer.query} bind:selected={drawer.selected} stacked />
  </Drawer>
{/if}

<style>
  .toolbar {
    margin-bottom: var(--lh);
  }
  .scope {
    margin: 0 0 var(--lh);
  }
  .src {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
    margin: 0 0 var(--lh);
  }
  .block {
    margin-bottom: calc(var(--lh) * 2);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 2ch;
  }
  h3 {
    margin: 0;
    font-size: inherit;
  }
  .name {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .def {
    color: var(--fg-faint);
  }
  .act {
    width: 14ch;
    text-align: right;
  }
  tr.pending td {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
  }
  tr.gone .name {
    color: var(--fg-faint);
    text-decoration: line-through;
  }
  .none {
    padding: var(--lh) 1ch;
  }
  .set,
  .files {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .files li {
    display: flex;
    gap: 2ch;
    align-items: baseline;
  }
  .o {
    text-align: left;
  }
  .val {
    color: var(--fg-muted);
  }
  .add {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--lh) 2ch;
    margin-top: var(--lh);
  }
  .field {
    display: grid;
    width: 40ch;
  }
  .problem {
    color: var(--red);
  }
  .problem::before {
    content: '✖ ';
  }
  .snippet {
    margin: var(--lh) 0;
  }
  .row {
    display: flex;
    gap: 2ch;
  }
</style>
