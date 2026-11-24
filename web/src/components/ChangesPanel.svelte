<script lang="ts">
  import { ApiError, api, type StatusFile } from '../lib/api'
  import { builtText } from '../lib/checks'
  import { tryTargets } from '../lib/deployments'
  import { deploys, watchDeployments } from '../lib/deployments.svelte'
  import { draft, pullOrPush, repo, tryOnMachine } from '../lib/repo.svelte'
  import { go, link } from '../lib/router.svelte'
  import { act, app, checkStaged, refreshStatus } from '../lib/state.svelte'
  import { confirmBox, report, toast } from '../lib/ui.svelte'
  import { basename, dirname } from '../lib/util'
  import Icon from './Icon.svelte'

  let committing = $state(false)
  let commitError = $state('')

  const files = $derived(app.status?.files ?? [])
  const staged = $derived(files.filter((f) => f.staged))
  const allStaged = $derived(files.length > 0 && files.every((f) => f.staged && !f.unstaged))
  const checked = $derived(!!app.status?.staged_checked)
  const stagedJob = $derived(app.jobs.find((j) => j.kind === 'check' && j.tree === app.status?.staged_tree))
  const checking = $derived(stagedJob?.state === 'queued' || stagedJob?.state === 'running')
  const hasMessage = $derived(!!draft.message.trim())
  const canCommit = $derived(checked && staged.length > 0 && hasMessage && !committing)
  const commitHint = $derived(
    !staged.length ? 'Tick the files to commit.' : !checked ? 'The staged files need a passing check.' : !hasMessage ? 'Write a commit message.' : '',
  )
  // Try on one machine: those the check built, from the branch their comin follows.
  $effect(() => watchDeployments())
  const tryable = $derived(tryTargets(deploys.overview, app.status?.branch ?? '', app.status?.staged_check?.built ?? []))
  let tryHost = $state('')
  const tryOn = $derived(tryable.find((m) => m.host === tryHost) ?? tryable[0])
  const somethingToTry = $derived(staged.length > 0 || (app.status?.ahead ?? 0) > 0)
  const canTry = $derived(checked && somethingToTry && !committing && !repo.busy && (!staged.length || hasMessage))
  const targetsHint = $derived(
    `flake check and a build of every machine they change${app.selectedTargets.length ? `, plus ${app.selectedTargets.length} dry build${app.selectedTargets.length === 1 ? '' : 's'}` : ''}`,
  )

  /** Plain words for git's status letters. */
  const words: Record<string, string> = { '?': 'New', A: 'New', M: 'Edited', D: 'Deleted', R: 'Renamed' }
  const word = (f: StatusFile) => words[f.unstaged || f.staged] ?? 'Changed'
  const classes: Record<string, string> = { '?': 'new', A: 'new', D: 'del', R: 'ren' }
  const cls = (f: StatusFile) => classes[f.unstaged || f.staged] ?? 'mod'

  function toggle(f: StatusFile) {
    const full = !!f.staged && !f.unstaged
    act(() => (full ? api.unstage([f.path]) : api.stage([f.path])))
  }
  const toggleAll = () => act(() => (allStaged ? api.unstage([]) : api.stage([])))
  const show = (f: StatusFile) => (app.inspect = { kind: 'diff', staged: false, path: f.path })

  async function discard(paths: string[]) {
    const what = paths.length === 1 ? paths[0] : `all ${paths.length} files with unstaged changes`
    const ok = await confirmBox({
      title: 'Discard changes',
      message: `Discard the unstaged changes in ${what}? New files are deleted. This cannot be undone.`,
      ok: 'Discard',
      danger: true,
    })
    if (ok) await act(() => api.discard(paths), { tree: true, tabs: true })
  }

  async function commit(force: boolean) {
    if (force) {
      const ok = await confirmBox({
        title: 'Commit without checks?',
        message: 'The staged files have no passing check, so the machines could get a configuration that does not build. Commit them anyway?',
        ok: 'Commit without checks',
        danger: true,
      })
      if (!ok) return
    }
    committing = true
    commitError = ''
    const msg = draft.message.trim()
    // A rollback, committed as it was staged, goes on to the push dialog.
    const rollback = !!draft.pushTree && draft.pushTree === app.status?.staged_tree
    let pushNext = false
    try {
      const { sha } = await api.commit(msg, force)
      draft.message = ''
      draft.pushTree = ''
      toast(`${sha.slice(0, 7)} · ${msg.split('\n')[0]}`, 'ok', 'Committed', { label: 'View in History', run: () => go(link('history', [], { rev: sha })) })
      app.logVersion++
      pushNext = rollback
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return
      commitError = e instanceof Error ? e.message : String(e)
      if (!(e instanceof ApiError && (e.status === 412 || e.status === 400))) report(e, 'Commit failed')
    } finally {
      committing = false
      await refreshStatus()
    }
    if (pushNext) await pullOrPush('push')
  }
