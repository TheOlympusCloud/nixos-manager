<script lang="ts">
  import { app, closeTab, type Tab } from '../lib/state.svelte'
  import { bigText } from '../lib/ascii'
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
            class="tab-main bare"
            aria-current={tab.id === app.active ? 'page' : undefined}
            title={'path' in tab && tab.path ? tab.path : tab.title}
            onclick={() => (app.active = tab.id)}
            onauxclick={(e) => e.button === 1 && closeTab(tab.id)}>
            {tab.title}
          </button>
          {#if tab.kind === 'file' && tab.dirty}
            <button class="tab-close dirty bare" aria-label="Close {tab.title} (unsaved changes)" title="Unsaved changes" onclick={() => closeTab(tab.id)}>●</button>
          {:else}
            <button class="tab-close bare" aria-label="Close {tab.title}" title="Close" onclick={() => closeTab(tab.id)}>×</button>
          {/if}
        </div>
      {/each}
    </div>
    <div class="rule" aria-hidden="true"></div>
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
      <h1 class="big" aria-label="Code" use:bigText={'Code'}></h1>
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
    min-height: 0;
  }
  /* Editor tabs are filled cells: the open one bold on --bg-active, the others muted. */
  .tabbar {
    display: flex;
    flex: none;
    overflow-x: auto;
    white-space: nowrap;
    scrollbar-width: none;
  }
  .tab {
    display: flex;
    flex: none;
    gap: 0 1ch;
    max-width: 40ch;
    padding: 0 2ch;
  }
  .tab.active {
    background: var(--bg-active);
  }
  .tab-main {
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
  }
  .tab.active .tab-main {
    color: var(--fg-strong);
    font-weight: 700;
  }
  /* A view (diff, commit, version) is not a file: ± before its name. */
  .tab.view .tab-main::before {
    color: var(--fg-muted);
    content: '± ';
  }
  .tab-close {
    min-width: 1ch;
    color: var(--fg-muted);
  }
  .tab-close.dirty {
    color: var(--fg-strong);
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
    display: grid;
    gap: var(--lh) 0;
    max-width: 72ch;
    margin: auto;
    padding: var(--lh) 2ch;
    color: var(--fg-muted);
  }
  .welcome p {
    color: var(--fg);
  }
  .welcome li::before {
    color: var(--accent);
    content: '‣ ';
  }
</style>
