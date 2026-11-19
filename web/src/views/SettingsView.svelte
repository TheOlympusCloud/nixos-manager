<script lang="ts">
  import ForgejoPanel from '../components/ForgejoPanel.svelte'
  import Icon from '../components/Icon.svelte'
  import TokensPanel from '../components/TokensPanel.svelte'
  import { app } from '../lib/state.svelte'

  const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
  const shortcuts: [string[], string][] = [
    [[mac ? '⌘' : 'Ctrl', 'K'], 'Search and run anything'],
    [['/'], 'Search in the current view'],
    [['g', 'f'], 'Fleet'],
    [['g', 'm'], 'Machines'],
    [['g', 'p'], 'Packages'],
    [['g', 's'], 'Services'],
    [['g', 'o'], 'Options'],
    [['g', 'a'], 'Health'],
    [['g', 'i'], 'Inputs'],
    [['g', 'c'], 'Changes'],
    [['g', 'h'], 'History'],
    [['g', 'e'], 'Code'],
    [['g', ','], 'Settings'],
    [['Esc'], 'Close the details panel or a dialog'],
    [[mac ? '⌘' : 'Ctrl', 'S'], 'Save the open file (Code)'],
    [[mac ? '⌘' : 'Ctrl', 'Enter'], 'Commit (in the commit message)'],
  ]

  const remote = $derived(app.info?.remote ?? '')
  const upstream = $derived(app.status?.upstream ?? '')
</script>

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <h1>Settings</h1>
      <span class="sub">The repository this console edits, API access, and shortcuts</span>
    </div>

    <div class="cols">
      <div class="col">
        <section class="card block" aria-labelledby="set-repo">
          <h2 id="set-repo">Repository</h2>
          <dl class="facts">
            <dt>Working copy</dt>
            <dd><span class="mono">{app.info?.root}</span><span class="env">NIXMGR_REPO</span></dd>
            <dt>Branch</dt>
            <dd><span class="mono">{app.status?.branch || app.info?.branch}</span></dd>
            <dt>Upstream</dt>
            <dd>{#if upstream}<span class="mono">{upstream}</span>{:else}<span class="none">{remote ? 'Not pushed yet' : 'None'}</span>{/if}</dd>
            <dt>Commit author</dt>
            <dd><span>{app.info?.author}</span><span class="env">NIXMGR_AUTHOR_NAME, NIXMGR_AUTHOR_EMAIL</span></dd>
          </dl>
          <p class="muted small">These come from the server’s environment variables (shown on the right) and change when it restarts with new values.</p>
          {#if remote && upstream}
            <p class="info">
              <Icon name="info" size={14} /><span>Machines running comin deploy what is pushed to <span class="mono">main</span>; a <span class="mono">testing-&lt;machine&gt;</span> branch deploys to that machine only, for a trial.</span>
            </p>
          {/if}
        </section>

        <ForgejoPanel />

        <div class="card block"><TokensPanel /></div>
      </div>

      <div class="col side">
        <section class="card block" aria-labelledby="set-keys">
          <h2 id="set-keys">Keyboard shortcuts</h2>
          <dl class="keys">
            {#each shortcuts as [keys, what] (what)}
              <dt>{#each keys as k, i (i)}{#if i && keys[0] === 'g'}<span class="then">then</span>{/if}<kbd>{k}</kbd>{/each}</dt>
              <dd>{what}</dd>
            {/each}
          </dl>
        </section>

        <section class="card block" aria-labelledby="set-about">
          <h2 id="set-about">About</h2>
          <p class="muted small">The story shown before login: how NixOS and this console fit together. <a href="#/intro">Watch the intro again</a>.</p>
        </section>
      </div>
    </div>
  </div>
</div>

<style>
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    gap: var(--s4);
    align-items: start;
  }
  .col {
    display: grid;
    gap: var(--s4);
    min-width: 0;
  }
  .block {
    display: grid;
    min-width: 0;
    gap: 12px;
    padding: var(--s4) var(--s5) var(--s5);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .facts {
    display: grid;
    grid-template-columns: 130px minmax(0, 1fr);
    gap: 10px 16px;
    margin: 0;
  }
  dt {
    color: var(--fg-muted);
  }
  dd {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 2px 12px;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .env {
    margin-left: auto;
    color: var(--fg-faint);
    font: 11px var(--mono);
  }
  .none {
    color: var(--fg-muted);
  }
  .info {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    color: var(--fg);
    line-height: 1.5;
  }
  .info :global(svg) {
    flex: none;
    margin-top: 3px;
    color: var(--accent);
  }
  .keys {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 8px 14px;
    margin: 0;
  }
  .keys dt {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .keys dd {
    color: var(--fg);
  }
  .then {
    color: var(--fg-muted);
    font-size: var(--fs-xs);
  }
  @media (max-width: 1100px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
