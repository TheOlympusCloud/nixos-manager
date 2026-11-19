<script lang="ts">
  import { onMount } from 'svelte'
  import CommandPalette, { togglePalette } from './components/CommandPalette.svelte'
  import Dialog from './components/Dialog.svelte'
  import Icon from './components/Icon.svelte'
  import Login from './components/Login.svelte'
  import NavRail from './components/NavRail.svelte'
  import PackageSearch from './components/PackageSearch.svelte'
  import Toasts from './components/Toasts.svelte'
  import TopBar from './components/TopBar.svelte'
  import { pkgSearch } from './lib/dialogs.svelte'
  import { fleet } from './lib/fleet.svelte'
  import { loadInsights } from './lib/health.svelte'
  import { preloadLayout } from './lib/layout'
  import { go, route, type View } from './lib/router.svelte'
  import { activeTab, app, fileTabs, formatTab, init, refresh, saveTab, syncOpenTabs } from './lib/state.svelte'
  import ChangesView from './views/ChangesView.svelte'
  import DeploymentsView from './views/DeploymentsView.svelte'
  import HealthView from './views/HealthView.svelte'
  import InputsView from './views/InputsView.svelte'
  import MachinePage from './views/MachinePage.svelte'
  import MachinesView from './views/MachinesView.svelte'
  import MatrixView from './views/MatrixView.svelte'
  import OptionsView from './views/OptionsView.svelte'
  import SettingsView from './views/SettingsView.svelte'

  onMount(init)

  // The code editor and the graph keep their state (unsaved edits, viewport) when you leave them.
  const visited = $state({ fleet: false, code: false })
  $effect(() => {
    if (route.view === 'fleet') visited.fleet = true
    if (route.view === 'code') visited.code = true
  })
  // The graph (Svelte Flow), CodeMirror and the history viewers load only when first needed.
  const loadFleet = () => import('./views/FleetView.svelte')
  const loadCode = () => import('./views/CodeWorkspace.svelte')
  const loadHistory = () => import('./views/HistoryView.svelte')
  // The intro plays before login (and again at #/intro); its WebGL scene loads on demand inside it.
  const loadIntro = () => import('./intro/Intro.svelte')
  let loginOpen = $state(false)
  $effect(() => {
    if (!app.needLogin && loginOpen) {
      loginOpen = false
      if (route.view === 'intro') go('#/fleet')
    }
  })
  const leaveIntro = () => go('#/fleet')

  // Once logged in, start the graph layout engine (a 1.4 MB worker) while the model evaluates.
  $effect(() => {
    if (app.info) preloadLayout()
  })
  // Every new evaluation brings new findings.
  $effect(() => {
    if (app.info && fleet.model?.tree) loadInsights()
  })

  let collapsed = $state(localStorage.getItem('nixmgr.rail') ? localStorage.getItem('nixmgr.rail') === 'collapsed' : innerWidth < 1200)

  // Files may change behind our back (API clients, git on the server); catch up when the window regains focus.
  let lastFocus = 0
  function onFocus() {
    if (!app.info || app.needLogin || Date.now() - lastFocus < 3000) return
    lastFocus = Date.now()
    refresh()
    syncOpenTabs()
  }

  const goKeys: Record<string, View> = { f: 'fleet', m: 'machines', p: 'packages', s: 'services', o: 'options', a: 'health', d: 'deployments', i: 'inputs', c: 'changes', h: 'history', e: 'code', ',': 'settings' }
  let pendingG = 0

  function onKey(e: KeyboardEvent) {
    const mod = e.ctrlKey || e.metaKey
    if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 'k' && app.info && !app.needLogin) {
      e.preventDefault() // in the editor and in text fields too: the palette works everywhere
      togglePalette()
      return
    }
    if (e.defaultPrevented) return // the editor already handled it
    const t = activeTab()
    if (route.view === 'code' && mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 's') {
      e.preventDefault() // never the browser's "save page" dialog
      if (t?.kind === 'file') saveTab(t)
      return
    }
    if (route.view === 'code' && e.shiftKey && e.altKey && e.code === 'KeyF' && t?.kind === 'file' && t.path.endsWith('.nix')) {
      e.preventDefault()
      formatTab(t)
      return
    }
    const target = e.target as HTMLElement
    if (mod || e.altKey || app.needLogin || target.closest('input, textarea, select, [contenteditable], .cm-editor, dialog')) return
    if (pendingG && Date.now() - pendingG < 1200 && goKeys[e.key]) {
      pendingG = 0
      e.preventDefault()
      go('#/' + goKeys[e.key])
    } else if (e.key === 'g') pendingG = Date.now()
    else if (e.key === '/') {
      // Focus the search box of the view on screen.
      const box = [...document.querySelectorAll<HTMLElement>('[data-search]')].find((el) => el.offsetParent !== null)
      if (box) {
        e.preventDefault()
        box.focus()
      }
    }
  }

  function beforeUnload(e: BeforeUnloadEvent) {
    if (fileTabs().some((t) => t.dirty)) e.preventDefault()
  }
