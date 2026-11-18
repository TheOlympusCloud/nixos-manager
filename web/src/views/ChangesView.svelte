<script lang="ts">
  import { onMount } from 'svelte'
  import ChangesPanel from '../components/ChangesPanel.svelte'
  import ChecksPanel from '../components/ChecksPanel.svelte'
  import DiffView from '../components/DiffView.svelte'
  import Icon from '../components/Icon.svelte'
  import JobLog from '../components/JobLog.svelte'
  import { api, type HostDiff } from '../lib/api'
  import { waitJob } from '../lib/fleet.svelte'
  import { firstLine } from '../lib/format'
  import { diffParts, diffSummary, splitDiffs } from '../lib/matrix'
  import { codeLink, machineLink } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'

  // ---- what changes on each machine ----
  let diffs = $state<HostDiff[] | null>(null)
  let diffError = $state('')
  let headError = $state('')
  let computing = $state(false)
  let alive = true
  let seq = 0

  async function loadDiffs() {
    const mine = ++seq
    try {
      const r = await api.modelDiff()
      if (mine !== seq) return
      if (r.state === 'computing') {
        computing = true
        await Promise.all(r.jobs.map(waitJob))
        if (alive && mine === seq) loadDiffs()
        return
      }
      diffs = r.hosts ?? []
      diffError = r.error ?? ''
      headError = r.headError ?? ''
      computing = false
    } catch (e) {
      if (mine === seq) diffError = e instanceof Error ? e.message : String(e)
      computing = false
    }
  }
  onMount(() => () => (alive = false))
  // Any change to the working copy or HEAD (save, edit by clicking, commit, pull) refreshes the comparison.
  let timer: ReturnType<typeof setTimeout> | undefined
  $effect(() => {
    void app.status
    clearTimeout(timer)
    timer = setTimeout(loadDiffs, diffs ? 300 : 0)
  })

  const split = $derived(splitDiffs(diffs ?? []))
  const statusLabel: Record<string, string> = { added: 'New', removed: 'Removed', changed: 'Changed', unchanged: 'Unchanged', error: 'Does not evaluate' }
  const chipName = (label: string, s: string) => (label === 'Features' ? s.replace(/^[a-z]+\./, '') : s)

  // ---- the file diff or job log next to the lists ----
  let side = $state<'commit' | 'checks'>('commit')
  $effect(() => {
    if (app.inspect?.kind === 'job') side = 'checks'
  })
  let text = $state<string | null>(null)
  let textError = $state('')
  async function loadText() {
    const i = app.inspect
    if (i?.kind === 'job') return
    text = null
    textError = ''
    try {
      text = await api.diff(i?.staged ?? false, i?.path)
    } catch (e) {
      textError = e instanceof Error ? e.message : String(e)
    }
  }
  $effect(() => {
    void app.inspect
    void app.status
    loadText()
  })
  const files = $derived(app.status?.files.length ?? 0)
</script>

