<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import ChangesPanel from '../components/ChangesPanel.svelte'
  import ChecksPanel from '../components/ChecksPanel.svelte'
  import DiffView, { fileAnchor } from '../components/DiffView.svelte'
  import Icon from '../components/Icon.svelte'
  import JobLog from '../components/JobLog.svelte'
  import MachineImpact from '../components/MachineImpact.svelte'
  import { api, type Commit } from '../lib/api'
  import { plural } from '../lib/format'
  import type { SettingChange } from '../lib/impact'
  import { fetchModelDiff, managedFiles, managedSettings, type ModelDiff } from '../lib/modeldiff'
  import { parsePatch, type PatchFile } from '../lib/patch'
  import { link } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { ago } from '../lib/util'

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
      const r = await fetchModelDiff(undefined, () => (computing = true), () => alive && mine === seq)
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
      el.classList.add('flash')
      setTimeout(() => el.classList.remove('flash'), 900)
    })
  })

  const count = $derived(app.status?.files.length ?? 0)
  const staged = $derived(app.status?.files.filter((f) => f.staged).length ?? 0)
  const adds = $derived((files ?? []).reduce((n, f) => n + f.adds, 0))
  const dels = $derived((files ?? []).reduce((n, f) => n + f.dels, 0))
</script>

<div class="cv">
  <aside class="side">
    <div class="side-tabs" role="tablist" aria-label="Commit or checks">
      <button role="tab" aria-selected={side === 'commit'} class:on={side === 'commit'} onclick={() => (side = 'commit')}>Commit {#if count}<span class="n num">{count}</span>{/if}</button>
      <button role="tab" aria-selected={side === 'checks'} class:on={side === 'checks'} onclick={() => (side = 'checks')}>Checks</button>
    </div>
    <div class="side-body" hidden={side !== 'commit'}><ChangesPanel /></div>
    <div class="side-body" hidden={side !== 'checks'}><ChecksPanel visible={side === 'checks'} /></div>
  </aside>

  <section class="pane" aria-label="Changes">
    {#if job}
      <div class="log-bar">
        <button class="ghost" onclick={() => (app.inspect = null)}><Icon name="left" size={14} />Back to the changes</button>
      </div>
      <div class="log">{#key job}<JobLog jobId={job} />{/key}</div>
    {:else}
      <div class="doc" bind:this={doc}>
        <header class="doc-head">
          <h1>Changes</h1>
          <p class="sub num">
            {#if count}
              {plural(count, 'changed file')}, {staged} staged{#if files}<span class="sep">·</span><span class="a">+{adds}</span>&nbsp;<span class="r">−{dels}</span>&nbsp;lines{/if}
            {:else}
              The working copy matches the last commit.
            {/if}
          </p>
        </header>

        <section class="block" aria-labelledby="ch-machines">
          <h2 id="ch-machines">On the machines</h2>
          {#if !count}
            <p class="calm"><Icon name="tick" size={15} />No machine changes.</p>
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
            <div class="skel-files" aria-hidden="true">{#each [0, 1, 2] as k (k)}<span class="skel" style:width="{72 - k * 14}%"></span>{/each}</div>
          {:else if !files.length}
            <div class="clean">
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
  .cv {
    display: grid;
    grid-template-columns: 340px 1fr;
    height: 100%;
  }
  /* Phones: the side panel above the content, one scroll for both. */
  @media (max-width: 760px) {
    .cv {
      grid-template-columns: minmax(0, 1fr);
      align-content: start;
      overflow-y: auto;
    }
    .side {
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
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
    flex: none;
    gap: 4px;
    padding: 6px 8px 0;
    border-bottom: 1px solid var(--border);
  }
  .side-tabs button {
    height: 36px;
    color: var(--fg-muted);
    background: none;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
  }
  .side-tabs button:hover:not(:disabled) {
    color: var(--fg-strong);
    background: none;
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
  .doc {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: var(--s5) var(--s5) var(--s6);
  }
  .doc-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 6px 14px;
    max-width: 1180px;
    margin-bottom: var(--s4);
  }
  .doc-head h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.01em;
  }
  .sub {
    color: var(--fg-muted);
  }
  .sep {
    margin: 0 6px;
  }
  .a {
    color: var(--green);
  }
  .r {
    color: var(--red);
  }
  .block {
    max-width: 1180px;
  }
  .block + .block {
    margin-top: var(--s5);
  }
  .block h2 {
    margin-bottom: 10px;
    font-size: var(--fs-lg);
  }
  .block :global(.diff) {
    padding: 0;
  }
  .skel-files {
    display: grid;
    gap: 12px;
    padding: 16px;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .calm {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .calm :global(svg) {
    color: var(--green);
  }
  .clean {
    display: grid;
    gap: 6px;
    padding: 14px 16px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    box-shadow: var(--edge);
  }
  .log-bar {
    display: flex;
    flex: none;
    align-items: center;
    min-height: 40px;
    padding: 4px 8px;
    background: var(--bg-side);
    border-bottom: 1px solid var(--border);
  }
  .log {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
</style>
