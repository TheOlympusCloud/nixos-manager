<script lang="ts">
  import { onMount } from 'svelte'
  import ChangesPanel from './components/ChangesPanel.svelte'
  import ChecksPanel from './components/ChecksPanel.svelte'
  import Dialog from './components/Dialog.svelte'
  import EditorArea from './components/EditorArea.svelte'
  import FileTree from './components/FileTree.svelte'
  import HistoryPanel from './components/HistoryPanel.svelte'
  import Icon, { type IconName } from './components/Icon.svelte'
  import Login from './components/Login.svelte'
  import Toasts from './components/Toasts.svelte'
  import TokensDialog from './components/TokensDialog.svelte'
  import TopBar from './components/TopBar.svelte'
  import { lspState } from './lib/lsp.svelte'
  import { activeTab, app, fileTabs, formatTab, init, refresh, saveTab, syncOpenTabs, type Panel } from './lib/state.svelte'
  import { statusLine } from './lib/ui.svelte'

  onMount(init)

  const panels: { id: Panel; label: string; icon: IconName }[] = [
    { id: 'files', label: 'Files', icon: 'file' },
    { id: 'changes', label: 'Changes', icon: 'branch' },
    { id: 'checks', label: 'Checks', icon: 'check' },
    { id: 'history', label: 'History', icon: 'history' },
  ]
  const changeCount = $derived(app.status?.files.length ?? 0)
  const jobRunning = $derived(app.jobs.some((j) => j.state === 'running' || j.state === 'queued'))

  function pick(id: Panel) {
    // Clicking the open panel's button hides the sidebar, like most editors.
    app.sidebar = !(app.sidebar && app.panel === id)
    app.panel = id
  }

  let side = $state(Number(localStorage.getItem('nixmgr.side')) || 300)
  function resize(e: PointerEvent) {
    const el = e.currentTarget as HTMLElement
    el.setPointerCapture(e.pointerId)
    const start = e.clientX - side
    const move = (ev: PointerEvent) => (side = Math.max(200, Math.min(640, ev.clientX - start)))
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', () => {
      el.removeEventListener('pointermove', move)
      localStorage.setItem('nixmgr.side', String(side))
    }, { once: true })
  }
  function resizeKey(e: KeyboardEvent) {
    const step = e.key === 'ArrowLeft' ? -20 : e.key === 'ArrowRight' ? 20 : 0
    if (step) side = Math.max(200, Math.min(640, side + step))
  }

  // Files may change behind our back (API clients, git on the server); catch up when the window regains focus.
  let lastFocus = 0
  function onFocus() {
    if (!app.info || app.needLogin || Date.now() - lastFocus < 3000) return
    lastFocus = Date.now()
    refresh()
    syncOpenTabs()
  }

  function onKey(e: KeyboardEvent) {
    if (e.defaultPrevented) return // the editor already handled it
    const t = activeTab()
    const mod = e.ctrlKey || e.metaKey
    if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 's') {
      e.preventDefault() // never the browser's "save page" dialog
      if (t?.kind === 'file') saveTab(t)
    } else if (e.shiftKey && e.altKey && e.code === 'KeyF' && t?.kind === 'file' && t.path.endsWith('.nix')) {
      e.preventDefault()
      formatTab(t)
    }
  }

  function beforeUnload(e: BeforeUnloadEvent) {
    if (fileTabs().some((t) => t.dirty)) e.preventDefault()
  }
</script>

<svelte:window onfocus={onFocus} onkeydown={onKey} onbeforeunload={beforeUnload} />

