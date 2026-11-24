<script lang="ts">
  import { untrack } from 'svelte'
  import EditorArea from '../components/EditorArea.svelte'
  import FileTree from '../components/FileTree.svelte'
  import Icon from '../components/Icon.svelte'
  import { cell, frame } from '../lib/ascii'
  import { lspState } from '../lib/lsp.svelte'
  const lspStatus = { off: 'nixd idle', connecting: 'nixd starting', ready: 'nixd ready', down: 'nixd reconnecting', missing: 'nixd not installed' }
  import { fileUrl } from '../lib/forgejo'
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
  <!-- The tree's width is the user's (px, resizable); it is drawn in whole cells. -->
  <aside class="tree" style:width="{Math.round(side / cell.cw)}ch" use:frame={{ title: 'Files' }}><FileTree /></aside>
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
  <div class="ed" use:frame={'square'}>
    <EditorArea />
    <div class="rule" aria-hidden="true"></div>
    <footer class="status">
      <span class="msg" role="status">{statusLine.text}</span>
      {#if path}<a href={link('history', path.split('/'))}><Icon name="history" /> History of this file</a>{/if}
      {#if path && app.info?.web && app.status?.upstream}<a href={fileUrl(app.info.web, app.status.branch, path)} target="_blank" rel="noopener noreferrer" title="The pushed version of {path} on Forgejo"><Icon name="external" /> Open in Forgejo</a>{/if}
      <span title={lspState.reason || 'Nix language server (nixd): completion, hover help and diagnostics'}
        ><span class="dot {lspState.status === 'ready' ? 'ok' : lspState.status === 'down' ? 'err' : lspState.status === 'connecting' ? 'busy' : 'off'}"></span> {lspStatus[lspState.status]}</span>
    </footer>
  </div>
</div>

<style>
  .code {
    display: flex;
    height: 100%;
    padding: var(--lh) 1ch;
  }
  /* Framed, so they never scroll themselves (the frame would scroll away): what is inside does. */
  .tree {
    flex: none;
    min-width: 0;
    overflow: hidden;
  }
  .splitter {
    flex: none;
    width: 1ch;
    cursor: col-resize;
  }
  .splitter:hover {
    background: var(--bg-hover);
  }
  .splitter:focus-visible {
    background: var(--accent);
  }
  .ed {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    overflow: hidden;
  }
  .status {
    display: flex;
    flex: none;
    gap: 0 3ch;
    height: var(--lh);
    overflow: hidden;
    color: var(--fg-muted);
    white-space: nowrap;
  }
  .status a {
    color: var(--fg-muted);
  }
  .status a:hover {
    color: var(--on-accent);
  }
  .msg {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg-strong);
    text-overflow: ellipsis;
  }
</style>
