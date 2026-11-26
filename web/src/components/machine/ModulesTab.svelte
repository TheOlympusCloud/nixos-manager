<script lang="ts">
  import { onMount } from 'svelte'
  import type { FleetModule, Host, ModuleOption } from '../../lib/api'
  import { frame } from '../../lib/ascii'
  import { fleet, loadManaged, loadModules, managedValue, setOption, unsetOption } from '../../lib/fleet.svelte'
  import { moduleOn, modulePath, moduleSource, offValue, parseValue, usesModules, valueText } from '../../lib/modules'
  import { SHARED } from '../../lib/packages'
  import { codeLink, link, machineLink } from '../../lib/router.svelte'
  import { showValue } from '../../lib/values'
  import Icon from '../Icon.svelte'
  import Switch from '../Switch.svelte'
  import ManagedModuleNotice from './ManagedModuleNotice.svelte'

  let { host }: { host: Host } = $props()

  onMount(() => {
    loadModules()
    loadManaged(SHARED)
  })

  const modules = $derived((fleet.modules ?? []).filter((m) => !m.error))
  const editable = $derived(usesModules(host) && !!fleet.managed[host.name])
  const hmOf = (name: string) => host.modules?.find((x) => x.name === name)

  let busy = $state('')
  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    await fn()
    busy = ''
  }

  function toggle(name: string, on: boolean) {
    const path = modulePath(name, 'enable')
    return run(name, async () => {
      if (on) await setOption(host.name, path, true, `Turned ${name} on for ${host.name}`)
      else if (offValue(hmOf(name), name, host.managedFile) === false) await setOption(host.name, path, false, `Turned ${name} off for ${host.name}`)
      else await unsetOption(host.name, path, `Turned ${name} off for ${host.name}`)
    })
  }

  const sourceLabel = (name: string, files: string[]) => {
    const s = moduleSource(files, name, host.managedFile)
    return s.kind === 'here' ? '✎ Set here' : s.kind === 'shared' ? '⇉ Every machine' : s.kind === 'code' ? `λ ${s.file}` : '· Default'
  }

  // An option's text while it is edited, by module and option.
  let drafts = $state<Record<string, string>>({})
  /** The value that applies: set here (at once), else the last evaluation's, else the default. */
  function current(mod: FleetModule, o: string, d: ModuleOption): { value: unknown; label: string; here: boolean } {
    const here = managedValue(host.name, modulePath(mod.name, o))
    if (here !== undefined) return { value: here, label: '✎ Set here', here: true }
    const hv = hmOf(mod.name)?.options[o]
    if (hv && moduleSource(hv.files, mod.name, host.managedFile).kind !== 'here') return { value: hv.value, label: sourceLabel(mod.name, hv.files), here: false }
    return { value: d.default, label: d.default === undefined ? 'Not set: needed' : '· Default', here: false }
  }
  function problemOf(key: string, d: ModuleOption) {
    const text = drafts[key]
    if (text === undefined) return ''
    const p = parseValue(d.type, text)
    return 'problem' in p ? `Must be ${p.problem}` : ''
  }
  function save(mod: FleetModule, o: string, d: ModuleOption) {
    const key = `${mod.name}.${o}`
    const p = parseValue(d.type, drafts[key] ?? '')
    if ('problem' in p) return
    run(key, async () => {
      if (await setOption(host.name, modulePath(mod.name, o), p.value, `Set ${mod.name}'s ${o} on ${host.name}`)) delete drafts[key]
    })
  }
  const reset = (mod: FleetModule, o: string) =>
    run(`${mod.name}.${o}`, async () => {
      if (await unsetOption(host.name, modulePath(mod.name, o), `${mod.name}'s ${o} on ${host.name} is back to what the module or the fleet says`)) delete drafts[`${mod.name}.${o}`]
    })
</script>

<ManagedModuleNotice {host} />

<div class="toolbar">
  <p class="muted">
    Modules bundle packages and settings; turn one on here and set its options for {host.name}. They are kept in
    <span class="mono">{host.managedFile ?? 'managed.json'}</span> as <span class="mono">fleet.modules.&lt;module&gt;</span>.
  </p>
  <span class="spacer"></span>
  <a class="btn" href={link('modules')}><Icon name="box" /> All modules</a>
</div>

