<script lang="ts">
  import { api, type Commit } from '../lib/api'
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
  <div class="panel-head">
    <h2>History</h2>
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
              <li class="item two-line" class:selected={current === `version:${c.sha}:${path}` || current === `compare:${c.sha}:${path}`}>
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
          {#each repoLog as c (c.sha)}
            <li class="item two-line" class:selected={current === 'commit:' + c.sha}>
              <button class="item-main" onclick={() => showCommit(c)}>
                <span class="name">{c.subject}</span>
                {@render meta(c)}
              </button>
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
  summary {
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  summary h3::before {
    display: inline-block;
    width: 5px;
    height: 5px;
    margin: 0 8px 2px 1px;
    content: '';
    border-right: 1.5px solid currentColor;
    border-bottom: 1.5px solid currentColor;
    transform: rotate(-45deg);
    transition: transform var(--t-fast) var(--ease);
  }
  details[open] > summary h3::before {
    transform: rotate(45deg);
  }
  .fname {
    margin-left: 4px;
    text-transform: none;
    letter-spacing: normal;
  }
  .row-tools {
    display: flex;
    flex: none;
    gap: 4px;
  }
  .row-tools button {
    height: 22px;
    padding: 0 7px;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    background: none;
  }
  .row-tools button:hover:not(:disabled) {
    color: var(--fg-strong);
  }
</style>
