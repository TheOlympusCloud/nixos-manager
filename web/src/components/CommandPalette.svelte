<script lang="ts" module>
  export const palette = $state({ open: false })
  export const togglePalette = () => (palette.open = !palette.open)
</script>

<script lang="ts">
  import { tick } from 'svelte'
  import { api, type OptionInfo } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { openPackageSearch } from '../lib/dialogs.svelte'
  import { fleet, healthy } from '../lib/fleet.svelte'
  import { highlight, rank, type Match } from '../lib/palette'
  import { pullOrPush } from '../lib/repo.svelte'
  import { codeLink, go, link, machineLink, type View } from '../lib/router.svelte'
  import { app, checkStaged, logout, startJob } from '../lib/state.svelte'
  import Dither from './cell/Dither.svelte'
  import Icon, { type IconName } from './Icon.svelte'

  type Item = { id: string; group: string; label: string; detail?: string; keywords?: string; icon: IconName; mono?: boolean; run: () => void }

  let dialog = $state<HTMLDialogElement>()
  let input = $state<HTMLInputElement>()
  let list = $state<HTMLElement>()
  let query = $state('')
  let cursor = $state(0)
  let returnFocus: HTMLElement | null = null

  const order = ['Recent', 'Actions', 'Views', 'Machines', 'Packages', 'Services and programs', 'Options', 'Files']
  const limits: Record<string, number> = { Machines: 6, Packages: 6, 'Services and programs': 6, Options: 8, Files: 8 }

  const views: [View, string, IconName, string][] = [
    ['fleet', 'Fleet', 'fleet', 'graph map overview'],
    ['machines', 'Machines', 'server', 'hosts computers'],
    ['packages', 'Packages', 'package', 'software matrix'],
    ['services', 'Services', 'services', 'programs features matrix'],
    ['modules', 'Modules', 'box', 'fleet modules bundles roles enable per machine options'],
    ['options', 'Options', 'sliders', 'settings nixos configure'],
    ['health', 'Health', 'activity', 'insights findings fix security problems'],
    ['deployments', 'Deployments', 'deploy', 'comin rollout deployed commit machines behind failed report token'],
    ['dependencies', 'Dependencies', 'grid', 'installed closure store paths libraries versions nix-tree why depends'],
    ['inputs', 'Inputs', 'input', 'flake lock nixpkgs update'],
    ['changes', 'Changes', 'diff', 'commit stage diff working copy'],
    ['history', 'History', 'history', 'log commits revert'],
    ['code', 'Code', 'code', 'editor files'],
    ['settings', 'Settings', 'settings', 'tokens mcp remote shortcuts'],
  ]

  const staged = $derived(!!app.status?.files.some((f) => f.staged))
  const actions = $derived<Item[]>([
    { id: 'a:new-machine', group: 'Actions', label: 'New machine…', keywords: 'add host create', icon: 'plus', run: () => go('#/machines?new=1') },
    {
      id: 'a:check',
      group: 'Actions',
      label: 'Run checks',
      detail: staged ? 'On the staged changes' : 'Stage changes first',
      keywords: 'flake check build test',
      icon: 'play',
      run: () => (staged ? checkStaged() : go('#/changes')),
    },
    { id: 'a:add-packages', group: 'Actions', label: 'Add packages…', keywords: 'install software', icon: 'package', run: () => openPackageSearch() },
    { id: 'a:update', group: 'Actions', label: 'Update all flake inputs', keywords: 'nix flake update nixpkgs upgrade', icon: 'refresh', run: () => (go(link('inputs')), startJob({ kind: 'update', inputs: [] })) },
    { id: 'a:pull', group: 'Actions', label: 'Pull', detail: app.info?.remote ? 'From the remote' : 'No remote configured', keywords: 'git fetch sync', icon: 'pull', run: () => pullOrPush('pull') },
    { id: 'a:push', group: 'Actions', label: 'Push', detail: app.info?.remote ? 'The machines deploy what is pushed' : 'No remote configured', keywords: 'git deploy sync', icon: 'push', run: () => pullOrPush('push') },
    { id: 'a:health', group: 'Actions', label: 'Open health', detail: 'Findings and one-click fixes', keywords: 'insights problems fix', icon: 'activity', run: () => go(link('health')) },
    { id: 'a:commit', group: 'Actions', label: 'Commit…', keywords: 'save changes', icon: 'commit', run: () => go('#/changes') },
    { id: 'a:logout', group: 'Actions', label: 'Log out', keywords: 'sign out', icon: 'logout', run: () => logout() },
  ])

  const modelItems = $derived.by<Item[]>(() => {
    const m = fleet.model
    if (!m) return []
    const out: Item[] = []
    for (const h of m.hosts)
      out.push({ id: 'm:' + h.name, group: 'Machines', label: h.name, detail: h.error ? 'Does not evaluate' : `NixOS ${h.release} · ${h.system}`, keywords: h.hostName, icon: 'server', run: () => go(machineLink(h.name)) })
    const pk = new Map<string, string[]>()
    const ft = new Map<string, { group: string; desc: string; hosts: string[] }>()
    for (const h of m.hosts) {
      if (h.error) continue
      for (const p of h.packages) pk.set(p.name, [...(pk.get(p.name) ?? []), h.name])
      for (const f of h.features) {
        const e = ft.get(f.name) ?? { group: f.group, desc: f.description, hosts: [] }
        e.hosts.push(h.name)
        ft.set(f.name, e)
      }
    }
    for (const [name, hosts] of pk)
      out.push({ id: 'p:' + name, group: 'Packages', label: name, mono: true, detail: `on ${hosts.join(', ')}`, icon: 'package', run: () => go(link('packages', [], { q: name })) })
    for (const [name, e] of ft)
      out.push({ id: 'f:' + name, group: 'Services and programs', label: name, detail: `${e.group} · on ${e.hosts.join(', ')}`, keywords: e.desc, icon: e.group === 'programs' ? 'terminal' : 'services', run: () => go(link('services', [], { q: name })) })
    return out
  })

  const fileItems = $derived<Item[]>(
    app.entries.filter((e) => !e.dir).map((e) => ({ id: 'file:' + e.path, group: 'Files', label: e.path, mono: true, icon: 'file', run: () => go(codeLink(e.path)) })),
  )

  // ---- NixOS options, searched on the server as you type ----
  let options = $state<OptionInfo[]>([])
  let optionNote = $state('')
  let optSeq = 0
  const optionHost = $derived(healthy()[0]?.name ?? '')
  $effect(() => {
    const q = query.trim()
    const host = optionHost
    const mine = ++optSeq
    if (!palette.open || q.length < 3 || !host || /\s/.test(q)) {
      options = []
      optionNote = ''
      return
    }
    optionNote = 'Searching options…'
    const t = setTimeout(async () => {
      try {
        const r = await api.searchOptions(host, q, { limit: 8 })
        if (mine !== optSeq) return
        options = r.state === 'ready' ? (r.results ?? []) : []
        optionNote = r.state === 'ready' ? '' : 'The option index is being built; try again in a minute.'
      } catch {
        if (mine === optSeq) ((options = []), (optionNote = ''))
      }
    }, 160)
    return () => clearTimeout(t)
  })
  const optionItems = $derived<Item[]>(
    options.map((o) => ({
      id: 'o:' + o.name,
      group: 'Options',
      label: o.name,
      mono: true,
      detail: o.description.split(/(?<=\.)\s/)[0].slice(0, 90),
      icon: 'sliders',
      run: () => go(link('options', [optionHost], { q: o.name, o: o.name })),
    })),
  )

  // ---- recent picks ----
  const RECENT = 'nixmgr.palette.recent'
  let recentIds = $state<string[]>(JSON.parse(localStorage.getItem(RECENT) ?? '[]'))
  function remember(id: string) {
    recentIds = [id, ...recentIds.filter((x) => x !== id)].slice(0, 6)
    localStorage.setItem(RECENT, JSON.stringify(recentIds))
  }

  const viewItems = $derived<Item[]>(views.map(([v, label, icon, keywords]) => ({ id: 'view:' + v, group: 'Views', label, keywords, icon, run: () => go(link(v)) })))
  const all = $derived([...actions, ...viewItems, ...modelItems, ...fileItems])

  const results = $derived.by<(Item & { match: Match })[]>(() => {
    const q = query.trim()
    if (!q) {
      const byId = new Map(all.map((i) => [i.id, i]))
      const recent = recentIds.flatMap((id) => (byId.has(id) ? [{ ...byId.get(id)!, group: 'Recent', match: { score: 0, hits: [] } }] : []))
      const shown = new Set(recentIds)
      return [...recent, ...[...actions, ...viewItems].filter((i) => !shown.has(i.id)).map((i) => ({ ...i, match: { score: 0, hits: [] } }))]
    }
    return rank([...all, ...optionItems], q, order, limits)
  })
  $effect(() => {
    void results
    cursor = 0
  })

  // palette.open drives the modal: Ctrl+K and the rail button set it, Esc and picking clear it.
  $effect(() => {
    if (palette.open && dialog && !dialog.open) {
      returnFocus = document.activeElement as HTMLElement | null
      query = ''
      dialog.showModal()
      tick().then(() => input?.focus())
    } else if (!palette.open && dialog?.open) close()
  })

  function close() {
    palette.open = false
    if (dialog?.open) dialog.close()
    returnFocus?.focus?.()
  }

  function pick(it: Item) {
    remember(it.id)
    close()
    it.run()
  }

  async function move(to: number) {
    cursor = Math.max(0, Math.min(results.length - 1, to))
    await tick()
    list?.querySelector(`#pal-${cursor}`)?.scrollIntoView({ block: 'nearest' })
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' || (e.ctrlKey && e.key === 'n')) move(cursor + 1)
    else if (e.key === 'ArrowUp' || (e.ctrlKey && e.key === 'p')) move(cursor - 1)
    else if (e.key === 'PageDown') move(cursor + 8)
    else if (e.key === 'PageUp') move(cursor - 8)
    else if (e.key === 'Home' && e.ctrlKey) move(0)
    else if (e.key === 'End' && e.ctrlKey) move(results.length - 1)
    else if (e.key === 'Enter' && results[cursor]) pick(results[cursor])
    else return
    e.preventDefault()
  }
