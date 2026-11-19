<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, api, type CommitDetail } from '../lib/api'
  import { codeLink, go } from '../lib/router.svelte'
  import { editorFor, openView, restoreVersion, type ViewTab } from '../lib/state.svelte'
  import CodeView from './CodeView.svelte'
  import DiffView from './DiffView.svelte'
  import JobLog from './JobLog.svelte'

  let { tab, onopen = openView }: { tab: ViewTab; onopen?: (tab: ViewTab) => void } = $props()

  let error = $state('')
  let diff = $state<string | null>(null)
  let commit = $state<CommitDetail | null>(null)
  let old = $state<string | null>(null)
  let current = $state<string | null>(null)

  /** Current content: the open editor's buffer (unsaved edits included), else the file on disk. */
  async function currentContent(path: string) {
    const view = editorFor(path)
    if (view) return view.state.doc.toString()
    return api.file(path).then(
      (f) => f.content,
      (e) => (e instanceof ApiError && e.status === 404 ? '' : Promise.reject(e)),
    )
  }

  async function load() {
    const t = tab
    error = ''
    try {
      if (t.kind === 'diff') diff = await api.diff(t.staged, t.path)
      else if (t.kind === 'commit') commit = await api.commitDetail(t.rev)
      else if (t.kind === 'version' || t.kind === 'compare') {
        const [a, b] = await Promise.all([api.fileAt(t.rev, t.path), t.kind === 'compare' ? currentContent(t.path) : null])
        old = a
        current = b
      }
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  // Re-opening a view remounts this pane, so loading on mount keeps it fresh.
  onMount(() => {
    if (tab.kind !== 'job') load()
  })

  const fmtDate = (d: string) => new Date(d).toLocaleString()
</script>

{#if tab.kind === 'job'}
  <JobLog jobId={tab.jobId} />
{:else}
  <div class="view-bar">
    {#if tab.kind === 'diff'}
      <div class="title">
        <strong>{tab.staged ? 'Staged changes' : 'Working tree changes'}</strong>
        <span class="muted">{tab.path ?? 'all files'}</span>
      </div>
      <div class="actions">
        <button onclick={load}>Refresh</button>
        {#if tab.path}<button onclick={() => tab.path && go(codeLink(tab.path))}>Open file</button>{/if}
      </div>
    {:else if tab.kind === 'commit'}
      {#if commit}
        <div class="title"><strong>{commit.message.split('\n')[0]}</strong></div>
        <div class="muted small">
          <code>{commit.sha.slice(0, 12)}</code> · {commit.author} &lt;{commit.email}&gt; · {fmtDate(commit.date)}
        </div>
      {:else}
        <div class="title"><strong>Commit {tab.rev.slice(0, 12)}</strong></div>
      {/if}
    {:else}
      {@const t = tab}
      <div class="title">
        <strong>{t.path}</strong>
        <span class="muted">{t.kind === 'compare' ? `${t.short} (left) ↔ current (right)` : `at ${t.short}`}</span>
      </div>
      <div class="actions">
        {#if t.kind === 'version'}
          <button onclick={() => onopen({ ...t, kind: 'compare', id: `compare:${t.rev}:${t.path}`, title: `${t.title} ↔ now` })}>Compare with current</button>
        {/if}
        <button class="danger" onclick={() => restoreVersion(t.path, t.rev, t.short)}>Restore this version</button>
      </div>
    {/if}
  </div>

  <div class="view-body">
    {#if error}
      <p class="error" role="alert">{error}</p>
    {:else if tab.kind === 'diff'}
      {#if diff === null}<p class="empty">Loading…</p>{:else}<DiffView text={diff} />{/if}
    {:else if tab.kind === 'commit'}
      {#if !commit}
        <p class="empty">Loading…</p>
      {:else}
        {#if commit.message.trim().includes('\n')}<pre class="message">{commit.message.trim()}</pre>{/if}
        <DiffView text={commit.diff} />
      {/if}
    {:else if old === null || (tab.kind === 'compare' && current === null)}
      <p class="empty">Loading…</p>
    {:else}
      <CodeView content={old} path={tab.path} compare={tab.kind === 'compare' ? (current ?? '') : undefined} />
    {/if}
  </div>
{/if}

<style>
  .title {
    display: flex;
    align-items: baseline;
    gap: 10px;
    min-width: 0;
  }
  .title strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .actions {
    display: flex;
    gap: 6px;
    margin-left: auto;
  }
  .small {
    font-size: 12px;
  }
  .view-body {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .message {
    margin: 0;
    padding: 10px 14px;
    font: 12.5px/1.5 var(--mono);
    white-space: pre-wrap;
    border-bottom: 1px solid var(--line);
  }
  .error {
    margin: 14px;
    color: var(--red);
    white-space: pre-wrap;
  }
</style>
