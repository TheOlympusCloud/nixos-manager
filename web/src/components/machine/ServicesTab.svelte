<script lang="ts">
  import type { FeatureGroup, Host } from '../../lib/api'
  import { enablePath, fleet, managedValue, setOption, unsetOption } from '../../lib/fleet.svelte'
  import { groupLabel, groupOrder } from '../../lib/matrix'
  import { machineLink } from '../../lib/router.svelte'
  import { managedFeatures, sourceOf } from '../../lib/values'
  import Drawer from '../Drawer.svelte'
  import Icon from '../Icon.svelte'
  import OptionBrowser from '../OptionBrowser.svelte'
  import SourceTag from '../SourceTag.svelte'
  import ServiceCatalog from './ServiceCatalog.svelte'
  import Switch from '../Switch.svelte'

  let { host }: { host: Host } = $props()

  type Card = { group: FeatureGroup; name: string; description: string; files: string[]; inModel: boolean }

  let filter = $state('')
  let busy = $state('')
  let drawer = $state<null | { kind: 'add' } | { kind: 'catalog' } | { kind: 'configure'; f: Card }>(null)
  let selected = $state('')

  const editable = $derived(!!host.managedFile && !!fleet.managed[host.name])
  // The evaluated features, plus switches set here that the evaluation does not show (yet).
  const cards = $derived.by((): Card[] => {
    const model = host.features.map((f) => ({ ...f, inModel: true }))
    const extra = managedFeatures(fleet.managed[host.name]?.settings)
      .filter((m) => !host.features.some((f) => f.group === m.group && f.name === m.name))
      .map((m) => ({ group: m.group, name: m.name, description: '', files: host.managedFile ? [host.managedFile] : [], inModel: false }))
    return [...model, ...extra]
  })
  const shown = $derived(cards.filter((f) => !filter.trim() || (f.name + ' ' + f.description).toLowerCase().includes(filter.trim().toLowerCase())))

  const setHere = (f: Card) => managedValue(host.name, enablePath(f.group, f.name))
  const handFiles = (f: Card) => f.files.filter((x) => x !== host.managedFile && x !== 'nixos')
  /** Evaluated as on only because of a value set here. */
  const onlyHere = (f: Card) => !!host.managedFile && f.files.includes(host.managedFile) && !handFiles(f).length
  /** Whether the switch shows on: the value set here wins; one reset here goes off; otherwise the evaluation. */
  function isOn(f: Card): boolean {
    const v = setHere(f)
    if (typeof v === 'boolean') return v
    return f.inModel && !(onlyHere(f) && !!fleet.managed[host.name])
  }
  /** On here but not evaluated yet, or the reverse. */
  const pendingOf = (f: Card) => (isOn(f) && !f.inModel ? 'on' : !isOn(f) && f.inModel ? 'off' : '')

  async function toggle(f: Card, on: boolean) {
    busy = f.group + f.name
    const path = enablePath(f.group, f.name)
    // Turning off something hand-written (or a default) takes an explicit false here, which wins over it.
    if (on) await setOption(host.name, path, true, `Enabled ${f.name} on ${host.name}`)
    else if (f.inModel && !onlyHere(f)) await setOption(host.name, path, false, `Disabled ${f.name} on ${host.name}`)
    else await unsetOption(host.name, path, `Disabled ${f.name} on ${host.name}`)
    busy = ''
  }
  async function reset(f: Card) {
    busy = f.group + f.name
    await unsetOption(host.name, enablePath(f.group, f.name), `${f.name} on ${host.name} follows ${handFiles(f)[0] ?? 'its Nix files'} again`)
    busy = ''
  }
  const sourceFor = (f: Card) => (setHere(f) !== undefined ? ({ kind: 'here' } as const) : sourceOf(f.files, host.managedFile))
</script>

