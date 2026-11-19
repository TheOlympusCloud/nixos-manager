<script lang="ts">
  import Icon, { type IconName } from '../components/Icon.svelte'
  import type { Insight } from '../lib/api'
  import { openPackageSearch } from '../lib/dialogs.svelte'
  import { fleet, hostByName } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { applyFix, health, loadInsights, updateInputFix } from '../lib/health.svelte'
  import { bySeverity, fixPlan, insightDetail, insightTitle, missingPackage, nixLines, severityLabel } from '../lib/insights'
  import { link, machineLink } from '../lib/router.svelte'
  import { app, showJob } from '../lib/state.svelte'
  import { copyText } from '../lib/util'

  const groups = $derived(bySeverity(health.insights ?? []))
  const sevIcon: Record<Insight['severity'], IconName> = { error: 'alert', warning: 'shield', info: 'info' }
  const affected = $derived(new Set((health.insights ?? []).flatMap((i) => i.hosts)).size)
  const jobState = (id: string) => app.jobs.find((j) => j.id === id)?.state ?? 'queued'
  let copied = $state('')
  async function copy(i: Insight) {
    if (await copyText(nixLines(i))) {
      copied = i.id
      setTimeout(() => copied === i.id && (copied = ''), 2000)
    }
  }
</script>

