<script lang="ts">
  import { onMount } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import { api, type PackageHit } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { pkgSearch } from '../lib/dialogs.svelte'
  import { addPackages, fleet, healthy, searchIndexed } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { isRepoFile } from '../lib/graph'
  import { machineLink } from '../lib/router.svelte'
  import Dither from './cell/Dither.svelte'
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

<dialog bind:this={el} class="ps screen" onclose={close} aria-labelledby="ps-title">
  <Dither />
  <div class="modal" use:frame={{ kind: 'double', shadow: true, animate: true, title: 'Add packages', tag: 'from the nixpkgs pinned in flake.lock' }}>
    <header class="sr-only">
      <h2 id="ps-title">Add packages</h2>
      <span>from the nixpkgs pinned in <span class="mono">flake.lock</span></span>
    </header>
    <!-- On the top line, left of the tag: ═ × ═ from the nixpkgs… -->
    <button class="icon close" aria-label="Close" onclick={() => el.close()}><Icon name="x" /></button>

    <div class="search-row">
      <b class="prompt">λ ›</b>
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
    <div class="rule"></div>

    <div class="results" aria-busy={loading}>
      {#if error}
        <p class="error-text pad" role="alert">{error}</p>
      {:else if indexJob}
        <div class="indexing" role="status">
          <p><span class="dot busy"></span> <strong>Indexing nixpkgs…</strong> The first search reads every package of the pinned nixpkgs; this takes a few minutes with real Nix. You can keep this dialog open or come back later.</p>
          <div class="log" use:frame={'square'}>{#key indexJob}<JobLog jobId={indexJob} compact />{/key}</div>
        </div>
      {:else if loading && !results}
        <ul class="list">
          {#each [0, 1, 2, 3] as i (i)}<li class="row skel-row"><span class="skel" style:width="{30 + i * 12}ch"></span></li>{/each}
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
                <span class="attr mono">{r.attr}</span><span class="ver mono">{r.version}</span>
                <span class="desc" title={r.description}>{#if r.description}{r.description}{/if}</span>
                <span class="where">{#each on as o (o.host)}<span class="badge" class:ok={o.from === 'Set here'} title="{r.pname} is already on {o.host}: {o.from}">{everywhere && on.length === 1 ? 'Already here' : `On ${o.host}`} · {o.from}</span>{/each}</span>
              </label>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="rule"></div>
    <footer>
      <div class="picked-row">
        <span class="label">Selected</span>
        {#if picked.size}
          <div class="chips">
            {#each [...picked] as a (a)}
              <span class="chip mono">{a}<button class="x icon" aria-label="Unselect {a}" onclick={() => picked.delete(a)}><Icon name="x" /></button></span>
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
        {#if !targets.size && editable.length}<span class="hint small"><i class="warn">▲</i> Pick at least one machine.</span>{/if}
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
  </div>
</dialog>

<style>
  /* A fixed height: the results scroll inside, never the framed (and shadowed) box. */
  .modal {
    display: flex;
    flex-direction: column;
    width: 96ch;
    height: min(calc(var(--lh) * 36), round(down, calc(100vh - var(--lh) * 4), var(--lh)));
  }
  .close {
    position: absolute;
    top: 0;
    right: 42ch;
    z-index: 2;
    width: 3ch;
    background: var(--bg-raised);
  }
  .search-row {
    display: flex;
    flex: none;
    gap: 1ch;
    white-space: nowrap;
  }
  .rule {
    flex: none;
  }
  .results {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .list {
    list-style: none;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
    padding: 0 1ch;
    white-space: nowrap;
    cursor: pointer;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .skel-row {
    cursor: default;
  }
  .skel-row:hover {
    background: none;
  }
  .attr {
    flex: none;
    min-width: 16ch;
    color: var(--fg-strong);
    font-weight: 700;
  }
  .ver {
    flex: none;
    width: 12ch;
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
  }
  .desc {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
  }
  /* Where a machine already has it: a second line under the name, only when there is one. */
  .where {
    display: flex;
    flex-basis: 100%;
    flex-wrap: wrap;
    gap: 0 2ch;
    padding-left: 5ch;
  }
  .row.have .attr {
    color: var(--fg-muted);
  }
  .pad,
  .indexing {
    padding: 0 1ch;
  }
  .indexing {
    display: grid;
    gap: var(--lh);
  }
  .log {
    display: flex;
    flex-direction: column;
    height: calc(var(--lh) * 10);
    overflow: hidden;
  }
  footer {
    display: grid;
    flex: none;
  }
  .picked-row,
  .targets {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .label {
    flex: none;
    width: 10ch;
  }
  .target {
    display: inline-flex;
    gap: 1ch;
    cursor: pointer;
  }
  .target.on {
    color: var(--fg-strong);
  }
  .hint {
    color: var(--yellow);
  }
  .code-only {
    display: flex;
    gap: 2ch;
  }
  .buttons {
    display: flex;
    align-items: baseline;
    gap: 2ch;
    margin-top: var(--lh);
  }
</style>
