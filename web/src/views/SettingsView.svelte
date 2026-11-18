<script lang="ts">
  import TokensPanel from '../components/TokensPanel.svelte'
  import { app } from '../lib/state.svelte'

  const shortcuts: [string[], string][] = [
    [['/'], 'Search in the current view'],
    [['g', 'f'], 'Fleet'],
    [['g', 'm'], 'Machines'],
    [['g', 'p'], 'Packages'],
    [['g', 's'], 'Services'],
    [['g', 'o'], 'Options'],
    [['g', 'c'], 'Changes'],
    [['g', 'h'], 'History'],
    [['g', 'e'], 'Code'],
    [['g', ','], 'Settings'],
    [['Esc'], 'Close the details panel or a dialog'],
    [['Ctrl', 'S'], 'Save the open file (Code)'],
  ]
</script>

<div class="page">
  <div class="page-inner narrow">
    <div class="page-head"><h1>Settings</h1></div>

    <section class="card block">
      <h2>Repository</h2>
      <dl>
        <dt>Working copy</dt><dd class="mono">{app.info?.root}</dd>
        <dt>Branch</dt><dd class="mono">{app.status?.branch || app.info?.branch}</dd>
        <dt>Remote</dt><dd class="mono">{app.info?.remote || 'none'}</dd>
        <dt>Upstream</dt><dd class="mono">{app.status?.upstream || 'none'}</dd>
        <dt>Commit author</dt><dd>{app.info?.author}</dd>
      </dl>
      <p class="muted small">These come from the server's configuration (<span class="mono">NIXMGR_*</span> environment variables) and change only when it restarts.</p>
    </section>

    <div class="card block"><TokensPanel /></div>

    <section class="card block">
      <h2>Keyboard shortcuts</h2>
      <dl class="keys">
        {#each shortcuts as [keys, what] (what)}
          <dt>{#each keys as k, i (i)}{#if i && keys[0] === 'g'}<span class="then">then</span>{/if}<kbd>{k}</kbd>{/each}</dt>
          <dd>{what}</dd>
        {/each}
      </dl>
    </section>
  </div>
</div>

<style>
  .narrow {
    max-width: 860px;
  }
  .block {
    display: grid;
    gap: 12px;
    margin-bottom: var(--s4);
    padding: var(--s4) var(--s5);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  dl {
    display: grid;
    grid-template-columns: 160px 1fr;
    gap: 8px 16px;
    margin: 0;
  }
  dt {
    color: var(--fg-muted);
  }
  dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .keys dt {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .then {
    color: var(--fg-muted);
    font-size: var(--fs-xs);
  }
</style>
