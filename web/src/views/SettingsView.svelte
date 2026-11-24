<script lang="ts">
  import ForgejoPanel from '../components/ForgejoPanel.svelte'
  import Icon from '../components/Icon.svelte'
  import NotificationsPanel from '../components/NotificationsPanel.svelte'
  import SecretsPanel from '../components/SecretsPanel.svelte'
  import TokensPanel from '../components/TokensPanel.svelte'
  import { bigText, frame } from '../lib/ascii'
  import { app } from '../lib/state.svelte'
  import { applyTheme, theme } from '../lib/theme.svelte'

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
      <div>
        <h1 class="big" aria-label="Settings" use:bigText={'Settings'}></h1>
        <p class="sub">The repository this console edits, notifications, secrets, API access, and shortcuts</p>
      </div>
    </div>

    <div class="cols">
      <div class="col">
        <section class="card" aria-labelledby="set-repo" use:frame={{ title: 'Repository' }}>
          <h2 id="set-repo" class="sr-only">Repository</h2>
          <dl class="kv">
            <dt>Working copy</dt>
            <dd><span class="mono">{app.info?.root}</span><span class="env">NIXMGR_REPO</span></dd>
            <dt>Branch</dt>
            <dd><span class="mono"><Icon name="branch" /> {app.status?.branch || app.info?.branch}</span></dd>
            <dt>Upstream</dt>
            <dd>{#if upstream}<span class="mono">{upstream}</span>{:else}<span class="muted">{remote ? 'Not pushed yet' : 'None'}</span>{/if}</dd>
            <dt>Commit author</dt>
            <dd><span>{app.info?.author}</span><span class="env">NIXMGR_AUTHOR_NAME, NIXMGR_AUTHOR_EMAIL</span></dd>
          </dl>
          <p class="muted gap">These come from the server’s environment variables (shown on the right) and change when it restarts with new values.</p>
          {#if remote && upstream}
            <p class="info">
              <Icon name="info" /><span>Machines running comin deploy what is pushed to <span class="mono">main</span>; a <span class="mono">testing-&lt;machine&gt;</span> branch deploys to that machine only, for a trial: Changes → Try on pushes one.</span>
            </p>
          {/if}
        </section>

        <ForgejoPanel />

        <NotificationsPanel />

        <SecretsPanel />

        <div class="card" use:frame={{ title: 'API tokens' }}><TokensPanel /></div>
      </div>

      <div class="col">
        <section class="card" aria-labelledby="set-keys" use:frame={{ title: 'Keyboard shortcuts' }}>
          <h2 id="set-keys" class="sr-only">Keyboard shortcuts</h2>
          <dl class="kv keys">
            {#each shortcuts as [keys, what] (what)}
              <dt>{#each keys as k, i (i)}{#if i && keys[0] === 'g'}<span class="then">then</span>{/if}<kbd>{k}</kbd>{/each}</dt>
              <dd>{what}</dd>
            {/each}
          </dl>
        </section>

        <section class="card" aria-labelledby="set-theme" use:frame={{ kind: 'dash', title: 'Theme' }}>
          <h2 id="set-theme" class="sr-only">Theme</h2>
          <div class="seg" role="group" aria-labelledby="set-theme">
            <button class="bare" aria-pressed={theme.name === 'night'} onclick={() => applyTheme('night')}>Night</button>
            <button class="bare" aria-pressed={theme.name === 'print'} onclick={() => applyTheme('print')}>Print</button>
          </div>
          <p class="muted">Night is the default. Print is the same console on greenbar paper, for daylight. This browser remembers the choice.</p>
        </section>

        <section class="card" aria-labelledby="set-about" use:frame={{ title: 'About' }}>
          <h2 id="set-about" class="sr-only">About</h2>
          <div class="about">
            <pre class="art" aria-hidden="true">{'  ╲  λ  ╱\n   ╲ │ ╱\n λ ──❄── λ\n   ╱ │ ╲\n  ╱  λ  ╲'}</pre>
            <p class="muted">The story shown before login: how NixOS and this console fit together. <a href="#/intro">Watch the intro again</a>.</p>
          </div>
        </section>
      </div>
    </div>
  </div>
</div>

<style>
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
    gap: var(--lh) 2ch;
    align-items: start;
  }
  .col {
    display: grid;
    gap: var(--lh);
    min-width: 0;
  }
  .kv {
    grid-template-columns: 15ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  dd {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
    overflow-wrap: anywhere;
  }
  .env {
    color: var(--fg-faint);
  }
  .gap {
    margin-top: var(--lh);
  }
  .info {
    display: flex;
    gap: 1ch;
  }
  .info :global(.ic) {
    color: var(--accent);
  }
  .keys {
    grid-template-columns: max-content minmax(0, 1fr);
  }
  .keys dt {
    display: flex;
    gap: 1ch;
  }
  .keys dd {
    color: var(--fg);
  }
  .then {
    color: var(--fg-muted);
  }
  /* The segmented control is made of buttons, which app.css strips of the │ between picks; put it back. */
  .seg > button + button::before {
    content: '│';
  }
  .seg > button:focus-visible {
    padding-right: 0;
  }
  .seg > button:focus-visible::after {
    content: '◂';
  }
  .seg + p {
    margin-top: var(--lh);
  }
  .about {
    display: flex;
    gap: 3ch;
  }
  .art {
    flex: none;
    color: var(--accent);
  }
  @media (max-width: 1100px) {
    .cols {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
