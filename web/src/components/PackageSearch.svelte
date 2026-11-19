<script lang="ts">
  import { onMount } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import { api, type PackageHit } from '../lib/api'
  import { pkgSearch } from '../lib/dialogs.svelte'
  import { addPackages, fleet, healthy, searchIndexed } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { isRepoFile } from '../lib/graph'
  import { machineLink } from '../lib/router.svelte'
  import Icon from './Icon.svelte'
  import JobLog from './JobLog.svelte'

  let el: HTMLDialogElement
  let query = $state(pkgSearch.query)
  let results = $state<PackageHit[] | null>(null)
  let indexJob = $state('')
  let error = $state('')
  let loading = $state(false)
  let adding = $state(false)
  const picked = new SvelteSet<string>()
  const targets = new SvelteSet<string>(pkgSearch.hosts)

  const machines = $derived(healthy())
  const editable = $derived(machines.filter((h) => h.managedFile))
  const codeOnly = $derived(machines.filter((h) => !h.managedFile))
  /** Picked packages, by attribute, so adding can skip machines that already have them. */
  const hits = new Map<string, PackageHit>()

  /** Where a machine already gets this package: Set here, In code · file, or Default; null when it does not. */
  function hasOn(hostName: string, r: PackageHit): string | null {
    const h = machines.find((x) => x.name === hostName)
    if (!h) return null
    if (fleet.managed[h.name]?.packages.includes(r.attr)) return 'Set here'
    const p = h.packages.find((x) => x.name === r.pname || x.name === r.attr)
    if (!p) return null
    if (h.managedFile && p.files.includes(h.managedFile)) return 'Set here'
    const file = p.files.find(isRepoFile)
    return file ? `In code · ${file}` : 'Default'
  }
  const onTargets = (r: PackageHit) => [...targets].flatMap((t) => (hasOn(t, r) ? [{ host: t, from: hasOn(t, r)! }] : []))

  onMount(() => {
    el.showModal()
    if (!targets.size) {
      // No machine given (e.g. from Packages): start from the ones picked last time.
      const last: string[] = JSON.parse(localStorage.getItem('nixmgr.pkgTargets') ?? '[]')
      for (const t of last) if (editable.some((h) => h.name === t)) targets.add(t)
      if (!targets.size && editable.length === 1) targets.add(editable[0].name)
    }
    if (query.trim()) search()
  })

  let seq = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  async function search() {
    clearTimeout(timer)
    const q = query.trim()
    const mine = ++seq
    error = ''
    if (!q) {
      results = null
      return
    }
    loading = true
    try {
      const r = await searchIndexed(() => api.searchPackages(q, 50), (job) => mine === seq && (indexJob = job), () => mine === seq && el?.open)
      if (mine !== seq || r === null) return
      results = r
      for (const h of r) hits.set(h.attr, h)
      indexJob = ''
    } catch (e) {
      if (mine === seq) error = e instanceof Error ? e.message : String(e)
    } finally {
      if (mine === seq) loading = false
    }
  }

  function close() {
    pkgSearch.open = false
  }

  /** Per machine, the picked packages it does not have yet. */
  const plan = $derived([...targets].map((t) => ({ host: t, attrs: [...picked].filter((a) => !(hits.has(a) && hasOn(t, hits.get(a)!))) })))
  const skipped = $derived(plan.reduce((n, p) => n + picked.size - p.attrs.length, 0))

  async function add() {
    adding = true
    localStorage.setItem('nixmgr.pkgTargets', JSON.stringify([...targets]))
    try {
      for (const p of plan) if (p.attrs.length && !(await addPackages(p.host, p.attrs))) return
      close()
    } finally {
      adding = false
    }
  }

  function toggle(set: SvelteSet<string>, v: string) {
    if (set.has(v)) set.delete(v)
    else set.add(v)
  }
</script>

