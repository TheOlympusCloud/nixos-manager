<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type OptionInfo } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { fleet, hostByName, loadManaged, managedValue, searchIndexed, setOption } from '../lib/fleet.svelte'
  import { fillPath, matchOption, optionSegments } from '../lib/options'
  import { managedOptions, showValue } from '../lib/values'
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
  /** Names for the picked option's <name> placeholders, when it was opened by a concrete name. */
  let pickedNames = $state<string[]>([])
  let missing = $state('')
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
    missing = ''
    if (!name) return void ((picked = null), (pickedNames = []))
    const hit = results?.find((o) => o.name === name)
    if (hit) return void ((picked = hit), (pickedNames = []))
    if (picked?.name === name) return
    resolve(name)
  })
  async function resolve(name: string) {
    const still = () => alive && selected === name
    try {
      const exact = await searchIndexed(() => api.searchOptions(host, name, { exact: true }), (job) => still() && (indexJob = job), still)
      if (!still() || exact === null) return
      indexJob = ''
      if (exact[0]) return void ((picked = exact[0]), (pickedNames = []))
      // A concrete name (users.users."pablo".shell): find the declaration with <name> placeholders.
      const path = fillPath(optionSegments(name), [])
      const r = await api.searchOptions(host, path.slice(-2).join('.'), { limit: 200 })
      const near = r.state === 'ready' ? (r.results ?? []) : []
      if (!still()) return
      for (const o of near) {
        const filled = matchOption(o.name, path)
        if (filled) return void ((picked = o), (pickedNames = filled))
      }
      picked = null
      missing = name
    } catch {
      if (still()) missing = name
    }
  }

  // Already set on this machine: in its managed file, or evaluated as a feature.
  const setHere = $derived(managedOptions(fleet.managed[host]?.settings))
  const isSetHere = (o: OptionInfo) => {
    const path = fillPath(optionSegments(o.name), [])
    return setHere.some((s) => (s.path.length === path.length ? matchOption(o.name, s.path) !== null : false))
  }

  const popular = [
    'time.timeZone',
    'networking.hostName',
    'i18n.defaultLocale',
    'services.openssh.enable',
    'services.openssh.settings.PermitRootLogin',
    'networking.firewall.allowedTCPPorts',
    'networking.networkmanager.enable',
    'nix.gc.automatic',
    'nix.settings.experimental-features',
    'boot.loader.systemd-boot.enable',
    'virtualisation.docker.enable',
    'system.autoUpgrade.enable',
  ]
  const prefixes = [
    ['services', 'Daemons: SSH, web servers, media, databases'],
    ['programs', 'Programs with system-wide setup: shells, Steam, git'],
    ['networking', 'Host name, firewall, NetworkManager, interfaces'],
    ['boot', 'Boot loader, kernel, initrd'],
    ['nix', 'Garbage collection, substituters, experimental features'],
    ['users', 'Accounts, groups, SSH keys'],
    ['hardware', 'Bluetooth, graphics, firmware'],
    ['virtualisation', 'Docker, Podman, libvirt, containers'],
    ['security', 'sudo, polkit, PAM, AppArmor'],
    ['systemd', 'Units, timers, journald'],
    ['environment', 'System packages, variables, /etc files'],
    ['fonts', 'Font packages and fontconfig'],
  ] as const
  const suggestions = ['jellyfin', 'immich', 'nextcloud', 'home-assistant', 'syncthing', 'vaultwarden', 'adguardhome', 'grafana', 'docker', 'caddy']

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
    await setOption(host, fillPath(optionSegments(o.name), []), true, `Enabled ${o.name.slice(0, -'.enable'.length).split('.').slice(1).join('.')} on ${host}`)
    enabling = ''
  }
  /** Already enabled: set in the managed file, or evaluated as a feature of the machine. */
  const isOn = (o: OptionInfo) => {
    const path = fillPath(optionSegments(o.name), [])
    const [group, ...rest] = path.slice(0, -1)
    return managedValue(host, path) === true || !!hostByName(host)?.features.some((f) => f.group === group && f.name === rest.join('.'))
  }
  // Side by side the list is a framed pane; stacked in a drawer, the drawer's frame holds it.
  const pane = (node: HTMLElement) => (stacked ? undefined : frame(node))
</script>

