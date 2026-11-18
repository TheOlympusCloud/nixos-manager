<script lang="ts">
  import { ApiError, api, type StatusFile } from '../lib/api'
  import { act, app, checkStaged, openView, refreshStatus } from '../lib/state.svelte'
  import { confirmBox, report, toast } from '../lib/ui.svelte'
  import { basename, dirname } from '../lib/util'
  import Icon from './Icon.svelte'

  let message = $state('')
  let committing = $state(false)
  let commitError = $state('')

  const files = $derived(app.status?.files ?? [])
  const staged = $derived(files.filter((f) => f.staged))
  const unstaged = $derived(files.filter((f) => f.unstaged))
  const checked = $derived(!!app.status?.staged_checked)
  const stagedJob = $derived(app.jobs.find((j) => j.kind === 'check' && j.tree === app.status?.staged_tree))
  const canCommit = $derived(checked && staged.length > 0 && !!message.trim() && !committing)
  const commitHint = $derived(
    !staged.length ? 'Stage changes to commit.' : !checked ? 'Staged changes need a passing check.' : !message.trim() ? 'Write a commit message.' : '',
  )
  const targetsHint = $derived(app.selectedTargets.length ? `flake check + ${app.selectedTargets.length} target(s)` : 'flake check only')

  function showDiff(f: StatusFile, isStaged: boolean) {
    openView({ kind: 'diff', id: `diff:${isStaged}:${f.path}`, title: `${basename(f.path)} (${isStaged ? 'staged' : 'working'})`, staged: isStaged, path: f.path })
  }

  async function discard(paths: string[]) {
    const what = paths.length === 1 ? paths[0] : `all ${paths.length} changed files`
    const ok = await confirmBox({
      title: 'Discard changes',
      message: `Discard unstaged changes in ${what}? Untracked files are deleted. This cannot be undone.`,
      ok: 'Discard',
      danger: true,
    })
    if (ok) await act(() => api.discard(paths), { tree: true, tabs: true })
  }

  async function commit(force: boolean) {
    if (force) {
      const ok = await confirmBox({
        title: 'Commit without checks',
        message: 'The staged changes have no passing check. Commit them anyway?',
        ok: 'Commit without checks',
        danger: true,
      })
      if (!ok) return
    }
    committing = true
    commitError = ''
    try {
      const { sha } = await api.commit(message.trim(), force)
      message = ''
      toast(`Committed ${sha.slice(0, 8)}`, 'ok')
      app.logVersion++
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return
      commitError = e instanceof Error ? e.message : String(e)
      if (!(e instanceof ApiError && (e.status === 412 || e.status === 400))) report(e, 'Commit failed')
    } finally {
      committing = false
      await refreshStatus()
    }
  }
</script>

