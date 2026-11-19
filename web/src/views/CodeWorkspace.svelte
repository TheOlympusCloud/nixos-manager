<script lang="ts">
  import { untrack } from 'svelte'
  import EditorArea from '../components/EditorArea.svelte'
  import FileTree from '../components/FileTree.svelte'
  import Icon from '../components/Icon.svelte'
  import { lspState } from '../lib/lsp.svelte'
  const lspStatus = { off: 'nixd idle', connecting: 'nixd starting', ready: 'nixd ready', down: 'nixd reconnecting', missing: 'nixd not installed' }
  import { codeLink, link, replace, route } from '../lib/router.svelte'
  import { activeTab, app, openFile } from '../lib/state.svelte'
  import { statusLine } from '../lib/ui.svelte'

  /** Hand-written Nix: the file tree and editor (with nixd). Stays mounted so unsaved edits survive leaving it. */
  let { active }: { active: boolean } = $props()

  // #/code/<path> opens that file; the open file is reflected back into the URL.
  $effect(() => {
    const path = route.view === 'code' ? route.parts.join('/') : ''
    if (path) untrack(() => activeTab()?.id !== 'file:' + path && openFile(path))
  })
  $effect(() => {
    if (!active) return
    const t = activeTab()
    untrack(() => {
      if (t?.kind === 'file') replace(codeLink(t.path))
      else if (!app.tabs.length && route.parts.length) replace('#/code')
    })
  })

  let side = $state(Number(localStorage.getItem('nixmgr.side')) || 280)
  function resize(e: PointerEvent) {
    const el = e.currentTarget as HTMLElement
    el.setPointerCapture(e.pointerId)
    const start = e.clientX - side
    const move = (ev: PointerEvent) => (side = Math.max(200, Math.min(560, ev.clientX - start)))
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', () => {
      el.removeEventListener('pointermove', move)
      localStorage.setItem('nixmgr.side', String(side))
    }, { once: true })
  }
  function resizeKey(e: KeyboardEvent) {
    const step = e.key === 'ArrowLeft' ? -20 : e.key === 'ArrowRight' ? 20 : 0
    if (step) side = Math.max(200, Math.min(560, side + step))
  }
  const path = $derived(activeTab()?.kind === 'file' ? (activeTab() as { path: string }).path : '')
</script>

<div class="code">
  <div class="work">
    <aside class="tree" style:width="{side}px"><FileTree /></aside>
    <!-- A focusable separator is an ARIA widget (resizable with the arrow keys). -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div
      class="splitter"
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize the file tree"
      aria-valuenow={side}
      aria-valuemin={200}
      aria-valuemax={560}
      tabindex="0"
      onpointerdown={resize}
      onkeydown={resizeKey}></div>
    <EditorArea />
  </div>
  <footer class="status">
    <span class="msg" role="status">{statusLine.text}</span>
    {#if path}<a href={link('history', path.split('/'))}><Icon name="history" size={12} />History of this file</a>{/if}
    <span title={lspState.reason || 'Nix language server (nixd): completion, hover help and diagnostics'}
      ><span class="dot {lspState.status === 'ready' ? 'ok' : lspState.status === 'down' ? 'err' : lspState.status === 'connecting' ? 'busy' : 'off'}"></span>{lspStatus[lspState.status]}</span>
  </footer>
</div>

<style>
  .code {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .work {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .tree {
    flex: none;
    min-width: 0;
    background: var(--bg-side);
  }
  .splitter {
    flex: none;
    z-index: 2;
    width: 5px;
    margin: 0 -2px;
    padding: 0 2px;
    background: var(--border);
    background-clip: content-box;
    cursor: col-resize;
  }
  .splitter:hover,
  .splitter:focus-visible {
    padding: 0;
    background: var(--accent);
    outline: none;
  }
  .status {
    display: flex;
    flex: none;
    align-items: center;
    gap: 16px;
    height: 26px;
    padding: 0 12px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    background: var(--bg-deep);
    border-top: 1px solid var(--border);
  }
  .status span,
  .status a {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .status a {
    color: var(--fg-muted);
    text-decoration: none;
  }
  .status a:hover {
    color: var(--fg-strong);
  }
  .msg {
    flex: 1;
    color: var(--fg);
  }
</style>
