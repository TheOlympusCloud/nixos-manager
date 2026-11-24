<script lang="ts">
  import { frame, scene } from '../lib/ascii'
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

  function toggle() {
    collapsed = !collapsed
    localStorage.setItem('nixmgr.rail', collapsed ? 'collapsed' : 'open')
  }

  // A small snowflake idles at the foot of the rail: six λ arms, lit in turn.
  const ART = ['  ╲  λ  ╱  ', '   ╲ │ ╱   ', 'λ ───❄─── λ', '   ╱ │ ╲   ', '  ╱  λ  ╲  ']
  const ARMS = [
    [0, 5],
    [0, 8],
    [2, 10],
    [4, 8],
    [4, 5],
    [2, 0],
  ]
  function snowflake(pre: HTMLElement) {
    const paint = scene(pre, (t) => {
      const [ly, lx] = ARMS[Math.floor(t / 4) % 6]
      return ART.map((row, y) => [...row].map((c, x) => (c === 'λ' && !(y === ly && Math.abs(x - lx) < 3) ? '·' : c)).join('')).join('\n')
    })
    return { destroy: paint.stop }
  }
</script>

{#snippet item(view: View, label: string, icon: IconName, key: string)}
  <a class="item" href={link(view)} aria-current={route.view === view ? 'page' : undefined} title={collapsed ? `${label} (g ${key})` : `g ${key}`}>
    <Icon name={icon} />
    <span class="label">{label}</span>
    {#if view === 'changes' && running}<span class="dot busy" aria-label="A check is running"></span>{/if}
    {#if view === 'changes' && changes}<span class="count num" aria-label="{changes} changed files">{changes}</span>
    {:else if view === 'deployments' && failedDeploys}<span class="count num err" aria-label="{failedDeploys} failed deployments">{failedDeploys}</span>
    {:else if view === 'health' && findings.length}<span class="count num" class:warn={!errors} class:err={errors} aria-label="{findings.length} errors and warnings">{findings.length}</span>
    {:else}<span class="key" aria-hidden="true">g{key}</span>{/if}
  </a>
{/snippet}

<nav class="rail" class:collapsed aria-label="Main" use:frame={{ animate: true }}>
  <a class="brand" href="#/fleet" aria-label="nixos-manager, fleet"><span class="mark">❄</span> <span class="label">nixos-manager</span></a>
  <button class="find bare" onclick={() => (palette.open = true)} title={collapsed ? `Search and run (${mac ? '⌘' : 'Ctrl'} K)` : undefined} aria-label="Search and run commands" aria-keyshortcuts={mac ? 'Meta+K' : 'Control+K'}>
    <Icon name="search" />
    <span class="label">Search…</span>
    <kbd class="keys" aria-hidden="true">{mac ? '⌘' : '^'}K</kbd>
  </button>
  <div class="rule" aria-hidden="true"></div>
  <div class="items">
    {#each items as i (i.view)}{@render item(i.view, i.label, i.icon, i.key)}{/each}
  </div>
  <div class="bottom">
    {#if !collapsed}<pre class="art nofx" aria-hidden="true" use:snowflake></pre>{/if}
    {@render item('settings', 'Settings', 'settings', ',')}
    <button class="item collapse bare" onclick={toggle} aria-label={collapsed ? 'Expand navigation' : 'Collapse navigation'} aria-expanded={!collapsed}>
      <Icon name="sidebar" />
      <span class="label">Collapse</span>
    </button>
  </div>
</nav>

<style>
  .rail {
    display: flex;
    flex: none;
    flex-direction: column;
    width: 26ch;
    height: calc(100vh - var(--lh));
    overflow: hidden;
    padding: var(--lh) 1ch;
  }
  .rail.collapsed {
    width: 7ch;
  }
  /* The name sits on the frame's top line, and takes you to the fleet. */
  .brand {
    position: absolute;
    top: 0;
    left: 2ch;
    z-index: 2;
    padding: 0 1ch;
    color: var(--fg-strong);
    font-weight: 700;
    white-space: nowrap;
    background: var(--bg);
  }
  .brand .mark {
    color: var(--accent);
  }
  .brand:hover {
    color: var(--on-accent);
    background: var(--accent);
  }
  .brand:hover .mark {
    color: inherit;
  }
  .collapsed .brand {
    left: 1ch;
  }
  .collapsed .brand .label {
    display: none;
  }
  .find {
    display: flex;
    gap: 1ch;
    width: 100%;
    padding: 0 1ch;
    color: var(--fg-muted);
    font-weight: 400;
    text-align: left;
    background: var(--bg-input);
  }
  .find:hover {
    color: var(--fg-strong);
    background: var(--bg-active);
  }
  .find .label {
    flex: 1;
  }
  .keys {
    padding: 0;
    color: inherit;
    background: none;
  }
  .items,
  .bottom {
    display: flex;
    flex-direction: column;
  }
  .bottom {
    margin-top: auto;
  }
  .item {
    display: flex;
    gap: 1ch;
    width: 100%;
    height: var(--lh);
    padding: 0 1ch;
    overflow: hidden;
    color: var(--fg);
    font-weight: 400;
    white-space: nowrap;
    text-align: left;
  }
  .item :global(.ic) {
    width: 2ch;
    color: var(--accent);
  }
  .item:hover {
    color: var(--fg-strong);
    background: var(--bg-hover);
  }
  /* Where you are: reverse video. */
  .item[aria-current='page'] {
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .item[aria-current='page'] :global(.ic),
  .item[aria-current='page'] .key {
    color: inherit;
  }
  .label {
    flex: 1;
    min-width: 0;
    color: inherit;
  }
  .key {
    color: var(--fg-faint);
  }
  .item .count {
    min-width: 3ch;
  }
  .collapsed :is(.label, .key, .keys) {
    display: none;
  }
  .collapsed .item .count {
    min-width: 0;
    padding: 0;
  }
  .collapsed .item .dot {
    display: none;
  }
  .art {
    margin: 0 auto var(--lh);
    color: var(--accent);
  }
</style>
