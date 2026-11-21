<script lang="ts">
  import type { DeployOverview, MachineDeployment } from '../../lib/api'
  import { progress, rollout, short, stateDetail, stateLabel, stateTone } from '../../lib/deployments'
  import { plural } from '../../lib/format'
  import { link, machineLink } from '../../lib/router.svelte'
  import { ago } from '../../lib/util'

  /** The newest commits of the branch comin follows, and which machines run each. */
  let { ov }: { ov: DeployOverview } = $props()

  const r = $derived(rollout(ov))
  const p = $derived(progress(ov))
  // Down to the oldest commit a machine still runs, and at least three.
  const shown = $derived(r.rows.slice(0, Math.max(3, r.rows.findLastIndex((row) => row.machines.length) + 1)))
  const hidden = $derived(r.rows.length - shown.length)
  const tag = (m: MachineDeployment) => (m.state === 'current' ? '' : stateLabel(m))
</script>

{#snippet chip(m: MachineDeployment, label: string)}
  <a class="mchip t-{stateTone[m.state]}" href={machineLink(m.host)} title="{m.host}: {stateDetail(m)}">
    <span class="dot" aria-hidden="true"></span>{m.host}{#if label}<span class="st">{label}</span>{/if}
  </a>
{/snippet}

<section class="rollout card" aria-labelledby="rollout-title">
  <header class="r-head">
    <h2 id="rollout-title">Rollout on <span class="mono">{ov.branch}</span></h2>
    {#if p.tip && p.total}
      <span class="muted small num">{p.done} of {plural(p.total, 'machine')} on {p.tip.short}</span>
      <span class="bar" role="progressbar" aria-label="Machines on the last commit" aria-valuemin="0" aria-valuemax={p.total} aria-valuenow={p.done}>
        <span style:width="{(p.done / p.total) * 100}%"></span>
      </span>
    {/if}
  </header>

  {#if r.testing.length}
    <ul class="lane testing" aria-label="Machines on another branch">
      {#each r.testing as m (m.host)}
        <li>
          <span class="lane-name mono">{m.branch}</span>
          <a class="sha mono" href={link('history', [], { rev: m.commit })}>{short(m.commit)}</a>
          <span class="subject">{m.subject ?? ''}</span>
          <span class="chips">{@render chip(m, tag(m))}</span>
        </li>
      {/each}
    </ul>
  {/if}

  {#if shown.length}
    <ol class="commits">
      {#each shown as row, i (row.commit.sha)}
        <li class:tip={i === 0} class:used={row.machines.length > 0}>
          <span class="node" aria-hidden="true"></span>
          <a class="sha mono" href={link('history', [], { rev: row.commit.sha })} title="Open {row.commit.short} in History">{row.commit.short}</a>
          <span class="subject" title={row.commit.subject}>{row.commit.subject}</span>
          <span class="when muted small">{row.commit.author} · {ago(row.commit.date)}</span>
          <span class="chips">
            {#each row.machines as m (m.host)}{@render chip(m, tag(m))}{/each}
            {#if i === 0 && !row.machines.length && p.total}<span class="muted small">no machine yet</span>{/if}
          </span>
        </li>
      {/each}
    </ol>
    {#if hidden}<p class="more muted small">{plural(hidden, 'older commit')} without machines not shown</p>{/if}
  {:else}
    <p class="muted empty-line">The branch {ov.branch} has no commits in this working copy yet. Push, or pull what Forgejo has.</p>
  {/if}

  {#if r.older.length || r.waiting.length}
    <div class="lane-foot">
      {#if r.older.length}
        <div class="lane">
          <span class="lane-name">Older or unknown</span>
          <span class="chips">{#each r.older as m (m.host)}{@render chip(m, m.commit ? `${stateLabel(m)} · ${short(m.commit)}` : stateLabel(m))}{/each}</span>
        </div>
      {/if}
      {#if r.waiting.length}
        <div class="lane">
          <span class="lane-name">Not reporting</span>
          <span class="chips">{#each r.waiting as m (m.host)}{@render chip(m, '')}{/each}</span>
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .rollout {
    padding: var(--s4) var(--s4) var(--s3);
  }
  .r-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s3);
    margin-bottom: var(--s3);
  }
  .r-head h2 {
    font-size: var(--fs-lg);
  }
  .bar {
    width: 120px;
    height: 4px;
    overflow: hidden;
    background: var(--bg-active);
    border-radius: 2px;
  }
  .bar span {
    display: block;
    height: 100%;
    background: var(--green);
    border-radius: 2px;
    transition: width 520ms var(--ease);
  }
  .commits {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .commits li,
  .lane.testing li {
    position: relative;
    display: grid;
    grid-template-columns: 18px 64px minmax(0, 1fr) auto;
    grid-template-areas: 'node sha subject chips' 'node . when chips';
    align-items: center;
    column-gap: var(--s2);
    min-height: 46px;
    padding: 6px 0;
  }
  .lane.testing li {
    grid-template-columns: auto 64px minmax(0, 1fr) auto;
    grid-template-areas: 'name sha subject chips';
    min-height: 36px;
    padding: 4px 8px;
    margin-bottom: var(--s2);
    background: color-mix(in srgb, var(--purple) 6%, transparent);
    border: 1px dashed color-mix(in srgb, var(--purple) 35%, transparent);
    border-radius: var(--r);
    list-style: none;
  }
  .lane.testing {
    margin: 0;
    padding: 0;
  }
  .lane.testing .lane-name {
    grid-area: name;
    color: var(--purple);
  }
  /* The line of the branch, through each commit's node. */
  .commits li::before {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 8px;
    width: 2px;
    content: '';
    background: var(--line);
  }
  .commits li:first-child::before {
    top: 50%;
  }
  .commits li:last-child::before {
    bottom: 50%;
  }
  .node {
    z-index: 1;
    grid-area: node;
    width: 10px;
    height: 10px;
    margin-left: 4px;
    background: var(--bg-raised);
    border: 2px solid var(--line-strong);
    border-radius: 50%;
  }
  .tip .node {
    background: var(--accent);
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 22%, transparent);
  }
  .used:not(.tip) .node {
    border-color: var(--yellow);
  }
  .sha {
    grid-area: sha;
    color: var(--accent);
    font-size: 12px;
    text-decoration: none;
  }
  .sha:hover {
    text-decoration: underline;
  }
  .subject {
    grid-area: subject;
    overflow: hidden;
    color: var(--fg-strong);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .when {
    grid-area: when;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    grid-area: chips;
    justify-content: flex-end;
    gap: 6px;
    max-width: 520px;
  }
  .mchip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 24px;
    padding: 0 9px;
    color: var(--fg-strong);
    font-size: var(--fs-sm);
    font-weight: 500;
    white-space: nowrap;
    text-decoration: none;
    background: var(--bg-hover);
    border: 1px solid var(--line);
    border-radius: 12px;
    transition: border-color var(--t-fast) var(--ease);
  }
  .mchip:hover {
    border-color: var(--line-strong);
  }
  .mchip .st {
    color: var(--c);
    font-weight: 400;
  }
  .t-ok {
    --c: var(--green);
  }
  .t-warn {
    --c: var(--yellow);
    border-color: color-mix(in srgb, var(--yellow) 35%, var(--line));
  }
  .t-err {
    --c: var(--red);
    background: color-mix(in srgb, var(--red) 8%, var(--bg-hover));
    border-color: color-mix(in srgb, var(--red) 45%, var(--line));
  }
  .t-off {
    --c: var(--fg-muted);
    color: var(--fg-muted);
    background: none;
    border-style: dashed;
  }
  .t-off .dot {
    --c: var(--fg-faint);
  }
  .more {
    margin: 2px 0 0 26px;
  }
  .empty-line {
    margin: var(--s2) 0;
  }
  .lane-foot {
    display: grid;
    gap: var(--s2);
    margin-top: var(--s3);
    padding-top: var(--s3);
    border-top: 1px solid var(--border);
  }
  .lane-foot .lane {
    display: grid;
    grid-template-columns: 130px 1fr;
    align-items: center;
    gap: var(--s2);
  }
  .lane-foot .lane-name {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .lane-foot .chips {
    justify-content: flex-start;
    max-width: none;
  }
  @media (max-width: 760px) {
    .commits li {
      grid-template-columns: 18px 60px minmax(0, 1fr);
      grid-template-areas: 'node sha subject' 'node . when' 'node chips chips';
    }
    .chips {
      justify-content: flex-start;
    }
  }
</style>