<dialog bind:this={el} class="ps" onclose={close} aria-labelledby="ps-title">
  <header>
    <h2 id="ps-title">Add packages</h2>
    <span class="muted small">from the nixpkgs pinned in <span class="mono">flake.lock</span></span>
    <span class="spacer"></span>
    <button class="icon" aria-label="Close" onclick={() => el.close()}><Icon name="x" /></button>
  </header>

  <div class="search-row">
    <span class="s-ico"><Icon name="search" size={15} /></span>
    <!-- svelte-ignore a11y_autofocus -->
    <input
      type="search"
      autofocus
      placeholder="Search packages by name or description"
      aria-label="Search packages"
      spellcheck="false"
      autocomplete="off"
      bind:value={query}
      oninput={() => {
        clearTimeout(timer)
        timer = setTimeout(search, 220)
      }}
      onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), search())} />
  </div>

  <div class="results" aria-busy={loading}>
    {#if error}
      <p class="error-text pad" role="alert">{error}</p>
    {:else if indexJob}
      <div class="indexing" role="status">
        <p><span class="dot busy"></span> <strong>Indexing nixpkgs…</strong> The first search reads every package of the pinned nixpkgs; this takes a few minutes with real Nix. You can keep this dialog open or come back later.</p>
        <div class="log">{#key indexJob}<JobLog jobId={indexJob} compact />{/key}</div>
      </div>
    {:else if loading && !results}
      <ul class="list">
        {#each [0, 1, 2, 3] as i (i)}<li class="row skel-row"><span class="skel" style:width="{30 + i * 12}%"></span></li>{/each}
      </ul>
    {:else if results === null}
      <p class="muted pad">Type a name, like <span class="mono">firefox</span> or <span class="mono">python3Packages.requests</span>.</p>
    {:else if !results.length}
      <p class="muted pad">No package matches “{query}”.</p>
    {:else}
      <ul class="list" aria-label="Packages found">
        {#each results as r (r.attr)}
          {@const on = onTargets(r)}
          {@const everywhere = targets.size > 0 && on.length === targets.size}
          <li>
            <label class="row" class:picked={picked.has(r.attr)} class:have={everywhere}>
              <input type="checkbox" checked={picked.has(r.attr)} disabled={everywhere && !picked.has(r.attr)} onchange={() => toggle(picked, r.attr)} />
              <span class="main">
                <span class="top"
                  ><span class="attr mono">{r.attr}</span><span class="ver mono">{r.version}</span>
                  {#each on as o (o.host)}<span class="badge" class:ok={o.from === 'Set here'} title="{r.pname} is already on {o.host}: {o.from}">{everywhere && on.length === 1 ? 'Already here' : `On ${o.host}`} · {o.from}</span>{/each}</span>
                {#if r.description}<span class="desc">{r.description}</span>{/if}
              </span>
            </label>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <footer>
    <div class="picked-row">
      <span class="label">Selected</span>
      {#if picked.size}
        <div class="chips">
          {#each [...picked] as a (a)}
            <span class="chip mono">{a}<button class="x" aria-label="Unselect {a}" onclick={() => picked.delete(a)}><Icon name="x" size={11} /></button></span>
          {/each}
        </div>
      {:else}
        <span class="muted small">nothing yet</span>
      {/if}
    </div>
    <div class="targets" role="group" aria-label="Machines to add to">
      <span class="label">Add to</span>
      {#each editable as h (h.name)}
        <label class="target" class:on={targets.has(h.name)} title="Kept in {h.managedFile}">
          <input type="checkbox" checked={targets.has(h.name)} onchange={() => toggle(targets, h.name)} />
          {h.name}
        </label>
      {:else}
        <span class="muted small">No machine is Editable here yet.</span>
      {/each}
      {#if !targets.size && editable.length}<span class="hint small">Pick at least one machine.</span>{/if}
    </div>
    {#if codeOnly.length}
      <p class="code-only small">
        <span class="label">Code only</span>
        <span class="muted"
          >{codeOnly.map((h) => h.name).join(', ')}: add packages in {codeOnly.length === 1 ? 'its' : 'their'} Nix files, or
          <a href={machineLink(codeOnly[0].name)} onclick={() => el.close()}>turn on editing</a>.</span>
      </p>
    {/if}
    <div class="buttons">
      {#if skipped}<span class="muted small">{plural(skipped, 'package')} already there will be skipped.</span>{:else if fleet.computing}<span class="muted small">Machines are re-evaluating; you can still add.</span>{/if}
      <span class="spacer"></span>
      <button onclick={() => el.close()}>Cancel</button>
      <button class="primary" disabled={!picked.size || !targets.size || adding || plan.every((p) => !p.attrs.length)} onclick={add}>
        {adding ? 'Adding…' : !picked.size ? 'Add packages' : !targets.size ? 'Choose a machine' : `Add ${plural(picked.size, 'package')} to ${plural(targets.size, 'machine')}`}
      </button>
    </div>
  </footer>
</dialog>

<style>
  .ps {
    display: flex;
    flex-direction: column;
    width: min(760px, 94vw);
    height: min(640px, 88vh);
    padding: 0;
    color: var(--fg);
    background: var(--bg-side);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--edge), var(--shadow-3);
  }
  .ps::backdrop {
    background: rgb(4 6 9 / 0.6);
  }
  header {
    display: flex;
    align-items: baseline;
    gap: var(--s3);
    padding: 14px 12px 10px 18px;
  }
  header .icon {
    align-self: center;
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .search-row {
    position: relative;
    padding: 0 18px 12px;
    border-bottom: 1px solid var(--border);
  }
  .s-ico {
    position: absolute;
    top: 9px;
    left: 29px;
    color: var(--fg-muted);
  }
  .search-row input {
    height: 34px;
    padding-left: 34px;
    font-size: var(--fs-lg);
  }
  .results {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .pad {
    padding: 18px;
  }
  .list {
    margin: 0;
    padding: 6px;
    list-style: none;
  }
  .row {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 9px 12px;
    border-radius: var(--r);
    cursor: pointer;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .row.picked {
    background: color-mix(in srgb, var(--accent-strong) 16%, transparent);
  }
  .row input {
    margin-top: 2px;
  }
  .skel-row {
    cursor: default;
    padding: 14px 12px;
  }
  .main {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }
  .top {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .attr {
    color: var(--fg-strong);
    font-size: 13px;
  }
  .ver {
    color: var(--fg-muted);
    font-size: 12px;
  }
  .desc {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .indexing {
    display: grid;
    gap: 12px;
    padding: 18px;
  }
  .indexing p {
    line-height: 1.55;
  }
  .log {
    display: flex;
    flex-direction: column;
    height: 180px;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  footer {
    display: grid;
    gap: 10px;
    padding: 12px 18px 14px;
    background: var(--bg-raised);
    border-top: 1px solid var(--border);
    border-radius: 0 0 var(--r-lg) var(--r-lg);
  }
  .picked-row,
  .targets {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .label {
    width: 64px;
  }
  .target {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 26px;
    padding: 0 10px;
    border: 1px solid var(--line);
    border-radius: 13px;
    cursor: pointer;
  }
  .target.on {
    color: var(--fg-strong);
    background: color-mix(in srgb, var(--accent-strong) 22%, transparent);
    border-color: color-mix(in srgb, var(--accent) 45%, transparent);
  }
  .hint {
    color: var(--yellow);
  }
  .code-only {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin: 0;
  }
  .row.have .attr {
    color: var(--fg-muted);
  }
  .top {
    flex-wrap: wrap;
  }
  .buttons {
    display: flex;
    align-items: center;
    gap: 8px;
  }
</style>
