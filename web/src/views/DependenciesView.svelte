<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import { api, ApiError, type FleetDeps } from '../lib/api'
  import { bar, bigText } from '../lib/ascii'
  import { filterRows, stateMessage } from '../lib/dependencies'
  import { plural } from '../lib/format'
  import { link, machineLink, route } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'

  /** Dependencies × machines, from the closures the machines report: which versions each one has installed. */
  let data = $state<FleetDeps | null>(null)
  let failed = $state('')
  let loading = $state(false)
  let loggedOut = $state(false)
  let filter = $state(route.query.q ?? '')
  $effect(() => {
    const q = route.query.q
    if (q !== undefined) untrack(() => (filter = q))
  })
  let onlyDiff = $state(false)
  // shortcut: renders the first PAGE rows and pages by button, as the package matrix does; virtualise if that gets slow.
  const PAGE = 300
  let limit = $state(PAGE)

  async function load() {
    loading = true
    loggedOut = false
    try {
      data = await api.fleetDependencies()
      failed = ''
    } catch (e) {
      failed = e instanceof Error ? e.message : String(e)
      loggedOut = e instanceof ApiError && e.status === 401
    }
    loading = false
  }
  onMount(load)
  // A load refused for an expired session runs again once the user has logged back in.
  $effect(() => {
    if (!app.needLogin && loggedOut) untrack(load)
  })

  const hosts = $derived(data?.hosts ?? [])
  const reporting = $derived(hosts.filter((h) => h.state === 'ready'))
  const missing = $derived(hosts.filter((h) => h.state !== 'ready'))
  const rows = $derived(data ? filterRows(data.rows, filter, onlyDiff) : [])
</script>

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <div>
        <h1 class="big" aria-label="Dependencies" use:bigText={'Dependencies'}></h1>
        <p class="sub num">
          {plural(rows.length, 'dependency', 'dependencies')} across {plural(reporting.length, 'machine')}{#if missing.length}{' · '}<span class="muted"
              >no tree from {missing.map((h) => h.host).join(', ')}</span
            >{/if}
        </p>
      </div>
      <div class="toolbar">
        <span class="field"><Icon name="filter" /><input class="search" type="search" placeholder="Filter dependencies" aria-label="Filter rows" data-search bind:value={filter} /></span>
        <label class="toggle"><input type="checkbox" bind:checked={onlyDiff} />Versions differ</label>
        <button onclick={load} disabled={loading}><Icon name="refresh" /> Refresh</button>
      </div>
    </div>
    <p class="muted intro">What each machine has installed, as it reported it: every package and library of the system it runs, with its versions. A row is marked when the machines that have it run different versions.</p>

    {#if failed}
      <div class="empty-state" role="alert">
        <p>Cannot load the fleet's dependencies: {failed}</p>
        <button onclick={load} disabled={loading}>Try again</button>
      </div>
    {:else if !data}
      <p class="muted" role="status"><span class="spinner" aria-hidden="true"></span> Loading…</p>
    {:else if !reporting.length}
      <div class="empty-state" role="status">
        <h2>No machine has sent what it has installed yet</h2>
        {#each missing as h (h.host)}<p><a href={machineLink(h.host, 'dependencies')}>{h.host}</a>: {stateMessage(h, h.host)}</p>{/each}
        {#if !missing.length}<p>Machines send it with their reports: set up reporting first.</p>{/if}
        <div class="row"><a class="btn" href={link('deployments')}>Deployments</a></div>
      </div>
    {:else}
      <div class="matrix-wrap">
        <table class="matrix data">
          <thead>
            <tr>
              <th class="corner">Dependency</th>
              {#each hosts as h (h.host)}
                <th class="col" title={stateMessage(h, h.host) || undefined}>
                  <a href={machineLink(h.host, 'dependencies')}>{h.host}</a>{#if h.state !== 'ready'}<span class="no-tree">no tree</span>{/if}
                </th>
              {/each}
              <th class="n-col">Machines</th>
            </tr>
          </thead>
          <tbody>
            {#each rows.slice(0, limit) as r (r.name)}
              {@const count = r.versions.filter((v) => v.length).length}
              <tr class:differs={r.differs}>
                <th class="row-head mono" scope="row">{#if r.differs}<span class="mark" title="The machines that have it run different versions">≠</span>{/if}{r.name}</th>
                {#each r.versions as vs, i (hosts[i].host)}
                  <td class="cell">
                    {#if vs.length}
                      <a class="ver mono" href={link('machines', [hosts[i].host, 'dependencies'], { q: r.name })} title="{r.name} on {hosts[i].host}">{vs.join(', ')}</a>
                    {/if}
                  </td>
                {/each}
                <td class="n num">{count}/{reporting.length} <span class="bar" aria-hidden="true">{@html bar(count / reporting.length, 4)}</span></td>
              </tr>
            {:else}
              <tr><td class="none muted" colspan={hosts.length + 2}>{filter ? `Nothing matches “${filter}”.` : 'Nothing to show with these filters.'}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if rows.length > limit}
        <div class="more"><button onclick={() => (limit += PAGE)}>Show {Math.min(PAGE, rows.length - limit)} more</button><span class="muted small">{limit} of {rows.length} shown</span></div>
      {/if}
    {/if}
  </div>
</div>

<style>
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
  .intro {
    max-width: 100ch;
    margin-bottom: var(--lh);
  }
  /* Its own scroller, so the header row and the name column stay put (as in the package matrix). */
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
  .no-tree {
    margin-left: 1ch;
    color: var(--fg-faint);
  }
  .matrix tbody th {
    position: sticky;
    top: auto;
    left: 0;
    z-index: 1;
    height: var(--lh);
    text-transform: none;
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
  .matrix tbody tr:hover th {
    background: var(--bg-hover);
  }
  .mark {
    margin-right: 1ch;
    color: var(--yellow);
  }
  .cell {
    min-width: 14ch;
  }
  .ver {
    display: inline-block;
    max-width: 28ch;
    overflow: hidden;
    color: inherit;
    text-overflow: ellipsis;
    vertical-align: bottom;
  }
  .differs .ver {
    color: var(--yellow);
  }
  .matrix :is(.n, .n-col) {
    color: var(--fg-muted);
    text-align: left;
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
