<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { api, ApiError, type Host, type MachineDeps } from '../../lib/api'
  import { frame } from '../../lib/ascii'
  import { children, findNodes, label, stateMessage, usedBy, why } from '../../lib/dependencies'
  import { bytes, plural } from '../../lib/format'
  import { link, route } from '../../lib/router.svelte'
  import { app } from '../../lib/state.svelte'
  import { ago } from '../../lib/util'
  import Icon from '../Icon.svelte'

  /** What the machine has installed, as it reported it: a tree from its system, nix-tree style. */
  let { host }: { host: Host } = $props()

  let deps = $state<MachineDeps | null>(null)
  let failed = $state('')
  let loading = $state(false)
  let loggedOut = $state(false)
  let filter = $state(route.query.q ?? '')
  let selected = $state(-1)
  // Open branches, by the chain of indices from the system that reaches them: an entry used in many places opens in each on its own.
  let open = $state<Record<string, boolean>>({})

  const ready = $derived(deps?.state === 'ready' ? deps : null)
  const users = $derived(ready ? usedBy(ready) : [])
  const hits = $derived(ready ? findNodes(ready, filter) : [])
  // shortcut: long lists of neighbours show their first SHOW; glibc is used by most of a system.
  const SHOW = 200

  async function load() {
    loading = true
    loggedOut = false
    try {
      deps = await api.machineDependencies(host.name)
      failed = ''
      if (deps.state === 'ready') {
        open = { [String(deps.root)]: true }
        const first = route.query.q ? findNodes(deps, route.query.q, 1)[0] : undefined
        await select(first ?? (selected >= 0 && selected < deps.nodes.length ? selected : deps.root))
      }
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

  // The fleet table links here with ?q=<name>.
  $effect(() => {
    const q = route.query.q
    if (q === undefined) return
    untrack(() => {
      filter = q
      const first = ready ? findNodes(ready, q, 1)[0] : undefined
      if (first !== undefined) select(first)
    })
  })

  /** Select entry i and open the tree along a shortest chain from the system to it. */
  async function select(i: number) {
    if (!ready) return
    selected = i
    const chain = why(ready, i)
    for (let k = 1; k < chain.length; k++) open[chain.slice(0, k).join('/')] = true
    await tick()
    document.querySelector(`[data-key="${chain.join('/')}"]`)?.scrollIntoView({ block: 'nearest' })
  }
</script>

{#snippet branch(d: MachineDeps, key: string, i: number)}
  {@const n = d.nodes[i]}
  <li>
    <div class="node" class:sel={i === selected} data-key={key}>
      {#if n.refs.length}
        <button class="bare twist" aria-expanded={!!open[key]} aria-label="{open[key] ? 'Close' : 'Open'} {label(n)}" onclick={() => (open[key] = !open[key])}>{open[key] ? '▾' : '▸'}</button>
      {:else}
        <span class="twist" aria-hidden="true">·</span>
      {/if}
      <button class="bare pick" aria-current={i === selected ? 'true' : undefined} onclick={() => select(i)}>
        <span class="mono">{n.name}</span>{#if n.version}<span class="mono muted">{n.version}</span>{/if}{#if n.output}<span class="out">{n.output}</span>{/if}
      </button>
      <span class="size num muted">{bytes(n.narSize)}</span>
    </div>
    {#if open[key] && n.refs.length}
      <ul>
        {#each children(d, i) as c (c)}{@render branch(d, key + '/' + c, c)}{/each}
      </ul>
    {/if}
  </li>
{/snippet}

{#snippet entries(d: MachineDeps, list: number[])}
  <ul class="list">
    {#each list.slice(0, SHOW) as c (c)}
      <li><button class="link mono" onclick={() => select(c)}>{label(d.nodes[c])}</button></li>
    {:else}
      <li class="muted">nothing</li>
    {/each}
    {#if list.length > SHOW}<li class="muted">and {list.length - SHOW} more</li>{/if}
  </ul>
{/snippet}

{#if failed}
  <div class="empty-state" role="alert">
    <p>Cannot load what {host.name} has installed: {failed}</p>
    <button onclick={load} disabled={loading}>Try again</button>
  </div>
{:else if !deps}
  <p class="muted" role="status"><span class="spinner" aria-hidden="true"></span> Loading what {host.name} has installed…</p>
{:else if !ready}
  <div class="empty-state" role="status">
    <h2>No dependency tree for {host.name}</h2>
    <p>{stateMessage(deps, host.name)}</p>
    {#if deps.state === 'error' && deps.message}<pre>{deps.message}</pre>{/if}
    <div class="row">
      {#if deps.state === 'none' || deps.state === 'shared'}<a class="btn" href={link('deployments')}>Deployments</a>{/if}
      <button onclick={load} disabled={loading}><Icon name="refresh" /> Check again</button>
    </div>
  </div>
{:else}
  {@const d = ready}
  <div class="head">
    <p>
      <span class="mono">{label(d.nodes[d.root])}</span>
      <span class="muted num">· {plural(d.nodes.length, 'store path')} · {bytes(d.size)}{#if d.at}{` · reported ${ago(d.at)}`}{/if}</span>
    </p>
    <div class="toolbar">
      <span class="field"><Icon name="filter" /><input class="search" type="search" placeholder="Find a package" aria-label="Find a package on {host.name}" data-search bind:value={filter} /></span>
      <button onclick={load} disabled={loading}><Icon name="refresh" /> Refresh</button>
    </div>
  </div>
  {#if d.current && d.current !== d.system}
    <p class="note">{host.name} now runs another system; its tree arrives with its next report.</p>
  {/if}
  {#if filter.trim()}
    <ul class="rows hits" aria-label="Matches">
      {#each hits as i (i)}
        <li class:on={i === selected}><button class="bare" aria-current={i === selected ? 'true' : undefined} onclick={() => select(i)}><span class="mono">{label(d.nodes[i])}</span></button></li>
      {:else}
        <li class="muted">Nothing on {host.name} is called “{filter.trim()}”.</li>
      {/each}
    </ul>
  {/if}

  <div class="split">
    <ul class="tree" aria-label="Dependencies of {host.name}">{@render branch(d, String(d.root), d.root)}</ul>

    {#if selected >= 0 && selected < d.nodes.length}
      {@const n = d.nodes[selected]}
      <section class="card detail" use:frame={{ title: n.name }}>
        <h2 class="sr-only">{label(n)}</h2>
        <dl class="kv">
          <dt>Version</dt>
          <dd class="mono">{n.version ?? 'none'}{#if n.output} · {n.output}{/if}</dd>
          <dt>Size</dt>
          <dd class="num">{bytes(n.narSize)}</dd>
          <dt>Store path</dt>
          <dd class="mono path">{n.path}</dd>
        </dl>
        <h3>Why it is here</h3>
        <p class="chain">
          {#each why(d, selected) as c, k (c)}{#if k}<span class="muted" aria-hidden="true">{' › '}</span>{/if}<button class="link mono" onclick={() => select(c)}>{label(d.nodes[c])}</button>{/each}
        </p>
        <h3>Uses <span class="num muted">{n.refs.length}</span></h3>
        {@render entries(d, children(d, selected))}
        <h3>Used by <span class="num muted">{users[selected]?.length ?? 0}</span></h3>
        {@render entries(d, users[selected] ?? [])}
      </section>
    {/if}
  </div>
{/if}

<style>
  .head {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 0 2ch;
    margin-bottom: var(--lh);
  }
  .field {
    display: inline-flex;
    gap: 1ch;
    color: var(--fg-muted);
  }
  .hits {
    max-height: calc(var(--lh) * 8);
    margin-bottom: var(--lh);
    overflow: auto;
  }
  .split {
    display: grid;
    grid-template-columns: minmax(40ch, 1fr) minmax(40ch, 1fr);
    gap: var(--lh) 2ch;
    align-items: start;
  }
  @media (max-width: 900px) {
    .split {
      grid-template-columns: 1fr;
    }
  }
  .tree {
    max-height: calc(100vh - var(--lh) * 16);
    overflow: auto;
  }
  .tree :global(ul) {
    padding-left: 2ch;
  }
  .node {
    display: flex;
    gap: 1ch;
    white-space: nowrap;
  }
  .node:hover {
    background: var(--bg-hover);
  }
  .node.sel .pick,
  .node.sel .pick :is(.muted, .out) {
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .twist {
    width: 1ch;
    color: var(--fg-muted);
  }
  .pick {
    display: inline-flex;
    gap: 1ch;
    min-width: 0;
    overflow: hidden;
    text-align: left;
  }
  .out {
    color: var(--fg-faint);
  }
  .size {
    margin-left: auto;
  }
  .detail {
    position: sticky;
    top: 0;
    min-width: 0;
  }
  .path {
    overflow-wrap: anywhere;
  }
  .detail h3 {
    margin-top: var(--lh);
  }
  .chain {
    overflow-wrap: anywhere;
  }
  .list {
    max-height: calc(var(--lh) * 10);
    overflow: auto;
  }
</style>