</script>

<section class="panel" aria-label="Commit">
  <div class="commit-box">
    <label for="commit-message" class="label">Commit message</label>
    <textarea
      id="commit-message"
      rows="3"
      bind:value={draft.message}
      placeholder="What changes, and why (Ctrl+Enter commits)"
      onkeydown={(e) => {
        if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && canCommit) commit(false)
      }}></textarea>

    <div class="check-state" role="status">
      {#if !staged.length}
        <span class="muted">Nothing staged yet.</span>
      {:else if checked}
        <span class="ok"><Icon name="tick" /> Checks passed{app.status?.staged_check ? `: ${builtText(app.status.staged_check)}` : ''}.</span>
      {:else if checking}
        <span class="busy"><span class="spinner" aria-hidden="true"></span>Checking the staged files…</span>
        <button class="link" onclick={() => stagedJob && (app.inspect = { kind: 'job', id: stagedJob.id })}>Log</button>
      {:else if stagedJob?.state === 'failed'}
        <span class="bad"><Icon name="alert" /> The checks failed.</span>
        <button class="link" onclick={() => stagedJob && (app.inspect = { kind: 'job', id: stagedJob.id })}>Show the log</button>
      {:else}
        <span class="warn">Not checked yet: run the checks before committing.</span>
      {/if}
    </div>

    <div class="buttons">
      <button onclick={checkStaged} disabled={!staged.length || checking} title="Runs {targetsHint} on the staged files; pick dry builds in Checks">
        <Icon name="play" /> Run checks
      </button>
      <button class="primary" disabled={!canCommit} onclick={() => commit(false)}>{committing ? 'Committing…' : `Commit${staged.length ? ` ${staged.length} file${staged.length === 1 ? '' : 's'}` : ''}`}</button>
    </div>
    {#if commitHint && files.length}<p class="muted small hint">{commitHint}</p>{/if}
    {#if commitError}<p class="error small" role="alert">{commitError}</p>{/if}
    {#if staged.length && hasMessage && !checked && !checking && !committing}
      <button class="link unchecked" onclick={() => commit(true)}>Commit without checks…</button>
    {/if}
    {#if checked && somethingToTry && tryOn}
      <div class="try" role="group" aria-label="Try on one machine first">
        <span class="muted small">Or first</span>
        {#if tryable.length > 1}
          <select aria-label="Machine to try on" value={tryOn.host} onchange={(e) => (tryHost = e.currentTarget.value)}>
            {#each tryable as m (m.host)}<option value={m.host}>{m.host}</option>{/each}
          </select>
        {/if}
        <button
          class="small"
          disabled={!canTry}
          onclick={() => tryOn && tryOnMachine(tryOn)}
          title={canTry
            ? `Push ${staged.length ? 'this, as a commit,' : 'the commits not pushed yet'} to ${tryOn.testing}: comin deploys it on ${tryOn.host} only, until you promote it or end the trial`
            : staged.length
              ? 'Write a commit message first.'
              : ''}>
          <Icon name="deploy" /> {repo.busy === 'try' ? 'Pushing…' : tryable.length > 1 ? 'Try on it' : `Try on ${tryOn.host}`}
        </button>
      </div>
    {/if}
  </div>

  <div class="files-head">
    <label class="all" title={allStaged ? 'Unstage everything' : 'Stage everything'}>
      <input type="checkbox" checked={allStaged} indeterminate={!allStaged && staged.length > 0} disabled={!files.length} onchange={toggleAll} />
      <span>Files</span>
      <span class="count num">{files.length}</span>
    </label>
    <span class="muted small">{staged.length} staged</span>
    <span class="spacer"></span>
    <button class="icon" title="Discard every unstaged change" aria-label="Discard every unstaged change" disabled={!files.some((f) => f.unstaged)} onclick={() => discard(files.filter((f) => f.unstaged).map((f) => f.path))}>
      <Icon name="undo" />
    </button>
    <button class="icon" title="Refresh" aria-label="Refresh the file list" onclick={refreshStatus}><Icon name="refresh" /></button>
  </div>

  <ul class="list files">
    {#each files as f (f.path)}
      {@const p = f.path.replace(/\/$/, '')}
      {@const full = !!f.staged && !f.unstaged}
      <li class="item">
        <input type="checkbox" class="stage" checked={full} indeterminate={!!f.staged && !!f.unstaged} aria-label="{full ? 'Unstage' : 'Stage'} {f.path}" onchange={() => toggle(f)} />
        <button class="item-main" title={f.from ? `${f.from} → ${f.path}` : f.path} onclick={() => show(f)}>
          <span class="name">{basename(p)}</span>
          <span class="dir">{dirname(p)}</span>
        </button>
        {#if f.unstaged}
          <span class="item-tools">
            <button class="icon" title="Discard the unstaged changes" aria-label="Discard changes in {f.path}" onclick={() => discard([f.path])}><Icon name="undo" /></button>
          </span>
        {/if}
        <span class="word w-{cls(f)}" title={f.staged && f.unstaged ? 'Partly staged' : f.staged ? 'Staged' : 'Not staged'}>{word(f)}</span>
      </li>
    {/each}
  </ul>
  {#if !files.length}<p class="empty">Nothing to commit: the working copy matches the last commit.</p>{/if}
</section>

<style>
  .panel {
    overflow: auto;
  }
  .commit-box {
    display: flex;
    flex: none;
    flex-direction: column;
    align-items: flex-start;
    margin-bottom: var(--lh);
  }
  textarea {
    margin-bottom: var(--lh);
  }
  .ok {
    color: var(--green);
  }
  .warn {
    color: var(--yellow);
  }
  .warn::before {
    content: '▲ ';
  }
  .bad {
    color: var(--red);
  }
  .busy {
    color: var(--accent);
  }
  .check-state {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .buttons {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .error {
    color: var(--red);
    white-space: pre-wrap;
  }
  .unchecked {
    color: var(--fg-muted);
  }
  .unchecked:hover:not(:disabled) {
    color: var(--on-accent);
    background: var(--red);
  }
  .try {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 1ch;
    margin-top: var(--lh);
  }
  .try select {
    width: auto;
    max-width: 50%;
  }
  .files-head {
    display: flex;
    flex: none;
    align-items: baseline;
    gap: 2ch;
  }
  .all {
    display: inline-flex;
    gap: 1ch;
    color: var(--fg-strong);
    font-weight: 700;
    cursor: pointer;
  }
  .stage {
    flex: none;
  }
  .word {
    --c: var(--yellow);
    flex: none;
    min-width: 7ch;
    color: var(--c);
    text-align: right;
  }
  .w-new {
    --c: var(--green);
  }
  .w-del {
    --c: var(--red);
  }
  .w-ren {
    --c: var(--blue);
  }
</style>
