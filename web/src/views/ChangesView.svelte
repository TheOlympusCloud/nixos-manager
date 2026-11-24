<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import ChangesPanel from '../components/ChangesPanel.svelte'
  import ChecksPanel from '../components/ChecksPanel.svelte'
  import DeployBadge from '../components/deployments/DeployBadge.svelte'
  import DiffView, { fileAnchor } from '../components/DiffView.svelte'
  import Icon from '../components/Icon.svelte'
  import JobLog from '../components/JobLog.svelte'
  import MachineImpact from '../components/MachineImpact.svelte'
  import { api, type Commit } from '../lib/api'
  import { plural } from '../lib/format'
  import type { SettingChange } from '../lib/impact'
  import { fetchModelDiff, managedFiles, managedSettings, type ModelDiff } from '../lib/modeldiff'
  import { parsePatch, type PatchFile } from '../lib/patch'
  import { checkOf } from '../lib/checks'
  import { excerpt, stateDetail, trialOf } from '../lib/deployments'
  import { deploys, watchDeployments } from '../lib/deployments.svelte'
  import { endTrial, promoteTrial, repo } from '../lib/repo.svelte'
  import { link, machineLink, replace, route } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { ago } from '../lib/util'
  import { bigText, flash, frame, ink } from '../lib/ascii'

  let alive = true
  onMount(() => () => (alive = false))

  // ---- what changes on each machine (working copy against the last commit) ----
  let diff = $state<ModelDiff | null>(null)
  let diffError = $state('')
  let computing = $state(false)
  let seq = 0
  async function loadDiff() {
    const mine = ++seq
    try {
      const r = await fetchModelDiff({}, () => (computing = true), () => alive && mine === seq)
      if (mine !== seq) return
      if (r) ((diff = r), (diffError = ''))
    } catch (e) {
      if (mine === seq) diffError = e instanceof Error ? e.message : String(e)
    }
    if (mine === seq) computing = false
  }

  // ---- the files: staged and unstaged changes in one list ----
  let files = $state<(PatchFile & { tag?: string })[] | null>(null)
  let filesError = $state('')
  let fseq = 0
  async function loadFiles() {
    const mine = ++fseq
    try {
      const [st, wt] = await Promise.all([api.diff(true), api.diff(false)])
      if (mine !== fseq) return
      const a = parsePatch(st)
      const b = parsePatch(wt)
      const both = new Set(a.filter((f) => b.some((g) => g.path === f.path)).map((f) => f.path))
      const tagged = [...a.map((f) => ({ ...f, tag: both.has(f.path) ? 'Staged part' : undefined })), ...b.map((f) => ({ ...f, tag: both.has(f.path) ? 'Unstaged part' : undefined }))]
      files = tagged.sort((x, y) => x.path.localeCompare(y.path))
      filesError = ''
    } catch (e) {
      if (mine === fseq) filesError = e instanceof Error ? e.message : String(e)
    }
  }

  // Options set here, read from the changed managed.json files (the model does not list options).
  let settings = $state<Record<string, SettingChange[]>>({})
  async function loadSettings() {
    const files = app.status?.files ?? []
    const paths = managedFiles(files.map((f) => f.path))
    const isNew = (f: (typeof files)[number]) => f.staged === 'A' || f.staged === '?' || (!f.staged && f.unstaged === '?')
    const absent = { before: files.filter(isNew).map((f) => f.path), after: files.filter((f) => (f.unstaged || f.staged) === 'D').map((f) => f.path) }
    settings = paths.length ? await managedSettings(paths, 'HEAD', null, absent) : {}
  }

  let last = $state<Commit | null>(null)
  // Any change to the working copy or HEAD (save, edit by clicking, commit, pull) refreshes both, once.
  let timer: ReturnType<typeof setTimeout> | undefined
  $effect(() => {
    void app.status
    void app.logVersion
    untrack(() => {
      clearTimeout(timer)
      timer = setTimeout(
        () => {
          loadDiff()
          loadFiles()
          loadSettings()
          if (!app.status?.files.length) api.log({ limit: 1 }).then((c) => (last = c[0] ?? null), () => (last = null))
        },
        diff ? 250 : 0,
      )
    })
    return () => clearTimeout(timer)
  })

  // ---- the side: commit or checks; a check's log replaces the changes until closed ----
  let side = $state<'commit' | 'checks'>('commit')
  const job = $derived.by(() => {
    const i = app.inspect
    if (i?.kind !== 'job') return null
    const j = app.jobs.find((x) => x.id === i.id)
    return !j || j.kind === 'check' ? i.id : null
  })
  $effect(() => {
    if (job) untrack(() => (side = 'checks'))
  })
  // #/changes?check=<commit>, from Health and from Forgejo's commit statuses: that commit's newest check.
  let linked = ''
  $effect(() => {
    const sha = route.view === 'changes' ? route.query.check : ''
    if (!sha) return
    const j = checkOf(app.jobs, sha)
    untrack(() => {
      if (linked !== sha) ((linked = sha), (side = 'checks'))
      if (!j) return // jobs not loaded yet, or none since the server started: Checks lists the recent ones
      app.inspect = { kind: 'job', id: j.id }
      replace(link('changes'))
    })
  })

  let doc = $state<HTMLElement>()
  // A file picked in the list scrolls the combined diff to it.
  $effect(() => {
    const i = app.inspect
    if (i?.kind !== 'diff' || !i.path || !files) return
    const path = i.path
    untrack(async () => {
      await tick()
      const el = doc?.querySelector<HTMLElement>('#' + CSS.escape(fileAnchor(path)))
      if (!el) return
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
      flash(el, path) // reverse video for two ticks of the clock
    })
  })

  // On a machine's testing branch: what that machine does with the trial, and the way out.
  $effect(() => watchDeployments())
  const trial = $derived(trialOf(deploys.overview, app.status?.branch ?? ''))
  const unpushed = $derived(app.status?.upstream ? app.status.ahead : 0)

  const count = $derived(app.status?.files.length ?? 0)
  const staged = $derived(app.status?.files.filter((f) => f.staged).length ?? 0)
  const adds = $derived((files ?? []).reduce((n, f) => n + f.adds, 0))
  const dels = $derived((files ?? []).reduce((n, f) => n + f.dels, 0))
