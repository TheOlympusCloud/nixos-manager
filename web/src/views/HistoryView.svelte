<script lang="ts">
  import CommitPane from '../components/CommitPane.svelte'
  import HistoryPanel from '../components/HistoryPanel.svelte'
  import Icon from '../components/Icon.svelte'
  import ViewPane from '../components/ViewPane.svelte'
  import type { Commit } from '../lib/api'
  import { link, replace, route } from '../lib/router.svelte'
  import { app, type ViewTab } from '../lib/state.svelte'

  /** `path` (from #/history/<path>) adds that file's history: versions, compare and restore. `?rev=` opens a commit. */
  let { path }: { path: string } = $props()

  let view = $state<ViewTab | null>(null)
  let commits = $state<Commit[] | null>(null)
  let n = 0
  function open(t: ViewTab) {
    view = { ...t, n: ++n }
    // A commit is part of the URL, so "View in History" and reloads land on it.
    replace(link('history', path ? path.split('/') : [], { rev: t.kind === 'commit' ? t.rev : undefined }))
  }
  let pick = $derived(path)
  const files = $derived(app.entries.filter((e) => !e.dir).map((e) => e.path))

  // Open the commit in the URL, else the latest one, once the log is known.
  function loaded(cs: Commit[]) {
    commits = cs
    const rev = route.query.rev
    if (view?.kind === 'commit' && (!rev || view.rev.startsWith(rev))) return
    const c = (rev && cs.find((x) => x.sha.startsWith(rev))) || (!path && !view ? cs[0] : undefined)
    if (c) open({ kind: 'commit', id: 'commit:' + c.sha, title: `${c.short} ${c.subject}`, rev: c.sha })
    else if (rev) view = { kind: 'commit', id: 'commit:' + rev, title: rev, rev, n: ++n }
  }

  function choose(e: SubmitEvent) {
    e.preventDefault()
    const p = pick.trim()
    replace(link('history', p ? p.split('/') : []))
  }
</script>

<div class="hv">
  <aside class="side">
    <form class="pick" onsubmit={choose}>
      <label for="hist-file" class="label">File history</label>
      <div class="row">
        <input id="hist-file" class="mono" list="hist-files" placeholder="Any file, e.g. flake.nix" spellcheck="false" autocomplete="off" data-search bind:value={pick} />
        <button type="submit">Show</button>
      </div>
      <datalist id="hist-files">{#each files as f (f)}<option value={f}></option>{/each}</datalist>
      {#if path}<button type="button" class="link small" onclick={() => ((pick = ''), replace('#/history'))}>Only the repository history</button>{/if}
    </form>
    <div class="panel-wrap">
      {#key path}<HistoryPanel {path} onopen={open} onload={loaded} current={view?.id ?? ''} />{/key}
    </div>
  </aside>
  <section class="pane">
    {#if view?.kind === 'commit'}
      {#key view.rev + '#' + view.n}<CommitPane rev={view.rev} />{/key}
    {:else if view}
      {#key view.id + '#' + view.n}<ViewPane tab={view} onopen={open} />{/key}
    {:else if commits && !commits.length}
      <div class="hint">
        <Icon name="history" size={22} />
        <p><strong>No commits yet.</strong></p>
        <p class="muted">Every commit you make in Changes shows here, with what it did to each machine. You can revert any of them.</p>
        <a class="btn" href="#/changes">Go to Changes</a>
      </div>
    {:else if commits}
      <div class="hint muted">
        <Icon name="history" size={22} />
        <p>Pick a commit to see what it did to the machines{path ? `, or a version of ${path} to view, compare or restore it` : ''}.</p>
      </div>
    {/if}
  </section>
</div>

<style>
  .hv {
    display: grid;
    grid-template-columns: 360px 1fr;
    height: 100%;
  }
  .side {
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg-side);
    border-right: 1px solid var(--border);
  }
  .pick {
    display: grid;
    gap: 6px;
    padding: 12px;
    border-bottom: 1px solid var(--border);
  }
  .pick .row {
    display: flex;
    gap: 6px;
  }
  .pick .link {
    justify-self: start;
  }
  .panel-wrap {
    flex: 1;
    min-height: 0;
  }
  .pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  .hint {
    display: grid;
    justify-items: center;
    gap: 10px;
    max-width: 360px;
    margin: 20vh auto 0;
    text-align: center;
    line-height: 1.5;
  }
  .hint :global(svg) {
    color: var(--fg-muted);
  }
</style>
