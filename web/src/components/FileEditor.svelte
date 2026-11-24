<script lang="ts">
  import { onMount } from 'svelte'
  import type { EditorView } from '@codemirror/view'
  import { createEditor, isNix } from '../lib/editor'
  import { app, attachEditor, detachEditor, formatTab, reloadTab, saveTab, updateDirty, type FileTab } from '../lib/state.svelte'

  let { tab, active }: { tab: FileTab; active: boolean } = $props()
  let host: HTMLElement
  let view: EditorView | undefined

  // onMount, not $effect: the editor must never be rebuilt from tab state, or unsaved edits would vanish.
  onMount(() => {
    const v = createEditor({
      parent: host,
      path: tab.path,
      content: tab.initial,
      root: app.info?.root ?? '',
      onSave: () => saveTab(tab),
      onFormat: () => isNix(tab.path) && formatTab(tab),
      onChange: () => updateDirty(tab),
    })
    view = v
    attachEditor(tab, v)
    return () => {
      detachEditor(tab.path, v)
      v.destroy()
    }
  })

  // Hidden editors cannot measure themselves; do it when the tab comes back.
  $effect(() => {
    if (active) view?.requestMeasure()
  })
</script>

<div class="file-bar">
  <span class="path" title={tab.path}>{tab.path}</span>
  {#if tab.dirty}<span class="muted small">● unsaved</span>{/if}
  <span class="spacer"></span>
  {#if isNix(tab.path)}
    <button onclick={() => formatTab(tab)} title="Format with nixfmt (Shift+Alt+F)">Format</button>
  {/if}
  <button class="primary" onclick={() => saveTab(tab)} disabled={!tab.dirty} title="Save (Ctrl+S)">Save</button>
</div>

{#if tab.conflict}
  <div class="conflict" role="alert">
    <p><strong>{tab.path} changed on disk</strong> after you opened it (another save, a git operation or an API client).</p>
    <div class="row">
      <button onclick={() => reloadTab(tab)}>Reload from disk</button>
      <span class="muted small">drops your edits</span>
      <button class="danger" onclick={() => saveTab(tab, true)}>Overwrite</button>
      <span class="muted small">replaces the file on disk with your version</span>
    </div>
  </div>
{/if}

<div class="cm-host" bind:this={host}></div>

<style>
  .file-bar {
    display: flex;
    flex: none;
    align-items: baseline;
    gap: 0 2ch;
    height: var(--lh);
  }
  .path {
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .spacer {
    flex: 1;
  }
  /* Changed on disk: a filled band of whole rows, ▲ in front. */
  .conflict {
    flex: none;
    margin-bottom: var(--lh);
    padding: 0 1ch;
    background: color-mix(in srgb, var(--yellow) 14%, var(--bg));
  }
  .conflict p::before {
    color: var(--yellow);
    content: '▲ ';
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .cm-host {
    flex: 1;
    min-height: 0;
  }
  .cm-host :global(.cm-editor) {
    height: 100%;
  }
</style>
