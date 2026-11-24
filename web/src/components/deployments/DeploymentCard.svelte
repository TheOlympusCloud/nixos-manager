<script lang="ts">
  import { frame } from '../../lib/ascii'
  import { driftDetail, excerpt, short, silenceDetail, stateDetail } from '../../lib/deployments'
  import { deployOf, deploys, watchDeployments } from '../../lib/deployments.svelte'
  import { link } from '../../lib/router.svelte'
  import { ago } from '../../lib/util'
  import DeployBadge from './DeployBadge.svelte'
  import FailureActions from './FailureActions.svelte'
  import Icon from '../Icon.svelte'

  /** What comin last did on one machine, for its Overview tab. */
  let { host }: { host: string } = $props()

  $effect(() => watchDeployments())
  const m = $derived(deployOf(host))
  const history = $derived((m?.reports ?? []).slice(0, 5))
</script>

<section class="section card dep-card" aria-labelledby="dep-title" use:frame={{ title: 'Deployment' }}>
  <div class="head">
    <h2 id="dep-title" class="sr-only">Deployment</h2>
    {#if m}<DeployBadge {m} />{/if}
    <span class="spacer"></span>
    <!-- Sits on the frame's top line, where a frame tag would be. -->
    <a class="link-small" href={link('deployments')}>All deployments <span aria-hidden="true">›</span></a>
  </div>
  {#if !deploys.overview}
    <span class="skel" style:width="30ch"></span>
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
      <dd><span class="mono"><Icon name="branch" /> {m.branch}</span>{#if m.behind && m.state !== 'failed'}<span class="muted"> · {m.behind} behind</span>{/if}</dd>
    </dl>
    {#if !m.failure}<p class="muted small state">{stateDetail(m)}</p>{/if}
    {#if m.silent}<p class="small state warn-line" role="status">{silenceDetail(m)}</p>{/if}
    {#if m.drift}<p class="small state warn-line" role="status">{driftDetail(m)}</p>{/if}
    {#if m.system?.reboot}<p class="muted small state">A reboot is pending: a deployment changed the kernel, its modules or the initrd, which only a reboot loads.</p>{/if}
    {#if m.failure}
      <div
        class="fail"
        role="alert"
        data-tone="err"
        use:frame={{ kind: 'square', title: `${m.failure.phase === 'build' ? 'Build' : 'Switch'} of ${short(m.failure.commit)} failed ${ago(m.failure.at)}` }}>
        <div class="fail-head sr-only">
          {m.failure.phase === 'build' ? 'Build' : 'Switch'} of <span class="mono">{short(m.failure.commit)}</span> failed {ago(m.failure.at)}
        </div>
        {#if m.failure.error}<pre>{excerpt(m.failure.error, 8, 900)}</pre>{/if}
        <p class="muted small">The whole log is on the machine: <span class="mono">journalctl -u comin</span></p>
        <FailureActions {m} />
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
    align-items: baseline;
    gap: 0 2ch;
  }
  /* The link stands on the frame's top line, right-aligned like a frame tag: ─ All deployments › ─╮ */
  .link-small {
    position: absolute;
    top: 0;
    right: 2ch;
    z-index: 2;
    padding: 0 1ch;
    color: var(--fg-muted);
    background: var(--bg);
  }
  .link-small:hover {
    color: var(--on-accent);
    background: var(--accent);
  }
  .kv {
    grid-template-columns: 10ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  .kv dd {
    display: flex;
    gap: 0 1ch;
    min-width: 0;
  }
  .kv dd :global(.ic) {
    color: var(--fg-muted);
  }
  .kv dd a {
    flex: none;
    font-weight: 700;
  }
  .subject {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .state {
    margin-top: var(--lh);
  }
  .warn-line {
    color: var(--yellow);
  }
  .warn-line::before {
    content: '▲ ';
  }
  .fail {
    margin-top: var(--lh);
  }
  .fail pre {
    max-height: calc(var(--lh) * 10);
    margin-bottom: var(--lh);
    overflow: auto;
    color: var(--fg);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .hist {
    margin-top: var(--lh);
  }
  .hist li {
    display: flex;
    gap: 0 1ch;
  }
</style>