</script>

<dialog bind:this={dialog} class="palette screen" aria-label="Command palette" onclose={() => palette.open && close()} onclick={(e) => e.target === dialog && close()}>
  <Dither />
  {#if palette.open}
    <!-- The frame's foot is the old key hints line (it was aria-hidden too). -->
    <div class="box modal" use:frame={{ kind: 'double', shadow: true, animate: true, tag: 'Esc', foot: `↑↓ move · Enter open · Esc close · Options from ${optionHost || 'the first machine'}` }}>
      <div class="search">
        <b class="prompt">λ ›</b>
        <input
          bind:this={input}
          bind:value={query}
          onkeydown={onKey}
          role="combobox"
          aria-expanded="true"
          aria-controls="pal-list"
          aria-activedescendant={results[cursor] ? 'pal-' + cursor : undefined}
          aria-autocomplete="list"
          aria-label="Search machines, packages, options, files and actions"
          placeholder="Search machines, packages, options, files, actions…"
          spellcheck="false"
          autocomplete="off" />
      </div>
      <div class="rule"></div>
      <ul class="results" id="pal-list" role="listbox" aria-label="Results" bind:this={list}>
        {#each results as r, i (r.group + r.id)}
          {#if i === 0 || results[i - 1].group !== r.group}
            <li class="group" role="presentation">{r.group === 'Recent' && !query.trim() ? 'Recent' : r.group}</li>
          {/if}
          <!-- svelte-ignore a11y_click_events_have_key_events (the input handles the keys: aria-activedescendant) -->
          <li id="pal-{i}" class="opt" role="option" aria-selected={i === cursor} onmousemove={() => (cursor = i)} onclick={() => pick(r)}>
            <span class="ico"><Icon name={r.icon} /></span>
            <span class="label" class:mono={r.mono}>
              {#each highlight(r.label, r.match.hits) as part, k (k)}{#if part.hit}<mark>{part.text}</mark>{:else}{part.text}{/if}{/each}
            </span>
            {#if r.detail}<span class="detail">{r.detail}</span>{/if}
          </li>
        {:else}
          <li class="none" role="presentation">Nothing matches “{query.trim()}”.</li>
        {/each}
        {#if optionNote}<li class="none" role="presentation">{optionNote}</li>{/if}
      </ul>
    </div>
  {/if}
</dialog>

<style>
  .box {
    align-self: start;
    width: 84ch;
    margin-top: calc(var(--lh) * 6);
  }
  .search {
    display: flex;
    gap: 1ch;
    white-space: nowrap;
  }
  /* A fixed height, so the box does not jump as results come and go; the list scrolls, not the framed box. */
  .results {
    height: min(calc(var(--lh) * 22), round(down, calc(100vh - var(--lh) * 14), var(--lh)));
    overflow: auto;
    list-style: none;
  }
  .group {
    margin-top: var(--lh);
    color: var(--fg-muted);
  }
  .group:first-child {
    margin-top: 0;
  }
  .opt {
    display: flex;
    gap: 1ch;
    padding: 0 1ch;
    white-space: nowrap;
    cursor: pointer;
  }
  .opt:hover {
    background: var(--bg-hover);
  }
  .opt[aria-selected='true'] {
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .ico {
    flex: none;
    width: 2ch;
    color: var(--accent);
  }
  .label {
    flex: none;
    max-width: 62%;
    overflow: hidden;
    color: var(--accent);
    text-overflow: ellipsis;
  }
  mark {
    color: var(--fg-strong);
    font-weight: 700;
    background: none;
  }
  .detail {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg-muted);
    text-align: right;
    text-overflow: ellipsis;
  }
  .opt[aria-selected='true'] :is(.ico, .label, .detail) {
    color: inherit;
  }
  /* A hit in the picked row is filled a step brighter, so it still shows in reverse video. */
  .opt[aria-selected='true'] mark {
    color: inherit;
    background: var(--accent-strong);
  }
  .none {
    padding: 0 1ch;
    color: var(--fg-muted);
  }
</style>
