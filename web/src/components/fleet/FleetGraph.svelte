<script lang="ts">
  import { Background, Controls, MiniMap, Panel, Position, SvelteFlow, useSvelteFlow, type Edge, type Node } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/style.css'
  import './fleet.css'
  import { tick } from 'svelte'
  import type { Model } from '../../lib/api'
  import { buildGraph, defaultLayers, related, type GNode, type Layers } from '../../lib/graph'
  import { fleet, loadManaged } from '../../lib/fleet.svelte'
  import { graphUi } from '../../lib/graphui.svelte'
  import { layout, type Direction } from '../../lib/layout'
  import { groupLabel, groupOrder } from '../../lib/matrix'
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

  const layerList: { key: keyof Layers; label: string; icon: IconName; title: string }[] = [
    { key: 'features', label: 'Services & programs', icon: 'services', title: 'Enabled services, programs, virtualisation and hardware' },
    { key: 'files', label: 'Files', icon: 'file', title: 'Repository files shared by several machines' },
    { key: 'packages', label: 'Packages', icon: 'package', title: 'Installed packages, grouped per machine' },
    { key: 'users', label: 'Users', icon: 'users', title: 'Users and their home-manager configuration' },
    { key: 'inputs', label: 'Inputs', icon: 'input', title: 'Flake inputs feeding every machine' },
  ]

  // The inspector's switches read the managed files.
  $effect(() => {
    for (const h of model.hosts) if (h.managedFile && !fleet.managed[h.name]) loadManaged(h.name)
  })

  const flow = useSvelteFlow()
  // Pixels; the top leaves room for the legend.
  const FIT = { padding: { top: 56, right: 36, bottom: 28, left: 36 }, duration: 300, maxZoom: 1.1 }
  const graph = $derived(buildGraph(model, layers, graphUi.expanded))
  const byId = $derived(new Map(graph.nodes.map((n) => [n.id, n])))
  let nodes = $state.raw<Node[]>([])
  let laidOut = $state(false)

  const nodeTypes = { machine: MachineNode, pkgs: GroupNode, pill: PillNode }
  const flowType = (n: GNode) => (n.kind === 'machine' ? 'machine' : n.kind === 'pkgs' ? 'pkgs' : 'pill')

  // Lay the graph out again whenever what it shows changes; keep the viewport unless the set of machines changed.
  let seq = 0
  let lastShape = ''
  $effect(() => {
    const g = graph
    const d = dir
    const inputs = layers.inputs
    const mine = ++seq
    layout(g.nodes, g.edges, d, inputs).then(async (pos) => {
      if (mine !== seq) return
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
      if (shape !== lastShape) {
        lastShape = shape
        await tick()
        // Let Svelte Flow measure the new nodes first, or it fits the old bounds.
        setTimeout(() => flow.fitView(FIT), 60)
      }
    })
  })

  // Hover wins over selection for what lights up.
  const focus = $derived(graphUi.hover || graphUi.selected)
  const rel = $derived(focus && byId.has(focus) ? related(graph.edges, focus) : null)
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
    if (graphUi.selected && !byId.has(graphUi.selected)) graphUi.selected = ''
  })
  const selected = $derived(byId.get(graphUi.selected) ?? null)

  // ---- search ----
  let query = $state('')
  let searchOpen = $state(false)
  let cursor = $state(0)
  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase()
    if (!q) return []
    return graph.nodes
      .filter((n) => n.label.toLowerCase().includes(q) || (n.kind === 'pkgs' && n.host.toLowerCase().includes(q)))
      .sort((a, b) => Number(!a.label.toLowerCase().startsWith(q)) - Number(!b.label.toLowerCase().startsWith(q)) || a.label.localeCompare(b.label))
      .slice(0, 8)
  })
  const kindLabel: Record<string, string> = { machine: 'Machine', feature: 'Feature', file: 'File', pkgs: 'Packages', package: 'Package', user: 'User', home: 'Home', input: 'Input' }

  function jump(n: GNode) {
    const node = nodes.find((x) => x.id === n.id)
    query = ''
    searchOpen = false
    graphUi.selected = n.id
    // Shifted right by half the details panel, so the node lands in the visible part of the canvas.
    if (node) flow.setCenter(node.position.x + n.width / 2 + 180 / 1.15, node.position.y + n.height / 2, { zoom: 1.15, duration: 450 })
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
    return {
      machines: model.hosts.length,
      failing: model.hosts.length - ok.length,
      managed: ok.filter((h) => h.managedFile).length,
      features: new Set(ok.flatMap((h) => h.features.map((f) => f.group + '.' + f.name))).size,
      packages: new Set(ok.flatMap((h) => h.packages.map((p) => p.name))).size,
    }
  })
</script>

<svelte:window onkeydown={onKey} />

