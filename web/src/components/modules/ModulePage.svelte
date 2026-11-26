<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { api, type ModuleOption, type ModuleOptionType } from '../../lib/api'
  import { bigText, frame } from '../../lib/ascii'
  import {
    addPackages,
    deleteModule,
    editModule,
    fleet,
    healthy,
    hostByName,
    loadManaged,
    managedValue,
    openOrCreate,
    removePackages,
    setOption,
    unsetOption,
  } from '../../lib/fleet.svelte'
  import { MODULE, moduleFile, moduleNixFile, moduleNixTemplate, moduleOn, modulePath, moduleSource, moduleTypes, offValue, optionNameOk, optionRef, parseValue, settingValue, usesModules, valueText } from '../../lib/modules'
  import { optionSegments, pathName } from '../../lib/options'
  import { SHARED } from '../../lib/packages'
  import { codeLink, go, link, machineLink } from '../../lib/router.svelte'
  import { managedOptions, showValue } from '../../lib/values'
  import Drawer from '../Drawer.svelte'
  import Icon from '../Icon.svelte'
  import ModelGate from '../ModelGate.svelte'
  import PackagePicker from '../PackagePicker.svelte'
  import Switch from '../Switch.svelte'

  let { name }: { name: string } = $props()
  const target = $derived(MODULE + name)
  const m = $derived(fleet.managed[target])
  const listed = $derived(fleet.modules?.find((x) => x.name === name))
  const options = $derived(Object.entries(m?.options ?? {}).sort(([a], [b]) => a.localeCompare(b)))
  const settings = $derived(managedOptions(m?.settings))
  const hosts = $derived(healthy())

  let loadError = $state(false)
  onMount(() => {
    loadManaged(target).then(() => (loadError = !fleet.managed[target]))
    loadManaged(SHARED)
  })
  // Each machine's managed.json, for its switch: also those the model lists only after this page opened.
  $effect(() => {
    for (const h of hosts) if (h.managedFile && !fleet.managed[h.name]) untrack(() => loadManaged(h.name))
  })

  let busy = $state('')
  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    await fn()
    busy = ''
  }

  // ---- description ----
  let description = $state<string | null>(null)
  const saveDescription = () =>
    run('description', async () => {
      if (await editModule(name, { description: description ?? '' }, `Changed the description of ${name}`)) description = null
    })

  // ---- machines ----
  const enablePath = $derived(modulePath(name, 'enable'))
  function toggle(host: string, on: boolean) {
    const h = hostByName(host)
    const hm = h?.modules?.find((x) => x.name === name)
    return run('m:' + host, async () => {
      if (on) await setOption(host, enablePath, true, `Turned ${name} on for ${host}`)
      else if (offValue(hm, name, h?.managedFile ?? null) === false) await setOption(host, enablePath, false, `Turned ${name} off for ${host}`)
      else await unsetOption(host, enablePath, `Turned ${name} off for ${host}`)
    })
  }
  const sharedOn = $derived(managedValue(SHARED, enablePath) === true)
  const toggleShared = (on: boolean) =>
    run('m:*', () => (on ? setOption(SHARED, enablePath, true, `Turned ${name} on for every machine`) : unsetOption(SHARED, enablePath, `${name} is no longer on for every machine`)))
  const sourceText = (files: string[], managedFile: string | null) => {
    const s = moduleSource(files, name, managedFile)
    return s.kind === 'here' ? 'Set here' : s.kind === 'shared' ? 'Every machine' : s.kind === 'code' ? s.file : ''
  }

  // ---- options ----
  type Draft = { name: string; type: ModuleOptionType; required: boolean; text: string; description: string; editing: boolean }
  let draft = $state<Draft | null>(null)
  const blank = (): Draft => ({ name: '', type: 'str', required: false, text: '', description: '', editing: false })
  function edit(o: string, d: ModuleOption) {
    draft = { name: o, type: d.type, required: d.default === undefined, text: valueText(d.type, d.default), description: d.description ?? '', editing: true }
  }
  const draftProblem = $derived.by(() => {
    if (!draft) return ''
    if (!optionNameOk(draft.name)) return 'A name: letters, digits, _ and -, starting with a letter or _ (not enable)'
    if (!draft.editing && m?.options?.[draft.name]) return `There is already an option ${draft.name}`
    if (draft.required) return ''
    const p = parseValue(draft.type, draft.text)
    return 'problem' in p ? `The default must be ${p.problem}` : ''
  })
  function saveOption(e: SubmitEvent) {
    e.preventDefault()
    const d = draft
    if (!d || draftProblem) return
    const decl: ModuleOption = { type: d.type, description: d.description.trim() || undefined }
    if (!d.required) decl.default = (parseValue(d.type, d.text) as { value: unknown }).value
    run('option', async () => {
      if (await editModule(name, { options: { [d.name]: decl } }, `${d.editing ? 'Changed' : 'Added'} the option ${d.name} of ${name}`)) draft = null
    })
  }
  const removeOption = (o: string) => run('o:' + o, () => editModule(name, { options: { [o]: null } }, `Removed the option ${o} of ${name}, and what machines set for it`))
  const usedBy = (o: string) => settings.filter((s) => JSON.stringify(s.value).includes(optionRef(o))).map((s) => s.name)

  // ---- packages ----
  let picking = $state(false)

  // ---- settings ----
  let settingName = $state('')
  let settingText = $state('')
  let settingValueProblem = $state('')
  const settingPath = $derived.by(() => {
    const segs = optionSegments(settingName.trim())
    return settingName.trim() && segs.every((s) => s.kind === 'attr' && s.text) ? segs.map((s) => s.text) : null
  })
  const settingProblem = $derived(!settingName.trim() ? '' : !settingPath ? 'An option name such as services.jellyfin.enable' : '')
  function addSetting(e: SubmitEvent) {
    e.preventDefault()
    const v = settingValue(settingText)
    if (!settingPath) return
    if ('problem' in v) return void (settingValueProblem = v.problem)
    settingValueProblem = ''
    const path = settingPath
    run('setting', async () => {
      if (await setOption(target, path, v.value, `${name} now sets ${pathName(path)}`)) settingName = settingText = ''
    })
  }
  // Option names from a machine that applies modules, as suggestions.
  let suggestions = $state<string[]>([])
  const docsHost = $derived(hosts.find(usesModules)?.name ?? hosts[0]?.name)
  let timer: ReturnType<typeof setTimeout> | undefined
  function suggest() {
    clearTimeout(timer)
    const q = settingName.trim()
    if (!docsHost || q.length < 3) return void (suggestions = [])
    timer = setTimeout(async () => {
      const r = await api.searchOptions(docsHost, q, { limit: 30 }).catch(() => null)
      if (r?.state === 'ready' && settingName.trim() === q) suggestions = (r.results ?? []).filter((o) => !o.readOnly).map((o) => o.name)
    }, 250)
  }
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate title="fleet modules">
      <p class="crumbs"><a href={link('modules')}>Modules</a> /</p>
      {#if loadError && !m}
        <div class="empty-state">
          <h2>No module called “{name}”</h2>
          <p>There is no <span class="mono">{moduleFile(name)}</span>, or it cannot be read.</p>
          <div class="row"><a class="btn" href={link('modules')}>All modules</a><a class="btn" href={codeLink(moduleFile(name))}><Icon name="code" /> Open in Code</a></div>
        </div>
      {:else if m}
        <header class="page-head">
          <div class="title">
            <h1 class="big" aria-label={name} use:bigText={name}></h1>
            {#if description === null}
              <p class="sub">
                {m.description || 'No description.'}
                <button class="link small" onclick={() => (description = m.description ?? '')}>Edit</button>
              </p>
            {:else}
              <form
                class="row desc"
                onsubmit={(e) => {
                  e.preventDefault()
                  saveDescription()
                }}>
                <input bind:value={description} aria-label="Description of {name}" placeholder="What the module is for" />
                <button class="primary small" type="submit" disabled={busy === 'description'}>Save</button>
                <button class="ghost small" type="button" onclick={() => (description = null)}>Cancel</button>
              </form>
            {/if}
          </div>
          <span class="spacer"></span>
          <div class="tools">
            <button onclick={() => openOrCreate(moduleNixFile(name), moduleNixTemplate(name))} title="{moduleNixFile(name)}: NixOS settings the module adds that its managed.json cannot hold">
              <Icon name="code" /> {listed?.custom ? 'Its Nix' : 'Add Nix'}
            </button>
            <a class="btn" href={codeLink(moduleFile(name))}><Icon name="file" /> managed.json</a>
            <button class="danger" onclick={async () => (await deleteModule(name)) && go(link('modules'))}><Icon name="trash" /> Delete</button>
          </div>
        </header>

        <section class="section" aria-labelledby="mod-machines">
          <h2 id="mod-machines">Machines</h2>
          <div class="table-wrap">
            <table class="data">
              <thead><tr><th>Machine</th><th>On</th><th>Set in</th><th>Its options</th></tr></thead>
              <tbody>
                <tr>
                  <td>Every machine <span class="muted small mono">modules/managed.json</span></td>
                  <td><Switch checked={sharedOn} busy={busy === 'm:*'} disabled={!fleet.managed[SHARED]} label="{name} on every machine" onchange={toggleShared} /></td>
                  <td class="muted small">A machine can still turn it off</td>
                  <td></td>
                </tr>
                {#each hosts as h (h.name)}
                  {@const hm = h.modules?.find((x) => x.name === name)}
                  {@const setHere = managedValue(h.name, enablePath)}
                  {@const on = moduleOn(hm, setHere, name, h.managedFile)}
                  <tr>
                    <td><a href={machineLink(h.name, 'modules')}>{h.name}</a></td>
                    <td>
                      {#if usesModules(h)}
                        <Switch checked={on} busy={busy === 'm:' + h.name} disabled={!fleet.managed[h.name]} label="{name} on {h.name}" onchange={(v) => toggle(h.name, v)} />
                      {:else}
                        <span class="muted small" title={h.managedFile ? 'Its managed.nix is older: update it on the Modules page' : 'It does not load a managed.json'}>{h.managedFile ? 'Older managed.nix' : 'Code only'}</span>
                      {/if}
                    </td>
                    <td class="small">{setHere !== undefined ? 'Set here' : hm ? sourceText(hm.files, h.managedFile) : ''}</td>
                    <td class="small mono">
                      {#each Object.entries(hm?.options ?? {}) as [o, v] (o)}
                        {@const s = moduleSource(v.files, name, h.managedFile)}
                        <span class="opt" class:own={s.kind !== 'default'} title={s.kind === 'default' ? 'Its default' : sourceText(v.files, h.managedFile)}>{o} = {showValue(v.value)}</span>
                      {/each}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </section>

        <section class="section" aria-labelledby="mod-options">
          <div class="head">
            <h2 id="mod-options">Options</h2>
            <span class="spacer"></span>
            {#if !draft}<button class="small" onclick={() => (draft = blank())}><Icon name="plus" /> Add option</button>{/if}
          </div>
          <p class="muted small">
            Each is <span class="mono">fleet.modules.{name}.&lt;option&gt;</span>, which a machine sets on its Modules tab. In a setting below, <span class="mono">{optionRef('port')}</span> stands for the option
            <span class="mono">port</span>'s value on each machine, alone or inside a text.
          </p>
          {#if options.length}
            <div class="table-wrap">
              <table class="data">
                <thead><tr><th>Option</th><th>Type</th><th>Default</th><th>Description</th><th><span class="sr-only">Actions</span></th></tr></thead>
                <tbody>
                  {#each options as [o, d] (o)}
                    {@const used = usedBy(o)}
                    <tr>
                      <td class="mono">{o}</td>
                      <td class="mono">{d.type}</td>
                      <td class="mono">{#if d.default === undefined}<span class="muted">Each machine sets it</span>{:else}{showValue(d.default)}{/if}</td>
                      <td>{d.description ?? ''}</td>
                      <td class="act">
                        <button class="small ghost" onclick={() => edit(o, d)}>Edit</button>
                        <button class="small ghost" disabled={busy === 'o:' + o || used.length > 0} title={used.length ? `Used by ${used.join(', ')}` : 'Remove it, and the values machines set for it'} onclick={() => removeOption(o)}>Remove</button>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else if !draft}
            <p class="muted">No options: the module is the same on every machine that has it on.</p>
          {/if}
          {#if draft}
            <form class="card form" use:frame={{ title: draft.editing ? `Option ${draft.name}` : 'New option' }} onsubmit={saveOption}>
              <label><span>Name</span><input class="mono" bind:value={draft.name} disabled={draft.editing} placeholder="port" /></label>
              <label>
                <span>Type</span>
                <select
                  value={draft.type}
                  onchange={(e) => {
                    const t = e.currentTarget.value as ModuleOptionType
                    if (draft) (draft.type = t), (draft.text = t === 'bool' ? 'false' : '')
                  }}>{#each moduleTypes as t (t)}<option value={t}>{t}</option>{/each}</select>
              </label>
              <label class="check"><input type="checkbox" bind:checked={draft.required} /> No default: each machine that turns it on sets it</label>
              {#if !draft.required}
                <label>
                  <span>Default</span>
                  {#if draft.type === 'bool'}
                    <select bind:value={draft.text}><option value="false">false</option><option value="true">true</option></select>
                  {:else if draft.type === 'listOf str' || draft.type === 'anything'}
                    <textarea class="mono" rows="3" bind:value={draft.text} placeholder={draft.type === 'listOf str' ? 'One per line' : 'JSON: { "a": 1 }'}></textarea>
                  {:else}
                    <input class="mono" bind:value={draft.text} inputmode={draft.type === 'str' ? 'text' : 'numeric'} />
                  {/if}
                </label>
              {/if}
              <label><span>Description</span><input bind:value={draft.description} placeholder="What a machine sets it to" /></label>
              {#if draftProblem}<p class="problem small">{draftProblem}</p>{/if}
              <div class="row">
                <button class="primary" type="submit" disabled={!!draftProblem || busy === 'option'}>{draft.editing ? 'Save option' : 'Add option'}</button>
                <button class="ghost" type="button" onclick={() => (draft = null)}>Cancel</button>
              </div>
            </form>
          {/if}
        </section>

        <section class="section" aria-labelledby="mod-packages">
          <div class="head">
            <h2 id="mod-packages">Packages</h2>
            <span class="spacer"></span>
            <button class="small" onclick={() => (picking = true)}><Icon name="plus" /> Add package</button>
          </div>
          {#if m.packages.length}
            <ul class="chips">
              {#each m.packages as p (p)}
                <li class="chip mono">{p} <button class="icon" aria-label="Remove {p} from {name}" title="Remove {p} from {name}" disabled={busy === 'p:' + p} onclick={() => run('p:' + p, () => removePackages(target, [p]))}><Icon name="x" /></button></li>
              {/each}
            </ul>
          {:else}
            <p class="muted">No packages.</p>
          {/if}
        </section>

        <section class="section" aria-labelledby="mod-settings">
          <h2 id="mod-settings">Settings</h2>
          <p class="muted small">NixOS options the module sets where it is on. They apply as hand-written Nix does: lists and sets merge with the machine's hand-written ones. A value a machine sets by clicking wins whole, a list included: it replaces the module's.</p>
          {#if settings.length}
            <div class="table-wrap">
              <table class="data">
                <tbody>
                  {#each settings as s (s.name)}
                    <tr>
                      <td class="mono">{s.name}</td>
                      <td class="mono">{showValue(s.value)}</td>
                      <td class="act"><button class="small ghost" disabled={busy === 's:' + s.name} onclick={() => run('s:' + s.name, () => unsetOption(target, s.path, `${name} no longer sets ${s.name}`))}>Remove</button></td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
          <form class="row add-setting" onsubmit={addSetting}>
            <input class="mono name" list="mod-option-names" bind:value={settingName} oninput={suggest} placeholder="services.jellyfin.enable" aria-label="Option to set" aria-invalid={!!settingProblem} />
            <datalist id="mod-option-names">{#each suggestions as o (o)}<option value={o}></option>{/each}</datalist>
            <span>=</span>
            <input class="mono value" bind:value={settingText} placeholder={options.length ? `true, 8080, "text", [ "${optionRef(options[0][0])}" ]` : 'true, 8080, "text", [ … ]'} aria-label="Its value" />
            <button class="primary small" type="submit" disabled={!settingPath || busy === 'setting'}>Set</button>
          </form>
          {#if settingProblem || settingValueProblem}<p class="problem small">{settingProblem || settingValueProblem}</p>{/if}
        </section>
      {:else}
        <p class="muted">Reading {moduleFile(name)}…</p>
      {/if}
    </ModelGate>
  </div>
</div>

{#if picking}
  <Drawer title="Add a package to {name}" onclose={() => (picking = false)} width={560}>
    <PackagePicker label="Add to {name}" onpick={(hit) => addPackages(target, [hit.attr])} />
  </Drawer>
{/if}

<style>
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 1ch 2ch;
  }
  .crumbs {
    margin: 0;
  }
  .section {
    margin-top: calc(var(--lh) * 2);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 2ch;
  }
  .desc input {
    min-width: 40ch;
  }
  .opt {
    margin-right: 2ch;
    color: var(--fg-faint);
  }
  .opt.own {
    color: var(--fg-strong);
  }
  .act {
    text-align: right;
    white-space: nowrap;
  }
  .form {
    display: grid;
    gap: calc(var(--lh) / 2);
    max-width: 72ch;
    margin-top: var(--lh);
  }
  .form label:not(.check) {
    display: grid;
    grid-template-columns: 14ch 1fr;
    align-items: baseline;
    gap: 2ch;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 1ch 2ch;
    padding: 0;
    list-style: none;
  }
  .add-setting {
    margin-top: var(--lh);
    align-items: baseline;
  }
  .add-setting .name {
    flex: 2;
  }
  .add-setting .value {
    flex: 3;
  }
  .problem {
    color: var(--danger);
  }
</style>
