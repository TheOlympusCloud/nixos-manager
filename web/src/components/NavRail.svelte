<script lang="ts">
  import { deploys, watchDeployments } from '../lib/deployments.svelte'
  import { health } from '../lib/health.svelte'
  import { serious } from '../lib/insights'
  import { link, route, type View } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { palette } from './CommandPalette.svelte'
  import Icon, { type IconName } from './Icon.svelte'

  let { collapsed = $bindable() }: { collapsed: boolean } = $props()

  const items: { view: View; label: string; icon: IconName; key: string }[] = [
    { view: 'fleet', label: 'Fleet', icon: 'fleet', key: 'f' },
    { view: 'machines', label: 'Machines', icon: 'server', key: 'm' },
    { view: 'packages', label: 'Packages', icon: 'package', key: 'p' },
    { view: 'services', label: 'Services', icon: 'services', key: 's' },
    { view: 'options', label: 'Options', icon: 'sliders', key: 'o' },
    { view: 'health', label: 'Health', icon: 'activity', key: 'a' },
    { view: 'deployments', label: 'Deployments', icon: 'deploy', key: 'd' },
    { view: 'inputs', label: 'Inputs', icon: 'input', key: 'i' },
    { view: 'changes', label: 'Changes', icon: 'diff', key: 'c' },
    { view: 'history', label: 'History', icon: 'history', key: 'h' },
    { view: 'code', label: 'Code', icon: 'code', key: 'e' },
  ]
  const changes = $derived(app.status?.files.length ?? 0)
  const findings = $derived(serious(health.insights))
  const errors = $derived(findings.some((i) => i.severity === 'error'))
  // Machines report deployments at any time: check gently everywhere, for this badge and the graph's.
  $effect(() => watchDeployments())
  const failedDeploys = $derived(deploys.overview?.machines.filter((m) => m.state === 'failed').length ?? 0)
  const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
  const running = $derived(app.jobs.some((j) => ['check', 'lock', 'update'].includes(j.kind) && (j.state === 'running' || j.state === 'queued')))

  // The highlight slides from the old page to the new one instead of jumping.
  const order = [...items.map((i) => i.view), 'settings']
  let railEl = $state<HTMLElement>()
  let marker = $state<{ y: number; h: number } | null>(null)
  $effect(() => {
    void route.view
    void collapsed
    const cur = railEl?.querySelector<HTMLElement>('a.item[aria-current="page"]')
    marker = cur && order.includes(route.view) ? { y: cur.offsetTop, h: cur.offsetHeight } : null
  })

  function toggle() {
    collapsed = !collapsed
    localStorage.setItem('nixmgr.rail', collapsed ? 'collapsed' : 'open')
  }
</script>