<div class="fleet">
  <div class="bar">
    <div class="search-box">
      <span class="search-ico"><Icon name="search" size={14} /></span>
      <input
        class="search"
        type="search"
        placeholder="Find a machine, service, file…"
        aria-label="Find a node in the graph"
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
          {#each matches as m, i (m.id)}
            <li id="fs-{i}" role="option" aria-selected={i === cursor}>
              <button class="ghost" tabindex="-1" onmousedown={(e) => e.preventDefault()} onclick={() => jump(m)}>
                <span class="r-label">{m.kind === 'pkgs' ? m.host : m.label}</span>
                <span class="r-kind">{kindLabel[m.kind]}</span>
              </button>
            </li>
          {:else}
            <li class="none">Nothing matches in the layers shown.</li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="layers" role="group" aria-label="Layers">
      {#each layerList as l (l.key)}
        <button class="layer" class:on={layers[l.key]} aria-pressed={layers[l.key]} title={l.title} onclick={() => (layers[l.key] = !layers[l.key])}>
          <Icon name={l.icon} size={14} />{l.label}
        </button>
      {/each}
    </div>

    <span class="spacer"></span>
    <span class="stats muted small num">
      {stats.machines} machines{#if stats.failing}<span class="bad"> · {stats.failing} failing</span>{/if} · {stats.managed} managed · {stats.features} features · {stats.packages} packages
    </span>
    <button
      class="icon"
      title={dir === 'RIGHT' ? 'Lay out top to bottom' : 'Lay out left to right'}
      aria-label={dir === 'RIGHT' ? 'Lay out top to bottom' : 'Lay out left to right'}
      onclick={() => (dir = dir === 'RIGHT' ? 'DOWN' : 'RIGHT')}>
      <Icon name={dir === 'RIGHT' ? 'dir-right' : 'dir-down'} />
    </button>
    <button class="icon" title="Fit the whole fleet in view" aria-label="Fit view" onclick={() => flow.fitView(FIT)}>
      <Icon name="fit" />
    </button>
  </div>

  <div class="canvas" class:ready={laidOut}>
    <SvelteFlow
      class="fleet-flow dark"
      bind:nodes
      {edges}
      {nodeTypes}
      minZoom={0.15}
      maxZoom={2}
      nodesConnectable={false}
      zoomOnDoubleClick={false}
      attributionPosition="bottom-center"
      onnodeclick={({ node }) => (graphUi.selected = node.id)}
      onselectionchange={({ nodes: sel }) => sel.length === 1 && (graphUi.selected = sel[0].id)}
      onnodepointerenter={({ node }) => (graphUi.hover = node.id)}
      onnodepointerleave={() => (graphUi.hover = '')}
      onpaneclick={() => (graphUi.selected = '')}>
      <Background gap={22} size={1.2} />
      <Controls position="bottom-left" showLock={false} fitViewOptions={FIT} />
      <MiniMap position="bottom-right" pannable zoomable nodeColor={minimapColor} nodeBorderRadius={6} width={180} height={120} />
      <Panel position="top-left" class="legend-panel">
        <div class="legend" aria-label="Legend">
          {#each groupOrder as g (g)}<span class="lg g-{g}"><span class="dot"></span>{groupLabel[g]}</span>{/each}
        </div>
      </Panel>
    </SvelteFlow>
    {#if !laidOut}
      <div class="placeholder" aria-hidden="true">
        {#each [0, 1, 2] as i (i)}<span class="skel ghost-card" style:top="{18 + i * 26}%"></span>{/each}
      </div>
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
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    min-height: 52px;
    padding: 10px var(--s4);
    background: var(--bg);
    border-bottom: 1px solid var(--border);
  }
  .search-box {
    position: relative;
    width: 260px;
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
    width: 320px;
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
    width: 100%;
    height: 30px;
    font-weight: 400;
  }
  .results [aria-selected='true'] button {
    background: var(--bg-active);
  }
  .r-label {
    overflow: hidden;
    color: var(--fg-strong);
    text-overflow: ellipsis;
  }
  .r-kind {
    color: var(--fg-muted);
    font-size: var(--fs-xs);
  }
  .none {
    padding: 8px;
    color: var(--fg-muted);
  }
  .layers {
    display: flex;
    flex-wrap: wrap;
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
  .stats .bad {
    color: var(--red);
  }
  .canvas {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
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
    background: var(--bg-deep);
  }
  .ghost-card {
    position: absolute;
    left: 8%;
    width: 240px;
    height: 120px;
    border-radius: 10px;
    opacity: 0.5;
  }
  .legend {
    display: flex;
    gap: 12px;
    padding: 6px 10px;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    background: color-mix(in srgb, var(--bg-side) 88%, transparent);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .lg {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .lg .dot {
    background: var(--c);
  }
  @media (max-width: 1280px) {
    .stats {
      display: none;
    }
  }
</style>