</script>

<svelte:window onfocus={onFocus} onkeydown={onKey} onbeforeunload={beforeUnload} />

{#if app.info}
  <div class="shell">
    <NavRail bind:collapsed />
    <div class="main">
      <TopBar />
      <main class="content">
        {#if visited.fleet}
          <div class="keep" hidden={route.view !== 'fleet'}>
            {#await loadFleet() then { default: FleetView }}<FleetView active={route.view === 'fleet'} />{/await}
          </div>
        {/if}
        {#if visited.code}
          <div class="keep" hidden={route.view !== 'code'}>
            {#await loadCode() then { default: CodeWorkspace }}<CodeWorkspace active={route.view === 'code'} />{/await}
          </div>
        {/if}
        {#if route.view === 'machines'}
          {#if route.parts[0]}
            {#key route.parts[0]}<MachinePage host={route.parts[0]} tab={route.parts[1] ?? 'overview'} />{/key}
          {:else}
            <MachinesView />
          {/if}
        {:else if route.view === 'packages' || route.view === 'services'}
          {#key route.view}<MatrixView kind={route.view} />{/key}
        {:else if route.view === 'options'}
          <OptionsView />
        {:else if route.view === 'health'}
          <HealthView />
        {:else if route.view === 'deployments'}
          <DeploymentsView />
        {:else if route.view === 'inputs'}
          <InputsView />
        {:else if route.view === 'changes'}
          <ChangesView />
        {:else if route.view === 'history'}
          {#await loadHistory() then { default: HistoryView }}<HistoryView path={route.parts.join('/')} />{/await}
        {:else if route.view === 'settings'}
          <SettingsView />
        {/if}
      </main>
    </div>
  </div>
  {#if route.view === 'intro'}
    {#await loadIntro() then { default: Intro }}<Intro onEnter={leaveIntro} back={leaveIntro} />{/await}
  {/if}
{:else if !app.needLogin}
  <div class="boot" role="status">
    <span class="boot-mark"><Icon name="mark" size={26} /></span>
    <p>Loading nixos-manager…</p>
    <button onclick={init}>Retry</button>
  </div>
{/if}

{#if app.needLogin && !app.info}
  <!-- If the intro cannot load (an upgrade changed its chunk, a flaky network), the login must still be reachable. -->
  {#await loadIntro() then { default: Intro }}<Intro onEnter={() => (loginOpen = true)} />{:catch}<Login />{/await}
{/if}
{#if app.needLogin && (app.info || loginOpen)}<Login dismiss={app.info ? undefined : () => (loginOpen = false)} />{/if}
{#if pkgSearch.open && !app.needLogin}<PackageSearch />{/if}
{#if app.info && !app.needLogin}<CommandPalette />{/if}
<Dialog />
<Toasts />

<style>
  .shell {
    display: flex;
    height: 100vh;
  }
  .main {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .content {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: clip;
    isolation: isolate;
  }
  /* Ambient light behind every page: two soft glows that drift very slowly. */
  .content::before {
    position: absolute;
    inset: -30%;
    z-index: -1;
    background:
      radial-gradient(38% 34% at 24% 22%, color-mix(in srgb, var(--accent-strong) 13%, transparent), transparent 100%),
      radial-gradient(34% 30% at 82% 78%, color-mix(in srgb, var(--purple) 7%, transparent), transparent 100%);
    animation: drift 46s ease-in-out infinite alternate;
    content: '';
    pointer-events: none;
  }
  @keyframes drift {
    to {
      transform: translate3d(6%, 5%, 0) rotate(4deg);
    }
  }
  /* Every view fades in as you arrive, kept ones included (they restart when shown again). */
  .content > :global(*) {
    animation: fade-in 240ms var(--ease) backwards;
  }
  .keep {
    position: absolute;
    inset: 0;
  }
  .keep[hidden] {
    display: none;
  }
  .boot {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s3);
    height: 100vh;
    color: var(--fg-muted);
  }
  .boot > * {
    animation: rise var(--t-slow) var(--ease) backwards;
  }
  .boot > p {
    animation-delay: 80ms;
  }
  .boot > button {
    animation-delay: 1.6s; /* only worth showing when loading drags on */
  }
  .boot-mark {
    position: relative;
    display: grid;
    place-items: center;
    width: 56px;
    height: 56px;
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    border-radius: 16px;
    box-shadow: 0 0 44px -6px color-mix(in srgb, var(--accent) 45%, transparent);
  }
  .boot-mark :global(svg) {
    animation: turn 9s linear infinite;
  }
  .boot-mark::after {
    position: absolute;
    inset: -1px;
    border: 1px solid color-mix(in srgb, var(--accent) 55%, transparent);
    border-radius: inherit;
    animation: halo 2.2s var(--ease) infinite;
    content: '';
  }
  @keyframes turn {
    to {
      transform: rotate(360deg);
    }
  }
  @keyframes halo {
    to {
      opacity: 0;
      transform: scale(1.45);
    }
  }
</style>
