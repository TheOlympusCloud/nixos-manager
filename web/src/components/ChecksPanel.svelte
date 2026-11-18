<script lang="ts">
  import { api, type Job } from '../lib/api'
  import { app, checkStaged, refreshJobs, startJob } from '../lib/state.svelte'
  import { promptBox } from '../lib/ui.svelte'
  import { ago } from '../lib/util'
  import Icon from './Icon.svelte'

  let { visible }: { visible: boolean } = $props()

  let loading = $state(false)
  let error = $state('')

  async function loadTargets() {
    loading = true
    error = ''
    try {
      const targets = await api.targets()
      app.targets = targets
      // Forget selections for targets that no longer exist.
      setSelected(app.selectedTargets.filter((a) => targets.some((t) => t.attr === a)))
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  $effect(() => {
    if (visible && app.targets === null && !loading && !error) loadTargets()
  })

  function setSelected(attrs: string[]) {
    app.selectedTargets = attrs
    localStorage.setItem('nixmgr.targets', JSON.stringify(attrs))
  }

  function toggle(attr: string, on: boolean) {
    setSelected(on ? [...app.selectedTargets, attr] : app.selectedTargets.filter((a) => a !== attr))
  }

  async function update() {
    const inputs = await promptBox({
      title: 'Update flake inputs',
      message: 'Input names separated by spaces. Use * to update all inputs.',
      input: { label: 'Inputs', value: '*' },
      ok: 'Update',
    })
    if (inputs === null) return
    const list = inputs.split(/[\s,]+/).filter((s) => s && s !== '*')
    await startJob({ kind: 'update', inputs: list })
  }

  const showJob = (j: Job) => (app.inspect = { kind: 'job', id: j.id })
  // Evaluations and search indexes report in their own views.
  const jobs = $derived(app.jobs.filter((j) => ['check', 'lock', 'update'].includes(j.kind)))
  const kinds: Record<string, string> = { check: 'Check', lock: 'Flake lock', update: 'Input update' }
  const short = (s?: string) => (s ? s.slice(0, 8) : '')
  const status = $derived(app.status)
</script>

<section class="panel" aria-label="Checks">
  <div class="panel-head">
    <h2>Checks</h2>
    <div class="tools">
      <button class="icon" title="Refresh jobs" aria-label="Refresh jobs" onclick={refreshJobs}><Icon name="refresh" /></button>
    </div>
  </div>

  <div class="scroll">
    <div class="staged" class:ok={status?.staged_checked}>
      {#if status && !status.files.some((f) => f.staged)}
        <span class="muted">Nothing staged. Checks run on the staged changes.</span>
      {:else if status?.staged_checked}
        Staged tree <code>{short(status.staged_tree)}</code> passed a check.
      {:else if status}
        Staged tree <code>{short(status.staged_tree)}</code> is <strong>not checked</strong>.
      {/if}
    </div>

    <div class="section-head">
      <h3>Targets</h3>
      <div class="tools">
        <button class="icon" title="Reload targets" aria-label="Reload targets" disabled={loading} onclick={loadTargets}><Icon name="refresh" /></button>
      </div>
    </div>
    <div class="targets">
      {#if loading}
        <p class="muted" role="status"><span class="spinner" aria-hidden="true"></span> Evaluating flake outputs…</p>
      {:else if error}
        <p class="error">{error}</p>
        <button onclick={loadTargets}>Retry</button>
      {:else if app.targets?.length === 0}
        <p class="muted">The flake has no NixOS, home-manager or darwin configurations.</p>
      {:else if app.targets}
        {#each app.targets as t (t.attr)}
          <label class="target" title={t.attr}>
            <input type="checkbox" checked={app.selectedTargets.includes(t.attr)} onchange={(e) => toggle(t.attr, e.currentTarget.checked)} />
            <span class="kind k-{t.kind}">{t.kind}</span>
            <span class="tname">{t.name}</span>
          </label>
        {/each}
        <div class="sel">
          <button class="link" onclick={() => setSelected(app.targets?.map((t) => t.attr) ?? [])}>All</button>
          <button class="link" onclick={() => setSelected([])}>None</button>
        </div>
      {/if}
    </div>

    <div class="actions">
      <button class="primary" onclick={checkStaged}><Icon name="play" size={12} /> Check staged changes</button>
      <p class="muted small">Runs <code>nix flake check</code>{app.selectedTargets.length ? ` and a dry build of ${app.selectedTargets.length} target(s)` : ''} on a snapshot of the staged changes.</p>
      <div class="row">
        <button onclick={() => startJob({ kind: 'lock' })} title="nix flake lock">Lock</button>
        <button onclick={update} title="nix flake update">Update inputs…</button>
      </div>
    </div>

    <div class="section-head"><h3>Jobs</h3></div>
    <ul class="list">
      {#each jobs as j (j.id)}
        <li class="item" class:selected={app.inspect?.kind === 'job' && app.inspect.id === j.id}>
          <button class="item-main job" onclick={() => showJob(j)}>
            <span class="state s-{j.state}">{j.state}</span>
            <span class="name">{kinds[j.kind]} {j.id}</span>
            <span class="dir">{j.tree ? short(j.tree) : ''} {ago(j.started)}</span>
          </button>
        </li>
      {:else}
        <li class="empty">No jobs yet.</li>
      {/each}
    </ul>
  </div>
</section>

<style>
  .staged {
    margin: 4px 12px 8px;
    padding: 8px 10px;
    font-size: 12px;
    color: var(--yellow);
    background: var(--bg-deep);
    border-radius: 4px;
  }
  .staged.ok {
    color: var(--green);
  }
  .targets {
    padding: 2px 12px 6px;
  }
  .targets p {
    margin: 4px 0;
  }
  .target {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 3px 0;
    cursor: pointer;
  }
  .kind {
    min-width: 46px;
    font-size: 11px;
    color: var(--fg-muted);
  }
  .k-nixos {
    color: var(--blue);
  }
  .k-home {
    color: var(--purple);
  }
  .tname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sel {
    display: flex;
    gap: 10px;
    margin-top: 4px;
  }
  .actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 6px 12px 12px;
  }
  .actions p {
    margin: 0;
  }
  .actions .primary {
    height: 32px;
  }
  .row {
    display: flex;
    gap: 6px;
  }
  .row button {
    flex: 1;
  }
  .job {
    gap: 8px;
  }
  .state {
    min-width: 54px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .s-passed {
    color: var(--green);
  }
  .s-failed {
    color: var(--red);
  }
  .s-running {
    color: var(--blue);
  }
  .s-queued {
    color: var(--fg-muted);
  }
  .error {
    color: var(--red);
    white-space: pre-wrap;
  }
  .small {
    font-size: 12px;
  }
</style>