<div class="ob" class:stacked class:has-form={!!picked}>
  <div class="list-pane" use:pane>
    <div class="search">
      <b class="prompt" aria-hidden="true">λ ›</b>
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
          <div class="log" use:frame={'square'}>{#key indexJob}<JobLog jobId={indexJob} compact />{/key}</div>
        </div>
      {:else if loading && !results}
        <div class="pad skels">{#each [0, 1, 2, 3, 4] as i (i)}<span class="skel" style:width="{16 + ((i * 7) % 20)}ch"></span>{/each}</div>
      {:else if results === null && enable}
        <div class="pad landing">
          <p class="muted">Search the services and programs {host} can turn on, by name or by what they do.</p>
          <div class="chips">{#each suggestions as q (q)}<button class="chip g-services" onclick={() => (query = q)}>{q}</button>{/each}</div>
        </div>
      {:else if results === null}
        <div class="landing">
          {#if setHere.length}
            <section>
              <h4>Set here on {host} <span class="num">{setHere.length}</span></h4>
              <ul>
                {#each setHere as o (o.name)}
                  <li class:sel={selected === o.name}>
                    <button class="o-row bare" onclick={() => (selected = o.name)}>
                      <span class="o-name mono">{o.name}</span>
                      <span class="o-type mono">{showValue(o.value)}</span>
                    </button>
                  </li>
                {/each}
              </ul>
            </section>
          {/if}
          <section>
            <h4>Popular</h4>
            <ul>
              {#each popular as name (name)}
                <li class:sel={selected === name}>
                  <button class="o-row bare" onclick={() => (selected = name)}><span class="o-name mono">{name}</span></button>
                </li>
              {/each}
            </ul>
          </section>
          <section>
            <h4>Browse</h4>
            <ul class="prefixes">
              {#each prefixes as [p, note] (p)}
                <li>
                  <button class="o-row bare" onclick={() => (query = p + '.')}>
                    <span class="o-name mono">{p}.</span>
                    <span class="o-type">{note}</span>
                  </button>
                </li>
              {/each}
            </ul>
          </section>
        </div>
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
                  <button class="o-row bare" aria-current={picked?.name === o.name} onclick={() => (selected = o.name)}>
                    <span class="o-line"><span class="o-name mono">{rest(o.name, group)}</span>{#if isSetHere(o)}<span class="mark here">Set here</span>{:else if o.name.endsWith('.enable') && isOn(o)}<span class="mark">On</span>{/if}</span>
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
        {#if stacked}<button class="ghost back" onclick={() => (selected = '')}><Icon name="left" /> Back to results</button>{/if}
        {#key host + picked.name + pickedNames.join('\0')}<OptionForm {host} option={picked} initialNames={pickedNames} />{/key}
      {:else if missing}
        <div class="empty-form muted" role="status" use:frame={'dash'}>
          <Icon name="search" />
          <p>{host} has no option called <span class="mono">{missing}</span>. A module it does not import may declare it.</p>
        </div>
      {:else}
        <div class="empty-form muted" use:frame={'dash'}>
          <Icon name="sliders" />
          <p>Pick an option to read its documentation and set it on {host}.</p>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .ob {
    display: grid;
    grid-template-columns: 50ch minmax(0, 1fr);
    gap: 0 2ch;
    height: 100%;
    min-height: 0;
  }
  .ob.stacked {
    grid-template-columns: minmax(0, 1fr);
  }
  .ob.stacked.has-form .list-pane,
  .ob.stacked:not(.has-form) .form-pane {
    display: none;
  }
  .list-pane {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .search {
    display: flex;
    flex: none;
    gap: 1ch;
    margin-bottom: var(--lh);
  }
  .search .prompt {
    flex: none;
    white-space: nowrap;
  }
  .results {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .pad {
    padding: 0 1ch;
  }
  .skels {
    display: grid;
    gap: var(--lh);
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
  section + section {
    margin-top: var(--lh);
  }
  h4 {
    position: sticky;
    top: 0;
    z-index: 1;
    padding: 0 1ch;
    color: var(--fg-muted);
    font-weight: 400;
    background: var(--bg);
  }
  /* In a drawer the sticky heading covers rows in the drawer's colour. */
  .stacked h4 {
    background: var(--bg-raised);
  }
  h4 .num {
    margin-left: 1ch;
    color: var(--fg-strong);
    font-weight: 700;
  }
  /* Rows fill under the pointer; the open option is reverse video. */
  /* The name keeps its width (wrapping only when it alone is too long); the type gets what is left. */
  .o-row {
    display: flex;
    gap: 2ch;
    width: 100%;
    height: auto;
    padding: 0 1ch;
    color: var(--fg);
    font-weight: 400;
    text-align: left;
    white-space: normal;
  }
  .o-row:hover:not(:disabled) {
    color: var(--fg);
    background: var(--bg-hover);
  }
  .sel .o-row,
  .sel .o-row:hover:not(:disabled) {
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .o-row:focus-visible :is(.o-name, .o-type, .mark),
  .sel .o-row :is(.o-name, .o-type, .mark) {
    color: inherit;
  }
  .o-name {
    color: var(--accent);
    overflow-wrap: anywhere;
  }
  .o-line {
    display: flex;
    flex: 0 1 auto;
    gap: 2ch;
    min-width: 0;
  }
  .mark {
    flex: none;
    color: var(--green);
  }
  .mark::before {
    content: '● ';
  }
  .mark.here::before {
    content: '✎ ';
  }
  .landing {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .landing.pad {
    gap: var(--lh);
  }
  .o-row .o-type {
    flex: 1 1 0;
  }
  .o-type,
  .o-desc {
    overflow: hidden;
    min-width: 0;
    max-width: 100%;
    color: var(--fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .flat > li + li {
    margin-top: var(--lh);
  }
  .en-row {
    display: flex;
    align-items: baseline;
    gap: 2ch;
    padding: 0 1ch;
  }
  .en-row:hover {
    background: var(--bg-hover);
  }
  .o-main {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .form-pane {
    min-width: 0;
    overflow: auto;
  }
  .back {
    margin-bottom: var(--lh);
  }
  .empty-form {
    display: grid;
    justify-items: center;
    gap: var(--lh);
    max-width: 40ch;
    margin: calc(var(--lh) * 8) auto 0;
    text-align: center;
  }
  .empty-form :global(.ic) {
    color: var(--accent);
  }
</style>