{#if app.info}
  <div class="shell">
    <TopBar />
    <div class="main">
      <nav class="activity" aria-label="Side panels">
        {#each panels as p (p.id)}
          <button
            class="act"
            class:on={app.sidebar && app.panel === p.id}
            aria-pressed={app.sidebar && app.panel === p.id}
            aria-label={p.label}
            title={p.label}
            onclick={() => pick(p.id)}>
            <Icon name={p.icon} size={20} />
            {#if p.id === 'changes' && changeCount}<span class="count-dot">{changeCount}</span>{/if}
            {#if p.id === 'checks' && jobRunning}<span class="run-dot" aria-label="A job is running"></span>{/if}
          </button>
        {/each}
      </nav>
      <aside class="sidebar" style:width="{side}px" hidden={!app.sidebar}>
        <div class="pane-wrap" hidden={app.panel !== 'files'}><FileTree /></div>
        <div class="pane-wrap" hidden={app.panel !== 'changes'}><ChangesPanel /></div>
        <div class="pane-wrap" hidden={app.panel !== 'checks'}><ChecksPanel visible={app.sidebar && app.panel === 'checks'} /></div>
        <div class="pane-wrap" hidden={app.panel !== 'history'}><HistoryPanel visible={app.sidebar && app.panel === 'history'} /></div>
      </aside>
      {#if app.sidebar}
        <!-- A focusable separator is an ARIA widget (resizable with the arrow keys). -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <div
          class="splitter"
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize side panel"
          aria-valuenow={side}
          aria-valuemin={200}
          aria-valuemax={640}
          tabindex="0"
          onpointerdown={resize}
          onkeydown={resizeKey}></div>
      {/if}
      <EditorArea />
    </div>
    <footer class="statusbar">
      <span><Icon name="branch" size={12} /> {app.status?.branch ?? app.info.branch}</span>
      {#if app.status}
        <span class:warn={!app.status.staged_checked && app.status.files.some((f) => f.staged)}>
          {app.status.files.length} change{app.status.files.length === 1 ? '' : 's'}
        </span>
      {/if}
      <span class="msg" role="status">{statusLine.text}</span>
      <span title="Nix language server (nixd)">nixd: {lspState.status === 'off' ? 'idle' : lspState.status}</span>
    </footer>
  </div>
{:else if !app.needLogin}
  <div class="loading">
    <p>Loading nixos-manager…</p>
    <button onclick={init}>Retry</button>
  </div>
{/if}

{#if app.needLogin}<Login />{/if}
{#if app.tokensOpen && !app.needLogin}<TokensDialog />{/if}
<Dialog />
<Toasts />

<style>
  .shell {
    display: flex;
    flex-direction: column;
    height: 100vh;
  }
  .main {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .activity {
    display: flex;
    flex: none;
    flex-direction: column;
    width: 46px;
    background: var(--bg-deep);
    border-right: 1px solid var(--border);
  }
  .act {
    position: relative;
    display: grid;
    place-items: center;
    height: 46px;
    color: var(--fg-muted);
    background: none;
    border: 0;
    border-left: 2px solid transparent;
    border-radius: 0;
  }
  .act:hover {
    color: var(--fg);
  }
  .act.on {
    color: var(--fg-strong);
    border-left-color: var(--accent);
  }
  .count-dot {
    position: absolute;
    right: 5px;
    bottom: 7px;
    min-width: 16px;
    padding: 0 4px;
    color: #fff;
    font-size: 10px;
    line-height: 16px;
    background: var(--accent-strong);
    border-radius: 8px;
  }
  .run-dot {
    position: absolute;
    right: 9px;
    bottom: 10px;
    width: 8px;
    height: 8px;
    background: var(--blue);
    border-radius: 50%;
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
  .sidebar {
    flex: none;
    min-width: 0;
    background: var(--bg-side);
  }
  .sidebar[hidden],
  .pane-wrap[hidden] {
    display: none;
  }
  .pane-wrap {
    height: 100%;
  }
  .splitter {
    flex: none;
    width: 4px;
    margin-left: -2px;
    margin-right: -2px;
    z-index: 2;
    cursor: col-resize;
    background: var(--border);
    background-clip: content-box;
    padding: 0 1.5px;
  }
  .splitter:hover,
  .splitter:focus-visible {
    background: var(--accent);
    padding: 0;
  }
  .statusbar {
    display: flex;
    flex: none;
    align-items: center;
    gap: 16px;
    height: 24px;
    padding: 0 10px;
    color: var(--fg-muted);
    font-size: 12px;
    background: var(--bg-deep);
    border-top: 1px solid var(--border);
  }
  .statusbar span {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .statusbar .msg {
    flex: 1;
    color: var(--fg);
  }
  .warn {
    color: var(--yellow);
  }
  .loading {
    display: grid;
    place-content: center;
    justify-items: center;
    height: 100vh;
    color: var(--fg-muted);
  }
  @media (prefers-reduced-motion: reduce) {
    .run-dot {
      animation: none;
    }
  }
</style>