{#snippet fileRow(f: StatusFile, isStaged: boolean)}
  {@const letter = isStaged ? f.staged : f.unstaged}
  {@const p = f.path.replace(/\/$/, '')}
  <li class="item">
    <button class="item-main" title={f.from ? `${f.from} → ${f.path}` : f.path} onclick={() => showDiff(f, isStaged)}>
      <span class="name">{basename(p)}</span>
      <span class="dir">{dirname(p)}</span>
    </button>
    <span class="item-tools">
      {#if isStaged}
        <button class="icon" title="Unstage" aria-label="Unstage {f.path}" onclick={() => act(() => api.unstage([f.path]))}><Icon name="minus" size={14} /></button>
      {:else}
        <button class="icon" title="Discard changes" aria-label="Discard changes in {f.path}" onclick={() => discard([f.path])}><Icon name="undo" size={14} /></button>
        <button class="icon" title="Stage" aria-label="Stage {f.path}" onclick={() => act(() => api.stage([f.path]))}><Icon name="plus" size={14} /></button>
      {/if}
    </span>
    <span class="badge b-{letter === '?' ? 'u' : letter}">{letter}</span>
  </li>
{/snippet}

<section class="panel" aria-label="Changes">
  <div class="panel-head">
    <h2>Changes</h2>
    <div class="tools">
      <button class="icon" title="Refresh" aria-label="Refresh" onclick={refreshStatus}><Icon name="refresh" /></button>
    </div>
  </div>

  <div class="scroll">
    <div class="commit-box">
      <label for="commit-message" class="label">Commit message</label>
      <textarea
        id="commit-message"
        rows="3"
        bind:value={message}
        placeholder="Describe the change (Ctrl+Enter to commit)"
        onkeydown={(e) => {
          if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && canCommit) commit(false)
        }}></textarea>

      <div class="check-state" class:ok={checked}>
        {#if !staged.length}
          <span class="muted">Nothing staged.</span>
        {:else if checked}
          <span>✓ Staged changes are checked.</span>
        {:else if stagedJob && (stagedJob.state === 'queued' || stagedJob.state === 'running')}
          <span>Check {stagedJob.id} is {stagedJob.state}…</span>
        {:else if stagedJob?.state === 'failed'}
          <span class="bad">✗ Check {stagedJob.id} failed.</span>
          <button class="link" onclick={() => openView({ kind: 'job', id: 'job:' + stagedJob.id, title: `check ${stagedJob.id}`, jobId: stagedJob.id })}>Show log</button>
        {:else}
          <span class="warn">Staged changes are not checked.</span>
        {/if}
      </div>

      <div class="buttons">
        <button onclick={checkStaged} disabled={!staged.length} title="Runs {targetsHint}; pick targets in the Checks panel">
          <Icon name="play" size={12} /> Run checks
        </button>
        <span class="muted small">{targetsHint}</span>
      </div>
      <button class="primary wide" disabled={!canCommit} onclick={() => commit(false)}>{committing ? 'Committing…' : 'Commit'}</button>
      {#if commitHint}<p class="muted small hint">{commitHint}</p>{/if}
      {#if commitError}<p class="error small" role="alert">{commitError}</p>{/if}
      <button class="link small danger-link" disabled={!staged.length || !message.trim() || committing} onclick={() => commit(true)}>
        Commit without checks…
      </button>
    </div>

    <div class="section-head">
      <h3>Staged <span class="count">{staged.length}</span></h3>
      <div class="tools">
        <button class="icon" title="Unstage all" aria-label="Unstage all" disabled={!staged.length} onclick={() => act(() => api.unstage([]))}><Icon name="minus" /></button>
      </div>
    </div>
    <ul class="list">
      {#each staged as f (f.path)}{@render fileRow(f, true)}{/each}
    </ul>

    <div class="section-head">
      <h3>Changes <span class="count">{unstaged.length}</span></h3>
      <div class="tools">
        <button class="icon" title="Discard all" aria-label="Discard all changes" disabled={!unstaged.length} onclick={() => discard(unstaged.map((f) => f.path))}><Icon name="undo" /></button>
        <button class="icon" title="Stage all" aria-label="Stage all" disabled={!unstaged.length} onclick={() => act(() => api.stage([]))}><Icon name="plus" /></button>
      </div>
    </div>
    <ul class="list">
      {#each unstaged as f (f.path)}{@render fileRow(f, false)}{/each}
    </ul>
    {#if !files.length}<p class="empty">No changes in the working copy.</p>{/if}
  </div>
</section>

<style>
  .commit-box {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 4px 12px 12px;
  }
  textarea {
    resize: vertical;
    min-height: 54px;
  }
  .check-state {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 6px;
    font-size: 12px;
  }
  .check-state.ok {
    color: var(--green);
  }
  .warn {
    color: var(--yellow);
  }
  .bad {
    color: var(--red);
  }
  .buttons {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .buttons button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .wide {
    width: 100%;
    padding: 5px;
  }
  .hint,
  .error {
    margin: 0;
  }
  .error {
    color: var(--red);
    white-space: pre-wrap;
  }
  .danger-link {
    align-self: flex-start;
    color: var(--red);
  }
  .small {
    font-size: 12px;
  }
</style>