<div class="cv">
  <div class="top">
    <div class="page-head">
      <h1>Changes</h1>
      <span class="sub">What the working copy changes compared with the last commit, then the files to review, check and commit.</span>
    </div>

    <section aria-labelledby="hd-title">
      <h2 id="hd-title" class="section-title">
        On the machines
        {#if computing}<span class="badge busy"><span class="dot busy"></span>Evaluating both versions…</span>{/if}
      </h2>
      {#if headError}
        <div class="card head-error" role="alert">
          <p><strong>The last commit does not evaluate</strong>, so every machine is compared with nothing and shows as new.</p>
          <pre class="errpre">{headError}</pre>
        </div>
      {/if}
      {#if diffError && !diffs?.length}
        <p class="error-text">{diffError}</p>
      {:else if diffs === null}
        <div class="hd-grid">{#each [0, 1, 2] as i (i)}<div class="card hd skel-card"><span class="skel" style:width="40%"></span><span class="skel" style:width="80%"></span></div>{/each}</div>
      {:else}
        {#if diffError}<p class="error-text small">The working copy does not evaluate: {firstLine(diffError)}</p>{/if}
        {#if split.changed.length}
          <div class="hd-grid">
            {#each split.changed as d (d.host)}
              <article class="card hd st-{d.status}">
                <header>
                  <a class="host" href={machineLink(d.host)}>{d.host}</a>
                  <span class="badge {d.status === 'error' ? 'err' : d.status === 'added' ? 'ok' : ''}">{statusLabel[d.status]}</span>
                  {#if d.status !== 'error'}<span class="summary muted small">{diffSummary(d)}</span>{/if}
                </header>
                {#if d.error}
                  <pre class="errpre">{d.error}</pre>
                {/if}
                {#each diffParts(d) as part (part.label)}
                  <div class="part">
                    <span class="p-label">{part.label}</span>
                    <div class="chips">
                      {#each part.added as a (a)}<span class="chip add" class:mono={part.label === 'Packages'} title={a}><Icon name="plus" size={11} />{chipName(part.label, a)}</span>{/each}
                      {#each part.removed as r (r)}<span class="chip del" class:mono={part.label === 'Packages'} title={r}><Icon name="minus" size={11} />{chipName(part.label, r)}</span>{/each}
                    </div>
                  </div>
                {/each}
              </article>
            {/each}
          </div>
        {:else}
          <p class="calm"><Icon name="tick" size={15} />No machine changes: {files ? 'the changed files do not change what any machine runs.' : 'the working copy matches the last commit.'}</p>
        {/if}
        {#if split.unchanged.length && split.changed.length}
          <p class="unchanged muted small">Unchanged: {split.unchanged.map((d) => d.host).join(', ')}</p>
        {/if}
      {/if}
    </section>
  </div>

  <div class="bench">
    <aside class="side">
      <div class="side-tabs" role="tablist" aria-label="Commit or checks">
        <button role="tab" aria-selected={side === 'commit'} class:on={side === 'commit'} onclick={() => (side = 'commit')}>Commit {#if files}<span class="n num">{files}</span>{/if}</button>
        <button role="tab" aria-selected={side === 'checks'} class:on={side === 'checks'} onclick={() => (side = 'checks')}>Checks</button>
      </div>
      <div class="side-body" hidden={side !== 'commit'}><ChangesPanel /></div>
      <div class="side-body" hidden={side !== 'checks'}><ChecksPanel visible={side === 'checks'} /></div>
    </aside>
    <section class="pane" aria-label="Details">
      {#if app.inspect?.kind === 'job'}
        {#key app.inspect.id}<JobLog jobId={app.inspect.id} />{/key}
      {:else}
        {@const i = app.inspect}
        <div class="view-bar">
          <strong>{i?.path ?? 'All files'}</strong>
          <span class="muted small what">{i?.staged ? 'staged, against the last commit' : 'working copy, against what is staged'}</span>
          {#if i?.path}<button class="link" onclick={() => (app.inspect = null)}>Show all files</button>{/if}
          {#if i?.path}<a class="btn" href={codeLink(i.path)}><Icon name="code" size={14} />Open in Code</a>{/if}
          <button class="icon" aria-label="Refresh the diff" title="Refresh" onclick={loadText}><Icon name="refresh" size={15} /></button>
        </div>
        <div class="diff-body">
          {#if textError}
            <p class="error-text pad">{textError}</p>
          {:else if text === null}
            <div class="pad">{#each [0, 1, 2, 3] as k (k)}<span class="skel" style:width="{70 - k * 12}%" style:margin-bottom="10px"></span>{/each}</div>
          {:else}
            <DiffView {text} />
          {/if}
        </div>
      {/if}
    </section>
  </div>
</div>

<style>
  .cv {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: auto;
  }
  .top {
    flex: none;
    max-height: 42vh;
    padding: var(--s5) var(--s5) var(--s4);
    overflow: auto;
    border-bottom: 1px solid var(--border);
  }
  .section-title {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .hd-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: var(--s3);
  }
  .hd {
    display: grid;
    align-content: start;
    gap: 10px;
    padding: 12px 14px;
  }
  .hd.st-error {
    border-color: color-mix(in srgb, var(--red) 30%, var(--border));
  }
  .hd header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }
  .host {
    color: var(--fg-strong);
    font-size: var(--fs-lg);
    font-weight: 600;
    text-decoration: none;
  }
  .host:hover {
    color: var(--accent);
  }
  .summary {
    flex-basis: 100%;
  }
  .part {
    display: grid;
    grid-template-columns: 72px 1fr;
    gap: 8px;
  }
  .p-label {
    padding-top: 2px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .head-error {
    display: grid;
    gap: 8px;
    margin-bottom: 12px;
    padding: 12px 14px;
    border-color: color-mix(in srgb, var(--red) 45%, transparent);
  }
  .head-error p {
    margin: 0;
  }
  .errpre {
    margin: 0;
    max-height: 120px;
    padding: 8px;
    overflow: auto;
    color: color-mix(in srgb, var(--red) 70%, var(--fg-strong));
    font: 11.5px/1.5 var(--mono);
    white-space: pre-wrap;
    background: var(--bg-deep);
    border-radius: var(--r);
  }
  .skel-card {
    gap: 14px;
    min-height: 90px;
  }
  .calm {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--fg);
  }
  .calm :global(svg) {
    color: var(--green);
  }
  .unchanged {
    margin-top: var(--s3);
  }
  .bench {
    display: grid;
    flex: 1;
    grid-template-columns: 340px 1fr;
    min-height: 300px;
  }
  .side {
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg-side);
    border-right: 1px solid var(--border);
  }
  .side-tabs {
    display: flex;
    gap: 4px;
    padding: 8px 8px 0;
    border-bottom: 1px solid var(--border);
  }
  .side-tabs button {
    height: 34px;
    color: var(--fg-muted);
    background: none;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
  }
  .side-tabs button.on {
    color: var(--fg-strong);
    border-bottom-color: var(--accent);
  }
  .n {
    padding: 0 6px;
    font-size: var(--fs-xs);
    line-height: 17px;
    background: var(--bg-active);
    border-radius: 9px;
  }
  .side-body {
    flex: 1;
    min-height: 0;
  }
  .side-body[hidden] {
    display: none;
  }
  .pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  .pane .view-bar {
    flex-wrap: nowrap;
  }
  .pane .view-bar strong {
    flex: none;
  }
  .what {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .diff-body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    background: var(--bg-deep);
  }
  .pad {
    padding: 16px;
  }
</style>
