<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { api, type PackageHit } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { searchIndexed } from '../lib/fleet.svelte'
  import Icon from './Icon.svelte'
  import JobLog from './JobLog.svelte'

  /** Search the pinned nixpkgs and pick one package; `label` names what picking does. */
  let { label, onpick, query = '' }: { label: string; onpick: (hit: PackageHit) => Promise<unknown> | void; query?: string } = $props()

  let q = $state(untrack(() => query))
  let results = $state<PackageHit[] | null>(null)
  let indexJob = $state('')
  let error = $state('')
  let busy = $state('')
  let alive = true
  onMount(() => {
    if (q.trim()) search() // for the query it opened with
    return () => (alive = false)
  })

  let seq = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  async function search() {
    clearTimeout(timer)
    const text = q.trim()
    const mine = ++seq
    error = ''
    if (!text) return void (results = null)
    try {
      const r = await searchIndexed(() => api.searchPackages(text, 20), (job) => mine === seq && (indexJob = job), () => alive && mine === seq)
      if (mine !== seq || r === null) return
      results = r
      indexJob = ''
    } catch (e) {
      if (mine === seq) error = e instanceof Error ? e.message : String(e)
    }
  }
  async function pick(hit: PackageHit) {
    busy = hit.attr
    try {
      await onpick(hit)
    } finally {
      busy = ''
    }
  }
</script>

<div class="picker">
  <div class="search-row">
    <b class="prompt" aria-hidden="true">λ ›</b>
    <!-- svelte-ignore a11y_autofocus -->
    <input
      type="search"
      autofocus
      data-search
      placeholder="Search packages by name or description"
      aria-label="Search packages"
      spellcheck="false"
      autocomplete="off"
      bind:value={q}
      oninput={() => {
        clearTimeout(timer)
        timer = setTimeout(search, 220)
      }}
      onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), search())} />
  </div>
  {#if error}
    <p class="error-text" role="alert">{error}</p>
  {:else if indexJob}
    <div role="status">
      <p><span class="dot busy"></span> <strong>Indexing nixpkgs…</strong> The first search reads every package of the pinned nixpkgs; with real Nix this takes a few minutes.</p>
      <div class="log" use:frame={'square'}>{#key indexJob}<JobLog jobId={indexJob} compact />{/key}</div>
    </div>
  {:else if results && !results.length}
    <p class="muted">No package matches “{q}”.</p>
  {:else if results}
    <ul class="hits">
      {#each results as r (r.attr)}
        <li>
          <span class="who">
            <span class="mono attr">{r.attr}</span> <span class="muted mono">{r.version}</span>
            <span class="desc">{r.description}</span>
          </span>
          <button class="small" disabled={!!busy} onclick={() => pick(r)}><Icon name="plus" /> {busy === r.attr ? 'Working…' : label}</button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    display: grid;
    gap: var(--lh);
  }
  .search-row {
    display: flex;
    gap: 1ch;
  }
  .search-row input {
    flex: 1;
  }
  .hits {
    display: grid;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .hits li {
    display: flex;
    gap: 2ch;
    justify-content: space-between;
    align-items: baseline;
  }
  .who {
    min-width: 0;
  }
  .attr {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .desc {
    display: block;
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .log {
    max-height: calc(var(--lh) * 8);
    overflow: auto;
  }
</style>
