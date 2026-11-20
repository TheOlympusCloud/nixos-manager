<script lang="ts">
  import { excerpt, short, silenceDetail, stateDetail } from '../../lib/deployments'
  import { deployOf, deploys, watchDeployments } from '../../lib/deployments.svelte'
  import { link } from '../../lib/router.svelte'
  import { ago } from '../../lib/util'
  import DeployBadge from './DeployBadge.svelte'

  /** What comin last did on one machine, for its Overview tab. */
  let { host }: { host: string } = $props()

  $effect(() => watchDeployments())
  const m = $derived(deployOf(host))
  const history = $derived((m?.reports ?? []).slice(0, 5))
</script>

<section class="section dep-card" aria-labelledby="dep-title">
  <div class="head">
    <h2 id="dep-title">Deployment</h2>
    {#if m}<DeployBadge {m} />{/if}
    <span class="spacer"></span>
    <a class="link-small" href={link('deployments')}>All deployments</a>
  </div>
  {#if !deploys.overview}
    <span class="skel" style:width="60%"></span>
  {:else if !m || (m.state === 'never' && !m.lastSeen)}
    <p class="muted">{host} has not reported a deployment yet, so what it runs is unknown here. <a href={link('deployments')}>Turn on reporting</a> to see the commit comin deployed and why a deployment failed.</p>
  {:else}
    <dl class="kv">
      <dt>Runs</dt>
      <dd>
        {#if m.commit}
          <a class="mono" href={link('history', [], { rev: m.commit })}>{short(m.commit)}</a>
          <span class="subject" title={m.subject}>{m.subject || 'a commit this working copy does not have'}</span>
        {:else}<span class="muted">nothing deployed successfully yet</span>{/if}
      </dd>
      {#if m.deployedAt}<dt>Deployed</dt><dd title={new Date(m.deployedAt).toLocaleString()}>{ago(m.deployedAt)}</dd>{/if}
      <dt>Follows</dt>
      <dd><span class="mono">{m.branch}</span>{#if m.behind && m.state !== 'failed'}<span class="muted"> · {m.behind} behind</span>{/if}</dd>
    </dl>
    {#if !m.failure}<p class="muted small state">{stateDetail(m)}</p>{/if}
    {#if m.silent}<p class="small state warn-line" role="status">{silenceDetail(m)}</p>{/if}
    {#if m.failure}
      <div class="fail" role="alert">
        <div class="fail-head">
          {m.failure.phase === 'build' ? 'Build' : 'Switch'} of <span class="mono">{short(m.failure.commit)}</span> failed {ago(m.failure.at)}
        </div>
        {#if m.failure.error}<pre>{excerpt(m.failure.error, 8, 900)}</pre>{/if}
        <p class="muted small">The whole log is on the machine: <span class="mono">journalctl -u comin</span></p>
      </div>
    {/if}
    {#if history.length > 1}
      <ol class="hist" aria-label="Recent reports">
        {#each history as r (r.at + r.commit + r.phase)}
          <li>
            <span class="dot {r.status === 'failed' ? 'err' : 'ok'}" aria-hidden="true"></span>
            <span class="mono">{short(r.commit)}</span>
            <span class="muted">{r.phase === 'build' ? `build ${r.status}` : r.status === 'done' ? 'deployed' : 'deploy failed'}</span>
            <span class="spacer"></span>
            <span class="muted small">{ago(r.at)}</span>
          </li>
        {/each}
      </ol>
    {/if}
  {/if}
</section>

<style>
  .head {
    display: flex;
    align-items: center;
    gap: var(--s2);
    margin-bottom: var(--s3);
  }
  .head h2 {
    font-size: var(--fs-lg);
  }
  .link-small {
    font-size: var(--fs-sm);
  }
  .kv {
    display: grid;
    grid-template-columns: 96px minmax(0, 1fr);
    gap: 8px 12px;
    margin: 0;
  }
  .kv dt {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .kv dd {
    display: flex;
    gap: 8px;
    min-width: 0;
    margin: 0;
  }
  .kv dd a {
    flex: none;
  }
  .subject {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    margin: var(--s2) 0 0;
  }
  .warn-line {
    color: var(--yellow);
  }
  .fail {
    margin-top: var(--s3);
    padding: var(--s3);
    background: color-mix(in srgb, var(--red) 6%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--red) 30%, var(--border));
    border-radius: var(--r);
  }
  .fail-head {
    color: var(--red);
    font-weight: 500;
  }
  .fail pre {
    max-height: 180px;
    margin: var(--s2) 0;
    overflow: auto;
    color: var(--fg);
    font: 12px/1.5 var(--mono);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .fail p {
    margin: 0;
  }
  .hist {
    margin: var(--s3) 0 0;
    padding: 0;
    list-style: none;
    border-top: 1px solid var(--border);
  }
  .hist li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 0;
    font-size: var(--fs-sm);
    border-bottom: 1px solid var(--border);
  }
  .hist .dot {
    width: 6px;
    height: 6px;
  }
</style>