{#snippet item(view: View, label: string, icon: IconName, key: string)}
  <a class="item" href={link(view)} aria-current={route.view === view ? 'page' : undefined} title={collapsed ? `${label} (g ${key})` : `g ${key}`}>
    <Icon name={icon} size={18} />
    <span class="label">{label}</span>
    {#if view === 'changes' && changes}{#key changes}<span class="count num" aria-label="{changes} changed files">{changes}</span>{/key}{/if}
    {#if view === 'deployments' && failedDeploys}<span class="count num sev err" aria-label="{failedDeploys} failed deployments">{failedDeploys}</span>{/if}
    {#if view === 'health' && findings.length}<span class="count num sev" class:err={errors} aria-label="{findings.length} errors and warnings">{findings.length}</span>{/if}
    {#if view === 'changes' && running}<span class="dot busy" aria-label="A check is running"></span>{/if}
  </a>
{/snippet}

<nav class="rail" class:collapsed aria-label="Main" bind:this={railEl}>
  {#if marker}<span class="marker" style:transform="translateY({marker.y}px)" style:height="{marker.h}px" aria-hidden="true"></span>{/if}
  <a class="brand" href="#/fleet" aria-label="nixos-manager, fleet">
    <span class="mark"><Icon name="mark" size={18} /></span>
    <span class="label">nixos-manager</span>
  </a>
  <button class="item find" onclick={() => (palette.open = true)} title={collapsed ? `Search and run (${mac ? '⌘' : 'Ctrl'} K)` : undefined} aria-label="Search and run commands" aria-keyshortcuts={mac ? 'Meta+K' : 'Control+K'}>
    <Icon name="search" size={18} />
    <span class="label">Search…</span>
    <span class="keys" aria-hidden="true"><kbd>{mac ? '⌘' : 'Ctrl'}</kbd><kbd>K</kbd></span>
  </button>
  <div class="items">
    {#each items as i (i.view)}{@render item(i.view, i.label, i.icon, i.key)}{/each}
  </div>
  <div class="bottom">
    {@render item('settings', 'Settings', 'settings', ',')}
    <button class="item collapse" onclick={toggle} aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'} aria-expanded={!collapsed}>
      <Icon name="sidebar" size={18} />
      <span class="label">Collapse</span>
    </button>
  </div>
</nav>

<style>
  .rail {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: var(--s1);
    width: 200px;
    height: 100%;
    padding: var(--s2);
    background: var(--bg-rail);
    border-right: 1px solid color-mix(in srgb, var(--fg-strong) 14%, var(--bg-rail));
    transition: width 260ms var(--ease);
  }
  .marker {
    position: absolute;
    top: 0;
    right: var(--s2);
    left: var(--s2);
    background: var(--bg-raised);
    border-radius: var(--r);
    box-shadow: var(--shadow-1);
    transition:
      transform 320ms var(--ease),
      height var(--t) var(--ease);
    pointer-events: none;
  }
  /* The accent bar on its left edge. */
  .marker::before {
    position: absolute;
    top: 8px;
    bottom: 8px;
    left: -1px;
    width: 3px;
    background: var(--accent);
    border-radius: 1px;
    content: '';
  }
  .rail.collapsed {
    width: 56px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 36px;
    margin-bottom: var(--s3);
    padding: 0 4px;
    color: var(--fg-strong);
    font-weight: 600;
    letter-spacing: -0.01em;
    text-decoration: none;
    white-space: nowrap;
  }
  .mark {
    display: grid;
    flex: none;
    place-items: center;
    width: 30px;
    height: 30px;
    color: var(--on-accent);
    background: var(--accent);
    border-radius: var(--r);
  }
  .mark :global(svg) {
    transition: transform 700ms var(--ease);
  }
  .brand:hover .mark :global(svg) {
    transform: rotate(60deg);
  }
  .items,
  .bottom {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .bottom {
    margin-top: auto;
  }
  .item {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 12px;
    width: 100%;
    height: 34px;
    padding: 0 11px;
    overflow: hidden;
    color: var(--fg-muted);
    font: 500 var(--fs-md) var(--sans);
    white-space: nowrap;
    text-align: left;
    text-decoration: none;
    background: none;
    border: 0;
    border-radius: var(--r);
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease);
  }
  .item :global(svg) {
    flex: none;
    transition:
      color var(--t) var(--ease),
      transform var(--t) var(--ease);
  }
  .item:hover {
    color: var(--fg-strong);
    background: color-mix(in srgb, var(--fg-strong) 7%, transparent);
  }
  .item:hover :global(svg) {
    transform: scale(1.12);
  }
  .item:active :global(svg) {
    transform: scale(0.94);
  }
  /* The sliding marker is this item's background. */
  .item[aria-current='page'],
  .item[aria-current='page']:hover {
    color: var(--fg-strong);
    background: none;
  }
  .item[aria-current='page'] :global(svg) {
    color: var(--accent);
  }
  .label {
    flex: 1;
    min-width: 0;
    transition: opacity var(--t-fast) var(--ease);
  }
  .collapsed .label {
    opacity: 0;
  }
  .count {
    min-width: 20px;
    padding: 0 6px;
    color: var(--on-accent);
    font-size: var(--fs-xs);
    font-weight: 700;
    line-height: 18px;
    text-align: center;
    background: var(--accent);
    border-radius: var(--r-sm);
    animation: bump var(--t-slow) var(--ease) backwards;
  }
  .count.sev {
    color: var(--fg-strong);
    background: var(--mark);
  }
  .count.sev.err {
    color: var(--on-accent);
    background: var(--danger);
  }
  .find {
    margin-bottom: var(--s2);
    color: var(--fg-muted);
    background: var(--bg-input);
    border: 1px solid color-mix(in srgb, var(--fg-strong) 16%, var(--bg-rail));
    box-shadow: none;
  }
  .find:hover {
    border-color: var(--line-strong);
  }
  .keys {
    display: inline-flex;
    gap: 2px;
    transition: opacity var(--t-fast) var(--ease);
  }
  .keys kbd {
    min-width: 0;
    padding: 0 4px;
    color: var(--fg-muted);
    font-size: 10px;
    line-height: 16px;
    background: var(--bg-raised);
  }
  .collapsed .keys {
    opacity: 0;
  }
  .collapsed .count {
    position: absolute;
    top: 3px;
    left: 22px;
    min-width: 16px;
    padding: 0 4px;
    line-height: 15px;
    font-size: 10px;
  }
  .item .dot {
    position: absolute;
    top: 7px;
    left: 26px;
  }
</style>