</script>

<div class="cv">
  <aside class="side" use:frame={{ title: side === 'commit' ? 'Commit' : 'Checks' }}>
    <div class="tabs" role="tablist" aria-label="Commit or checks" use:ink={side}>
      <button class="bare" role="tab" aria-selected={side === 'commit'} class:on={side === 'commit'} onclick={() => (side = 'commit')}>Commit {#if count}<small class="num">{count}</small>{/if}</button>
      <button class="bare" role="tab" aria-selected={side === 'checks'} class:on={side === 'checks'} onclick={() => (side = 'checks')}>Checks</button>
    </div>
    <div class="side-body" hidden={side !== 'commit'}><ChangesPanel /></div>
    <div class="side-body" hidden={side !== 'checks'}><ChecksPanel visible={side === 'checks'} /></div>
  </aside>

  <section class="pane" aria-label="Changes">
    {#if job}
      <div class="log-bar">
        <button class="ghost" onclick={() => (app.inspect = null)}><Icon name="left" /> Back to the changes</button>
      </div>
      <div class="log" use:frame={'square'}>{#key job}<JobLog jobId={job} />{/key}</div>
    {:else}
      <div class="doc" bind:this={doc}>
        <header class="doc-head">
          <h1 class="big" aria-label="Changes" use:bigText={'Changes'}></h1>
          <p class="sub num">
            {#if count}
              {plural(count, 'changed file')}, {staged} staged{#if files}<span class="sep">·</span><span class="a">+{adds}</span>&nbsp;<span class="r">−{dels}</span>&nbsp;lines{/if}
            {:else}
              The working copy matches the last commit.
            {/if}
          </p>
        </header>

        {#if trial}
          {@const promoteWhy = count ? 'Commit or discard the changes first.' : unpushed ? `Push first: ${trial.host} has not tried ${plural(unpushed, 'commit')} yet.` : ''}
          <section class="block" aria-labelledby="ch-trial">
            <h2 id="ch-trial">Trying on {trial.host}</h2>
            <div class="trial" use:frame>
              <p class="muted small">
                What is pushed to <span class="mono">{trial.testing}</span> deploys on {trial.host} only, with switch-to-configuration test: a reboot brings back what
                <span class="mono">{trial.main}</span> gave it.
              </p>
              <p class="trial-state">
                <DeployBadge m={trial} />
                <span>{trial.branch === trial.testing ? stateDetail(trial) : `comin ignores ${trial.testing} now: it is not on top of ${trial.main} on the remote, so ${trial.host} follows ${trial.main}. Promote it, or try again from ${trial.main}.`}</span>
                <a href={machineLink(trial.host)}>{trial.host}</a>
              </p>
              {#if trial.failure?.error}<pre>{excerpt(trial.failure.error)}</pre>{/if}
              <div class="row">
                <button class="primary" disabled={!!promoteWhy || !!repo.busy} title={promoteWhy || `Switch to ${trial.main} and stage ${trial.testing} as one change, to check, commit and push`} onclick={() => promoteTrial(trial)}>
                  Promote to {trial.main}
                </button>
                <button disabled={!!repo.busy} title="Point {trial.testing} back at {trial.main}, so comin switches {trial.host} back to it" onclick={() => endTrial(trial)}>End the trial</button>
              </div>
              {#if promoteWhy}<p class="muted small">{promoteWhy}</p>{/if}
            </div>
          </section>
        {/if}

        <section class="block" aria-labelledby="ch-machines">
          <h2 id="ch-machines">On the machines</h2>
          {#if !count}
            <p class="calm"><Icon name="tick" /> No machine changes.</p>
          {:else if diffError && !diff}
            <p class="error-text">{diffError}</p>
          {:else}
            <MachineImpact {diff} {computing} {settings} quiet="The changed files do not change what any machine runs." />
          {/if}
        </section>

        <section class="block" aria-labelledby="ch-files">
          <h2 id="ch-files">Files</h2>
          {#if filesError}
            <p class="error-text">{filesError}</p>
          {:else if files === null}
            <div class="skel-files" aria-hidden="true">{#each [0, 1, 2] as k (k)}<span class="skel" style:width="{60 - k * 12}ch"></span>{/each}</div>
          {:else if !files.length}
            <div class="clean" use:frame>
              <p>No file differs from the last commit.</p>
              {#if last}
                <p class="muted small">
                  Last commit: <a href={link('history', [], { rev: last.sha })}>{last.subject}</a> · {last.short} · {ago(last.date)}
                </p>
              {/if}
            </div>
          {:else}
            <DiffView {files} />
          {/if}
        </section>
      </div>
    {/if}
  </section>
</div>

<style>
  /* The side (commit or checks) and the document, like the prototype's .split. */
  .cv {
    display: flex;
    gap: 1ch;
    height: 100%;
    padding: var(--lh) 1ch;
  }
  .side {
    display: flex;
    flex: none;
    flex-direction: column;
    width: 44ch;
    min-height: 0;
  }
  /* Phones: the side panel above the content, one scroll for both. */
  @media (max-width: 760px) {
    .cv {
      flex-direction: column;
      overflow-y: auto;
    }
    .side {
      width: auto;
    }
  }
  /* Tabs are words, not [ buttons ]: strong under the pointer. */
  .tabs > button:hover:not(:disabled) {
    color: var(--fg-strong);
    background: none;
  }
  .side-body {
    flex: 1;
    min-height: 0;
  }
  .pane {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  .doc {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .doc-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 3ch;
    margin-bottom: var(--lh);
  }
  .sub {
    color: var(--fg-muted);
  }
  .sep {
    margin: 0 1ch;
  }
  .a {
    color: var(--green);
  }
  .r {
    color: var(--red);
  }
  .block {
    max-width: 140ch;
  }
  .block + .block {
    margin-top: var(--lh);
  }
  .block h2 {
    margin-bottom: var(--lh);
  }
  .block :global(.diff) {
    padding: 0;
  }
  .trial {
    display: grid;
    gap: var(--lh);
  }
  .trial-state {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .trial .row {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .trial pre {
    padding: 0 1ch;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    background: var(--bg-input);
  }
  .calm {
    display: flex;
    gap: 1ch;
  }
  .calm :global(.ic) {
    color: var(--green);
  }
  .log-bar {
    flex: none;
    margin-bottom: var(--lh);
  }
  .log {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
</style>
