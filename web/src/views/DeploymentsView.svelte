<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import DeployBadge from '../components/deployments/DeployBadge.svelte'
  import DeploySetup from '../components/deployments/DeploySetup.svelte'
  import Rollout from '../components/deployments/Rollout.svelte'
  import type { DeployState } from '../lib/api'
  import { excerpt, rollingOut, short } from '../lib/deployments'
  import { deploys, loadDeployments, watchDeployments } from '../lib/deployments.svelte'
  import { link, machineLink } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { ago } from '../lib/util'

  $effect(() => watchDeployments())
  // A push (or a pull) moves the branch: look again now rather than at the next poll.
  let lastAhead: number | undefined
  $effect(() => {
    const ahead = app.status?.ahead
    if (lastAhead !== undefined && ahead !== lastAhead) loadDeployments()
    lastAhead = ahead
  })

  const ov = $derived(deploys.overview)
  const live = $derived(rollingOut(ov))
  const count = (...s: DeployState[]) => ov?.machines.filter((m) => s.includes(m.state)).length ?? 0
  const tiles = $derived([
    { label: 'Up to date', value: count('current'), tone: 'ok', note: `on the last commit of their branch` },
    { label: 'Behind', value: count('behind'), tone: 'warn', note: live ? 'catching up now' : 'an older commit' },
    { label: 'Failed', value: count('failed'), tone: 'err', note: 'the last deployment failed' },
    { label: 'Silent or unknown', value: count('silent', 'unknown', 'never'), tone: 'off', note: 'no recent report, or none' },
  ])
  // Failures first, then the machines that need a look, then the rest.
  const order: Record<DeployState, number> = { failed: 0, behind: 1, unknown: 2, silent: 3, never: 4, current: 5 }
  const rows = $derived([...(ov?.machines ?? [])].sort((a, b) => order[a.state] - order[b.state] || a.host.localeCompare(b.host)))
  let refreshing = $state(false)
  async function refresh() {
    refreshing = true
    await loadDeployments()
    refreshing = false
  }
