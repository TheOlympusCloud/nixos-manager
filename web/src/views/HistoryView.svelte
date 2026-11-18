<script lang="ts">
  import HistoryPanel from '../components/HistoryPanel.svelte'
  import Icon from '../components/Icon.svelte'
  import ViewPane from '../components/ViewPane.svelte'
  import { link, replace } from '../lib/router.svelte'
  import { app, type ViewTab } from '../lib/state.svelte'

  /** `path` (from #/history/<path>) adds that file's history: versions, compare and restore. */
  let { path }: { path: string } = $props()

  let view = $state<ViewTab | null>(null)
  let n = 0
  const open = (t: ViewTab) => (view = { ...t, n: ++n })
  let pick = $derived(path)
  const files = $derived(app.entries.filter((e) => !e.dir).map((e) => e.path))

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
      {#key path}<HistoryPanel {path} onopen={open} current={view?.id ?? ''} />{/key}
    </div>
  </aside>
  <section class="pane">
    {#if view}
      {#key view.id + '#' + view.n}<ViewPane tab={view} onopen={open} />{/key}
    {:else}
      <div class="hint muted">
        <Icon name="history" size={22} />
        <p>Pick a commit to see what it changed{path ? `, or a version of ${path} to view, compare or restore it` : ''}.</p>
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
    max-width: 320px;
    margin: 20vh auto 0;
    text-align: center;
  }
</style>
