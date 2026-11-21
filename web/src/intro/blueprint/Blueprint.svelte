<script lang="ts">
  // The engineering drawing of the fleet's configuration graph, scrubbed by t (0..1): nodes and edges
  // are stroke-drawn, scanned and ticked; one machine fails with a real Nix error, a fix redraws it,
  // and the sheet is stamped. All timing comes from timeline.ts; this file only draws.
  import { ERROR_LINES, FIX_LINES, NODES, EDGES, STAMPS, STEPS, FAILING, blueprintAt, type Node } from './timeline'

  let { t, narrow = false }: { t: number; narrow?: boolean } = $props()

  const W = $derived(narrow ? 640 : 1280)
  const H = $derived(narrow ? 1100 : 800)
  const state = $derived(blueprintAt(t))
  const step = $derived(STEPS.findIndex((s) => s.id === state.step))

  type Box = { x: number; y: number; w: number; h: number; label: string }
  const widths = { flake: 150, host: 196, module: 236, managed: 250, pkg: 164 }
  function box(n: Node): Box {
    if (narrow) {
      const w = n.kind === 'pkg' ? 138 : 142
      return { x: 86 + n.m[1] * 156 - w / 2, y: 80 + n.m[0] * 205, w, h: 40, label: n.short ?? n.label.replace(/^(hosts|modules)\//, '') }
    }
    const w = widths[n.kind]
    const h = n.kind === 'pkg' ? 38 : 44
    const x = [130, 400, 700, 1040][n.col] - w / 2
    const y = [[380], [200, 380, 560], [150, 300, 450, 600], [80, 158, 236, 314, 392, 470, 548]][n.col][n.row]
    return { x, y: y - h / 2, w, h, label: n.label }
  }
  const boxes = $derived(Object.fromEntries(NODES.map((n) => [n.id, box(n)])) as Record<string, Box>)

  // A chamfered rectangle (the six-fold corner of the snowflake, quietly), drawn from its top-left.
  function outline(b: Box, c = 7) {
    const { x, y, w, h } = b
    return `M${x + c} ${y}H${x + w - c}L${x + w} ${y + c}V${y + h - c}L${x + w - c} ${y + h}H${x + c}L${x} ${y + h - c}V${y + c}Z`
  }
  function edge(a: Box, b: Box) {
    if (narrow) {
      const x1 = a.x + a.w / 2, y1 = a.y + a.h, x2 = b.x + b.w / 2, y2 = b.y
      const m = (y1 + y2) / 2
      return `M${x1} ${y1}C${x1} ${m} ${x2} ${m} ${x2} ${y2}`
    }
    const x1 = a.x + a.w, y1 = a.y + a.h / 2, x2 = b.x, y2 = b.y + b.h / 2
    const m = (x1 + x2) / 2
    return `M${x1} ${y1}C${m} ${y1} ${m} ${y2} ${x2} ${y2}`
  }
  const kindOf = Object.fromEntries(NODES.map((n) => [n.id, n.kind]))
  const stroke = (id: string) => {
    const s = state.nodes[id].status
    return s === 'fail' ? 'var(--i-orange)' : s === 'ok' ? 'var(--i-green)' : kindOf[id] === 'managed' ? 'var(--i-violet)' : 'var(--i-blue)'
  }
  const pi = $derived(boxes[FAILING])
  const tw = $derived(narrow ? 580 : 392)
  const errorOpacity = $derived(state.error * (1 - state.fix))
</script>

<svg class="bp" class:narrow viewBox="0 0 {W} {H}" preserveAspectRatio="xMidYMid meet" role="img" aria-labelledby="bp-title bp-desc">
  <title id="bp-title">The fleet's configuration graph, drawn and checked</title>
  <desc id="bp-desc">
    flake.nix connects to hosts/workstation, hosts/homeserver and hosts/pi; the hosts import shared modules and their managed.json; those pull in packages and
    services. Every node is evaluated and ticked. hosts/pi fails with "{ERROR_LINES[0]}", a fix rewrites its imports, and the sheet is stamped "{STAMPS[0]}" and
    "{STAMPS[1]}".
  </desc>
  <defs>
    <pattern id="bp-minor" width="20" height="20" patternUnits="userSpaceOnUse">
      <path d="M20 0H0V20" fill="none" stroke="var(--grid)" stroke-width="1" />
    </pattern>
    <pattern id="bp-major" width="100" height="100" patternUnits="userSpaceOnUse">
      <path d="M100 0H0V100" fill="none" stroke="var(--grid-major)" stroke-width="1" />
    </pattern>
    <linearGradient id="bp-scan" x1="0" x2="1">
      <stop offset="0" stop-color="var(--i-blue)" stop-opacity="0" />
      <stop offset="1" stop-color="var(--i-blue)" stop-opacity="0.55" />
    </linearGradient>
    <marker id="bp-tick" viewBox="0 0 10 10" refX="5" refY="5" markerWidth="8" markerHeight="8" orient="auto">
      <path d="M2 8L8 2" stroke="var(--bp-ink)" stroke-width="1.2" />
    </marker>
    <marker id="bp-dot" viewBox="0 0 10 10" refX="5" refY="5" markerWidth="6" markerHeight="6">
      <circle cx="5" cy="5" r="3" fill="var(--i-blue)" />
    </marker>
    {#each NODES as n (n.id)}
      <clipPath id="bp-clip-{n.id}"><path d={outline(boxes[n.id])} /></clipPath>
    {/each}
  </defs>

  <!-- paper: a whiteprint, ink-blue lines on the sheet -->
  <rect width={W} height={H} fill="var(--bg-raised)" />
  <rect width={W} height={H} fill="url(#bp-minor)" opacity={state.paper} />
  <rect width={W} height={H} fill="url(#bp-major)" opacity={state.paper} />
  <rect x="0.5" y="0.5" width={W - 1} height={H - 1} fill="none" stroke="var(--bp-ink)" opacity={state.paper} />
  <rect x="12.5" y="12.5" width={W - 25} height={H - 25} fill="none" stroke="var(--bp-faint)" opacity={state.paper} />

  <!-- annotations: dimension lines, leaders, notes -->
  <g class="paper" opacity={state.paper} fill="none" stroke="var(--bp-ink)" stroke-width="1">
    {#if narrow}
      <path d="M40 44H600" marker-start="url(#bp-tick)" marker-end="url(#bp-tick)" />
      <text x="320" y="36" class="dim" text-anchor="middle">evaluation · flake → hosts → modules → packages</text>
    {:else}
      <path d="M302 52H1116" marker-start="url(#bp-tick)" marker-end="url(#bp-tick)" />
      <text x="709" y="44" class="dim" text-anchor="middle">evaluation · flake → hosts → modules → packages</text>
      <path d="M270 178V582" marker-start="url(#bp-tick)" marker-end="url(#bp-tick)" />
      <text class="dim" transform="translate(262 380) rotate(-90)" text-anchor="middle">3 machines</text>
      <path d="M1150 60V568" marker-start="url(#bp-tick)" marker-end="url(#bp-tick)" />
      <text class="dim" transform="translate(1162 314) rotate(90)" text-anchor="middle">7 of 100 000+ packages · reached by hash</text>
      <!-- leader: flake inputs -->
      <path d="M{boxes.flake.x + 40} {boxes.flake.y}V130H60" marker-start="url(#bp-dot)" />
      <text x="60" y="118" class="note">inputs.nixpkgs = github:NixOS/nixpkgs/nixos-25.05</text>
      <text x="60" y="104" class="note dim">flake.lock · pinned · reproducible</text>
      <!-- leader: managed.json -->
      <path d="M{boxes.pim.x + boxes.pim.w / 2} {boxes.pim.y + boxes.pim.h}V660H670" marker-start="url(#bp-dot)" />
      <text x="670" y="674" class="note">managed.json · edited by clicking</text>
      <text x="670" y="690" class="note dim">hand-written nix is never rewritten</text>
    {/if}
  </g>

  <!-- edges: a broad pale stroke, like a highlighter pass, under crisp ink -->
  <g fill="none" stroke-linecap="round">
    <g stroke="var(--i-blue)" stroke-width="5" opacity="0.1">
      {#each EDGES as [a, b], i (a + b)}
        <path d={edge(boxes[a], boxes[b])} pathLength="1" stroke-dasharray="1" stroke-dashoffset={1 - state.edges[i]} />
      {/each}
    </g>
    {#each EDGES as [a, b], i (a + b)}
      {@const failing = a === FAILING && state.nodes[a].status === 'fail'}
      <path
        d={edge(boxes[a], boxes[b])}
        pathLength="1"
        stroke={failing ? 'var(--i-orange)' : 'var(--i-blue)'}
        stroke-width="1.25"
        stroke-dasharray={failing ? '0.03 0.02' : '1'}
        stroke-dashoffset={failing ? 0 : 1 - state.edges[i]}
      />
    {/each}
  </g>

  <!-- nodes -->
  {#each NODES as n (n.id)}
    {@const b = boxes[n.id]}
    {@const s = state.nodes[n.id]}
    {@const col = stroke(n.id)}
    <g class="node" data-status={s.status}>
      <path d={outline(b)} fill={col} fill-opacity={s.status === 'idle' ? 0.05 * s.draw : s.status === 'fail' ? 0.14 : 0.09} stroke="none" />
      <path d={outline(b)} fill="none" stroke={col} stroke-width={s.status === 'fail' || s.status === 'fixing' ? 1.6 : 1.2} pathLength="1" stroke-dasharray="1" stroke-dashoffset={1 - (s.status === 'fixing' ? Math.max(s.scan, 0.02) : s.draw)} />
      {#if s.draw > 0.4}
        <text x={b.x + 14} y={b.y + b.h / 2 + 4.5} class="label" class:pkg={n.kind === 'pkg'} fill={s.status === 'fail' ? 'var(--i-orange)' : 'var(--i-fg)'} opacity={Math.min(1, (s.draw - 0.4) / 0.4)}>{b.label}</text>
      {/if}
      {#if s.status === 'scanning' || s.status === 'fixing'}
        <g clip-path="url(#bp-clip-{n.id})">
          <rect x={b.x + (s.scan - 0.35) * b.w} y={b.y} width={0.35 * b.w} height={b.h} fill="url(#bp-scan)" />
          <line x1={b.x + s.scan * b.w} x2={b.x + s.scan * b.w} y1={b.y} y2={b.y + b.h} stroke="var(--i-blue)" stroke-width="1.5" />
        </g>
      {/if}
      {#if s.status === 'ok' || s.status === 'fail'}
        <g transform="translate({b.x + b.w - 15} {b.y + b.h / 2})">
          <circle r="8" fill={col} fill-opacity="0.18" stroke={col} stroke-width="1" />
          {#if s.status === 'ok'}
            <path d="M-3.5 0.2L-1 2.8L3.6 -2.6" fill="none" stroke={col} stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
          {:else}
            <path d="M-2.8 -2.8L2.8 2.8M2.8 -2.8L-2.8 2.8" fill="none" stroke={col} stroke-width="1.6" stroke-linecap="round" />
          {/if}
        </g>
      {/if}
    </g>
  {/each}

  <!-- the error callout -->
  {#if errorOpacity > 0}
    {@const ex = narrow ? 40 : 300}
    {@const ey = narrow ? pi.y + 70 : 640}
    <g opacity={errorOpacity} transform="translate({ex} {ey})">
      <path d="M{pi.x + pi.w / 2 - ex} {pi.y + pi.h - ey}V-14H0" fill="none" stroke="var(--i-orange)" stroke-width="1" stroke-dasharray="3 3" />
      <rect x="0" y="-14" width={narrow ? 560 : 356} height={14 + ERROR_LINES.length * 17 + 12} fill="var(--bp-red-wash)" stroke="var(--i-orange)" stroke-width="1" />
      {#each ERROR_LINES as line, i (i)}
        <text x="12" y={6 + i * 17} class="code" fill={i === 0 ? 'var(--i-orange)' : 'var(--i-fg)'} xml:space="preserve">{line}</text>
      {/each}
    </g>
  {/if}

  <!-- the fix callout -->
  {#if state.fix > 0}
    {@const fx = 40}
    {@const fy = narrow ? pi.y + 56 : 630}
    <g opacity={state.fix} transform="translate({fx} {fy})">
      <path d="M{pi.x - fx} {pi.y + pi.h / 2 - fy}H{narrow ? 100 : 236}V0" fill="none" stroke="var(--i-green)" stroke-width="1" marker-start="url(#bp-dot)" />
      <rect x="0" y="0" width={narrow ? 560 : 336} height={22 + FIX_LINES.length * 17 + 10} fill="var(--bp-green-wash)" stroke="var(--i-green)" stroke-width="1" />
      <text x="12" y="16" class="note" fill="var(--i-green)">patch · hosts/pi/default.nix</text>
      {#each FIX_LINES as line, i (i)}
        <text x="12" y={38 + i * 17} class="code" fill={line.startsWith('+') ? 'var(--i-green)' : 'var(--i-orange)'} xml:space="preserve">{line}</text>
      {/each}
    </g>
  {/if}

  <!-- the stamps -->
  {#if state.stamp > 0}
    {@const sx = narrow ? 320 : 1060}
    {@const sy = narrow ? 80 + 3.5 * 205 - 8 : 630}
    {@const k = 1 + (1 - state.stamp) * 0.5}
    <g opacity={state.stamp} transform="translate({sx} {sy}) rotate(-4) scale({k})">
      {#each STAMPS as s, i (s)}
        <g transform="translate(0 {i * 36})">
          <rect x="-150" y="-13" width="300" height="28" fill="var(--bp-green-wash)" stroke="var(--i-green)" stroke-width="1.5" />
          <rect x="-146" y="-9" width="292" height="20" fill="none" stroke="var(--i-green)" stroke-width="0.5" opacity="0.7" />
          <text y="5" class="stamp" text-anchor="middle" fill="var(--i-green)">{s.toUpperCase()}</text>
        </g>
      {/each}
    </g>
  {/if}

  <!-- title block -->
  <g class="title" opacity={state.paper} transform="translate({narrow ? 30 : 862} {narrow ? H - 112 : 700})" fill="none" stroke="var(--bp-ink)" stroke-width="1">
    <rect x="0" y="0" width={tw} height="80" />
    <path d="M0 26H{tw}M0 53H{tw}M{tw * 0.5} 26V80" />
    <text x="10" y="17" class="tb strong">NIXOS-MANAGER</text>
    <text x={tw - 10} y="17" class="tb" text-anchor="end">FLEET · CONFIGURATION GRAPH</text>
    <text x="10" y="44" class="tb"><tspan class="dim">DRAWN</tspan> nix eval</text>
    <text x={tw * 0.5 + 10} y="44" class="tb"><tspan class="dim">CHECK</tspan> nix flake check</text>
    <text x="10" y="71" class="tb"><tspan class="dim">REV</tspan> 4e1f9a2</text>
    <text x={tw * 0.5 + 10} y="71" class="tb"><tspan class="dim">SHEET</tspan> 1 of 1 · <tspan class="dim">STATUS</tspan> {state.step === 'stamp' ? 'passed' : state.step === 'fail' ? 'failing' : state.step}</text>
  </g>

  <!-- the act readout -->
  <g class="readout" transform="translate({narrow ? 30 : 40} {narrow ? H - 14 : H - 18})">
    <text class="tb strong" fill="var(--i-blue)">{String(step + 1).padStart(2, '0')} / {String(STEPS.length).padStart(2, '0')}</text>
    <text x="64" class="tb" fill="var(--i-fg)">{STEPS[step].label.toUpperCase()}</text>
  </g>
</svg>

<style>
  .bp {
    display: block;
    width: 100%;
    height: 100%;
    --bp-ink: color-mix(in srgb, var(--i-blue) 70%, transparent);
    --bp-faint: color-mix(in srgb, var(--i-blue) 35%, transparent);
    --bp-red-wash: color-mix(in srgb, var(--i-orange) 6%, transparent);
    --bp-green-wash: color-mix(in srgb, var(--i-green) 7%, transparent);
    font-family: var(--i-mono);
    box-shadow: var(--shadow-3);
  }
  text {
    stroke: none;
    user-select: none;
  }
  .label {
    font-size: 12.5px;
    font-weight: 500;
  }
  .label.pkg {
    font-size: 12px;
  }
  /* The narrow sheet's boxes are smaller; its labels shrink to fit the mono face. */
  .narrow .label {
    font-size: 10.5px;
  }
  .dim {
    font-size: 10.5px;
    fill: var(--i-muted);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .note {
    font-size: 11.5px;
    fill: var(--i-fg);
  }
  .note.dim {
    fill: var(--i-muted);
    text-transform: none;
    letter-spacing: 0.02em;
  }
  .code {
    font-size: 12px;
    white-space: pre;
  }
  .stamp {
    font-size: 12.5px;
    font-weight: 700;
    letter-spacing: 0.16em;
  }
  .tb {
    font-size: 10px;
    fill: var(--i-fg);
  }
  .tb.strong {
    font-weight: 700;
  }
  .tb .dim {
    fill: var(--i-muted);
    font-size: 9px;
    letter-spacing: 0;
  }
  .node path {
    transition: stroke 260ms var(--i-ease), fill 260ms var(--i-ease);
  }
</style>
