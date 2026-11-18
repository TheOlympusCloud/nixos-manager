<script lang="ts">
  import { api, type Commit } from '../lib/api'
  import { activePath, app, openView, restoreVersion } from '../lib/state.svelte'
  import { report } from '../lib/ui.svelte'
  import { ago, basename } from '../lib/util'
  import Icon from './Icon.svelte'

  let { visible }: { visible: boolean } = $props()

  let repoLog = $state<Commit[] | null>(null)
  let fileLog = $state<Commit[] | null>(null)
  const path = $derived(activePath())

  $effect(() => {
    if (!visible) return
    void app.logVersion
    api.log().then((c) => (repoLog = c), report)
  })

  $effect(() => {
    if (!visible) return
    void app.logVersion
    const p = path
    fileLog = null
    if (p) api.log({ path: p }).then((c) => p === path && (fileLog = c), report)
  })

  const showCommit = (c: Commit) => openView({ kind: 'commit', id: 'commit:' + c.sha, title: `${c.short} ${c.subject}`, rev: c.sha })
  const showVersion = (c: Commit, kind: 'version' | 'compare') =>
    openView({ kind, id: `${kind}:${c.sha}:${path}`, title: `${basename(path)} @ ${c.short}${kind === 'compare' ? ' ↔ now' : ''}`, path, rev: c.sha, short: c.short })
</script>

{#snippet meta(c: Commit)}
  <span class="dir">{c.short} · {c.author} · <time datetime={c.date} title={new Date(c.date).toLocaleString()}>{ago(c.date)}</time></span>
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
              <li class="item two-line">
                <button class="item-main" onclick={() => showVersion(c, 'version')} title="View {path} at {c.short}">
                  <span class="name">{c.subject}</span>
                  {@render meta(c)}
                </button>
                <span class="item-tools">
                  <button class="small-btn" onclick={() => showVersion(c, 'compare')} aria-label="Compare {c.short} with current">Compare</button>
                  <button class="small-btn" onclick={() => restoreVersion(path, c.sha, c.short)} aria-label="Restore {path} from {c.short}">Restore</button>
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
            <li class="item two-line">
              <button class="item-main" onclick={() => showCommit(c)}>
                <span class="name">{c.subject}</span>
                {@render meta(c)}
              </button>
            </li>
          {:else}
            <li class="empty">No commits yet.</li>
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
    content: '▸ ';
  }
  details[open] > summary h3::before {
    content: '▾ ';
  }
  .fname {
    margin-left: 4px;
    text-transform: none;
    letter-spacing: normal;
  }
  .small-btn {
    padding: 1px 6px;
    font-size: 11px;
  }
</style>
