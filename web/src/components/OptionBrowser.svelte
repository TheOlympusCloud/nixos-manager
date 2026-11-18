<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type OptionInfo } from '../lib/api'
  import { fleet, hostByName, loadManaged, managedValue, searchIndexed, setOption } from '../lib/fleet.svelte'
  import { fillPath, optionSegments } from '../lib/options'
  import Icon from './Icon.svelte'
  import JobLog from './JobLog.svelte'
  import OptionForm from './OptionForm.svelte'

  /**
   * Search a machine's NixOS options and edit one. `enable` mode lists only
   * `….enable` switches and turns them on directly (adding a service).
   */
  let {
    host,
    query = $bindable(''),
    selected = $bindable(''),
    enable = false,
    stacked = false,
  }: { host: string; query?: string; selected?: string; enable?: boolean; stacked?: boolean } = $props()

  let results = $state<OptionInfo[] | null>(null)
  let indexJob = $state('')
  let error = $state('')
  let loading = $state(false)
  let picked = $state<OptionInfo | null>(null)
  let alive = true

  onMount(() => {
    if (!fleet.managed[host]) loadManaged(host)
    return () => (alive = false)
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
      const r = await searchIndexed(() => api.searchOptions(host, q, { limit: 200 }), (job) => mine === seq && (indexJob = job), () => alive && mine === seq)
      if (mine !== seq || r === null) return
      results = enable ? r.filter((o) => o.name.endsWith('.enable') && !o.readOnly && optionSegments(o.name).every((s) => s.kind === 'attr')) : r
      indexJob = ''
    } catch (e) {
      if (mine === seq) error = e instanceof Error ? e.message : String(e)
    } finally {
      if (mine === seq) loading = false
    }
  }

  // Search again when the machine or the query (from the URL) changes.
  $effect(() => {
    void host
    void query
    timer = setTimeout(search, 200)
    return () => clearTimeout(timer)
  })

  // Open the selected option (from the URL), fetching it if it is not among the results.
  $effect(() => {
    const name = selected
    if (!name) return void (picked = null)
    const hit = results?.find((o) => o.name === name)
    if (hit) return void (picked = hit)
    if (picked?.name === name) return
    api.searchOptions(host, name, { exact: true }).then(
      (r) => {
        if (selected === name && r.state === 'ready' && r.results?.[0]) picked = r.results[0]
      },
      () => {},
    )
  })

  /** Group by the first two segments: services.openssh, programs.steam, … */
  const groups = $derived.by(() => {
    const out = new Map<string, OptionInfo[]>()
    for (const o of results ?? []) {
      const segs = optionSegments(o.name)
      const key = segs.length > 2 ? segs.slice(0, 2).map((s) => s.text).join('.') : segs[0].text
      out.set(key, [...(out.get(key) ?? []), o])
    }
    return [...out.entries()]
  })
  const rest = (name: string, group: string) => (name.startsWith(group + '.') ? name.slice(group.length + 1) : name)

  let enabling = $state('')
  async function turnOn(o: OptionInfo) {
    enabling = o.name
    await setOption(host, fillPath(optionSegments(o.name), []), true)
    enabling = ''
  }
  /** Already enabled: set in the managed file, or evaluated as a feature of the machine. */
  const isOn = (o: OptionInfo) => {
    const path = fillPath(optionSegments(o.name), [])
    const [group, ...rest] = path.slice(0, -1)
    return managedValue(host, path) === true || !!hostByName(host)?.features.some((f) => f.group === group && f.name === rest.join('.'))
  }
</script>