</script>

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <h1>Deployments</h1>
      <span class="sub">What comin deployed on each machine, as the machines report it</span>
      <span class="spacer"></span>
      {#if live}<span class="badge busy" title="A machine is catching up with a recent commit: checking every few seconds"><span class="dot busy"></span>Rolling out</span>{/if}
      <button class="ghost" onclick={refresh} disabled={refreshing} title="Ask for the reports again"><Icon name="refresh" size={14} />Refresh</button>
    </div>

    {#if deploys.error && !ov}
      <div class="empty-state" role="alert">
        <h2>Cannot load the deployments</h2>
        <pre>{deploys.error}</pre>
        <div class="row"><button class="primary" onclick={refresh}><Icon name="refresh" size={14} />Try again</button></div>
      </div>
    {:else if !ov}
      <div class="tiles" aria-hidden="true">{#each [0, 1, 2, 3] as k (k)}<div class="tile"><span class="skel" style:width="50%"></span><span class="skel" style:width="30%"></span></div>{/each}</div>
    {:else if !ov.reporting}
      <section class="intro card">
        <div class="intro-text">
          <span class="intro-mark"><Icon name="deploy" size={20} /></span>
          <div>
            <h2>No machine has reported a deployment yet</h2>
            <p class="muted">
              Every machine runs comin, which pulls <span class="mono">{ov.branch}</span> from Forgejo and switches to it, so a push is a deploy. With reporting on, each machine tells this
              console which commit it switched to, and why a switch or build failed. Then you can watch a push roll out here, and Health warns about machines that fail or fall behind.
            </p>
          </div>
        </div>
        <DeploySetup {ov} />
      </section>
    {:else}
      <div class="tiles">
        {#each tiles as t (t.label)}
          <div class="tile t-{t.tone}" class:zero={!t.value}>
            <span class="t-label"><span class="dot" aria-hidden="true"></span>{t.label}</span>
            <span class="t-value num">{t.value}</span>
            <span class="t-note">{t.note}</span>
          </div>
        {/each}
      </div>

      <Rollout {ov} />

      <section class="section" aria-labelledby="machines-title">
        <h2 id="machines-title">Machines</h2>
        <div class="table-wrap card">
          <table class="data">
            <thead><tr><th>Machine</th><th>State</th><th>Runs</th><th>Deployed</th><th>Branch</th><th>Last report</th></tr></thead>
            <tbody>
              {#each rows as m (m.host)}
                <tr class:failed={!!m.failure}>
                  <td class="host"><a href={machineLink(m.host)}>{m.host}</a></td>
                  <td><DeployBadge {m} /></td>
                  <td class="runs">
                    {#if m.commit}
                      <a class="mono" href={link('history', [], { rev: m.commit })}>{short(m.commit)}</a>
                      <span class="subject" title={m.subject}>{m.subject || 'not in this working copy'}</span>
                    {:else}<span class="muted">—</span>{/if}
                  </td>
                  <td class="when">{#if m.deployedAt}<span title={new Date(m.deployedAt).toLocaleString()}>{ago(m.deployedAt)}</span>{:else}<span class="muted">—</span>{/if}</td>
                  <td class="mono branch">{m.branch}</td>
                  <td class="when muted">{m.lastSeen ? ago(m.lastSeen) : 'never'}</td>
                </tr>
                {#if m.failure}
                  <tr class="err-row">
                    <td></td>
                    <td colspan="5">
                      <div class="err-box">
                        <span class="err-title">{m.failure.phase === 'build' ? 'Build' : 'Switch'} of <span class="mono">{short(m.failure.commit)}</span> failed {ago(m.failure.at)}</span>
                        <pre>{excerpt(m.failure.error) || 'comin gave no error message'}</pre>
                      </div>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <details class="section setup card">
        <summary><Icon name="key" size={14} />Setup: the address, the report token and how to install it</summary>
        <div class="setup-body"><DeploySetup {ov} /></div>
      </details>
    {/if}
  </div>
</div>

<style>
  .page-inner {
    max-width: 1180px;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--s3);
    margin-bottom: var(--s4);
  }
  .tile {
    display: grid;
    gap: 2px;
    padding: 12px 14px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    box-shadow: var(--edge);
  }
  .t-label {
    display: flex;
    align-items: center;
    gap: 7px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .t-value {
    color: var(--fg-strong);
    font-size: var(--fs-2xl);
    font-weight: 600;
    letter-spacing: -0.02em;
  }
  .t-note {
    color: var(--fg-muted);
    font-size: var(--fs-xs);
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
    --c: var(--fg-faint);
  }
  .tile.t-err:not(.zero) {
    background: color-mix(in srgb, var(--red) 6%, var(--bg-raised));
    border-color: color-mix(in srgb, var(--red) 30%, var(--border));
  }
  .tile.t-warn:not(.zero) {
    border-color: color-mix(in srgb, var(--yellow) 25%, var(--border));
  }
  .tile.zero .t-value {
    color: var(--fg-faint);
  }
  .intro {
    display: grid;
    gap: var(--s5);
    max-width: 860px;
    padding: var(--s5);
  }
  .intro-text {
    display: grid;
    grid-template-columns: 40px minmax(0, 1fr);
    gap: var(--s3);
  }
  .intro-mark {
    display: grid;
    place-items: center;
    width: 38px;
    height: 38px;
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    border-radius: 10px;
  }
  .intro h2 {
    margin-bottom: var(--s2);
    font-size: var(--fs-xl);
  }
  .intro p {
    max-width: 70ch;
    margin: 0;
    line-height: 1.6;
  }
  .table-wrap {
    overflow: auto;
  }
  table.data th:first-child,
  table.data td:first-child {
    padding-left: 16px;
  }
  .host a {
    color: var(--fg-strong);
    font-weight: 500;
    text-decoration: none;
  }
  .host a:hover {
    color: var(--accent);
  }
  .runs {
    max-width: 380px;
  }
  .runs a {
    margin-right: 8px;
    color: var(--accent);
    text-decoration: none;
  }
  .runs .subject {
    display: inline-block;
    max-width: 290px;
    overflow: hidden;
    vertical-align: bottom;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .when,
  .branch {
    white-space: nowrap;
  }
  tr.failed td {
    border-bottom: 0;
  }
  .err-row td {
    padding-top: 0;
    padding-bottom: 10px;
  }
  table.data tbody tr.err-row:hover td {
    background: none;
  }
  .err-box {
    padding: 8px 12px;
    background: color-mix(in srgb, var(--red) 6%, var(--bg-deep));
    border: 1px solid color-mix(in srgb, var(--red) 25%, var(--border));
    border-radius: var(--r);
  }
  .err-title {
    color: var(--red);
    font-size: var(--fs-sm);
    font-weight: 500;
  }
  .err-box pre {
    margin: 4px 0 0;
    color: var(--fg);
    font: 12px/1.5 var(--mono);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .setup {
    padding: 0;
  }
  .setup summary {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: var(--s3) var(--s4);
    color: var(--fg-strong);
    font-weight: 500;
    cursor: pointer;
  }
  .setup summary :global(svg) {
    color: var(--fg-muted);
  }
  .setup-body {
    padding: var(--s2) var(--s4) var(--s4);
    border-top: 1px solid var(--border);
  }
  .setup[open] summary {
    border-bottom: 0;
  }
</style>
