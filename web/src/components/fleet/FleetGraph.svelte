<script lang="ts">
  import { Background, Controls, MiniMap, Position, SvelteFlow, useSvelteFlow, type Edge, type Node } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/style.css'
  import './fleet.css'
  import { tick } from 'svelte'
  import type { Model } from '../../lib/api'
  import { buildGraph, defaultLayers, related, searchModel, type GNode, type Hit, type Layers } from '../../lib/graph'
  import { fleet, loadManaged } from '../../lib/fleet.svelte'
  import { plural } from '../../lib/format'
  import { graphUi, toggleExpanded } from '../../lib/graphui.svelte'
  import { layout, type Direction } from '../../lib/layout'
  import { groupLabel, groupOrder } from '../../lib/matrix'
  import HealthSummary from '../HealthSummary.svelte'
  import Icon, { type IconName } from '../Icon.svelte'
  import Backdrop from './Backdrop.svelte'
  import GroupNode from './GroupNode.svelte'
  import Inspector from './Inspector.svelte'
  import MachineNode from './MachineNode.svelte'
  import PillNode from './PillNode.svelte'

  let { model, active }: { model: Model; active: boolean } = $props()

  const saved = (() => {
    try {
      return JSON.parse(localStorage.getItem('nixmgr.graph') ?? '{}')
    } catch {
      return {}
    }
  })()
  let layers = $state<Layers>({ ...defaultLayers, ...saved.layers })
  let dir = $state<Direction>(saved.dir === 'DOWN' ? 'DOWN' : 'RIGHT')
  $effect(() => localStorage.setItem('nixmgr.graph', JSON.stringify({ layers, dir })))

  const layerList: { key: keyof Layers; label: string; short?: string; icon: IconName; title: string }[] = [
    { key: 'features', label: 'Services & programs', short: 'Services', icon: 'services', title: 'Services, programs, virtualisation and hardware. Shared ones are nodes; the rest sit in their machine' },
    { key: 'files', label: 'Files', icon: 'file', title: 'Repository files shared by several machines' },
    { key: 'packages', label: 'Packages', icon: 'package', title: 'Installed packages, grouped per machine' },
    { key: 'users', label: 'Users', icon: 'users', title: 'Users and their home-manager configuration' },
    { key: 'inputs', label: 'Inputs', icon: 'input', title: 'Flake inputs feeding every machine' },
  ]

  // The inspector's switches read the managed files (loadManaged shares requests already in flight).
  $effect(() => {
    for (const h of model.hosts) if (h.managedFile && !fleet.managed[h.name]) loadManaged(h.name)
  })

  const flow = useSvelteFlow()
  // Pixels around the fleet when fitting it in view.
  const PAD = { top: 28, right: 36, bottom: 28, left: 36 }
  // Below this zoom the text is too small to read: pan from the machines instead.
  const MIN_FIT = 0.75
  const graph = $derived(buildGraph(model, layers, graphUi.expanded))
  const byId = $derived(new Map(graph.nodes.map((n) => [n.id, n])))
  /** A drawn node, or a feature drawn as a chip in its machine. */
  const lookup = (id: string): GNode | undefined => byId.get(id) ?? graph.solo.get(id)
  let nodes = $state.raw<Node[]>([])
  let laidOut = $state(false)
  let layoutError = $state('')
  let layoutRun = $state(0)
  /** A search result to center on once the layout that draws it lands. */
  let pendingAt = ''

  const nodeTypes = { machine: MachineNode, pkgs: GroupNode, pill: PillNode }
  const flowType = (n: GNode) => (n.kind === 'machine' ? 'machine' : n.kind === 'pkgs' ? 'pkgs' : 'pill')

  async function fit(duration = 300) {
    if (!nodes.length) return
    await flow.fitView({ padding: PAD, duration, maxZoom: 1.1 })
    if (flow.getZoom() >= MIN_FIT - 0.01) return
    // Too big to fit legibly: show the machines at a readable size; the rest is a pan away.
    const machines = nodes.filter((n) => (n.data as GNode).kind === 'machine')
    const b = flow.getNodesBounds(machines.length ? machines : nodes)
    await flow.setCenter(b.x + b.width / 2, b.y + b.height / 2, { zoom: MIN_FIT, duration })
  }

  function centerOn(id: string) {
    const node = nodes.find((x) => x.id === id)
    const g = node && (node.data as GNode)
    if (!node || !g) return false
    // Shifted right by half the details panel, so the node lands in the visible part of the canvas.
    flow.setCenter(node.position.x + g.width / 2 + 180 / 1.15, node.position.y + g.height / 2, { zoom: 1.15, duration: 450 })
    return true
  }

  // Lay the graph out again whenever what it shows changes; keep the viewport unless the set of machines changed.
  let seq = 0
  let lastShape = ''
  $effect(() => {
    const g = graph
    const d = dir
    const inputs = layers.inputs
    void layoutRun
    const mine = ++seq
    layout(g.nodes, g.edges, d, inputs).then(
      async (pos) => {
        if (mine !== seq) return
        layoutError = ''
        const [source, target] = d === 'RIGHT' ? [Position.Right, Position.Left] : [Position.Bottom, Position.Top]
        nodes = g.nodes.map((n) => ({
          id: n.id,
          type: flowType(n),
          position: pos.get(n.id) ?? { x: 0, y: 0 },
          data: n,
          width: n.width,
          height: n.height,
          sourcePosition: source,
          targetPosition: target,
          connectable: false,
          deletable: false,
          ariaLabel: `${n.kind === 'pkgs' ? 'packages of ' + n.host : n.kind} ${n.kind === 'pkgs' ? '' : n.label}`.trim(),
        }))
        laidOut = true
        const shape = `${d}|${Object.values(layers).join()}|${model.hosts.map((h) => h.name).join()}`
        const changed = shape !== lastShape
        lastShape = shape
        if (!changed && !pendingAt) return
        await tick()
        // Let Svelte Flow measure the new nodes first, or it fits the old bounds.
        setTimeout(() => {
          if (pendingAt && centerOn(pendingAt)) pendingAt = ''
          else if (changed) fit()
        }, 60)
      },
      (e) => {
        if (mine === seq) layoutError = e instanceof Error ? e.message : String(e)
      },
    )
  })

  // Hover wins over selection for what lights up; a chip feature lights its machine.
  const focus = $derived(graphUi.hover || graphUi.selected)
  const rel = $derived.by(() => {
    if (!focus) return null
    if (byId.has(focus)) return related(graph.edges, focus)
    const chip = graph.solo.get(focus)
    if (!chip) return null
    const r = related(graph.edges, 'm:' + chip.hosts[0].host)
    r.nodes.add(focus)
    return r
  })
  $effect(() => {
    graphUi.lit = rel?.nodes ?? null
  })
  const edges = $derived<Edge[]>(
    graph.edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      selectable: false,
      focusable: false,
      deletable: false,
      class: [e.group && 'grouped', rel && (rel.edges.has(e.id) ? 'hl' : 'dim')].filter(Boolean).join(' '),
      style: e.group ? `--c: var(--g-${e.group})` : undefined,
    })),
  )

  // Drop a selection whose node disappeared (layer turned off, machine removed).
  $effect(() => {
    if (graphUi.selected && !lookup(graphUi.selected)) graphUi.selected = ''
  })
  const selected = $derived(lookup(graphUi.selected) ?? null)

  // ---- search: the whole model, turning on whatever layer draws the result ----
  let query = $state('')
  let searchOpen = $state(false)
  let cursor = $state(0)
  const matches = $derived(searchModel(model, query))

  function jump(hit: Hit) {
    query = ''
    searchOpen = false
    let redraw = false
    if (hit.layer && !layers[hit.layer]) {
      layers[hit.layer] = true
      redraw = true
    }
    if (hit.expand && !hit.expand.some((h) => graphUi.expanded.includes(h))) {
      toggleExpanded(hit.expand[0])
      redraw = true
    }
    graphUi.selected = hit.select
    if (redraw || !centerOn(hit.at)) pendingAt = hit.at
  }

  function searchKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') cursor = Math.min(cursor + 1, matches.length - 1)
    else if (e.key === 'ArrowUp') cursor = Math.max(cursor - 1, 0)
    else if (e.key === 'Enter' && matches[cursor]) jump(matches[cursor])
    else if (e.key === 'Escape') {
      query = ''
      ;(e.currentTarget as HTMLElement).blur()
    } else return
    e.preventDefault()
  }

  function onKey(e: KeyboardEvent) {
    if (!active || e.key !== 'Escape' || !graphUi.selected) return
    const t = e.target as HTMLElement
    if (t.closest('input, textarea, select, dialog')) return
    graphUi.selected = ''
  }

  const minimapColor = (n: Node) => {
    const g = n.data as GNode
    if (g.kind === 'machine') return g.host.error ? '#8a3a40' : '#4a6a90'
    if (g.kind === 'feature') return `var(--g-${g.group})`
    return '#3a4654'
  }

  const stats = $derived.by(() => {
    const ok = model.hosts.filter((h) => !h.error)
    return [
      plural(model.hosts.length, 'machine'),
      `${ok.filter((h) => h.managedFile).length} editable here`,
      plural(new Set(ok.flatMap((h) => h.features.map((f) => f.group + '.' + f.name))).size, 'feature'),
      plural(new Set(ok.flatMap((h) => h.packages.map((p) => p.name))).size, 'package'),
    ].join(' · ')
  })
  const nextDir = $derived(dir === 'RIGHT' ? { to: 'DOWN' as const, icon: 'dir-down' as const, label: 'Lay out top to bottom' } : { to: 'RIGHT' as const, icon: 'dir-right' as const, label: 'Lay out left to right' })
