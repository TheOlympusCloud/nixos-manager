<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import type { Feature, FeatureGroup } from '../lib/api'
  import { openPackageSearch } from '../lib/dialogs.svelte'
  import { fleet, healthy, loadManaged, removePackageByName, toggleFeature } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { isRepoFile } from '../lib/graph'
  import { featureMatrix, groupLabel, packageMatrix, type FeatCell, type PkgCell } from '../lib/matrix'
  import { link, machineLink, route } from '../lib/router.svelte'

  /** Packages × machines or features × machines, with editing in the cells of managed machines. */
  let { kind }: { kind: 'packages' | 'services' } = $props()

  const hosts = $derived(healthy())
  const broken = $derived((fleet.model?.hosts ?? []).filter((h) => h.error))
  onMount(() => {
    for (const h of hosts) if (h.managedFile && !fleet.managed[h.name]) loadManaged(h.name)
  })

  let filter = $state(route.query.q ?? '')
  // The palette and other views link here with ?q=<name>.
  $effect(() => {
    const q = route.query.q
    if (q !== undefined) untrack(() => (filter = q))
  })
  let hideDefaults = $state(true)
  let onlyDiff = $state(false)
  // shortcut: renders the first PAGE rows and pages by button; virtualise if fleets reach thousands of rows.
  const PAGE = 300
  let limit = $state(PAGE)

  const q = $derived(filter.trim().toLowerCase())
  const pkgRows = $derived(
    kind === 'packages'
      ? packageMatrix(hosts).filter((r) => (!hideDefaults || !r.nixos) && (!onlyDiff || r.count < hosts.length) && (!q || r.name.toLowerCase().includes(q)))
      : [],
  )
  const featRows = $derived(
    kind === 'services'
      ? featureMatrix(hosts).filter((r) => (!onlyDiff || r.count < hosts.length) && (!q || (r.key + ' ' + r.description).toLowerCase().includes(q)))
      : [],
  )
  const total = $derived(kind === 'packages' ? pkgRows.length : featRows.length)

  // Cells edited since the model was evaluated: shown as pending until the new evaluation lands.
  let pending = $state<Record<string, 'on' | 'off'>>({})
  $effect(() => {
    void fleet.model
    if (!fleet.computing) untrack(() => (pending = {}))
  })
  async function run(key: string, to: 'on' | 'off', fn: () => Promise<unknown>) {
    pending[key] = to
    if ((await fn()) === false) delete pending[key]
  }
  const source = (c: PkgCell | FeatCell) => (c.managed ? 'managed' : c.files.some(isRepoFile) ? 'repo' : 'default')
  const sourceTitle = (c: PkgCell | FeatCell) => (c.managed ? 'Set here (managed.json)' : c.files.some(isRepoFile) ? `In code · ${c.files.filter(isRepoFile).join(', ')}` : 'Default (NixOS or a flake input)')
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate>
      <div class="page-head">
        <h1>{kind === 'packages' ? 'Packages' : 'Services'}</h1>
        <span class="sub num">
          {kind === 'packages' ? plural(total, 'package') : plural(total, 'feature')} across {plural(hosts.length, 'machine')}{#if broken.length}{' · '}<span class="bad"
              >{broken.map((h) => h.name).join(', ')} not evaluated</span
            >{/if}
        </span>
        <span class="spacer"></span>
        <span class="legend muted small">
          <span class="lg"><span class="sw managed"></span>Set here</span>
          <span class="lg"><span class="sw repo"></span>In code</span>
          {#if kind === 'packages'}<span class="lg"><span class="sw default"></span>Default</span>{/if}
        </span>
      </div>

      <div class="toolbar">
        <input class="search" type="search" placeholder={kind === 'packages' ? 'Filter packages' : 'Filter services and programs'} aria-label="Filter rows" data-search bind:value={filter} />
        {#if kind === 'packages'}
          <label class="toggle"><input type="checkbox" bind:checked={hideDefaults} />Hide defaults</label>
        {/if}
        <label class="toggle"><input type="checkbox" bind:checked={onlyDiff} />Only where machines differ</label>
        <span class="spacer"></span>
        {#if kind === 'packages'}<button class="primary" onclick={() => openPackageSearch()}><Icon name="plus" size={14} />Add packages</button>{/if}
      </div>

      <div class="matrix-wrap">
        <table class="matrix">
          <thead>
            <tr>
              <th class="corner">{kind === 'packages' ? 'Package' : 'Feature'}</th>
              {#each hosts as h (h.name)}
                <th class="col">
                  <a href={machineLink(h.name, kind)} title={h.managedFile ? `${h.name} · Editable here` : `${h.name} · Code only: change it in code`}>{h.name}</a>
                  {#if !h.managedFile}<span class="code-only">Code only</span>{/if}
                </th>
              {/each}
              <th class="n-col">Machines</th>
            </tr>
          </thead>
          <tbody>
            {#if kind === 'packages'}
              {#each pkgRows.slice(0, limit) as r (r.name)}
                <tr>
                  <th class="row-head mono" scope="row">{r.name}</th>
                  {#each r.cells as c, i (hosts[i].name)}
                    {@const h = hosts[i]}
                    <td class="cell">
                      {#if c && pending[r.name + h.name]}
                        <span class="has pending" title="Removing from {h.name}; the machine is evaluated again">
                          <span class="spinner" aria-hidden="true"></span><span class="ver mono">{c.version}</span>
                        </span>
                      {:else if c}
                        <span class="has {source(c)}" title={sourceTitle(c)}>
                          <span class="ver mono">{c.version}</span>
                          {#if c.managed}
                            <button class="cell-btn" aria-label="Remove {r.name} from {h.name}" title="Remove from {h.name}" onclick={() => run(r.name + h.name, 'off', () => removePackageByName(h.name, r.name))}>
                              <Icon name="x" size={12} />
                            </button>
                          {/if}
                        </span>
                      {:else if h.managedFile}
                        <button class="cell-add" aria-label="Add {r.name} to {h.name}" title="Add to {h.name}…" onclick={() => openPackageSearch(r.name, [h.name])}><Icon name="plus" size={13} /></button>
                      {/if}
                    </td>
                  {/each}
                  <td class="n num">{r.count}/{hosts.length}</td>
                </tr>
              {/each}
            {:else}
              {#each featRows.slice(0, limit) as r, idx (r.key)}
                {#if idx === 0 || featRows[idx - 1].group !== r.group}
                  <tr class="group-row"><th colspan={hosts.length + 2} scope="rowgroup"><span class="dot" style:--c="var(--g-{r.group})"></span>{groupLabel[r.group as FeatureGroup] ?? r.group}</th></tr>
                {/if}
                <tr>
                  <th class="row-head" scope="row" title={r.description}><span class="fname">{r.name}</span></th>
                  {#each r.cells as c, i (hosts[i].name)}
                    {@const h = hosts[i]}
                    {@const key = r.key + h.name}
                    <td class="cell">
                      {#if pending[key]}
                        <span class="tick pending g-{r.group}" role="status" title="Turning {pending[key]} on {h.name}; the machine is evaluated again">
                          <span class="spinner" aria-hidden="true"></span><span class="sr-only">Turning {r.name} {pending[key]} on {h.name}</span>
                        </span>
                      {:else if c && (!c.managed || !h.managedFile || c.files.some((f) => isRepoFile(f) && f !== h.managedFile))}
                        <span class="tick {source(c)} g-{r.group}" title="{h.name}: {sourceTitle(c)}"><Icon name="tick" size={13} /></span>
                      {:else if c}
                        <button class="tick managed g-{r.group} toggle-btn" aria-pressed="true" aria-label="Turn off {r.name} on {h.name}" title="On (set here); click to turn off" onclick={() => run(key, 'off', () => toggleFeature(h.name, r.group, r.name, false))}>
                          <Icon name="tick" size={13} />
                        </button>
                      {:else if h.managedFile}
                        <button class="cell-add" aria-pressed="false" aria-label="Turn on {r.name} on {h.name}" title="Turn on for {h.name}" onclick={() => run(key, 'on', () => toggleFeature(h.name, r.group, r.name, true))}>
                          <Icon name="plus" size={13} />
                        </button>
                      {/if}
                    </td>
                  {/each}
                  <td class="n num">
                    <span class="count">{r.count}/{hosts.length}</span>
                    <a class="opts" href={link('options', [r.cells.findIndex(Boolean) >= 0 ? hosts[r.cells.findIndex(Boolean)].name : hosts[0].name], { q: r.key + '.' })} title="Configure {r.name}" aria-label="Configure the options of {r.name}">
                      <Icon name="sliders" size={13} />
                    </a>
                  </td>
                </tr>
              {/each}
            {/if}
            {#if !total}
              <tr><td class="none muted" colspan={hosts.length + 2}>{filter ? `Nothing matches “${filter}”.` : 'Nothing to show with these filters.'}</td></tr>
            {/if}
          </tbody>
        </table>
      </div>
      {#if total > limit}
        <div class="more"><button onclick={() => (limit += PAGE)}>Show {Math.min(PAGE, total - limit)} more</button><span class="muted small">{limit} of {total} shown</span></div>
      {/if}
    </ModelGate>
  </div>
</div>

<style>
  .bad {
    color: var(--red);
  }
  .toolbar {
    margin-bottom: var(--s3);
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    margin-left: 6px;
    cursor: pointer;
  }
  .legend {
    display: flex;
    gap: 12px;
    margin-right: 8px;
  }
  .lg {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .sw {
    width: 10px;
    height: 10px;
    border-radius: 3px;
  }
  .sw.managed {
    background: color-mix(in srgb, var(--accent-strong) 70%, transparent);
  }
  .sw.repo {
    background: var(--line-strong);
  }
  .sw.default {
    border: 1px dashed var(--line-strong);
  }
  .matrix-wrap {
    overflow: auto;
    max-height: calc(100vh - 214px);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .matrix {
    min-width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    font-variant-numeric: tabular-nums;
  }
  .matrix thead th {
    position: sticky;
    top: 0;
    z-index: 2;
    height: 38px;
    padding: 0 12px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    font-weight: 500;
    text-align: left;
    white-space: nowrap;
    background: var(--bg-side);
    border-bottom: 1px solid var(--line);
  }
  .matrix thead .col a {
    color: var(--fg-strong);
    font-weight: 600;
    text-decoration: none;
  }
  .matrix thead .col a:hover {
    color: var(--accent);
  }
  .corner,
  .row-head {
    position: sticky;
    left: 0;
    z-index: 1;
    min-width: 220px;
    max-width: 320px;
    background: var(--bg);
  }
  .matrix thead .corner {
    z-index: 3;
    background: var(--bg-side);
  }
  .row-head {
    padding: 0 12px;
    overflow: hidden;
    color: var(--fg-strong);
    font-size: 12.5px;
    font-weight: 400;
    text-align: left;
    text-overflow: ellipsis;
    white-space: nowrap;
    border-right: 1px solid var(--border);
  }
  .matrix td,
  .row-head {
    height: 34px;
    border-bottom: 1px solid var(--border);
  }
  .matrix tbody tr:hover td,
  .matrix tbody tr:hover .row-head {
    background: var(--bg-raised);
  }
  .cell {
    min-width: 128px;
    padding: 0 8px;
  }
  .has {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 24px;
    max-width: 150px;
    padding: 0 4px 0 8px;
    border-radius: var(--r-sm);
  }
  .has.repo {
    background: var(--bg-active);
  }
  .has.managed {
    background: color-mix(in srgb, var(--accent-strong) 16%, var(--bg-raised));
  }
  .has.managed .ver {
    color: var(--fg-strong);
  }
  .has.default {
    border: 1px dashed var(--line-strong);
  }
  .ver {
    overflow: hidden;
    color: var(--fg);
    font-size: 11.5px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .has:not(.managed) {
    padding-right: 8px;
  }
  .cell-btn,
  .cell-add {
    width: 22px;
    height: 22px;
    padding: 0;
    color: var(--fg-muted);
    background: none;
    border: 1px solid transparent;
  }
  .cell-btn:hover:not(:disabled) {
    color: var(--fg-strong);
    background: color-mix(in srgb, var(--red) 14%, transparent);
    border-color: transparent;
  }
  .cell-add {
    opacity: 0;
    border-style: dashed;
    border-color: var(--line-strong);
  }
  tr:hover .cell-add,
  .cell-add:focus-visible {
    opacity: 1;
  }
  .cell-add:hover:not(:disabled) {
    color: var(--fg-strong);
    background: var(--bg-hover);
  }
  .tick {
    display: inline-grid;
    place-items: center;
    width: 24px;
    height: 24px;
    padding: 0;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 14%, transparent);
    border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
    border-radius: var(--r-sm);
  }
  .tick.default {
    background: none;
    border-style: dashed;
  }
  .tick.managed {
    color: var(--on-accent);
    background: var(--c);
  }
  .toggle-btn:hover:not(:disabled) {
    background: color-mix(in srgb, var(--red) 18%, transparent);
    border-color: color-mix(in srgb, var(--red) 50%, transparent);
  }
  .group-row th {
    position: sticky;
    left: 0;
    height: 30px;
    padding: 10px 12px 4px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    font-weight: 500;
    text-align: left;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
  }
  .group-row .dot {
    margin-right: 8px;
    background: var(--c);
  }
  .fname {
    font-weight: 500;
  }
  .n,
  .n-col {
    width: 110px;
    white-space: nowrap;
    padding-right: 14px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    text-align: right;
  }
  .n .opts {
    display: inline-grid;
    place-items: center;
    width: 22px;
    height: 22px;
    margin-left: 6px;
    color: var(--fg-faint);
    vertical-align: middle;
    border-radius: var(--r-sm);
  }
  tr:hover .opts {
    color: var(--fg-muted);
  }
  .n .opts:hover {
    color: var(--accent);
    background: var(--bg-hover);
  }
  .code-only {
    margin-left: 6px;
    color: var(--fg-faint);
    font-size: var(--fs-xs);
    font-weight: 400;
  }
  .has.pending {
    opacity: 0.7;
    border: 1px dashed var(--line-strong);
  }
  .tick.pending {
    background: none;
    border-style: dashed;
  }
  .tick.pending .spinner,
  .has.pending .spinner {
    margin: 0;
  }
  .none {
    padding: 18px 12px;
  }
  .more {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 12px;
  }
</style>
