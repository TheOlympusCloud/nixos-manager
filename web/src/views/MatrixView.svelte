<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import { bar, bigText } from '../lib/ascii'
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
  const isShared = (c: PkgCell | FeatCell) => 'shared' in c && c.shared
  const source = (c: PkgCell | FeatCell) => (c.managed ? 'managed' : isShared(c) ? 'shared' : c.files.some(isRepoFile) ? 'repo' : 'default')
  const sourceTitle = (c: PkgCell | FeatCell) =>
    c.managed
      ? 'Set here (managed.json)'
      : isShared(c)
        ? 'Set for every machine (modules/managed.json)'
        : c.files.some(isRepoFile)
          ? `In code · ${c.files.filter(isRepoFile).join(', ')}`
          : 'Default (NixOS or a flake input)'
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate>
      <div class="page-head">
        <div>
          <h1 class="big" aria-label={kind === 'packages' ? 'Packages' : 'Services'} use:bigText={kind === 'packages' ? 'Packages' : 'Services'}></h1>
          <p class="sub num">
            {kind === 'packages' ? plural(total, 'package') : plural(total, 'feature')} across {plural(hosts.length, 'machine')}{#if broken.length}{' · '}<span class="bad"
                >{broken.map((h) => h.name).join(', ')} not evaluated</span
              >{/if}
          </p>
        </div>
        <div class="toolbar">
          <span class="field"><Icon name="filter" /><input class="search" type="search" placeholder={kind === 'packages' ? 'Filter packages' : 'Filter services and programs'} aria-label="Filter rows" data-search bind:value={filter} /></span>
          {#if kind === 'packages'}
            <label class="toggle"><input type="checkbox" bind:checked={hideDefaults} />Hide defaults</label>
          {/if}
          <label class="toggle"><input type="checkbox" bind:checked={onlyDiff} />Only where machines differ</label>
          {#if kind === 'packages'}<button class="primary" onclick={() => openPackageSearch()}><Icon name="plus" /> Add packages</button>{/if}
        </div>
      </div>

      <div class="legend muted">
        <span class="lg managed"><span class="sw"></span>Set here</span>
        {#if kind === 'packages'}<span class="lg shared"><span class="sw"></span>Every machine</span>{/if}
        <span class="lg repo"><span class="sw"></span>In code</span>
        {#if kind === 'packages'}<span class="lg default"><span class="sw"></span>Default</span>{/if}
      </div>

      <div class="matrix-wrap">
        <table class="matrix data">
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
                            <button class="cell-btn icon" aria-label="Remove {r.name} from {h.name}" title="Remove from {h.name}" onclick={() => run(r.name + h.name, 'off', () => removePackageByName(h.name, r.name))}>
                              <Icon name="x" />
                            </button>
                          {/if}
                        </span>
                      {:else if h.managedFile}
                        <button class="cell-add icon" aria-label="Add {r.name} to {h.name}" title="Add to {h.name}…" onclick={() => openPackageSearch(r.name, [h.name])}><Icon name="plus" /></button>
                      {/if}
                    </td>
                  {/each}
                  <td class="n num">{r.count}/{hosts.length} <span class="bar" aria-hidden="true">{@html bar(r.count / hosts.length, 4)}</span></td>
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
                        <span class="tick {source(c)} g-{r.group}" title="{h.name}: {sourceTitle(c)}"><Icon name="tick" /></span>
                      {:else if c}
                        <button class="tick managed g-{r.group} toggle-btn icon" aria-pressed="true" aria-label="Turn off {r.name} on {h.name}" title="On (set here); click to turn off" onclick={() => run(key, 'off', () => toggleFeature(h.name, r.group, r.name, false))}>
                          <Icon name="tick" />
                        </button>
                      {:else if h.managedFile}
                        <button class="cell-add icon" aria-pressed="false" aria-label="Turn on {r.name} on {h.name}" title="Turn on for {h.name}" onclick={() => run(key, 'on', () => toggleFeature(h.name, r.group, r.name, true))}>
                          <Icon name="plus" />
                        </button>
                      {/if}
                    </td>
                  {/each}
                  <td class="n num">
                    <span>{r.count}/{hosts.length}</span>
                    <span class="bar" aria-hidden="true">{@html bar(r.count / hosts.length, 4)}</span>
                    <a class="opts" href={link('options', [r.cells.findIndex(Boolean) >= 0 ? hosts[r.cells.findIndex(Boolean)].name : hosts[0].name], { q: r.key + '.' })} title="Configure {r.name}" aria-label="Configure the options of {r.name}">
                      <Icon name="sliders" />
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
  .bad::before {
    content: '✖ ';
  }
  /* The tools take what the title leaves and wrap inside it, as in the other page heads. */
  .page-head .toolbar {
    flex: 1 1 0;
    justify-content: flex-end;
    min-width: 40ch;
  }
  .field {
    display: inline-flex;
    gap: 1ch;
    color: var(--fg-muted);
  }
  .toggle {
    display: inline-flex;
    gap: 1ch;
    cursor: pointer;
  }
  /* A cell is coloured by where its value comes from; the legend uses the same colours. */
  .legend {
    display: flex;
    gap: 2ch;
    margin-bottom: var(--lh);
  }
  .lg {
    display: inline-flex;
    gap: 1ch;
  }
  .sw::before {
    content: '■';
  }
  .managed {
    color: var(--green);
  }
  .shared {
    color: var(--accent);
  }
  .repo {
    color: var(--fg-muted);
  }
  .default {
    color: var(--fg-faint);
  }
  /* Its own scroller, so the header row and the name column stay put. */
  .matrix-wrap {
    max-height: calc(100vh - var(--lh) * 18);
    overflow: auto;
  }
  .matrix {
    min-width: 100%;
    width: auto;
  }
  .matrix :is(th, td) {
    white-space: nowrap;
  }
  .matrix thead th {
    z-index: 2;
  }
  .matrix thead .corner {
    position: sticky;
    left: 0;
    z-index: 3;
  }
  .code-only {
    margin-left: 1ch;
    color: var(--fg-faint);
  }
  .code-only::before {
    content: 'λ ';
  }
  /* Row heads are th, so they undo the caps header of table.data. */
  .matrix tbody th {
    position: sticky;
    top: auto;
    left: 0;
    z-index: 1;
    height: var(--lh);
    text-transform: none;
    /* Opaque over cells scrolled under it, and on the page's greenbar (print theme) where it lies. */
    background: var(--bars), var(--bg);
    background-attachment: fixed;
  }
  .matrix tbody th::after {
    content: none;
  }
  .row-head {
    min-width: 26ch;
    max-width: 38ch;
    overflow: hidden;
    color: var(--fg-strong);
    font-weight: 700;
    text-overflow: ellipsis;
  }
  .matrix tbody tr:not(.group-row):hover th {
    background: var(--bg-hover);
  }
  .cell {
    min-width: 14ch;
  }
  .has {
    display: inline-flex;
    gap: 1ch;
    max-width: 20ch;
  }
  .ver {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .has.pending {
    color: var(--fg-faint);
  }
  .has .spinner {
    margin: 0;
  }
  .cell-btn {
    min-width: 1ch;
  }
  /* Glyph buttons start on the cell's first column, like the values beside them. */
  .cell-add,
  .toggle-btn {
    text-align: left;
  }
  .cell-add {
    color: var(--fg-faint);
  }
  .tick {
    display: inline-block;
    min-width: 1ch;
  }
  .tick.pending .spinner {
    margin: 0;
  }
  /* Clicking a ✓ set here turns it off: red under the pointer. */
  .toggle-btn:hover:not(:disabled) {
    color: var(--on-accent);
    background: var(--red);
  }
  .matrix tbody .group-row th {
    height: calc(var(--lh) * 2);
    color: var(--fg-muted);
    font-weight: 400;
    text-transform: uppercase;
    vertical-align: bottom;
  }
  .group-row .dot {
    margin-right: 1ch;
  }
  .group-row .dot::before {
    content: '▪';
  }
  .matrix tbody .group-row:hover {
    background: none;
  }
  .matrix :is(.n, .n-col) {
    color: var(--fg-muted);
    text-align: left;
  }
  .n .opts {
    margin-left: 1ch;
    color: var(--fg-faint);
  }
  .n .opts:hover {
    color: var(--on-accent);
  }
  .none {
    height: calc(var(--lh) * 3);
    vertical-align: middle;
  }
  .more {
    display: flex;
    gap: 2ch;
    margin-top: var(--lh);
  }
</style>
