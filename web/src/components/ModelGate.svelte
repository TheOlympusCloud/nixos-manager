<script lang="ts">
  import type { Snippet } from 'svelte'
  import { errorFile, evaluateAgain, fleet, loadModel } from '../lib/fleet.svelte'
  import { frame } from '../lib/ascii'
  import { codeLink } from '../lib/router.svelte'
  import Icon from './Icon.svelte'
  import JobLog from './JobLog.svelte'

  /** Shows `children` once the fleet model has machines; otherwise the computing, error or empty state. */
  let { children, title = 'fleet' }: { children: Snippet; title?: string } = $props()

  const m = $derived(fleet.model)
  const noNix = $derived(!!m?.error && /is Nix installed|cannot run nix/i.test(m.error))
  const broken = '╭──────────╮\n│ ✖  ✖     │\n│   ━━━    │\n╰──────────╯'
  const empty = '┌┄┄┄┄┄┄┐ ┌┄┄┄┄┄┄┐\n┆  +   ┆ ┆      ┆\n└┄┄┄┄┄┄┘ └┄┄┄┄┄┄┘'
</script>

{#if m && !m.error && m.hosts.length}
  {@render children()}
{:else if !m && fleet.loadError}
  <div class="empty-state" role="alert" data-tone="err" use:frame>
    <pre class="art err" aria-hidden="true">{broken}</pre>
    <h2>Cannot load the {title}</h2>
    <pre>{fleet.loadError}</pre>
    <div class="row"><button class="primary" onclick={loadModel}><Icon name="refresh" /> Try again</button></div>
  </div>
{:else if !m}
  <div class="empty-state wide" role="status" use:frame>
    <h2><span class="spinner" aria-hidden="true"></span>Evaluating the flake…</h2>
    <p>Nix is evaluating every machine in the working copy. The first evaluation takes a while; later ones reuse what has not changed.</p>
    <div class="skel-cards" aria-hidden="true">
      {#each [0, 1, 2] as i (i)}<span class="skel" style:width="{i < 2 ? 40 : 28}ch"></span>{/each}
    </div>
    {#if fleet.job}<div class="log" use:frame={'square'}>{#key fleet.job}<JobLog jobId={fleet.job} compact />{/key}</div>{/if}
  </div>
{:else if m.error}
  <div class="empty-state" role="alert" data-tone="err" use:frame>
    <pre class="art err" aria-hidden="true">{broken}</pre>
    {#if noNix}
      <h2>Nix is not installed on the server</h2>
      <p>nixos-manager evaluates the flake with Nix to show what each machine runs. Install Nix where the server runs (or use the container image, which includes it), then restart nixos-manager.</p>
    {:else}
      <h2>The flake does not evaluate</h2>
      <p>Nix could not list the machines of the working copy. Fix the error below in Code, then run the checks.</p>
    {/if}
    <pre>{m.error}</pre>
    <div class="row">
      {#if !noNix}
        <a class="btn primary" href={codeLink(errorFile(m.error) ?? 'flake.nix')}><Icon name="code" /> Open {errorFile(m.error) ?? 'flake.nix'} in Code</a>
        <a class="btn" href="#/changes">Changes and checks</a>
      {/if}
      <button class="btn" onclick={evaluateAgain} disabled={fleet.computing}><Icon name="refresh" /> Evaluate again</button>
    </div>
    {#if !noNix}<p class="small muted">The machines are evaluated again as soon as the working copy changes.</p>{/if}
    {#if fleet.computing}<p class="small"><span class="dot busy"></span> Re-evaluating…</p>{/if}
  </div>
{:else}
  <div class="empty-state" use:frame={'dash'}>
    <pre class="art" aria-hidden="true">{empty}</pre>
    <h2>No machines yet</h2>
    <p>
      The flake has no <span class="mono">nixosConfigurations</span>. The starter flake shows the layout: one folder per machine under
      <span class="mono">hosts/</span>, listed in <span class="mono">flake.nix</span>. Copy <span class="mono">hosts/example</span>, give it the machine's name and
      hardware configuration, and add it to <span class="mono">flake.nix</span>.
    </p>
    <div class="row">
      <a class="btn primary" href={codeLink('flake.nix')}><Icon name="code" /> Open flake.nix</a>
      <a class="btn" href="#/code">Browse the repository</a>
    </div>
  </div>
{/if}

<style>
  /* The states of the prototype's #/states: centred in a frame, glyph art over the heading. */
  .empty-state {
    justify-items: center;
    text-align: center;
  }
  /* app.css styles every `.empty-state pre` as an error block: not the frame's glyphs. */
  .empty-state > :global(pre.fr) {
    width: auto;
    max-height: none;
    padding: 0;
    overflow: visible;
    white-space: pre;
    background: none;
  }
  .wide {
    max-width: 90ch;
  }
  h2 {
    text-transform: none;
  }
  h2::before {
    content: none;
  }
  .empty-state pre:not(.art) {
    text-align: left;
  }
  .art {
    width: auto;
    padding: 0;
    color: var(--accent);
    text-align: left;
    white-space: pre;
    background: none;
  }
  .art.err {
    color: var(--red);
  }
  .skel-cards {
    display: grid;
    justify-items: center;
    gap: var(--lh);
    width: 100%;
  }
  .log {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: calc(var(--lh) * 13);
    overflow: hidden;
    text-align: left;
  }
  .small {
    display: flex;
    align-items: baseline;
    gap: 1ch;
  }
</style>
