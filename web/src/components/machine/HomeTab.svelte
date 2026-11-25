<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type Host } from '../../lib/api'
  import { frame } from '../../lib/ascii'
  import { addHomePackages, copyToMachines, excludePackages, fleet, linkHomeSecret, loadManaged, managedValue, openOrCreate, removeHomePackages, restorePackages, setHome, targetName, unlinkHomeSecret } from '../../lib/fleet.svelte'
  import { pathName } from '../../lib/options'
  import { SHARED, SHARED_FILE, appliesShared, copyTargets, dotfilesDir, homeNixFile, homeNixTemplate, homePathProblem, homeRows, homeStateVersion, linksSecrets, secretTemplatesDir, type PackageRow } from '../../lib/packages'
  import { codeLink, link, machineLink } from '../../lib/router.svelte'
  import { app, refresh } from '../../lib/state.svelte'
  import { confirmBox, quietly, report } from '../../lib/ui.svelte'
  import { managedOptions, showValue } from '../../lib/values'
  import CopyTo from '../CopyTo.svelte'
  import Drawer from '../Drawer.svelte'
  import Icon from '../Icon.svelte'
  import OptionBrowser from '../OptionBrowser.svelte'
  import PackagePicker from '../PackagePicker.svelte'
  import AppsSection from './AppsSection.svelte'
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
  const filesIn = (dir: string) => app.entries.filter((e) => !e.dir && e.path.startsWith(dir)).map((e) => e.path.slice(dir.length))
  const dotfiles = $derived(filesIn(filesDir))
  const templatesDir = $derived(secretTemplatesDir(host, user))
  const templates = $derived(filesIn(templatesDir))
  /** Secrets linked as whole files: the machine's own, then those set for every machine that it does not set itself. */
  const links = $derived([
    ...Object.entries(mine?.secrets ?? {}).map(([path, secret]) => ({ path, secret, where: host.name })),
    ...Object.entries(shared?.home?.[user]?.secrets ?? {})
      .filter(([p]) => !mine?.secrets?.[p])
      .map(([path, secret]) => ({ path, secret, where: SHARED })),
  ])
  const secretNames = $derived((host.secrets ?? []).map((s) => s.name))
  const canLink = $derived(full && linksSecrets(host) && !!host.secrets)
  /** Every path in ~ set here: a second one for the same path fails the check. */
  const taken = $derived([...dotfiles, ...templates, ...links.map((l) => l.path)])
  /** What home-manager links into ~ from modules and programs, not from the files and secrets here. */
  const linked = $derived((u?.home?.files ?? []).filter((f) => !taken.includes(f)))
  const nixFile = $derived(homeNixFile(host, user))
  const nixExists = $derived(app.entries.some((e) => e.path === nixFile))

  const prefix = 'home-manager.users.<name>.'
  const setHere = $derived(managedOptions(managed?.settings).filter((o) => o.path[0] === 'home-manager' && o.path[1] === 'users' && o.path[2] === user && o.path.length > 3))

  let busy = $state('')
  let drawer = $state<null | { kind: 'add' } | { kind: 'options'; query: string; selected: string }>(null)
  let newFile = $state('')
  const fileProblem = $derived(homePathProblem(newFile, taken))
  let secretFile = $state('')
  /** The secret to link, or '' for a file with secrets in it. */
  let secretFrom = $state('')
  const secretProblem = $derived(homePathProblem(secretFile, taken))

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
        (t === SHARED
          ? `in ${SHARED_FILE}, on every machine whose managed.json has ${user}'s home.${p.attr ? ` ${host.name} adds it itself, so it stays there.` : ''}`
          : `in ${host.managedFile}.`) +
        ' Restore puts it back.',
      ok: 'Remove',
    })
    if (ok) await run(p.name, () => excludePackages(t, [p.name], user))
  }

  // Copy to…: home.stateVersion stays each machine's own, so where the user has no home
  // yet it is turned on afterwards with that machine's release, as Turn on does.
  // Secrets in ~ go only where managed.nix applies them.
  const copiesSecrets = $derived(Object.keys(mine?.secrets ?? {}).length > 0 || templates.length > 0)
  const copyHosts = $derived(
    copyTargets(fleet.model?.hosts ?? [], host.name, user)
      .filter((h) => !copiesSecrets || linksSecrets(h))
      .map((h) => ({
      name: h.name,
      note: h.users.find((x) => x.name === user)?.home || fleet.managed[h.name]?.home?.[user] ? undefined : `turns home-manager on (${homeStateVersion(h)})`,
    })),
  )
  async function copyHome(to: string[]) {
    await Promise.all(to.filter((t) => !fleet.managed[t]).map(loadManaged))
    const hasNone = (t: string) => {
      const th = fleet.model?.hosts.find((h) => h.name === t)
      return !!th && !th.users.find((x) => x.name === user)?.home && !fleet.managed[t]?.home?.[user] && managedValue(t, ['home-manager', 'users', user, 'home', 'stateVersion']) === undefined
    }
    const turnOn = to.filter(hasNone)
    const done = await copyToMachines(host.name, { home: user, to }, `${user}'s home`)
    await quietly(async () => {
      for (const t of done.filter((t) => turnOn.includes(t))) await setHome(t, user, true, homeStateVersion(fleet.model!.hosts.find((h) => h.name === t)!))
    })
  }

  async function addFile(e: SubmitEvent) {
    e.preventDefault()
    const f = newFile.trim()
    if (!f || fileProblem) return
    // The user must be in managed.json's home for the files next to it to apply.
    if (!mine && !(await setHome(host.name, user, true))) return
    if (await openOrCreate(filesDir + f, '')) newFile = ''
  }
  async function deleteFile(dir: string, f: string) {
    const ok = await confirmBox({ title: `Delete ~/${f}?`, message: `${dir}${f} is deleted from the repository, so home-manager no longer links it into ${user}'s home.`, ok: 'Delete', danger: true })
    if (!ok) return
    await run('file:' + dir + f, async () => {
      await api.remove(dir + f).catch((e) => report(e, `Could not delete ${f}`))
      await refresh()
    })
  }

  async function unlink(l: { path: string; secret: string; where: string }) {
    if (l.where === SHARED) {
      const ok = await confirmBox({
        title: `Unlink ~/${l.path} on every machine?`,
        message: `The link is in ${SHARED_FILE}, so ${user}'s home loses ~/${l.path} on every machine whose managed.json has it, not only on ${host.name}. The secret ${l.secret} stays. Undo puts the link back.`,
        ok: 'Unlink',
      })
      if (!ok) return
    }
    await run('link:' + l.path, () => unlinkHomeSecret(l.where, user, l.path, l.secret))
  }

  async function addSecret(e: SubmitEvent) {
    e.preventDefault()
    const f = secretFile.trim()
    if (!f || secretProblem) return
    if (secretFrom) {
      if (await linkHomeSecret(host.name, user, f, secretFrom)) secretFile = ''
      return
    }
    if (!mine && !(await setHome(host.name, user, true))) return
    if (await openOrCreate(templatesDir + f, '')) secretFile = ''
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
    <div class="src">
      {#if mine}<span class="chip ok" title="{user} is in managed.json's home">✎ Managed here</span>{/if}
      {#if u?.home && !mine}<span class="chip">λ Written by hand</span>{/if}
      {#if !u?.home}<span class="badge busy">{fleet.computing ? 'Evaluating…' : 'Not in the evaluation yet'}</span>{/if}
      {#if mine && full}
        <button class="link small" disabled={busy === 'on'} onclick={turnOff}>Stop managing here</button>
        <CopyTo
          title="Give {user} on these machines the home {user} has on {host.name}:"
          note="Each machine gets {user}'s packages and removed packages, joined with its own; the options set here, except home.stateVersion; custom.nix, and the files and secrets in ~ (a machine without those secrets fails its check until it gets them). A machine that already has another value for any of them is left as it is. Home-manager configuration written by hand elsewhere is not copied."
          targets={copyHosts}
          copy={copyHome}
        />
      {/if}
    </div>

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
      <p class="muted small">Kept in <span class="mono">{filesDir}</span> and linked into {user}'s home (<span class="mono">home.file</span>; <span class="mono">.config/…</span> is <span class="mono">xdg.configFile</span>). They are copied to the Nix store, which every user of the machine can read: never keep a token or password here (<span class="mono">.netrc</span>, <span class="mono">.config/gh/hosts.yml</span>…): see Secrets in ~ below.</p>
      <ul class="files">
        {#each dotfiles as f (f)}
          <li>
            <a class="mono" href={codeLink(filesDir + f)}><Icon name="file" /> ~/{f}</a>
            {#if full}<button class="icon" aria-label="Delete ~/{f}" disabled={busy === 'file:' + filesDir + f} onclick={() => deleteFile(filesDir, f)}><Icon name="trash" /></button>{/if}
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

    <section class="block" aria-labelledby="hs-title">
      <div class="head">
        <h3 id="hs-title">Secrets in ~</h3>
      </div>
      <p class="muted small">
        A secret of {host.name} as a whole file, which becomes {user}'s, or a file in <span class="mono">{templatesDir}</span> where each
        <span class="mono">@secret:&lt;name&gt;@</span> is that secret and the rest is plain text. sops-nix writes them on the machine, and {user}'s home links to them in
        <span class="mono">/run/secrets</span>: no value reaches the Nix store.
      </p>
      <ul class="files">
        {#each links as l (l.path)}
          <li>
            <span class="mono">~/{l.path}</span>
            <span class="small">= the secret <span class="mono">{l.secret}</span></span>
            {#if l.where === SHARED}<span class="chip ok" title="Set for every machine, in {SHARED_FILE}">⇉ Every machine</span>{/if}
            {#if host.secrets && !secretNames.includes(l.secret)}<span class="chip warn" title="Its check fails until {host.name} gets it, in Settings → Secrets">✖ Not one of {host.name}'s secrets</span>{/if}
            {#if full}<button class="icon" aria-label="Unlink ~/{l.path}" title="Unlink (the secret stays)" disabled={busy === 'link:' + l.path} onclick={() => unlink(l)}
                ><Icon name="trash" /></button
              >{/if}
          </li>
        {/each}
        {#each templates as f (f)}
          <li>
            <a class="mono" href={codeLink(templatesDir + f)}><Icon name="file" /> ~/{f}</a>
            <span class="small muted">· text with secrets</span>
            {#if full}<button class="icon" aria-label="Delete ~/{f}" disabled={busy === 'file:' + templatesDir + f} onclick={() => deleteFile(templatesDir, f)}><Icon name="trash" /></button>{/if}
          </li>
        {/each}
      </ul>
      {#if canLink}
        <form class="add" onsubmit={addSecret}>
          <label class="field"><span class="label">File, relative to ~</span><input class="mono" bind:value={secretFile} placeholder=".netrc" spellcheck="false" aria-invalid={!!secretProblem} /></label>
          <label class="field"
            ><span class="label">Content</span>
            <select bind:value={secretFrom}>
              <option value="">Text with secrets, edited in Code</option>
              {#each secretNames as n (n)}<option value={n}>The secret {n}</option>{/each}
            </select>
          </label>
          <button type="submit" disabled={!secretFile.trim() || !!secretProblem}><Icon name="key" /> {secretFrom ? 'Link' : 'Create and edit'}</button>
        </form>
        {#if secretProblem}<p class="problem small">{secretProblem}</p>{/if}
        <p class="muted small">{host.name} gets {secretNames.length ? secretNames.join(', ') : 'no secrets yet'}. Add more in <a href={link('settings')}>Settings → Secrets</a>.</p>
      {:else if full && !host.secrets}
        <p class="muted small">{host.name} has no sops-nix: secrets need <span class="mono">modules/secrets.nix</span>, as <a href={link('settings')}>Settings → Secrets</a> explains.</p>
      {:else if full && !linksSecrets(host)}
        <p class="muted small">This needs a newer <span class="mono">modules/managed.nix</span>: <b>Update managed.nix</b> above.</p>
      {/if}
    </section>

    <AppsSection {host} {user} {managed} packages={[...host.packages.map((p) => p.name), ...(u?.home?.packages ?? [])]} {taken} editable={full} />
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
