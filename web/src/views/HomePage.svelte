<script lang="ts">
  import { api } from '../lib/api'
  import { bigText, frame } from '../lib/ascii'
  import { addPackages, errorFile, evaluateAgain, excludePackages, fleet, loadManaged, openOrCreate, removePackages, restorePackages } from '../lib/fleet.svelte'
  import { pathName } from '../lib/options'
  import { HOME, homeDir, homePathProblem, standaloneNixTemplate, standaloneRows, type PackageRow } from '../lib/packages'
  import { codeLink } from '../lib/router.svelte'
  import { app, refresh } from '../lib/state.svelte'
  import { confirmBox, report } from '../lib/ui.svelte'
  import { managedOptions, showValue } from '../lib/values'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import OptionBrowser from '../components/OptionBrowser.svelte'
  import PackagePicker from '../components/PackagePicker.svelte'
  import TurnOnEditing from '../components/machine/TurnOnEditing.svelte'

  /** A standalone home-manager configuration (homeConfigurations.<name>): its packages, options, files and Nix, edited as home:<name>. */
  let { name }: { name: string } = $props()

  const target = $derived(HOME + name)
  const home = $derived(fleet.model?.homes.find((h) => h.name === name))
  const managed = $derived(fleet.managed[target])
  const editable = $derived(!!home?.managedFile && !!managed)
  const pending = $derived(fleet.pending.includes(target) && fleet.computing)
  $effect(() => {
    if (home?.managedFile && !fleet.managed[target]) loadManaged(target)
  })

  const rows = $derived(home ? standaloneRows(home, managed) : [])
  const dir = $derived(home ? homeDir(home) : `homes/${name}`)
  const filesDir = $derived(`${dir}/files/`)
  const nixFile = $derived(`${dir}/custom.nix`)
  const nixExists = $derived(app.entries.some((e) => e.path === nixFile))
  const dotfiles = $derived(app.entries.filter((e) => !e.dir && e.path.startsWith(filesDir)).map((e) => e.path.slice(filesDir.length)))
  /** What home-manager links into ~ from modules and programs, not from files/. */
  const linked = $derived((home?.files ?? []).filter((f) => !dotfiles.includes(f)))
  const setHere = $derived(managedOptions(managed?.settings))

  let showTurnOn = $state(false)
  let busy = $state('')
  let drawer = $state<null | { kind: 'add' } | { kind: 'options'; query: string; selected: string }>(null)
  let newFile = $state('')
  const fileProblem = $derived(homePathProblem(newFile, dotfiles))

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }
  async function remove(p: PackageRow) {
    if (p.attr) return run(p.name, () => removePackages(target, [p.attr!]))
    const ok = await confirmBox({
      title: `Remove ${p.name} from ${name}?`,
      message: `${p.name} comes from ${name}'s home-manager configuration (a module or a program), which stays as it is: it is taken out of its packages by name, in ${home?.managedFile}. Restore puts it back.`,
      ok: 'Remove',
    })
    if (ok) await run(p.name, () => excludePackages(target, [p.name]))
  }
  async function addFile(e: SubmitEvent) {
    e.preventDefault()
    const f = newFile.trim()
    if (!f || fileProblem) return
    if (await openOrCreate(filesDir + f, '')) newFile = ''
  }
  async function deleteFile(f: string) {
    const ok = await confirmBox({ title: `Delete ~/${f}?`, message: `${filesDir}${f} is deleted from the repository, so home-manager no longer links it into the home.`, ok: 'Delete', danger: true })
    if (!ok) return
    await run('file:' + f, async () => {
      await api.remove(filesDir + f).catch((e) => report(e, `Could not delete ${f}`))
      await refresh()
    })
  }
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate>
      {#if !home}
        <div class="empty-state">
          <h2>No home configuration called “{name}”</h2>
          <p>It is not in <span class="mono">homeConfigurations</span> of the working copy. It may have been renamed or removed.</p>
          <div class="row"><a class="btn" href="#/machines">All machines</a></div>
        </div>
      {:else}
        <header class="mh">
          <div class="page-head">
            <div class="title">
              <h1 class="big" class:err={!!home.error} aria-label={home.name} use:bigText={home.name}></h1>
              <p class="state-row">
                {#if !home.error && !pending}<span class="dot ok" aria-hidden="true"></span>{/if}
                <span class="badge">home-manager</span>
                {#if home.error}
                  <span class="badge err">Does not evaluate</span>
                {:else if pending}
                  <span class="badge busy">Re-evaluating</span>
                {:else if home.managedFile}
                  <span class="badge ok here" title="Packages and options can be changed by clicking; they are kept in {home.managedFile}">Editable here</span>
                {:else}
                  <span class="badge code" title="{home.name} does not load a managed.json, so it is changed in its Nix files">Code only</span>
                  {#if !showTurnOn}<button class="link small" onclick={() => (showTurnOn = true)}>Turn on editing</button>{/if}
                {/if}
              </p>
            </div>
            <span class="spacer"></span>
            {#if editable}
              <button onclick={() => openOrCreate(nixFile, standaloneNixTemplate(name))} title="{name}'s own home-manager module, {nixFile}"><Icon name="code" /> {nixExists ? 'Edit' : 'Create'} Nix</button>
              <button onclick={() => (drawer = { kind: 'options', query: '', selected: '' })}><Icon name="sliders" /> Options</button>
            {/if}
          </div>
          <p class="muted">A standalone home-manager configuration (<span class="mono">homeConfigurations</span>), for a machine that is not NixOS or not managed here.</p>
        </header>

        {#if home.error}
          {@const file = errorFile(home.error)}
          <section class="card errcard" role="alert" data-tone="err" use:frame={{ title: 'Evaluation failed' }}>
            <h2 class="sr-only">Evaluation failed</h2>
            <pre>{home.error}</pre>
            <div class="row">
              {#if file}<a class="btn primary" href={codeLink(file)}><Icon name="code" /> Open {file}</a>{/if}
              <button class="btn" onclick={evaluateAgain} disabled={fleet.computing}><Icon name="refresh" /> Evaluate again</button>
            </div>
          </section>
        {:else}
          {#if !home.managedFile && showTurnOn}<TurnOnEditing host={name} home onclose={() => (showTurnOn = false)} />{/if}

          <section class="block" aria-labelledby="sp-title">
            <div class="head">
              <h3 id="sp-title">Packages <span class="num">{rows.filter((r) => !r.removed).length}</span></h3>
              <span class="spacer"></span>
              {#if editable}<button class="small primary" onclick={() => (drawer = { kind: 'add' })}><Icon name="plus" /> Add packages</button>{/if}
            </div>
            <table class="data">
              <thead><tr><th>Package</th><th>From</th><th><span class="sr-only">Actions</span></th></tr></thead>
              <tbody>
                {#each rows as p (p.name)}
                  <tr class:pending={p.pending} class:gone={p.removed}>
                    <td class="mono name">{p.name}</td>
                    <td>
                      {#if p.pending}<span class="badge busy">Adding{fleet.computing ? '…' : ''}</span>{/if}
                      {#if p.removed}<span class="chip warn">⌫ Removed here</span>{:else if p.attr}<span class="chip ok">✎ Set here</span>{:else}<span class="def">· home-manager configuration</span>{/if}
                    </td>
                    <td class="act">
                      {#if editable && p.removed}
                        <button class="small" disabled={busy === p.name || !managed?.removedPackages?.includes(p.name)} onclick={() => run(p.name, () => restorePackages(target, [p.name]))}><Icon name="undo" /> Restore</button>
                      {:else if editable}
                        <button class="icon" aria-label="Remove {p.name} from {name}" title="Remove (you can undo)" disabled={busy === p.name} onclick={() => remove(p)}><Icon name="trash" /></button>
                      {/if}
                    </td>
                  </tr>
                {:else}
                  <tr><td colspan="3" class="muted none">No packages in {name}.</td></tr>
                {/each}
              </tbody>
            </table>
          </section>

          <section class="block" aria-labelledby="so-title">
            <div class="head"><h3 id="so-title">Programs and options</h3></div>
            {#if home.programs.length}
              <div class="chips">
                {#each home.programs as p (p)}
                  {#if editable}<button class="chip g-programs" onclick={() => (drawer = { kind: 'options', query: `programs.${p}.`, selected: '' })}>{p}</button>{:else}<span class="chip g-programs">{p}</span>{/if}
                {/each}
              </div>
            {/if}
            {#if setHere.length}
              <ul class="set">
                {#each setHere as o (o.name)}
                  <li>
                    <button class="bare o" onclick={() => (drawer = { kind: 'options', query: '', selected: o.name })}><span class="mono">{pathName(o.path)}</span> <code class="val">{showValue(o.value)}</code></button>
                  </li>
                {/each}
              </ul>
            {:else if editable}
              <p class="muted small">Nothing set here yet: <b>Options</b> searches every home-manager option.</p>
            {/if}
          </section>

          <section class="block" aria-labelledby="sf-title">
            <div class="head"><h3 id="sf-title">Files in ~</h3></div>
            {#if editable}
              <p class="muted small">Kept in <span class="mono">{filesDir}</span> and linked into the home (<span class="mono">home.file</span>). They are copied to the Nix store, which every user of the machine can read: never keep a token or password here.</p>
            {/if}
            <ul class="files">
              {#each dotfiles as f (f)}
                <li>
                  <a class="mono" href={codeLink(filesDir + f)}><Icon name="file" /> ~/{f}</a>
                  {#if editable}<button class="icon" aria-label="Delete ~/{f}" disabled={busy === 'file:' + f} onclick={() => deleteFile(f)}><Icon name="trash" /></button>{/if}
                </li>
              {/each}
              {#each linked as f (f)}
                <li class="muted"><span class="mono">~/{f}</span> <span class="small">· from home-manager modules</span></li>
              {/each}
            </ul>
            {#if editable}
              <form class="add" onsubmit={addFile}>
                <label class="field"><span class="label">New file, relative to ~</span><input class="mono" bind:value={newFile} placeholder=".config/starship.toml" spellcheck="false" aria-invalid={!!fileProblem} /></label>
                <button type="submit" disabled={!newFile.trim() || !!fileProblem}><Icon name="file-plus" /> Create and edit</button>
              </form>
              {#if fileProblem}<p class="problem small">{fileProblem}</p>{/if}
            {/if}
          </section>
        {/if}
      {/if}
    </ModelGate>
  </div>
</div>

{#if drawer?.kind === 'add'}
  <Drawer title="Add packages to {name}" onclose={() => (drawer = null)} width={640}>
    <PackagePicker label="Add" onpick={(hit) => addPackages(target, [hit.attr])} />
  </Drawer>
{:else if drawer?.kind === 'options'}
  <Drawer title="{name}'s home-manager options" onclose={() => (drawer = null)} width={760}>
    <OptionBrowser host={target} bind:query={drawer.query} bind:selected={drawer.selected} stacked />
  </Drawer>
{/if}

<style>
  .mh {
    margin-bottom: var(--lh);
  }
  .mh .page-head {
    flex-wrap: nowrap;
  }
  .title {
    min-width: 0;
  }
  .state-row {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .state-row .dot {
    margin-right: -1ch;
  }
  .badge.here::before {
    content: '✎ ';
  }
  .badge.code {
    padding: 0;
    color: var(--fg-muted);
    background: none;
  }
  .badge.code::before {
    content: 'λ ';
  }
  .errcard {
    display: grid;
    gap: var(--lh);
  }
  .errcard pre {
    padding: 0 1ch;
    overflow: auto;
    white-space: pre-wrap;
    background: var(--bg-input);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
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
</style>