<div class="ob" class:stacked class:has-form={!!picked}>
  <div class="list-pane">
    <div class="search">
      <span class="s-ico"><Icon name="search" size={14} /></span>
      <input
        type="search"
        placeholder={enable ? 'Search services and programs, e.g. jellyfin' : 'Search options, e.g. services.openssh'}
        aria-label="Search NixOS options"
        spellcheck="false"
        autocomplete="off"
        data-search
        bind:value={query}
        onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), search())} />
    </div>
    <div class="results" aria-busy={loading}>
      {#if error}
        <p class="error-text pad" role="alert">{error}</p>
      {:else if indexJob}
        <div class="pad indexing" role="status">
          <p><span class="dot busy"></span> <strong>Indexing the options of {host}…</strong> This reads every option of the machine once; with real Nix it takes a minute or two.</p>
          <div class="log">{#key indexJob}<JobLog jobId={indexJob} compact />{/key}</div>
        </div>
      {:else if loading && !results}
        <div class="pad skels">{#each [0, 1, 2, 3, 4] as i (i)}<span class="skel" style:width="{40 + ((i * 17) % 50)}%"></span>{/each}</div>
      {:else if results === null}
        <p class="muted pad">Search by option name or words in its description.</p>
      {:else if !results.length}
        <p class="muted pad">No {enable ? 'service or program' : 'option'} matches “{query}”.</p>
      {:else if enable}
        <ul class="flat">
          {#each results as o (o.name)}
            <li>
              <div class="en-row">
                <span class="o-main">
                  <span class="o-name mono">{o.name.slice(0, -'.enable'.length)}</span>
                  <span class="o-desc">{o.description.split('\n')[0]}</span>
                </span>
                {#if isOn(o)}
                  <span class="badge ok">On</span>
                {:else}
                  <button class="small" disabled={!!enabling} onclick={() => turnOn(o)}>{enabling === o.name ? 'Enabling…' : 'Enable'}</button>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      {:else}
        {#each groups as [group, opts] (group)}
          <section>
            <h4 class="mono">{group}</h4>
            <ul>
              {#each opts as o (o.name)}
                <li class:sel={picked?.name === o.name}>
                  <button class="o-row" aria-current={picked?.name === o.name} onclick={() => (selected = o.name)}>
                    <span class="o-name mono">{rest(o.name, group)}</span>
                    <span class="o-type">{o.type}</span>
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      {/if}
    </div>
  </div>
  {#if !enable}
    <div class="form-pane">
      {#if picked}
        {#if stacked}<button class="ghost back" onclick={() => (selected = '')}><Icon name="left" size={14} />Back to results</button>{/if}
        {#key host + picked.name}<OptionForm {host} option={picked} />{/key}
      {:else}
        <div class="empty-form muted">
          <Icon name="sliders" size={22} />
          <p>Pick an option to read its documentation and set it on {host}.</p>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .ob {
    display: grid;
    grid-template-columns: minmax(300px, 400px) minmax(0, 1fr);
    height: 100%;
    min-height: 0;
  }
  .ob.stacked {
    grid-template-columns: minmax(0, 1fr);
  }
  .flat {
    padding-top: 6px;
  }
  .ob.stacked.has-form .list-pane,
  .ob.stacked:not(.has-form) .form-pane {
    display: none;
  }
  .list-pane {
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-right: 1px solid var(--border);
  }
  .stacked .list-pane {
    border-right: 0;
  }
  .search {
    position: relative;
    padding: 12px;
    border-bottom: 1px solid var(--border);
  }
  .s-ico {
    position: absolute;
    top: 20px;
    left: 22px;
    color: var(--fg-muted);
  }
  .search input {
    padding-left: 30px;
  }
  .results {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding-bottom: 12px;
  }
  .pad {
    padding: 14px;
  }
  .skels {
    display: grid;
    gap: 14px;
  }
  .indexing {
    display: grid;
    gap: 10px;
    line-height: 1.55;
  }
  .log {
    display: flex;
    flex-direction: column;
    height: 160px;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  h4 {
    position: sticky;
    top: 0;
    z-index: 1;
    padding: 10px 14px 4px;
    color: var(--fg-muted);
    font-size: 11.5px;
    font-weight: 500;
    background: var(--bg-side);
  }
  ul {
    margin: 0;
    padding: 0 6px;
    list-style: none;
  }
  .o-row {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    width: 100%;
    height: auto;
    padding: 6px 8px;
    font-weight: 400;
    text-align: left;
    background: none;
    border: 0;
  }
  .o-row:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .sel .o-row,
  .sel .o-row:hover:not(:disabled) {
    background: var(--bg-active);
  }
  .o-name {
    color: var(--fg-strong);
    font-size: 12.5px;
    overflow-wrap: anywhere;
  }
  .o-type,
  .o-desc {
    overflow: hidden;
    max-width: 100%;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .en-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 8px;
    border-radius: var(--r);
  }
  .en-row:hover {
    background: var(--bg-hover);
  }
  .o-main {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .o-desc {
    font-size: var(--fs-sm);
  }
  .form-pane {
    min-width: 0;
    padding: 18px 22px 28px;
    overflow: auto;
  }
  .back {
    margin: -6px 0 10px -8px;
  }
  .empty-form {
    display: grid;
    justify-items: center;
    gap: 10px;
    max-width: 280px;
    margin: 18vh auto 0;
    text-align: center;
  }
</style>