<div class="toolbar">
  <input class="search" type="search" placeholder="Filter services and programs" aria-label="Filter services and programs" data-search bind:value={filter} />
  {#if fleet.computing && cards.some((f) => pendingOf(f))}<span class="badge busy">Re-evaluating {host.name}</span>{/if}
  <span class="spacer"></span>
  {#if host.managedFile}
    <button disabled={!editable} onclick={() => (drawer = { kind: 'catalog' })}><Icon name="grid" size={14} />Popular services</button>
    <button class="primary" disabled={!editable} onclick={() => (drawer = { kind: 'add' })}><Icon name="plus" size={14} />Add service or program</button>
  {:else}
    <span class="muted small">{host.name} is Code only: <a href={machineLink(host.name)}>turn on editing</a> to switch services by clicking.</span>
  {/if}
</div>

{#if !cards.length}
  <div class="empty card">
    <p><strong>No services or programs are enabled on {host.name}.</strong></p>
    <p class="muted">Features show up here as soon as something turns them on: a hand-written module, a flake input, or {host.managedFile ? 'the Add button above' : 'this app once editing is on'}.</p>
  </div>
{:else if !shown.length}
  <p class="muted">Nothing matches “{filter}”.</p>
{/if}

{#each groupOrder as g (g)}
  {@const fs = shown.filter((f) => f.group === g)}
  {#if fs.length}
    <section class="section">
      <h2 class="g-head"><span class="dot" style:--c="var(--g-{g})"></span>{groupLabel[g]} <span class="muted num">{fs.length}</span></h2>
      <div class="cards">
        {#each fs as f (f.name)}
          {@const pending = pendingOf(f)}
          {@const on = isOn(f)}
          <article class="card feat g-{g}" class:off={!on} class:pending={!!pending}>
            <div class="f-top">
              <h3>{f.name}</h3>
              {#if editable}
                <Switch checked={on} busy={busy === g + f.name} label="{f.name} on {host.name}" onchange={(v) => toggle(f, v)} />
              {/if}
            </div>
            <p class="desc">{f.description || (f.inModel ? 'No description.' : `Turned on here; ${host.name} has not been evaluated with it yet.`)}</p>
            <div class="f-foot">
              <div class="chips">
                {#if pending}
                  <span class="badge busy" title={fleet.computing ? 'The machine is being evaluated again' : 'The last evaluation does not show this change; check Changes'}
                    >{pending === 'on' ? 'Turning on' : 'Turning off'}{fleet.computing ? '…' : ''}</span>
                {/if}
                <SourceTag source={sourceFor(f)} />
                {#if editable && setHere(f) !== undefined && f.inModel && handFiles(f).length}
                  <button class="small ghost" disabled={busy === g + f.name} onclick={() => reset(f)} title="Remove the value set here; {handFiles(f)[0]} decides again">Reset</button>
                {/if}
              </div>
              {#if f.inModel}<button class="small ghost" onclick={() => ((selected = ''), (drawer = { kind: 'configure', f }))}><Icon name="sliders" size={13} />Configure</button>{/if}
            </div>
          </article>
        {/each}
      </div>
    </section>
  {/if}
{/each}

{#if drawer?.kind === 'add'}
  <Drawer title="Add a service or program to {host.name}" onclose={() => (drawer = null)} width={560}>
    <OptionBrowser host={host.name} enable stacked />
  </Drawer>
{:else if drawer?.kind === 'catalog'}
  <Drawer title="Popular services for {host.name}" onclose={() => (drawer = null)} width={560}>
    <ServiceCatalog {host} />
  </Drawer>
{:else if drawer?.kind === 'configure'}
  <Drawer title="Configure {drawer.f.name} on {host.name}" onclose={() => (drawer = null)} width={720}>
    <OptionBrowser host={host.name} query="{drawer.f.group}.{drawer.f.name}." bind:selected stacked />
  </Drawer>
{/if}

<style>
  .toolbar {
    margin-bottom: var(--s2);
  }
  .empty {
    display: grid;
    gap: 6px;
    max-width: 640px;
    margin-top: var(--s4);
    padding: 16px 18px;
    line-height: 1.5;
  }
  .g-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .g-head .dot {
    background: var(--c);
  }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: var(--s3);
  }
  .feat {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 14px;
    transition:
      opacity var(--t) var(--ease),
      border-color var(--t) var(--ease);
  }
  .feat.off:not(.pending) {
    opacity: 0.6;
  }
  .feat.pending {
    border-style: dashed;
    border-color: color-mix(in srgb, var(--accent) 35%, var(--border));
  }
  .feat.pending.off h3 {
    color: var(--fg-muted);
    text-decoration: line-through;
    text-decoration-color: var(--fg-faint);
  }
  .f-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  h3 {
    overflow-wrap: anywhere;
    font-size: var(--fs-md);
  }
  .desc {
    display: -webkit-box;
    overflow: hidden;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.5;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .f-foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-top: auto;
  }
  .f-foot .chips {
    align-items: center;
    min-width: 0;
  }
</style>
