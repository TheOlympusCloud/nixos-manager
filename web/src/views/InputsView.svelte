<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import JobLog from '../components/JobLog.svelte'
  import MachineImpact from '../components/MachineImpact.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import ScheduledUpdate from '../components/ScheduledUpdate.svelte'
  import { api, type Input } from '../lib/api'
  import { bigText, frame } from '../lib/ascii'
  import { fleet } from '../lib/fleet.svelte'
  import { age, days, plural } from '../lib/format'
  import { fetchModelDiff, type ModelDiff } from '../lib/modeldiff'
  import { draft } from '../lib/repo.svelte'
  import { go } from '../lib/router.svelte'
  import { act, app, startJob } from '../lib/state.svelte'
  import { confirmBox } from '../lib/ui.svelte'
  import { copyText } from '../lib/util'

  const inputs = $derived(fleet.model?.inputs ?? [])
  const active = (s?: string) => s === 'queued' || s === 'running'
  // The input job on screen: the one asked to be shown, else the latest lock or update.
  const job = $derived.by(() => {
    const i = app.inspect
    const shown = i?.kind === 'job' ? app.jobs.find((j) => j.id === i.id && (j.kind === 'update' || j.kind === 'lock')) : undefined
    return shown ?? app.jobs.find((j) => j.kind === 'update' || j.kind === 'lock')
  })
  const running = $derived(active(job?.state))
  let logOpen = $state(true)

  /** github:NixOS/nixpkgs/nixos-26.05 → the repository (and branch) on GitHub. */
  function github(i: Input): { repo: string; ref: string } | null {
    const m = /^github:([^/]+)\/([^/]+)(?:\/(.+))?$/.exec(i.url)
    return m ? { repo: `https://github.com/${m[1]}/${m[2]}`, ref: m[3] ?? '' } : null
  }
  const ageClass = (i: Input) => (!i.lastModified ? '' : days(i.lastModified) > 60 ? 'old' : days(i.lastModified) > 30 ? 'aging' : '')

  let copied = $state('')
  async function copy(rev: string) {
    if (await copyText(rev)) {
      copied = rev
      setTimeout(() => copied === rev && (copied = ''), 2000)
    }
  }

  function update(names: string[]) {
    logOpen = true
    startJob({ kind: 'update', inputs: names })
  }

  // ---- what the uncommitted flake.lock changes do ----
  let diff = $state<ModelDiff | null>(null)
  let computing = $state(false)
  let alive = true
  let seq = 0
  onMount(() => () => (alive = false))
  async function loadDiff() {
    const mine = ++seq
    const r = await fetchModelDiff({}, () => (computing = true), () => alive && mine === seq).catch(() => null)
    if (mine !== seq) return
    computing = false
    if (r) diff = r
  }
  let timer: ReturnType<typeof setTimeout> | undefined
  $effect(() => {
    void app.status
    untrack(() => {
      clearTimeout(timer)
      timer = setTimeout(loadDiff, diff ? 300 : 0)
    })
    return () => clearTimeout(timer)
  })
  const lockChanged = $derived(!!app.status?.files.some((f) => f.path === 'flake.lock'))
  const moved = $derived(diff?.inputs ?? [])
  const otherFiles = $derived((app.status?.files.length ?? 0) - (lockChanged ? 1 : 0))

  function review() {
    if (!draft.message.trim()) draft.message = moved.length ? `flake.lock: update ${moved.map((c) => c.name).join(', ')}` : 'flake.lock: update'
    go('#/changes')
  }
  async function discard() {
    const ok = await confirmBox({ title: 'Discard the update?', message: 'flake.lock goes back to the last commit; every input returns to the revision it had.', ok: 'Discard', danger: true })
    if (ok) await act(() => api.discard(['flake.lock']), { tree: true, tabs: true })
  }
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate title="flake inputs">
      <div class="page-head">
        <div>
          <h1 class="big" aria-label="Inputs" use:bigText={'Inputs'}></h1>
          <p class="sub">{plural(inputs.length, 'flake input')} every machine is built from, as locked in <span class="mono">flake.lock</span></p>
        </div>
        <span class="spacer"></span>
        <div class="tools">
          <button onclick={() => ((logOpen = true), startJob({ kind: 'lock' }))} disabled={running} title="nix flake lock: add inputs declared in flake.nix to flake.lock, without updating the others">
            <Icon name="key" /> Lock
          </button>
          <button class="primary" onclick={() => update([])} disabled={running || !inputs.length} title="nix flake update: move every input to its newest revision">
            <Icon name="refresh" /> Update all
          </button>
        </div>
      </div>

      {#if inputs.length}
        <div class="table-wrap">
          <table class="data">
            <thead>
              <tr><th>Input</th><th>Source</th><th>Revision</th><th>Updated</th><th><span class="sr-only">Actions</span></th></tr>
            </thead>
            <tbody>
              {#each inputs as i (i.name)}
                {@const gh = github(i)}
                {@const cls = ageClass(i)}
                <tr>
                  <td class="name mono">{i.name}</td>
                  <td class="src">
                    {#if gh}<a class="mono" href={gh.ref ? `${gh.repo}/tree/${gh.ref}` : gh.repo} target="_blank" rel="noreferrer noopener" title="Open on GitHub">{i.url}</a>
                    {:else}<span class="mono">{i.url || i.type}</span>{/if}
                  </td>
                  <td class="rev">
                    {#if i.rev}
                      {#if gh}<a class="mono" href="{gh.repo}/commit/{i.rev}" target="_blank" rel="noreferrer noopener" title="This revision on GitHub">{i.rev.slice(0, 7)}</a>
                      {:else}<span class="mono">{i.rev.slice(0, 7)}</span>{/if}
                      <button class="icon copy" aria-label="Copy the full revision of {i.name}" title={copied === i.rev ? 'Copied' : 'Copy the full revision'} onclick={() => copy(i.rev)}>
                        <Icon name={copied === i.rev ? 'tick' : 'copy'} />
                      </button>
                    {:else}<span class="muted">not locked</span>{/if}
                  </td>
                  <td class="when">
                    {#if i.lastModified}
                      <span class="age {cls}" title={new Date(i.lastModified * 1000).toLocaleString()}>
                        <span class="dot {cls === 'old' ? 'err' : cls === 'aging' ? 'warn' : 'off'}"></span> {age(i.lastModified)}
                      </span>
                      <span class="date muted">{new Date(i.lastModified * 1000).toLocaleDateString()}</span>
                      {#if cls === 'old'}<span class="badge err">Outdated</span>{/if}
                    {/if}
                  </td>
                  <td class="act">
                    <button class="small" onclick={() => update([i.name])} disabled={running} title="nix flake update {i.name}">Update</button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        <p class="legend muted">Inputs older than 30 days show in amber, older than 60 days in red: updating brings security fixes and new versions.</p>
      {:else}
        <div class="empty-state">
          <h2>No inputs locked</h2>
          <p>The flake has no <span class="mono">flake.lock</span> yet, or no inputs. Declare inputs in <span class="mono">flake.nix</span>, then lock them.</p>
          <div class="row"><button class="primary" onclick={() => startJob({ kind: 'lock' })}><Icon name="key" /> Lock the flake</button></div>
        </div>
      {/if}

      <!-- The running job on the left, the scheduled update on the right (each takes the row alone when the other is not there). -->
      <div class="duo">
        {#if job}
          <section
            class="section job"
            aria-label="Last input job"
            data-tone={running ? 'accent' : job.state === 'failed' ? 'err' : undefined}
            use:frame={{ title: job.kind === 'lock' ? 'Lock' : 'Update', animate: true }}>
            <div class="job-head">
              <h2 class="sr-only">{job.kind === 'lock' ? 'Lock' : 'Update'}</h2>
              <span class="badge {running ? 'busy' : job.state === 'passed' ? 'ok' : job.state === 'failed' ? 'err' : ''}">{running ? (job.kind === 'lock' ? 'Locking…' : 'Updating…') : job.state}</span>
              {#if job.started}<span class="muted">{new Date(job.started).toLocaleTimeString()}</span>{/if}
              <span class="spacer"></span>
              <button class="link" onclick={() => (logOpen = !logOpen)} aria-expanded={logOpen}>{logOpen ? 'Hide the log' : 'Show the log'}</button>
            </div>
            {#if logOpen}<div class="log">{#key job.id}<JobLog jobId={job.id} compact />{/key}</div>{/if}
          </section>
        {/if}
        {#if inputs.length}<ScheduledUpdate />{/if}
      </div>

      {#if lockChanged || moved.length}
        <section class="section">
          <div class="changed-head">
            <h2>What the update changes</h2>
            <span class="muted">flake.lock is not committed yet{otherFiles > 0 ? `; ${plural(otherFiles, 'other uncommitted file')} may add to this (see Changes)` : ''}.</span>
          </div>
          <MachineImpact {diff} {computing} quiet="The new revisions change nothing any machine runs." />
          <div class="row-actions">
            <button class="primary" onclick={review}><Icon name="commit" /> Review and commit</button>
            <button class="danger" onclick={discard}><Icon name="undo" /> Discard the update</button>
          </div>
        </section>
      {/if}
    </ModelGate>
  </div>
</div>

<style>
  .table-wrap {
    overflow: auto;
  }
  .name {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .src a {
    color: var(--fg-muted);
  }
  .rev a {
    color: var(--fg);
  }
  .rev,
  .when {
    white-space: nowrap;
  }
  .copy {
    margin-left: 1ch;
  }
  .age {
    display: inline-block;
    min-width: 14ch;
  }
  .age.aging {
    color: var(--yellow);
  }
  .age.old {
    color: var(--red);
  }
  .date {
    margin-right: 2ch;
  }
  .act {
    width: 1%;
    text-align: right;
  }
  .duo {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(60ch, 1fr));
    gap: 0 2ch;
    align-items: start;
  }
  .job-head,
  .changed-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .changed-head {
    margin-bottom: var(--lh);
  }
  .log {
    display: flex;
    flex-direction: column;
    height: calc(var(--lh) * 12);
    margin-top: var(--lh);
    overflow: hidden;
  }
  .row-actions {
    display: flex;
    gap: 2ch;
    margin-top: var(--lh);
  }
</style>
