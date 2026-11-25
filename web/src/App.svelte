<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Boot from './components/Boot.svelte'
  import CommandPalette, { togglePalette } from './components/CommandPalette.svelte'
  import Dialog from './components/Dialog.svelte'
  import Login from './components/Login.svelte'
  import ModeLine from './components/ModeLine.svelte'
  import NavRail from './components/NavRail.svelte'
  import PackageSearch from './components/PackageSearch.svelte'
  import Toasts from './components/Toasts.svelte'
  import TopBar from './components/TopBar.svelte'
  import { sweep } from './lib/ascii'
  import { pkgSearch } from './lib/dialogs.svelte'
  import { fleet } from './lib/fleet.svelte'
  import { loadInsights } from './lib/health.svelte'
  import { preloadLayout } from './lib/layout'
  import { HOME } from './lib/packages'
  import { go, route, type View } from './lib/router.svelte'
  import { activeTab, app, fileTabs, formatTab, init, refresh, saveTab, syncOpenTabs } from './lib/state.svelte'
  import ChangesView from './views/ChangesView.svelte'
  import DeploymentsView from './views/DeploymentsView.svelte'
  import DependenciesView from './views/DependenciesView.svelte'
  import HealthView from './views/HealthView.svelte'
  import HomePage from './views/HomePage.svelte'
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
  // The intro plays before login (and again at #/intro); it loads on demand.
  const loadIntro = () => import('./intro/Intro.svelte')
  let loginOpen = $state(false)
  $effect(() => {
    if (!app.needLogin && loginOpen) {
      loginOpen = false
      if (route.view === 'intro') go('#/fleet')
    }
  })
  const leaveIntro = () => go('#/fleet')

  // Every view is repainted top to bottom as you arrive, kept ones included: a new view, or another machine or
  // tab of the machine page. A derived string, so a new query (a search, a picked commit) or file does not repaint.
  let content = $state<HTMLElement>()
  const arrival = $derived(route.view === 'machines' ? `machines/${route.parts[0] ?? ''}/${route.parts[1] ?? ''}` : route.view)
  $effect(() => {
    void arrival
    if (content) tick().then(() => content && sweep(content))
  })

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

  const goKeys: Record<string, View> = { f: 'fleet', m: 'machines', p: 'packages', s: 'services', o: 'options', a: 'health', d: 'deployments', n: 'dependencies', i: 'inputs', c: 'changes', h: 'history', e: 'code', ',': 'settings' }
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
      <main class="content" bind:this={content}>
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
            {#key route.parts[0]}
              {#if route.parts[0].startsWith(HOME)}<HomePage name={route.parts[0].slice(HOME.length)} />{:else}<MachinePage host={route.parts[0]} tab={route.parts[1] ?? 'overview'} />{/if}
            {/key}
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
        {:else if route.view === 'dependencies'}
          <DependenciesView />
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
      <ModeLine />
    </div>
  </div>
  {#if route.view === 'intro'}
    {#await loadIntro() then { default: Intro }}<Intro onEnter={leaveIntro} back={leaveIntro} />{/await}
  {/if}
{:else if !app.needLogin}
  <Boot />
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
    gap: 1ch;
    height: 100vh;
    padding: 0 1ch;
  }
  .main {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    height: 100vh;
    isolation: isolate; /* a drawer's scrim inside .content still covers the top bar */
  }
  .content {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: clip;
  }
  .keep {
    position: absolute;
    inset: 0;
  }
  .keep[hidden] {
    display: none;
  }
</style>
