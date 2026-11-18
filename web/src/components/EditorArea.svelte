<script lang="ts">
  import { app, closeTab, type Tab } from '../lib/state.svelte'
  import FileEditor from './FileEditor.svelte'
  import ViewPane from './ViewPane.svelte'

  const paneKey = (t: Tab) => (t.kind === 'file' ? t.id : `${t.id}#${t.n}`)
</script>

<main class="editor-area">
  {#if app.tabs.length}
    <div class="tabbar" role="toolbar" aria-label="Open editors">
      {#each app.tabs as tab (tab.id)}
        <div class="tab" class:active={tab.id === app.active} class:view={tab.kind !== 'file'}>
          <button
            class="tab-main"
            aria-current={tab.id === app.active ? 'page' : undefined}
            title={'path' in tab && tab.path ? tab.path : tab.title}
            onclick={() => (app.active = tab.id)}
            onauxclick={(e) => e.button === 1 && closeTab(tab.id)}>
            {tab.title}
          </button>
          {#if tab.kind === 'file' && tab.dirty}
            <button class="tab-close dirty" aria-label="Close {tab.title} (unsaved changes)" title="Unsaved changes" onclick={() => closeTab(tab.id)}>●</button>
          {:else}
            <button class="tab-close" aria-label="Close {tab.title}" title="Close" onclick={() => closeTab(tab.id)}>×</button>
          {/if}
        </div>
      {/each}
    </div>
    <div class="panes">
      {#each app.tabs as tab (paneKey(tab))}
        <div class="pane" hidden={tab.id !== app.active}>
          {#if tab.kind === 'file'}
            <FileEditor {tab} active={tab.id === app.active} />
          {:else}
            <ViewPane {tab} />
          {/if}
        </div>
      {/each}
    </div>
  {:else}
    <div class="welcome">
      <h1>Code</h1>
      <p>Hand-written Nix for everything the visual editors do not cover. Open a file from the tree.</p>
      <ul>
        <li><kbd>Ctrl</kbd> <kbd>S</kbd> save · <kbd>Shift</kbd> <kbd>Alt</kbd> <kbd>F</kbd> format with nixfmt</li>
        <li><kbd>F12</kbd> go to definition · <kbd>F2</kbd> rename symbol · <kbd>Ctrl</kbd> <kbd>Space</kbd> completion</li>
        <li>Then review, check and commit in <a href="#/changes">Changes</a>.</li>
      </ul>
    </div>
  {/if}
</main>

<style>
  .editor-area {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    background: var(--bg);
  }
  .tabbar {
    display: flex;
    flex: none;
    overflow-x: auto;
    background: var(--bg-side);
    border-bottom: 1px solid var(--border);
    scrollbar-width: thin;
  }
  .tab {
    display: flex;
    flex: none;
    align-items: center;
    max-width: 240px;
    border-right: 1px solid var(--border);
    border-top: 1px solid transparent;
  }
  .tab.active {
    background: var(--bg);
    border-top-color: var(--accent);
  }
  .tab.view .tab-main {
    font-style: italic;
  }
  .tab-main {
    overflow: hidden;
    padding: 7px 4px 7px 12px;
    color: var(--fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
    background: none;
    border: 0;
    border-radius: 0;
  }
  .tab.active .tab-main {
    color: var(--fg-strong);
  }
  .tab-close {
    width: 22px;
    height: 22px;
    margin-right: 4px;
    padding: 0;
    color: var(--fg-muted);
    font-size: 15px;
    line-height: 1;
    background: none;
    border: 0;
  }
  .tab-close:hover {
    background: var(--bg-hover);
    color: var(--fg);
  }
  .tab-close.dirty {
    color: var(--fg);
    font-size: 11px;
  }
  .panes {
    position: relative;
    flex: 1;
    min-height: 0;
  }
  .pane {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
  }
  .pane[hidden] {
    display: none;
  }
  .welcome {
    margin: auto;
    max-width: 520px;
    padding: 24px;
    color: var(--fg-muted);
  }
  .welcome h1 {
    margin: 0 0 8px;
    color: var(--fg);
    font-size: 20px;
    font-weight: 600;
  }
  .welcome ul {
    padding-left: 18px;
    line-height: 2;
  }
</style>
