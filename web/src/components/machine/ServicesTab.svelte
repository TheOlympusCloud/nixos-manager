<script lang="ts">
  import type { Feature, Host } from '../../lib/api'
  import { enablePath, fleet, managedValue, toggleFeature } from '../../lib/fleet.svelte'
  import { isRepoFile } from '../../lib/graph'
  import { groupLabel, groupOrder } from '../../lib/matrix'
  import { codeLink } from '../../lib/router.svelte'
  import Drawer from '../Drawer.svelte'
  import Icon from '../Icon.svelte'
  import OptionBrowser from '../OptionBrowser.svelte'
  import Switch from '../Switch.svelte'

  let { host }: { host: Host } = $props()

  let filter = $state('')
  let busy = $state('')
  let drawer = $state<null | { kind: 'add' } | { kind: 'configure'; f: Feature }>(null)
  let selected = $state('')

  const shown = $derived(host.features.filter((f) => !filter.trim() || (f.name + ' ' + f.description).toLowerCase().includes(filter.trim().toLowerCase())))
  const fromManaged = (f: Feature) => !!host.managedFile && f.files.includes(host.managedFile)
  const handFile = (f: Feature) => f.files.find((x) => isRepoFile(x) && x !== host.managedFile)

  async function toggle(f: Feature, on: boolean) {
    busy = f.group + f.name
    await toggleFeature(host.name, f.group, f.name, on)
    busy = ''
  }
  const pending = $derived(fleet.pending.includes(host.name) && fleet.computing)
</script>

<div class="toolbar">
  <input class="search" type="search" placeholder="Filter services and programs" aria-label="Filter services and programs" data-search bind:value={filter} />
  {#if pending}<span class="badge busy">Re-evaluating; turned-off features disappear once it finishes</span>{/if}
  <span class="spacer"></span>
  {#if host.managedFile}
    <button class="primary" onclick={() => (drawer = { kind: 'add' })}><Icon name="plus" size={14} />Add service or program</button>
  {/if}
</div>

{#each groupOrder as g (g)}
  {@const fs = shown.filter((f) => f.group === g)}
  {#if fs.length}
    <section class="section">
      <h2 class="g-head"><span class="dot" style:--c="var(--g-{g})"></span>{groupLabel[g]} <span class="muted num">{fs.length}</span></h2>
      <div class="cards">
        {#each fs as f (f.name)}
          {@const hand = handFile(f)}
          <article class="card feat g-{g}">
            <div class="f-top">
              <h3>{f.name}</h3>
              {#if fromManaged(f) && !hand}
                <Switch checked={fleet.managed[host.name] ? managedValue(host.name, enablePath(f.group, f.name)) === true : true} busy={busy === g + f.name} label="{f.name} on {host.name}" onchange={(on) => toggle(f, on)} />
              {:else if hand}
                <a class="in-code small" href={codeLink(hand)} title="Enabled in {hand}; change it there">In code</a>
              {:else}
                <span class="in-code small" title="Enabled by NixOS or a flake input">Default</span>
              {/if}
            </div>
            <p class="desc">{f.description || 'No description.'}</p>
            <div class="f-foot">
              <div class="chips">
                {#each f.files as file (file)}
                  {#if file === host.managedFile}
                    <span class="chip managed">managed</span>
                  {:else if isRepoFile(file)}
                    <a class="chip mono" href={codeLink(file)}>{file}</a>
                  {:else}
                    <span class="chip">NixOS default</span>
                  {/if}
                {/each}
              </div>
              <button class="small ghost" onclick={() => ((selected = ''), (drawer = { kind: 'configure', f }))}><Icon name="sliders" size={13} />Configure</button>
            </div>
          </article>
        {/each}
      </div>
    </section>
  {/if}
{:else}
  <p class="muted">No features enabled.</p>
{/each}
{#if !shown.length && filter}<p class="muted">Nothing matches “{filter}”.</p>{/if}

{#if drawer?.kind === 'add'}
  <Drawer title="Add a service or program to {host.name}" onclose={() => (drawer = null)} width={560}>
    <OptionBrowser host={host.name} enable stacked />
  </Drawer>
{:else if drawer?.kind === 'configure'}
  <Drawer title="Configure {drawer.f.name} on {host.name}" onclose={() => (drawer = null)} width={720}>
    <OptionBrowser host={host.name} query="{drawer.f.group}.{drawer.f.name}." bind:selected stacked />
  </Drawer>
{/if}

<style>
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
  .chip.managed {
    --c: var(--accent);
  }
  .in-code {
    flex: none;
    color: var(--fg-muted);
    text-decoration: none;
  }
  a.in-code:hover {
    color: var(--accent);
  }
  a.chip {
    text-decoration: none;
  }
  a.chip:hover {
    color: var(--fg-strong);
    border-color: var(--line-strong);
  }
</style>
