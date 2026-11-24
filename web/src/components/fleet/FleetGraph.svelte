<script lang="ts">
  import { Background, BackgroundVariant, MiniMap, Position, SvelteFlow, useSvelteFlow, useViewport, type Edge, type Node } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/base.css'
  import './fleet.css'
  import { tick } from 'svelte'
  import type { Model } from '../../lib/api'
  import { cell, frame, on, still } from '../../lib/ascii'
  import { buildGraph, defaultLayers, packChips, related, searchModel, type GNode, type Hit, type Layers } from '../../lib/graph'
  import { fleet, loadManaged } from '../../lib/fleet.svelte'
  import { plural } from '../../lib/format'
  import { graphUi, toggleExpanded } from '../../lib/graphui.svelte'
  import { layout, type Direction } from '../../lib/layout'
  import { groupLabel, groupOrder } from '../../lib/matrix'
  import HealthSummary from '../HealthSummary.svelte'
  import Icon, { type IconName } from '../Icon.svelte'
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

  // Nodes are framed text, so they measure whole cells. lib/graph.ts estimates labels at 7.3 px a character;
  // a machine is 40 columns, which holds the chip rows packChips planned.
  const inCells = (n: GNode): GNode => {
    const cols = n.kind === 'machine' ? 40 : Math.ceil(n.width / 7.3)
    const rows = n.kind === 'machine' ? 5 + (n.chips.length ? 1 + packChips(n.chips).rows : 0) : n.height > 40 ? 4 : 3
    return { ...n, width: cols * cell.cw, height: rows * cell.lh }
  }
  // Half a cell off the background's grid, so its dots sit in the middle of a cell, as a · would.
  const snap = (p: { x: number; y: number }) => ({ x: (Math.round(p.x / cell.cw) + 0.5) * cell.cw, y: (Math.round(p.y / cell.lh) + 0.5) * cell.lh })

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
    // Shifted right by half the details panel (51 columns), so the node lands in the visible part of the canvas.
    flow.setCenter(node.position.x + g.width / 2 + (25.5 * cell.cw) / 1.15, node.position.y + g.height / 2, { zoom: 1.15, duration: 450 })
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
    const sized = g.nodes.map(inCells)
    // One row more for the layout, so snapping to rows always leaves a blank row between two nodes.
    layout(sized.map((n) => ({ ...n, height: n.height + cell.lh })), g.edges, d, inputs).then(
      async (pos) => {
        if (mine !== seq) return
        layoutError = ''
        const [source, target] = d === 'RIGHT' ? [Position.Right, Position.Left] : [Position.Bottom, Position.Top]
        nodes = sized.map((n) => ({
          id: n.id,
          type: flowType(n),
          position: snap(pos.get(n.id) ?? { x: 0, y: 0 }),
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
      type: 'step',
      selectable: false,
      focusable: false,
      deletable: false,
      class: [e.group && 'grouped', rel && (rel.edges.has(e.id) ? 'hl' : 'dim')].filter(Boolean).join(' '),
      style: e.group ? `--c: var(--g-${e.group})` : undefined,
    })),
  )
  // Lit wires carry packets: fleet.css shifts their dashes one cell per tick of the clock (still under reduced motion).
  let canvas = $state<HTMLElement>()
  $effect(() => {
    const el = canvas
    if (!rel || still || !el) return
    return on((t) => el.style.setProperty('--hop', String(t % 4)))
  })
  const viewport = useViewport()

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
    if (g.kind === 'machine') return g.host.error ? 'color-mix(in srgb, var(--red) 45%, var(--bg-raised))' : 'color-mix(in srgb, var(--accent) 35%, var(--bg-raised))'
    if (g.kind === 'feature') return `var(--g-${g.group})`
    return 'var(--line-strong)'
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
      <b class="prompt" aria-hidden="true">λ ›</b>
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
        <div class="results" use:frame={{ kind: 'square', shadow: true, animate: true }}>
          <ul class="rows" id="fleet-search-list" role="listbox">
            {#each matches as m, i (m.kind + m.select + m.label)}
              <li id="fs-{i}" role="option" aria-selected={i === cursor}>
                <button class="bare" tabindex="-1" onmousedown={(e) => e.preventDefault()} onclick={() => jump(m)}>
                  <span class="r-label" class:mono={m.kind === 'File' || m.kind === 'Package'}>{m.label}</span>
                  {#if m.sub}<span class="r-sub">{m.sub}</span>{/if}
                  <span class="r-kind">{m.kind}</span>
                </button>
              </li>
            {:else}
              <li class="none">Nothing in the fleet matches.</li>
            {/each}
          </ul>
        </div>
      {/if}
    </div>

    <div class="layers seg" role="group" aria-label="Layers">
      {#each layerList as l (l.key)}
        <button class="layer" class:on={layers[l.key]} aria-pressed={layers[l.key]} aria-label={l.label} title={l.title} onclick={() => (layers[l.key] = !layers[l.key])}>
          <Icon name={l.icon} /><span class="layer-label" class:long={!!l.short}>{l.label}</span>{#if l.short}<span class="layer-label short">{l.short}</span>{/if}
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

  <div class="canvas" class:ready={laidOut} bind:this={canvas}>
    <SvelteFlow
      class="fleet-flow"
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
      <!-- The dot grid: a dot every 4 columns and 2 rows, a cross every 16 and 8 (it pans and zooms with the graph). -->
      <Background id="minor" variant={BackgroundVariant.Dots} gap={[4 * cell.cw, 2 * cell.lh]} size={2} patternColor="var(--grid-major)" />
      <Background id="major" variant={BackgroundVariant.Cross} gap={[16 * cell.cw, 8 * cell.lh]} size={7} patternColor="var(--grid-major)" />
      <div class="zoom" role="group" aria-label="Control Panel" use:frame={'square'}>
        <button class="icon" title="Zoom In" aria-label="Zoom In" disabled={viewport.current.zoom >= 2} onclick={() => flow.zoomIn()}>+</button>
        <button class="icon" title="Zoom Out" aria-label="Zoom Out" disabled={viewport.current.zoom <= 0.15} onclick={() => flow.zoomOut()}>−</button>
      </div>
      <div class="minimap" use:frame={{ kind: 'square', title: 'map' }}>
        <MiniMap pannable zoomable nodeColor={minimapColor} nodeBorderRadius={0} width={22 * cell.cw} height={6 * cell.lh} />
      </div>
    </SvelteFlow>
    {#if !laidOut}
      <div class="placeholder" role={layoutError ? 'alert' : undefined} aria-hidden={layoutError ? undefined : 'true'}>
        {#if layoutError}
          <div class="layout-error">
            <p>{layoutError}</p>
            <button onclick={() => layoutRun++}><Icon name="refresh" /> Try again</button>
          </div>
        {:else}
          {#each [0, 1, 2] as i (i)}<span class="ghost-card" style:top="calc(var(--lh) * {1 + i * 8})" use:frame={'dash'}><span class="skel"></span></span>{/each}
        {/if}
      </div>
    {:else if layoutError}
      <p class="layout-warn" role="alert" data-tone="err" use:frame={'square'}>{layoutError} <button class="link" onclick={() => layoutRun++}>Try again</button></p>
    {/if}
    {#if selected}<Inspector node={selected} {model} onclose={() => (graphUi.selected = '')} />{/if}
  </div>
</div>

<style>
  .fleet {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: var(--lh) 1ch;
  }
  /* One row of tools over a blank one, as in a terminal. */
  .bar {
    display: flex;
    flex: none;
    align-items: baseline;
    gap: 2ch;
    height: calc(var(--lh) * 2);
    position: relative;
    z-index: 2; /* its search results open over the canvas */
    white-space: nowrap;
    container-type: inline-size;
  }
  .search-box {
    position: relative;
    display: flex;
    flex: 0 1 32ch;
    gap: 1ch;
    min-width: 20ch;
  }
  .layers,
  .legend,
  .stats,
  .bar > .icon {
    flex: none;
  }
  .bar > .icon {
    min-width: 0;
    padding: 0 1ch;
  }
  .results {
    position: absolute;
    top: var(--lh);
    left: 0;
    z-index: 20;
    width: 64ch;
    background: var(--bg-raised);
  }
  .results button {
    display: flex;
    gap: 2ch;
    width: 100%;
    color: inherit;
    font-weight: inherit;
    text-align: left;
  }
  .r-label {
    flex: none;
    max-width: 32ch;
    overflow: hidden;
    color: var(--fg-strong);
    text-overflow: ellipsis;
  }
  .r-sub {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
  }
  .r-kind {
    flex: none;
    margin-left: auto;
    color: var(--fg-faint);
  }
  [aria-selected='true'] :is(.r-label, .r-sub, .r-kind),
  .results button:is(:hover, :focus-visible) > * {
    color: inherit;
  }
  .none {
    color: var(--fg-muted);
    cursor: default;
  }
  .layer-label {
    margin-left: 1ch;
  }
  .legend {
    display: flex;
    gap: 2ch;
    color: var(--fg-muted);
  }
  .lg .dot {
    --c: inherit; /* the group's colour, from .g-* on the item */
  }
  .lg-name {
    margin-left: 1ch;
  }
  .canvas {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
  .canvas :global(.svelte-flow) {
    visibility: hidden;
  }
  .canvas.ready :global(.svelte-flow) {
    visibility: visible;
  }
  /* Zoom and map are glyph boxes over the canvas. */
  .zoom,
  .minimap {
    position: absolute;
    bottom: var(--lh);
    z-index: 5;
  }
  .zoom {
    left: 2ch;
    display: flex;
    gap: 1ch;
    background: var(--bg);
  }
  .zoom button {
    min-width: 1ch;
  }
  .minimap {
    right: 2ch;
    background: var(--bg-raised);
  }
  .minimap :global(.svelte-flow__panel) {
    position: static;
    margin: 0;
  }
  .placeholder {
    position: absolute;
    inset: 0;
  }
  .ghost-card {
    position: absolute;
    left: 4ch;
    width: 40ch;
    height: calc(var(--lh) * 6);
  }
  .ghost-card .skel {
    width: 24ch;
  }
  .layout-error {
    display: grid;
    justify-items: start;
    gap: var(--lh);
    max-width: 72ch;
    margin: calc(var(--lh) * 8) auto 0;
  }
  .layout-warn {
    position: absolute;
    top: var(--lh);
    left: 2ch;
    z-index: 5;
    background: var(--bg-raised);
  }
  .layer-label.short {
    display: none;
  }
  /* Narrower bars drop the least useful text first, so the toolbar never wraps (widths in columns, measured). */
  @container (max-width: 259ch) {
    .stats {
      display: none;
    }
  }
  @container (max-width: 204ch) {
    .lg-name {
      display: none;
    }
    .legend {
      gap: 1ch;
    }
  }
  @container (max-width: 169ch) {
    .legend {
      display: none;
    }
  }
  @container (max-width: 149ch) {
    .layer-label.long {
      display: none;
    }
    .layer-label.short {
      display: inline;
    }
  }
  @container (max-width: 130ch) {
    .layer-label,
    .layer-label.short {
      display: none;
    }
  }
</style>
