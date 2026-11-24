<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import { bigText, frame, odometer } from '../lib/ascii'
  import DeployBadge from '../components/deployments/DeployBadge.svelte'
  import DeploySetup from '../components/deployments/DeploySetup.svelte'
  import FailureActions from '../components/deployments/FailureActions.svelte'
  import Rollout from '../components/deployments/Rollout.svelte'
  import { api, type MachineDeployment } from '../lib/api'
  import { excerpt, forgetQuestion, rollingOut, short, sortRank, tileOf } from '../lib/deployments'
  import { deploys, loadDeployments, watchDeployments } from '../lib/deployments.svelte'
  import { link, machineLink } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { confirmBox, report } from '../lib/ui.svelte'
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
  const count = (tile: ReturnType<typeof tileOf>) => ov?.machines.filter((m) => tileOf(m) === tile).length ?? 0
  const tiles = $derived([
    { label: 'Up to date', value: count('current'), tone: 'ok', note: `on the last commit of their branch` },
    { label: 'Behind', value: count('behind'), tone: 'warn', note: live ? 'catching up now' : 'an older commit' },
    { label: 'Failed', value: count('failed'), tone: 'err', note: 'the last deployment failed' },
    { label: 'Silent or unknown', value: count('silent'), tone: 'off', note: 'no recent report, or none' },
  ])
  const rows = $derived([...(ov?.machines ?? [])].sort((a, b) => sortRank(a) - sortRank(b) || a.host.localeCompare(b.host)))
  let refreshing = $state(false)
  async function refresh() {
    refreshing = true
    await loadDeployments()
    refreshing = false
  }
  async function forget(m: MachineDeployment) {
    if (!(await confirmBox({ title: 'Forget machine', message: forgetQuestion(m), ok: 'Forget', danger: true }))) return
    try {
      await api.forgetMachine(m.host)
    } catch (e) {
      report(e, `Cannot forget ${m.host}`)
    }
    await loadDeployments()
  }
</script>

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <div>
        <h1 class="big" aria-label="Deployments" use:bigText={'Deployments'}></h1>
        <p class="sub">What comin deployed on each machine, as the machines report it</p>
      </div>
      <span class="spacer"></span>
      <div class="tools">
        {#if live}<span class="badge busy" title="A machine is catching up with a recent commit: checking every few seconds">Rolling out</span>{/if}
        <button onclick={refresh} disabled={refreshing} title="Ask for the reports again"><Icon name="refresh" /> Refresh</button>
      </div>
    </div>

    {#if deploys.error && !ov}
      <div class="empty-state" role="alert">
        <h2>Cannot load the deployments</h2>
        <pre>{deploys.error}</pre>
        <div class="row"><button class="primary" onclick={refresh}><Icon name="refresh" /> Try again</button></div>
      </div>
    {:else if !ov}
      <div class="tiles" aria-hidden="true">{#each [0, 1, 2, 3] as k (k)}<div class="tile" use:frame><span class="skel" style:width="12ch"></span><span class="skel" style:width="8ch"></span><span class="skel" style:width="18ch"></span></div>{/each}</div>
    {:else if !ov.reporting}
      <section class="intro card" use:frame={{ title: 'No machine has reported a deployment yet' }}>
        <div class="intro-text">
          <span class="intro-mark"><Icon name="deploy" /></span>
          <div>
            <h2 class="sr-only">No machine has reported a deployment yet</h2>
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
          <div class="tile" class:zero={!t.value} use:frame={{ title: t.label, animate: true }}>
            <span class="sr-only">{t.label}</span>
            <pre class="big t-value {t.tone}" use:odometer={t.value}></pre>
            <span class="t-note">{t.note}</span>
          </div>
        {/each}
      </div>

      <Rollout {ov} />

      <section class="section" aria-labelledby="machines-title">
        <h2 id="machines-title">Machines</h2>
        <div class="table-wrap">
          <table class="data">
            <thead><tr><th>Machine</th><th>State</th><th>Runs</th><th>Deployed</th><th>Branch</th><th>Last report</th><th><span class="sr-only">Actions</span></th></tr></thead>
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
                  <td class="mono branch"><Icon name="branch" /> {m.branch}</td>
                  <td class="when muted">{m.lastSeen ? ago(m.lastSeen) : 'never'}</td>
                  <td class="act">
                    {#if m.reports.length || m.lastSeen}
                      <button class="small forget" onclick={() => forget(m)} aria-label="Forget {m.host}" title="Delete its deployment reports, e.g. once it left the fleet">Forget</button>
                    {/if}
                  </td>
                </tr>
                {#if m.failure}
                  <tr class="err-row">
                    <td></td>
                    <td colspan="6">
                      <div class="err-box" data-tone="err" use:frame={{ title: `${m.failure.phase === 'build' ? 'Build' : 'Switch'} of ${short(m.failure.commit)} failed ${ago(m.failure.at)}` }}>
                        <span class="err-title sr-only">{m.failure.phase === 'build' ? 'Build' : 'Switch'} of <span class="mono">{short(m.failure.commit)}</span> failed {ago(m.failure.at)}</span>
                        <pre>{excerpt(m.failure.error) || 'comin gave no error message'}</pre>
                        <FailureActions {m} />
                      </div>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <details class="section setup">
        <summary><Icon name="key" /> Setup: the address, the report token and how to install it</summary>
        <div class="setup-body"><DeploySetup {ov} /></div>
      </details>
    {/if}
  </div>
</div>

<style>
  .page-head .tools {
    align-items: baseline;
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(22ch, 1fr));
    gap: var(--lh) 2ch;
    margin-bottom: var(--lh);
  }
  .tile {
    display: grid;
    align-content: start;
  }
  .t-note {
    color: var(--fg-muted);
  }
  .tile.zero .t-value {
    color: var(--fg-faint);
  }
  .intro {
    display: grid;
    gap: var(--lh) 0;
    max-width: 100ch;
  }
  .intro-text {
    display: grid;
    grid-template-columns: 2ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  .intro-mark {
    color: var(--accent);
  }
  .intro p {
    max-width: 80ch;
  }
  .table-wrap {
    overflow: auto;
  }
  .host a {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .host a:hover {
    color: var(--on-accent);
  }
  .runs {
    white-space: nowrap;
  }
  .runs .subject {
    display: inline-block;
    max-width: 28ch;
    margin-left: 1ch;
    overflow: hidden;
    color: var(--fg-muted);
    vertical-align: top;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .when,
  .branch {
    white-space: nowrap;
  }
  .branch :global(.ic) {
    color: var(--fg-muted);
  }
  td.act {
    text-align: right;
  }
  .forget {
    color: var(--fg-muted);
  }
  table.data tbody tr.err-row:hover {
    background: none;
  }
  .err-box {
    margin-bottom: var(--lh);
  }
  .err-box pre {
    color: var(--fg);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .setup summary {
    color: var(--fg-muted);
  }
  .setup summary :global(.ic) {
    color: var(--fg-muted);
  }
  .setup summary:hover {
    color: var(--fg-strong);
  }
  .setup-body {
    margin-top: var(--lh);
  }
</style>
