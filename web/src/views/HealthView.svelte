<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import Bar from '../components/cell/Bar.svelte'
  import type { Insight } from '../lib/api'
  import { openPackageSearch } from '../lib/dialogs.svelte'
  import { fleet, hostByName, updateManagedModule } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { bigText, frame } from '../lib/ascii'
  import { applyFix, checkCommitFix, health, loadInsights, updateInputFix } from '../lib/health.svelte'
  import { bySeverity, fixPlan, insightDetail, insightTitle, missingPackage, nixLines, severityLabel } from '../lib/insights'
  import { link, machineLink } from '../lib/router.svelte'
  import { app, showJob } from '../lib/state.svelte'
  import { copyText } from '../lib/util'

  const groups = $derived(bySeverity(health.insights ?? []))
  const sigil: Record<Insight['severity'], string> = { error: '✖', warning: '▲', info: '◆' }
  const tone: Record<Insight['severity'], string | undefined> = { error: 'err', warning: 'warn', info: undefined }
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
          <Bar v={st.done / st.total} w={18} live />
        </div>
        <p class="fix-note">Writing {st.done + 1} of {st.total} · {st.host}</p>
      {:else if st?.error}
        <p class="fix-note bad" role="alert">{st.error}</p>
        <button onclick={() => (delete health.fixing[i.id], applyFix(i))}>Try again</button>
      {:else if st?.checked}
        <p class="fix-note warn-note">Written on {plan.editable.join(', ')}, but the new evaluation still reports it: a definition in code may force the old value (mkForce).</p>
        <a class="link-small" href="#/changes">Review in Changes</a>
      {:else if st}
        <p class="fix-note ok"><Icon name="tick" /> Applied on {plan.editable.join(', ') || 'the machines'}</p>
        <a class="link-small" href="#/changes">Review and commit in Changes</a>
      {:else}
        {#if plan.steps.length}
          <button class:primary={i.severity !== 'info'} onclick={() => applyFix(i)}>
            <Icon name="tick" /> {i.fix.label}{plan.codeOnly.length || plan.noKey.length ? ` on ${plan.editable.join(', ')}` : ''}
          </button>
        {:else if plan.codeOnly.length}
          <p class="fix-note unavailable"><Icon name="code" /> {i.fix.label}: change it in code, {plan.codeOnly.join(', ')} {plan.codeOnly.length === 1 ? 'is' : 'are'} Code only</p>
        {/if}
        {#each plan.noKey as h (h)}
          <p class="fix-note unavailable"><Icon name="key" /> Not on {h}: no user there has an SSH key, so this would lock everyone out. <a href={machineLink(h, 'users')}>Add a key in Users</a> first.</p>
        {/each}
      {/if}
    </div>
  {:else if i.fix?.updateInput}
    {@const name = i.fix.updateInput}
    <div class="fix">
      {#if st?.job && !st.finished}
        <button class="is-busy" disabled><span class="spinner" aria-hidden="true"></span>Updating {name}…</button>
        <button class="link link-small" onclick={() => showJob(st.job, 'update')}>Show the log</button>
      {:else if st?.error}
        <p class="fix-note bad" role="alert">{st.error}</p>
        <button class="link link-small" onclick={() => showJob(st.job, 'update')}>Show the log</button>
      {:else if st?.job}
        <p class="fix-note ok"><Icon name="tick" /> {name} updated ({jobState(st.job)})</p>
        <a class="link-small" href={link('inputs')}>See what changed</a>
      {:else}
        <button class:primary={i.severity !== 'info'} onclick={() => updateInputFix(i)}><Icon name="refresh" /> {i.fix.label}</button>
      {/if}
    </div>
  {:else if i.fix?.check}
    {@const short = i.fix.check.slice(0, 7)}
    <div class="fix">
      {#if st?.job && !st.finished}
        <button class="is-busy" disabled><span class="spinner" aria-hidden="true"></span>Checking {short}…</button>
        <button class="link link-small" onclick={() => showJob(st.job)}>Show the log</button>
      {:else if st?.error}
        <p class="fix-note bad" role="alert">{st.error}</p>
        <button class="link link-small" onclick={() => showJob(st.job)}>Show the log</button>
      {:else}
        <button class:primary={i.severity !== 'info'} onclick={() => checkCommitFix(i)}><Icon name="play" /> {i.fix.label}</button>
        {#if i.link}<a class="link-small" href={i.link}>Show the last check's log</a>{/if}
      {/if}
    </div>
  {:else if i.link?.startsWith('#/changes')}
    <div class="fix"><a class="btn" href={i.link}>Show the log</a></div>
  {:else if missingPackage(i)}
    {@const pkg = missingPackage(i)}
    {@const h = hostByName(i.hosts[0])}
    <div class="fix">
      {#if h?.managedFile}
        <button onclick={() => openPackageSearch(pkg, [h.name])}><Icon name="plus" /> Add {pkg} to {h.name}…</button>
      {:else}
        <p class="fix-note">{i.hosts[0]} is Code only: add {pkg} in its configuration.</p>
      {/if}
    </div>
  {:else if i.id === 'managed-old'}
    <div class="fix"><button class="primary" onclick={updateManagedModule}><Icon name="refresh" /> Update managed.nix</button></div>
  {:else if i.id === 'eval-error' && i.hosts.length === 1}
    <div class="fix"><a class="btn" href={machineLink(i.hosts[0])}>Open {i.hosts[0]}</a></div>
  {:else if i.link === '#/deployments'}
    <div class="fix"><a class="btn" href={i.link}>Open Deployments</a></div>
  {/if}
{/snippet}

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <div>
        <h1 class="big" aria-label="Health" use:bigText={'Health'}></h1>
        <p class="sub">
          {#if health.insights}
            {health.insights.length ? `${plural(health.insights.length, 'finding')}${affected ? ` on ${plural(affected, 'machine')}` : ''}, from the last evaluation` : 'From the last evaluation'}
          {:else}
            Security and upkeep checks of every machine
          {/if}
        </p>
      </div>
      <span class="spacer"></span>
      <div class="tools">
        {#if health.computing || fleet.computing}<span class="badge busy">Evaluating…</span>{/if}
        <button onclick={loadInsights} disabled={health.computing} title="Ask for the findings again"><Icon name="refresh" /> Check again</button>
      </div>
    </div>

    {#if health.error && !health.insights}
      <div class="empty-state" role="alert">
        <h2>Cannot check the fleet</h2>
        <pre>{health.error}</pre>
        <div class="row"><button class="primary" onclick={loadInsights}><Icon name="refresh" /> Try again</button></div>
      </div>
    {:else if !health.insights}
      <div class="list" aria-hidden="true">
        {#each [0, 1, 2] as k (k)}<div class="finding skel-row" use:frame={'dash'}><span class="skel" style:width="34ch"></span><span class="skel" style:width="62ch"></span></div>{/each}
      </div>
    {:else if !health.insights.length}
      <div class="all-clear" data-tone="ok" use:frame>
        <span class="ok-mark"><Icon name="tick" /></span>
        <div>
          <h2>Nothing to fix</h2>
          <p class="muted">Every machine evaluates, keeps its firewall on, accepts SSH keys only, collects garbage, and the flake inputs are recent.</p>
        </div>
      </div>
    {:else}
      {#each groups as g (g.severity)}
        <section class="group" aria-labelledby="sev-{g.severity}">
          <h2 id="sev-{g.severity}" class="g-title sev-{g.severity}">{severityLabel[g.severity]}<span class="n num">{g.items.length}</span></h2>
          <ul class="list">
            {#each g.items as i (i.id)}
              {@const plan = fixPlan(i, fleet.model?.hosts ?? [])}
              <li class="finding sev-{i.severity}" data-tone={tone[i.severity]} use:frame={{ title: `${sigil[i.severity]} ${insightTitle(i)}` }}>
                <div class="main">
                  <h3 class="sr-only">{insightTitle(i)}</h3>
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
                        <button class="icon" aria-label="Copy the Nix lines" title={copied === i.id ? 'Copied' : 'Copy'} onclick={() => copy(i)}><Icon name={copied === i.id ? 'tick' : 'copy'} /></button>
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
  .group + .group {
    margin-top: var(--lh);
  }
  .g-title {
    margin-bottom: var(--lh);
    color: var(--c);
  }
  .g-title .n {
    margin-left: 1ch;
    color: var(--fg-muted);
    font-weight: 400;
  }
  .sev-error {
    --c: var(--red);
  }
  .sev-warning {
    --c: var(--yellow);
  }
  .sev-info {
    --c: var(--blue);
  }
  .list {
    display: grid;
    gap: var(--lh);
  }
  /* A finding is a card: title on the frame line, the fix at the right of its last row. */
  .finding {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0 3ch;
    align-items: end;
  }
  .main {
    display: grid;
    min-width: 0;
  }
  .detail {
    max-width: 96ch;
  }
  .host-chip {
    color: var(--fg);
  }
  .host-chip .dot {
    margin-right: 1ch;
  }
  .host-chip:hover .dot {
    --c: var(--on-accent);
  }
  .tag {
    margin-left: 1ch;
    color: var(--fg-faint);
  }
  .tag::before {
    content: 'λ ';
  }
  .host-chip:hover .tag {
    color: inherit;
  }
  .fix {
    display: grid;
    justify-items: end;
    min-width: 24ch;
    max-width: 40ch;
    text-align: right;
  }
  .fix-note {
    color: var(--fg-muted);
  }
  .fix-note.unavailable {
    text-align: left;
  }
  .fix-note.warn-note {
    color: var(--yellow);
  }
  .fix-note.ok {
    color: var(--green);
  }
  .fix-note.bad {
    color: var(--red);
  }
  .fix-note.bad::before {
    content: '✖ ';
  }
  .by-hand summary {
    width: fit-content;
    color: var(--fg-muted);
  }
  .by-hand summary:hover {
    color: var(--fg);
    background: var(--bg-hover);
  }
  .by-hand p {
    margin: var(--lh) 0;
  }
  .code-row {
    display: flex;
    gap: 1ch;
    max-width: 80ch;
  }
  .code-row pre {
    flex: 1;
    padding: 0 1ch;
    overflow: auto;
    background: var(--bg-input);
  }
  .skel-row {
    grid-template-columns: minmax(0, 1fr);
  }
  .all-clear {
    display: flex;
    gap: 2ch;
    max-width: 80ch;
    margin-top: calc(var(--lh) * 2);
  }
  .all-clear h2 {
    margin-bottom: var(--lh);
  }
  .ok-mark {
    color: var(--green);
  }
  @media (max-width: 900px) {
    .finding {
      grid-template-columns: minmax(0, 1fr);
    }
    .fix {
      justify-items: start;
      text-align: left;
    }
  }
</style>