</script>

<svelte:window onkeydown={onKey} />

<div class="fleet">
  <div class="bar">
    <div class="search-box">
      <span class="search-ico"><Icon name="search" size={14} /></span>
      <input
        class="search"
        type="search"
        placeholder="Search the fleet…"
        aria-label="Find anything in the fleet"
        role="combobox"
        aria-expanded={searchOpen && matches.length > 0}
        aria-controls="fleet-search-list"
        aria-activedescendant={matches[cursor] ? 'fs-' + cursor : undefined}
        autocomplete="off"
        spellcheck="false"
        data-search
        bind:value={query}
        oninput={() => ((cursor = 0), (searchOpen = true))}
        onfocus={() => (searchOpen = true)}
        onblur={() => setTimeout(() => (searchOpen = false), 120)}
        onkeydown={searchKey} />
      {#if searchOpen && query.trim()}
        <ul class="results" id="fleet-search-list" role="listbox">
          {#each matches as m, i (m.kind + m.select + m.label)}
            <li id="fs-{i}" role="option" aria-selected={i === cursor}>
              <button class="ghost" tabindex="-1" onmousedown={(e) => e.preventDefault()} onclick={() => jump(m)}>
                <span class="r-text">
                  <span class="r-label" class:mono={m.kind === 'File' || m.kind === 'Package'}>{m.label}</span>
                  {#if m.sub}<span class="r-sub">{m.sub}</span>{/if}
                </span>
                <span class="r-kind">{m.kind}</span>
              </button>
            </li>
          {:else}
            <li class="none">Nothing in the fleet matches.</li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="layers" role="group" aria-label="Layers">
      {#each layerList as l (l.key)}
        <button class="layer" class:on={layers[l.key]} aria-pressed={layers[l.key]} aria-label={l.label} title={l.title} onclick={() => (layers[l.key] = !layers[l.key])}>
          <Icon name={l.icon} size={14} /><span class="layer-label" class:long={!!l.short}>{l.label}</span>{#if l.short}<span class="layer-label short">{l.short}</span>{/if}
        </button>
      {/each}
    </div>

    {#if layers.features}
      <ul class="legend" aria-label="Feature colours">
        {#each groupOrder as g (g)}<li class="lg g-{g}" title={groupLabel[g]}><span class="dot"></span><span class="lg-name">{groupLabel[g]}</span></li>{/each}
      </ul>
    {/if}

    <span class="spacer"></span>
    <HealthSummary />
    <span class="stats muted small num">{stats}</span>
    <button class="icon" title={nextDir.label} aria-label={nextDir.label} onclick={() => (dir = nextDir.to)}>
      <Icon name={nextDir.icon} />
    </button>
    <button class="icon" title="Fit the fleet in view" aria-label="Fit view" onclick={() => fit()}>
      <Icon name="fit" />
    </button>
  </div>

  <div class="canvas" class:ready={laidOut}>
    <Backdrop view={() => flow.getViewport()} {active} />
    <SvelteFlow
      class="fleet-flow dark"
      bind:nodes
      {edges}
      {nodeTypes}
      minZoom={0.15}
      maxZoom={2}
      nodesConnectable={false}
      zoomOnDoubleClick={false}
      proOptions={{ hideAttribution: true }}
      onnodeclick={({ node }) => (graphUi.selected = node.id)}
      onselectionchange={({ nodes: sel }) => sel.length === 1 && (graphUi.selected = sel[0].id)}
      onnodepointerenter={({ node }) => (graphUi.hover = node.id)}
      onnodepointerleave={() => (graphUi.hover = '')}
      onpaneclick={() => (graphUi.selected = '')}>
      <Background gap={22} size={1.2} />
      <Controls position="bottom-left" showLock={false} showFitView={false} />
      <MiniMap position="bottom-right" pannable zoomable nodeColor={minimapColor} nodeBorderRadius={6} width={180} height={120} />
    </SvelteFlow>
    {#if !laidOut}
      <div class="placeholder" role={layoutError ? 'alert' : undefined} aria-hidden={layoutError ? undefined : 'true'}>
        {#if layoutError}
          <div class="layout-error">
            <p>{layoutError}</p>
            <button onclick={() => layoutRun++}><Icon name="refresh" size={14} />Try again</button>
          </div>
        {:else}
          {#each [0, 1, 2] as i (i)}<span class="skel ghost-card" style:top="{18 + i * 26}%"></span>{/each}
        {/if}
      </div>
    {:else if layoutError}
      <p class="layout-warn" role="alert">{layoutError} <button class="link" onclick={() => layoutRun++}>Try again</button></p>
    {/if}
    {#if selected}<Inspector node={selected} {model} onclose={() => (graphUi.selected = '')} />{/if}
  </div>
</div>

<style>
  .fleet {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: var(--s2);
    min-height: 52px;
    position: relative;
    z-index: 2; /* its search results open over the canvas */
    padding: 10px var(--s4);
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    container-type: inline-size;
  }
  .search-box {
    position: relative;
    flex: 0 1 240px;
    min-width: 170px;
  }
  .layers,
  .legend,
  .stats,
  .bar > .icon {
    flex: none;
  }
  .search-ico {
    position: absolute;
    top: 8px;
    left: 10px;
    color: var(--fg-muted);
    pointer-events: none;
  }
  .search {
    padding-left: 30px;
  }
  .results {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    z-index: 20;
    width: 360px;
    margin: 0;
    padding: 4px;
    list-style: none;
    background: var(--bg-raised);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-2);
  }
  .results button {
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    height: auto;
    min-height: 32px;
    padding: 5px 8px;
    font-weight: 400;
    text-align: left;
  }
  .results [aria-selected='true'] button {
    background: var(--bg-active);
  }
  .r-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .r-label {
    overflow: hidden;
    color: var(--fg-strong);
    text-overflow: ellipsis;
  }
  .r-sub {
    overflow: hidden;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    text-overflow: ellipsis;
  }
  .r-kind {
    flex: none;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
  }
  .none {
    padding: 8px;
    color: var(--fg-muted);
  }
  .layers {
    display: flex;
    gap: 4px;
    padding: 3px;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  .layer {
    height: 26px;
    padding: 0 10px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    background: none;
    border-color: transparent;
  }
  .layer:hover:not(:disabled) {
    color: var(--fg-strong);
    background: var(--bg-hover);
    border-color: transparent;
  }
  .layer.on {
    color: var(--fg-strong);
    background: var(--bg-active);
    border-color: var(--line);
  }
  .layer.on :global(svg) {
    color: var(--accent);
  }
  .legend {
    display: flex;
    gap: 10px;
    margin: 0 0 0 4px;
    padding: 0;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    list-style: none;
  }
  .lg {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .lg .dot {
    width: 7px;
    height: 7px;
    background: var(--c);
  }
  .canvas {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    background: radial-gradient(120% 90% at 50% 110%, color-mix(in srgb, var(--accent-strong) 16%, var(--bg-deep)), var(--bg-deep) 62%);
  }
  .canvas :global(.svelte-flow) {
    opacity: 0;
    transition: opacity var(--t) var(--ease);
  }
  .canvas.ready :global(.svelte-flow) {
    opacity: 1;
  }
  .placeholder {
    position: absolute;
    inset: 0;
  }
  .ghost-card {
    position: absolute;
    left: 8%;
    width: 240px;
    height: 120px;
    border-radius: 10px;
    opacity: 0.5;
  }
  .layout-error {
    display: grid;
    justify-items: start;
    gap: var(--s3);
    max-width: 440px;
    margin: 18vh auto 0;
    color: var(--fg);
  }
  .layout-warn {
    position: absolute;
    top: 12px;
    left: 50%;
    z-index: 5;
    padding: 6px 12px;
    color: var(--fg);
    font-size: var(--fs-sm);
    background: var(--bg-raised);
    border: 1px solid color-mix(in srgb, var(--red) 35%, var(--line));
    border-radius: var(--r);
    transform: translateX(-50%);
  }
  .layer-label.short {
    display: none;
  }
  /* Narrower bars drop the least useful text first, so the toolbar never wraps (widths measured). */
  @container (max-width: 1740px) {
    .stats {
      display: none;
    }
  }
  @container (max-width: 1420px) {
    .lg-name {
      display: none;
    }
    .legend {
      gap: 7px;
    }
  }
  @container (max-width: 1200px) {
    .search-box {
      flex-basis: 180px;
    }
    .legend {
      display: none;
    }
    .layer {
      padding: 0 8px;
    }
    .layer-label.long {
      display: none;
    }
    .layer-label.short {
      display: inline;
    }
  }
  @container (max-width: 970px) {
    .layer-label,
    .layer-label.short {
      display: none;
    }
    .layer {
      width: 30px;
      padding: 0;
    }
  }
</style>
