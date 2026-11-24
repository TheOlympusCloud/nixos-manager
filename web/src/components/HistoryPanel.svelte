<script lang="ts">
  import { api, type Commit } from '../lib/api'
  import { commitUrl, onRemote } from '../lib/forgejo'
  import { app, restoreVersion, type ViewTab } from '../lib/state.svelte'
  import { report } from '../lib/ui.svelte'
  import { ago, basename } from '../lib/util'
  import Icon from './Icon.svelte'

  /** Repository history, plus the history of `path` when one is picked. Views open through `onopen`; `onload` gets the commits. */
  let { path, onopen, current = '', onload }: { path: string; onopen: (tab: ViewTab) => void; current?: string; onload?: (commits: Commit[]) => void } = $props()

  let repoLog = $state<Commit[] | null>(null)
  let fileLog = $state<Commit[] | null>(null)
  $effect(() => {
    void app.logVersion
    api.log().then((c) => {
      repoLog = c
      onload?.(c)
    }, report)
  })

  $effect(() => {
    void app.logVersion
    const p = path
    fileLog = null
    if (p) api.log({ path: p }).then((c) => p === path && (fileLog = c), report)
  })

  const showCommit = (c: Commit) => onopen({ kind: 'commit', id: 'commit:' + c.sha, title: `${c.short} ${c.subject}`, rev: c.sha })
  const mine = (c: Commit) => app.info?.author.startsWith(c.author)
  const showVersion = (c: Commit, kind: 'version' | 'compare') =>
    onopen({ kind, id: `${kind}:${c.sha}:${path}`, title: `${basename(path)} @ ${c.short}${kind === 'compare' ? ' ↔ now' : ''}`, path, rev: c.sha, short: c.short })
</script>

{#snippet meta(c: Commit)}
  <span class="dir"><span class="mono">{c.short}</span> · {mine(c) ? 'you' : c.author} · <time datetime={c.date} title={new Date(c.date).toLocaleString()}>{ago(c.date)}</time></span>
{/snippet}

<section class="panel" aria-label="History">
  <!-- The view frames this panel and titles it "History"; the refresh button stands on that frame's top line. -->
  <div class="panel-head">
    <h2 class="sr-only">History</h2>
    <div class="tools">
      <button class="icon" title="Refresh" aria-label="Refresh history" onclick={() => app.logVersion++}><Icon name="refresh" /></button>
    </div>
  </div>

  <div class="scroll">
    {#if path}
      <details open>
        <summary class="section-head"><h3>File <span class="fname">{basename(path)}</span></h3></summary>
        <ul class="list">
          {#if fileLog === null}
            <li class="empty">Loading…</li>
          {:else}
            {#each fileLog as c (c.sha)}
              <li class="item two-line version" class:selected={current === `version:${c.sha}:${path}` || current === `compare:${c.sha}:${path}`}>
                <button class="item-main" onclick={() => showVersion(c, 'version')} title="View {path} at {c.short}">
                  <span class="name">{c.subject}</span>
                  {@render meta(c)}
                </button>
                <span class="row-tools">
                  <button class="small" onclick={() => showVersion(c, 'compare')} aria-label="Compare {path} at {c.short} with now" title="Side by side with the current file">Compare</button>
                  <button class="small" onclick={() => restoreVersion(path, c.sha, c.short)} aria-label="Restore {path} from {c.short}" title="Put this version back in the working copy">Restore</button>
                </span>
              </li>
            {:else}
              <li class="empty">No commits touch this file yet.</li>
            {/each}
          {/if}
        </ul>
      </details>
    {/if}

    <details open>
      <summary class="section-head"><h3>Repository</h3></summary>
      <ul class="list">
        {#if repoLog === null}
          <li class="empty">Loading…</li>
        {:else}
          {#each repoLog as c, i (c.sha)}
            <li class="item two-line" class:selected={current === 'commit:' + c.sha}>
              <button class="item-main" onclick={() => showCommit(c)}>
                <span class="name">{c.subject}</span>
                {@render meta(c)}
              </button>
              {#if app.info?.web && onRemote(i, app.status?.upstream, app.status?.ahead)}
                <span class="item-tools">
                  <a class="ext" href={commitUrl(app.info.web, c.sha)} target="_blank" rel="noopener noreferrer" aria-label="Open {c.short} in Forgejo" title="Open {c.short} in Forgejo"><Icon name="external" /></a>
                </span>
              {/if}
            </li>
          {:else}
            <li class="empty">No commits yet. Commit from Changes and they appear here.</li>
          {/each}
        {/if}
      </ul>
    </details>
  </div>
</section>

<style>
  /* On the top line of the frame around the panel (the nearest positioned ancestor). */
  .panel-head {
    height: 0;
    min-height: 0;
    margin: 0;
    padding: 0;
  }
  .panel-head .tools {
    position: absolute;
    top: 0;
    right: 2ch;
    z-index: 2;
    padding: 0 1ch;
    background: var(--bg);
  }
  summary {
    justify-content: flex-start;
  }
  summary h3 {
    color: var(--fg-muted);
    font-weight: 400;
  }
  .fname {
    color: var(--fg-strong);
    font-weight: 700;
  }
  /* Commits down a rail: ○ on each, ◉ on the one open, │ joining them. */
  .item {
    padding-right: 0;
  }
  .item::before,
  .item::after {
    position: absolute;
    left: 1ch;
    color: var(--line-strong);
  }
  .item::before {
    top: 0;
    content: '○';
  }
  .item:not(:last-child)::after {
    top: var(--lh);
    content: '│';
  }
  .item.version:not(:last-child)::after {
    white-space: pre;
    content: '│\A│';
  }
  .item .item-main {
    padding-left: 3ch;
  }
  .item .name {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .item.selected {
    color: var(--on-accent);
    background: var(--accent);
  }
  .item.selected::before {
    content: '◉';
  }
  .item.selected::before,
  .item.selected::after,
  .item.selected :is(.item-main, .name, .dir, .ext) {
    color: var(--on-accent);
  }
  /* A version is three rows: subject, sha · author · when, then its actions. */
  .item.version {
    flex-direction: column;
    align-items: stretch;
  }
  .row-tools {
    display: flex;
    flex: none;
    gap: 0 1ch;
    padding-left: 3ch;
  }
  .item.selected .row-tools button:not(:hover, :focus-visible) {
    color: var(--on-accent);
  }
  .item.selected .row-tools button:not(:hover, :focus-visible)::before,
  .item.selected .row-tools button:not(:hover, :focus-visible)::after {
    color: var(--on-accent);
  }
  .item.selected .item-tools {
    opacity: 1;
  }
  .ext {
    display: inline-block;
    width: 2ch;
    color: var(--fg-muted);
    text-align: center;
  }
  .ext:hover,
  .ext:focus-visible {
    color: var(--on-accent);
    background: var(--accent);
  }
</style>