{#if !host.managedFile}
  <p class="muted">{host.name} is Code only: <a href={machineLink(host.name)}>turn on editing</a> to turn modules on by clicking, or set <span class="mono">fleet.modules.&lt;module&gt;.enable</span> in its Nix.</p>
{/if}

{#if fleet.modules === null}
  <p class="muted">Reading the modules…</p>
{:else if !modules.length}
  <div class="empty card" use:frame={'dash'}>
    <p><strong>There are no modules yet.</strong></p>
    <p class="muted"><a href={link('modules')}>Create one</a> to give machines a set of packages and settings with one switch.</p>
  </div>
{:else}
  <div class="cards">
    {#each modules as mod (mod.name)}
      {@const hm = hmOf(mod.name)}
      {@const setHere = managedValue(host.name, modulePath(mod.name, 'enable'))}
      {@const on = moduleOn(hm, setHere, mod.name, host.managedFile)}
      {@const pending = on !== !!hm?.enable}
      <article class="card mod" class:off={!on} use:frame={{ title: mod.name, kind: pending ? 'dash' : 'round' }}>
        <div class="top">
          <h3 class="sr-only">{mod.name}</h3>
          {#if usesModules(host)}
            <Switch checked={on} busy={busy === mod.name} disabled={!editable} label="{mod.name} on {host.name}" onchange={(v) => toggle(mod.name, v)} />
          {/if}
          <p class="desc">{mod.description || 'No description.'}</p>
          <a class="small" href={link('modules', [mod.name])}>Module</a>
        </div>
        <p class="small muted">
          {#if pending}<span class="badge busy">{on ? 'Turning on' : 'Turning off'}{fleet.computing ? '…' : ''}</span>{/if}
          {setHere !== undefined ? '✎ Set here' : hm?.enable ? sourceLabel(mod.name, hm.files) : ''}
          {#if mod.packages.length}· {mod.packages.join(', ')}{/if}
          {#if mod.custom}· <a class="mono" href={codeLink(`modules/fleet/${mod.name}/custom.nix`)}>custom.nix</a>{/if}
        </p>
        {#if on && Object.keys(mod.options ?? {}).length}
          <ul class="opts">
            {#each Object.entries(mod.options ?? {}) as [o, d] (o)}
              {@const key = `${mod.name}.${o}`}
              {@const cur = current(mod, o, d)}
              {@const problem = problemOf(key, d)}
              <li>
                <label for="opt-{key}" class="mono" title={d.description}>{o}</label>
                {#if editable}
                  {#if d.type === 'bool'}
                    <select id="opt-{key}" value={drafts[key] ?? valueText(d.type, cur.value)} onchange={(e) => (drafts[key] = e.currentTarget.value)}>
                      {#if cur.value === undefined}<option value="" disabled>Pick</option>{/if}
                      <option value="false">false</option><option value="true">true</option>
                    </select>
                  {:else if d.type === 'listOf str' || d.type === 'anything'}
                    <textarea id="opt-{key}" class="mono" rows="2" value={drafts[key] ?? valueText(d.type, cur.value)} oninput={(e) => (drafts[key] = e.currentTarget.value)} placeholder={d.type === 'listOf str' ? 'One per line' : 'JSON'}></textarea>
                  {:else}
                    <input id="opt-{key}" class="mono" value={drafts[key] ?? valueText(d.type, cur.value)} oninput={(e) => (drafts[key] = e.currentTarget.value)} aria-invalid={!!problem} />
                  {/if}
                  <span class="src small" class:need={cur.value === undefined}>{cur.label}</span>
                  {#if drafts[key] !== undefined}
                    <button class="small primary" disabled={!!problem || busy === key} onclick={() => save(mod, o, d)}>Save</button>
                    <button class="small ghost" onclick={() => delete drafts[key]}>Undo</button>
                  {:else if cur.here}
                    <button class="small ghost" disabled={busy === key} onclick={() => reset(mod, o)} title="Remove the value set here">Reset</button>
                  {/if}
                  {#if problem}<span class="problem small">{problem}</span>{/if}
                {:else}
                  <span class="mono">{cur.value === undefined ? '' : showValue(cur.value)}</span> <span class="src small">{cur.label}</span>
                {/if}
                {#if d.description}<p class="muted small odesc">{d.description}</p>{/if}
              </li>
            {/each}
          </ul>
        {/if}
      </article>
    {/each}
  </div>
{/if}

<style>
  .toolbar {
    display: flex;
    align-items: baseline;
    gap: 2ch;
    margin-bottom: var(--lh);
  }
  .toolbar p {
    margin: 0;
  }
  .cards {
    display: grid;
    gap: var(--lh);
  }
  .mod.off .desc {
    color: var(--fg-muted);
  }
  .top {
    display: flex;
    align-items: baseline;
    gap: 2ch;
  }
  .desc {
    flex: 1;
    margin: 0;
  }
  .opts {
    display: grid;
    gap: calc(var(--lh) / 2);
    padding: 0;
    margin: var(--lh) 0 0;
    list-style: none;
  }
  .opts li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .opts label {
    min-width: 16ch;
  }
  .opts input,
  .opts textarea,
  .opts select {
    width: 32ch;
    max-width: 100%;
  }
  .odesc {
    flex-basis: 100%;
    margin: 0 0 0 18ch;
  }
  .src {
    color: var(--fg-muted);
  }
  .need,
  .problem {
    color: var(--danger);
  }
</style>
