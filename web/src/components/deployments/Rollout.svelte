<script lang="ts">
  import type { DeployOverview, MachineDeployment } from '../../lib/api'
  import { bar, frame } from '../../lib/ascii'
  import { progress, rollout, short, stateDetail, stateLabel, stateTone } from '../../lib/deployments'
  import { plural } from '../../lib/format'
  import { link, machineLink } from '../../lib/router.svelte'
  import { ago } from '../../lib/util'
  import Icon from '../Icon.svelte'

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
    <span class="dot {stateTone[m.state]}" aria-hidden="true"></span>{m.host}{#if label}<span class="st">{label}</span>{/if}
  </a>
{/snippet}

<!-- Commits run down a rail; each machine is parked on the commit it runs. -->
<section
  class="rollout card"
  aria-labelledby="rollout-title"
  use:frame={{ title: `Rollout on ${ov.branch}`, tag: p.tip && p.total ? `${p.done} of ${plural(p.total, 'machine')} on ${p.tip.short}` : undefined }}>
  <header class="r-head">
    <h2 id="rollout-title" class="sr-only">Rollout on <span class="mono">{ov.branch}</span></h2>
    {#if p.tip && p.total}
      <span class="sr-only">{p.done} of {plural(p.total, 'machine')} on {p.tip.short}</span>
      <span class="bar" role="progressbar" aria-label="Machines on the last commit" aria-valuemin="0" aria-valuemax={p.total} aria-valuenow={p.done}>{@html bar(p.done / p.total, 60)}</span>
    {/if}
  </header>

  {#if shown.length}
    <ol class="commits">
      {#each shown as row, i (row.commit.sha)}
        <li class:tip={i === 0} class:used={row.machines.length > 0}>
          <span class="node" aria-hidden="true">{row.machines.length || i === 0 ? '◉' : '○'}</span>
          <a class="sha mono" href={link('history', [], { rev: row.commit.sha })} title="Open {row.commit.short} in History">{row.commit.short}</a>
          <span class="subject" title={row.commit.subject}>{row.commit.subject}</span>
          <span class="when muted small">{row.commit.author} · {ago(row.commit.date)}</span>
          <span class="chips">
            {#each row.machines as m (m.host)}{@render chip(m, tag(m))}{/each}
            {#if i === 0 && !row.machines.length && p.total}<span class="muted small">no machine yet</span>{/if}
          </span>
          {#if i < shown.length - 1}
            <!-- A machine catching up climbs the rail toward the commit above it. -->
            <span class="rail" aria-hidden="true">│{#if shown[i + 1].machines.some((m) => m.state === 'behind')}<span class="climb"></span>{/if}</span>
          {/if}
        </li>
      {/each}
    </ol>
    {#if hidden}<p class="more muted small"><span class="faint" aria-hidden="true">┆</span>{plural(hidden, 'older commit')} without machines not shown</p>{/if}
  {:else}
    <p class="muted empty-line">The branch {ov.branch} has no commits in this working copy yet. Push, or pull what Forgejo has.</p>
  {/if}

  {#if r.testing.length}
    <ul class="lane testing" aria-label="Machines on another branch">
      {#each r.testing as m (m.host)}
        <li>
          <span class="fork" aria-hidden="true">├─</span>
          <span class="lane-name mono"><Icon name="branch" /> {m.branch}</span>
          <span class="fork" aria-hidden="true">── ◉</span>
          <a class="sha mono" href={link('history', [], { rev: m.commit })}>{short(m.commit)}</a>
          <span class="subject">{m.subject ?? ''}</span>
          <span class="chips">{@render chip(m, tag(m))}</span>
        </li>
      {/each}
    </ul>
  {/if}

  {#if r.older.length || r.waiting.length}
    <div class="lane-foot">
      {#if r.older.length}
        <div class="lane">
          <span class="lane-name"><span class="node" aria-hidden="true">○</span>Older or unknown</span>
          <span class="chips">{#each r.older as m (m.host)}{@render chip(m, m.commit ? `${stateLabel(m)} · ${short(m.commit)}` : stateLabel(m))}{/each}</span>
        </div>
      {/if}
      {#if r.waiting.length}
        <div class="lane">
          <span class="lane-name"><span class="node" aria-hidden="true">○</span>Not reporting</span>
          <span class="chips">{#each r.waiting as m (m.host)}{@render chip(m, '')}{/each}</span>
        </div>
      {/if}
    </div>
  {/if}
</section>

<style>
  .r-head {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
    margin-bottom: var(--lh);
  }
  .r-head .bar {
    overflow: hidden;
  }
  /* One row per commit, the rail in the first column, a rail row between commits. */
  .commits li,
  .lane.testing li {
    display: grid;
    grid-template-columns: 2ch 7ch minmax(12ch, 40ch) minmax(0, 26ch) minmax(0, 1fr);
    gap: 0 2ch;
    white-space: nowrap;
  }
  .lane.testing li {
    grid-template-columns: 2ch max-content 4ch 7ch minmax(12ch, 1fr) minmax(0, max-content);
    gap: 0 1ch;
  }
  .node,
  .rail,
  .fork {
    color: var(--line-strong);
  }
  .tip .node {
    color: var(--accent);
  }
  .used:not(.tip) .node {
    color: var(--yellow);
  }
  .rail {
    grid-column: 1;
  }
  /* ⇡ steps up the rail on the clock (the phase lib/ascii.ts writes on <html>); still: the first frame. */
  .climb {
    margin-left: 1ch;
    color: var(--yellow);
  }
  .climb::before {
    content: '⇡';
  }
  :global(:is([data-t8='1'], [data-t8='5'])) .climb::before {
    content: '↑';
  }
  :global(:is([data-t8='3'], [data-t8='7'])) .climb::before {
    content: ' ';
  }
  .sha {
    font-weight: 700;
  }
  .subject,
  .when {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .subject {
    color: var(--fg-strong);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .mchip {
    color: var(--c);
  }
  .mchip:hover,
  .mchip:hover .dot {
    color: var(--on-accent);
  }
  .mchip .dot {
    margin-right: 1ch;
  }
  .mchip .st {
    margin-left: 1ch;
  }
  .t-ok {
    --c: var(--green);
  }
  .t-warn {
    --c: var(--yellow);
  }
  .t-err {
    --c: var(--red);
  }
  .t-off {
    --c: var(--fg-muted);
  }
  .more .faint {
    display: inline-block;
    width: 4ch;
  }
  /* A weak ┆ stretch of the rail leads to the lanes off it. */
  .lane.testing::before,
  .lane-foot::before {
    display: block;
    color: var(--line-strong);
    content: '┆';
  }
  .lane.testing .lane-name {
    color: var(--accent);
  }
  .lane-foot .lane {
    display: grid;
    grid-template-columns: 20ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  .lane-foot .lane-name {
    color: var(--fg-muted);
  }
  .lane-foot .node {
    display: inline-block;
    width: 4ch;
  }
</style>