{#snippet fixArea(i: Insight)}
  {@const st = health.fixing[i.id]}
  {#if i.fix?.settings?.length}
    {@const plan = fixPlan(i, fleet.model?.hosts ?? [])}
    <div class="fix">
      {#if st && !st.finished}
        <div class="progress" role="progressbar" aria-valuemin="0" aria-valuemax={st.total} aria-valuenow={st.done} aria-label="Applying {i.fix.label}">
          <span style:width="{(st.done / st.total) * 100}%"></span>
        </div>
        <p class="fix-note">Writing {st.done + 1} of {st.total} · {st.host}</p>
      {:else if st?.error}
        <p class="fix-note bad" role="alert">{st.error}</p>
        <button onclick={() => (delete health.fixing[i.id], applyFix(i))}>Try again</button>
      {:else if st?.checked}
        <p class="fix-note warn-note">Written on {plan.editable.join(', ')}, but the new evaluation still reports it: a definition in code may force the old value (mkForce).</p>
        <a class="link-small" href="#/changes">Review in Changes</a>
      {:else if st}
        <p class="fix-note ok"><Icon name="tick" size={13} />Applied on {plan.editable.join(', ') || 'the machines'}</p>
        <a class="link-small" href="#/changes">Review and commit in Changes</a>
      {:else}
        {#if plan.steps.length}
          <button class:primary={i.severity !== 'info'} onclick={() => applyFix(i)}>
            <Icon name="tick" size={14} />{i.fix.label}{plan.codeOnly.length || plan.noKey.length ? ` on ${plan.editable.join(', ')}` : ''}
          </button>
        {:else if plan.codeOnly.length}
          <p class="fix-note unavailable"><Icon name="code" size={13} />{i.fix.label}: change it in code, {plan.codeOnly.join(', ')} {plan.codeOnly.length === 1 ? 'is' : 'are'} Code only</p>
        {/if}
        {#each plan.noKey as h (h)}
          <p class="fix-note unavailable"><Icon name="key" size={13} />Not on {h}: no user there has an SSH key, so this would lock everyone out. <a href={machineLink(h, 'users')}>Add a key in Users</a> first.</p>
        {/each}
      {/if}
    </div>
  {:else if i.fix?.updateInput}
    {@const name = i.fix.updateInput}
    <div class="fix">
      {#if st?.job && !st.finished}
        <button disabled><span class="spinner" aria-hidden="true"></span>Updating {name}…</button>
        <button class="link-small" onclick={() => showJob(st.job, 'update')}>Show the log</button>
      {:else if st?.error}
        <p class="fix-note bad" role="alert">{st.error}</p>
        <button class="link-small" onclick={() => showJob(st.job, 'update')}>Show the log</button>
      {:else if st?.job}
        <p class="fix-note ok"><Icon name="tick" size={13} />{name} updated ({jobState(st.job)})</p>
        <a class="link-small" href={link('inputs')}>See what changed</a>
      {:else}
        <button class:primary={i.severity !== 'info'} onclick={() => updateInputFix(i)}><Icon name="refresh" size={14} />{i.fix.label}</button>
      {/if}
    </div>
  {:else if missingPackage(i)}
    {@const pkg = missingPackage(i)}
    {@const h = hostByName(i.hosts[0])}
    <div class="fix">
      {#if h?.managedFile}
        <button onclick={() => openPackageSearch(pkg, [h.name])}><Icon name="plus" size={14} />Add {pkg} to {h.name}…</button>
      {:else}
        <p class="fix-note">{i.hosts[0]} is Code only: add {pkg} in its configuration.</p>
      {/if}
    </div>
  {:else if i.id === 'eval-error' && i.hosts.length === 1}
    <div class="fix"><a class="btn" href={machineLink(i.hosts[0])}>Open {i.hosts[0]}</a></div>
  {/if}
{/snippet}

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <h1>Health</h1>
      <span class="sub">
        {#if health.insights}
          {health.insights.length ? `${plural(health.insights.length, 'finding')}${affected ? ` on ${plural(affected, 'machine')}` : ''}, from the last evaluation` : 'From the last evaluation'}
        {:else}
          Security and upkeep checks of every machine
        {/if}
      </span>
      <span class="spacer"></span>
      {#if health.computing || fleet.computing}<span class="badge busy"><span class="dot busy"></span>Evaluating…</span>{/if}
      <button class="ghost" onclick={loadInsights} disabled={health.computing} title="Ask for the findings again"><Icon name="refresh" size={14} />Check again</button>
    </div>

    {#if health.error && !health.insights}
      <div class="empty-state" role="alert">
        <h2>Cannot check the fleet</h2>
        <pre>{health.error}</pre>
        <div class="row"><button class="primary" onclick={loadInsights}><Icon name="refresh" size={14} />Try again</button></div>
      </div>
    {:else if !health.insights}
      <div class="list" aria-hidden="true">
        {#each [0, 1, 2] as k (k)}<div class="finding skel-row"><span class="skel" style:width="34%"></span><span class="skel" style:width="62%"></span></div>{/each}
      </div>
    {:else if !health.insights.length}
      <div class="all-clear">
        <span class="ok-mark"><Icon name="tick" size={20} /></span>
        <div>
          <h2>Nothing to fix</h2>
          <p class="muted">Every machine evaluates, keeps its firewall on, accepts SSH keys only, collects garbage, and the flake inputs are recent.</p>
        </div>
      </div>
    {:else}
      {#each groups as g (g.severity)}
        <section class="group" aria-labelledby="sev-{g.severity}">
          <h2 id="sev-{g.severity}" class="g-title sev-{g.severity}"><span class="dot"></span>{severityLabel[g.severity]}<span class="n num">{g.items.length}</span></h2>
          <ul class="list">
            {#each g.items as i (i.id)}
              {@const plan = fixPlan(i, fleet.model?.hosts ?? [])}
              <li class="finding sev-{i.severity}">
                <span class="sev-ico" aria-hidden="true"><Icon name={sevIcon[i.severity]} size={16} /></span>
                <div class="main">
                  <h3>{insightTitle(i)}</h3>
                  <p class="detail">{insightDetail(i)}</p>
                  <div class="chips">
                    {#each i.hosts as h (h)}
                      {@const host = hostByName(h)}
                      <a class="chip host-chip" class:code-only={!!host && !host.error && !host.managedFile} href={machineLink(h)} title={host?.error ? `${h} does not evaluate` : host?.managedFile ? `${h} · Editable here` : `${h} · Code only`}>
                        <span class="dot {host?.error ? 'err' : 'ok'}"></span>{h}{#if host && !host.error && !host.managedFile}<span class="tag">Code only</span>{/if}
                      </a>
                    {/each}
                    {#if !i.hosts.length && i.fix?.updateInput}
                      <a class="chip mono" href={link('inputs')} title="A flake input every machine is built from">{i.fix.updateInput}</a>
                      <span class="muted small">every machine</span>
                    {/if}
                  </div>
                  {#if i.fix?.settings?.length && plan.codeOnly.length}
                    <details class="by-hand">
                      <summary>{plan.codeOnly.join(', ')} {plan.codeOnly.length === 1 ? 'is' : 'are'} Code only: how to fix {plan.codeOnly.length === 1 ? 'it' : 'them'} in code</summary>
                      <p class="muted small">Turn on editing on the machine page so fixes apply here, or add this to {plan.codeOnly.length === 1 ? `hosts/${plan.codeOnly[0]}/default.nix` : 'each machine’s configuration'}:</p>
                      <div class="code-row">
                        <pre>{nixLines(i)}</pre>
                        <button class="icon" aria-label="Copy the Nix lines" title={copied === i.id ? 'Copied' : 'Copy'} onclick={() => copy(i)}><Icon name={copied === i.id ? 'tick' : 'copy'} size={14} /></button>
                      </div>
                    </details>
                  {/if}
                </div>
                {@render fixArea(i)}
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    {/if}
  </div>
</div>

<style>
  .page-inner {
    max-width: 1080px;
  }
  .group {
    margin-top: var(--s5);
  }
  .g-title {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: var(--s2);
    color: var(--fg);
    font-size: var(--fs-md);
    font-weight: 600;
  }
  .g-title .n {
    color: var(--fg-muted);
    font-weight: 500;
  }
  .sev-error {
    --c: var(--red);
  }
  .sev-warning {
    --c: var(--yellow);
  }
  .sev-info {
    --c: var(--accent);
  }
  .list {
    display: grid;
    margin: 0;
    padding: 0;
    list-style: none;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .finding {
    display: grid;
    grid-template-columns: 28px 1fr minmax(200px, auto);
    gap: 4px 14px;
    align-items: start;
    padding: 14px 16px;
  }
  .finding + .finding {
    border-top: 1px solid var(--border);
  }
  .sev-ico {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 13%, transparent);
    border-radius: 8px;
  }
  .main {
    display: grid;
    gap: 6px;
    min-width: 0;
  }
  h3 {
    padding-top: 4px;
    font-size: var(--fs-md);
  }
  .detail {
    max-width: 72ch;
    color: var(--fg-muted);
    line-height: 1.5;
  }
  .chips {
    align-items: center;
    margin-top: 2px;
  }
  a.chip {
    text-decoration: none;
  }
  .host-chip {
    --c: var(--fg-muted);
    color: var(--fg);
    text-decoration: none;
  }
  .host-chip:hover {
    color: var(--fg-strong);
    border-color: var(--line-strong);
  }
  .host-chip .dot {
    width: 6px;
    height: 6px;
  }
  .tag {
    margin-left: 2px;
    color: var(--fg-faint);
    font-size: var(--fs-xs);
  }
  .fix {
    display: grid;
    justify-items: end;
    gap: 6px;
    min-width: 200px;
    max-width: 280px;
    text-align: right;
  }
  .fix-note {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.45;
  }
  .fix-note.unavailable {
    align-items: flex-start;
    max-width: 240px;
    text-align: left;
  }
  .fix-note.unavailable :global(svg) {
    flex: none;
    margin-top: 3px;
  }
  .fix-note.warn-note {
    max-width: 260px;
    color: var(--yellow);
  }
  .fix-note.ok {
    color: var(--green);
  }
  .fix-note.bad {
    color: var(--red);
  }
  .link-small {
    height: auto;
    padding: 0;
    color: var(--accent);
    font-size: var(--fs-sm);
    font-weight: 400;
    background: none;
    border: 0;
  }
  button.link-small:hover:not(:disabled) {
    text-decoration: underline;
    background: none;
  }
  .progress {
    width: 200px;
    height: 4px;
    overflow: hidden;
    background: var(--bg-active);
    border-radius: 2px;
  }
  .progress span {
    display: block;
    height: 100%;
    background: var(--accent);
    transition: width var(--t) var(--ease);
  }
  .by-hand {
    margin-top: 2px;
    font-size: var(--fs-sm);
  }
  .by-hand summary {
    width: fit-content;
    color: var(--fg-muted);
    cursor: pointer;
  }
  .by-hand summary:hover {
    color: var(--fg);
  }
  .by-hand p {
    margin: 8px 0 6px;
  }
  .code-row {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    max-width: 640px;
  }
  .code-row pre {
    flex: 1;
    margin: 0;
    padding: 8px 10px;
    overflow: auto;
    color: var(--fg);
    font: 12px/1.55 var(--mono);
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .skel-row {
    display: grid;
    gap: 10px;
    grid-template-columns: 1fr;
    padding: 18px 16px;
  }
  .all-clear {
    display: flex;
    gap: 14px;
    align-items: flex-start;
    max-width: 640px;
    margin-top: var(--s5);
    padding: var(--s4) var(--s5);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .all-clear h2 {
    margin-bottom: 4px;
    font-size: var(--fs-lg);
  }
  .ok-mark {
    display: grid;
    flex: none;
    place-items: center;
    width: 36px;
    height: 36px;
    color: var(--green);
    background: color-mix(in srgb, var(--green) 13%, transparent);
    border-radius: 10px;
  }
  @media (max-width: 900px) {
    .finding {
      grid-template-columns: 28px 1fr;
    }
    .fix {
      grid-column: 2;
      justify-items: start;
      text-align: left;
    }
  }
</style>
