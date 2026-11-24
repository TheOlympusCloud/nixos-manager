<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type UpdateSchedule, type UpdateStatus } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { plural } from '../lib/format'
  import { fetchModelDiff, type ModelDiff } from '../lib/modeldiff'
  import { app, refreshJobs, refreshStatus } from '../lib/state.svelte'
  import { confirmBox, toast } from '../lib/ui.svelte'
  import { hourText, scheduleDays, updateBusy, updateLine } from '../lib/updates'
  import { ago } from '../lib/util'
  import Icon from './Icon.svelte'
  import JobLog from './JobLog.svelte'
  import MachineImpact from './MachineImpact.svelte'
  import Switch from './Switch.svelte'

  /** Inputs → Scheduled update: the schedule, Update now, and the latest update, to merge with one click. */
  let st = $state<UpdateStatus | null>(null)
  let error = $state('')
  let busy = $state<'' | 'schedule' | 'run' | 'merge' | 'discard'>('')
  let logOpen = $state(false)
  const up = $derived(st?.update)
  const line = $derived(up ? updateLine(up, st?.outdated) : null)
  const running = $derived(updateBusy(up))
  // The check's log, else nix flake update's, while the server still has the job.
  const logJob = $derived([up?.check, up?.job].find((id) => id && app.jobs.some((j) => j.id === id)))
  const show = (e: unknown) => (error = e instanceof Error ? e.message : String(e))
  const hours = Array.from({ length: 24 }, (_, h) => h)

  let alive = true
  let timer: ReturnType<typeof setTimeout> | undefined
  async function load() {
    clearTimeout(timer)
    try {
      st = await api.updates()
      error = ''
      await refreshJobs()
    } catch (e) {
      show(e)
    }
    if (alive && updateBusy(st?.update)) timer = setTimeout(load, 4000)
  }
  onMount(() => {
    load()
    return () => {
      alive = false
      clearTimeout(timer)
    }
  })

  async function schedule(change: Partial<UpdateSchedule>) {
    if (!st) return
    busy = 'schedule'
    try {
      st = await api.setUpdateSchedule({ ...st.schedule, ...change })
      error = ''
    } catch (e) {
      show(e)
    } finally {
      busy = ''
    }
  }
  async function run() {
    busy = 'run'
    try {
      st = await api.runUpdate()
      error = ''
    } catch (e) {
      show(e)
    } finally {
      busy = ''
    }
    load()
  }
  async function merge() {
    if (!up?.commit) return
    const changed = up.changed ?? []
    const ok = await confirmBox({
      title: `Merge the update into ${up.branch}?`,
      message: `It is pushed to ${up.branch} on the remote, so comin deploys it: ${changed.length ? `${plural(changed.length, 'machine')} change (${changed.join(', ')}).` : "no machine's system changes."}`,
      ok: 'Merge and deploy',
    })
    if (!ok) return
    busy = 'merge'
    try {
      toast(await api.mergeUpdate(up.commit), 'ok')
      error = ''
    } catch (e) {
      show(e)
    } finally {
      busy = ''
    }
    await Promise.all([load(), refreshStatus()])
  }
  async function discard() {
    busy = 'discard'
    try {
      st = await api.discardUpdate()
      error = ''
    } catch (e) {
      show(e)
    } finally {
      busy = ''
    }
  }

  // What the update changes: its commit against the one it was made on top of.
  let diff = $state<ModelDiff | null>(null)
  let computing = $state(false)
  let diffOf = ''
  $effect(() => {
    const c = up?.commit && (up.state === 'ready' || up.state === 'failed') ? up.commit : ''
    if (c === diffOf) return
    diffOf = c
    diff = null
    computing = false
    if (!c) return
    fetchModelDiff(
      { rev: c },
      () => (computing = true),
      () => alive && diffOf === c,
    ).then(
      (r) => {
        if (diffOf !== c) return
        diff = r
        computing = false
      },
      () => diffOf === c && (computing = false),
    )
  })
</script>

<section class="section" aria-labelledby="sched-head" use:frame={{ title: 'Scheduled update' }}>
  <div class="head">
    <h2 id="sched-head" class="sr-only">Scheduled update</h2>
    <span class="muted">Every input, updated on the <span class="mono">flake-update</span> branch and checked. Nothing reaches the machines until you merge it.</span>
    <span class="spacer"></span>
    <button onclick={run} disabled={!st || running || !!busy} title="Update every input now, on top of what the machines deploy, push it to flake-update and check it">
      <Icon name="refresh" /> {busy === 'run' ? 'Starting…' : 'Update now'}
    </button>
  </div>
  {#if st}
    <div class="body">
      <div class="schedule">
        <Switch checked={st.schedule.enabled} label="Update the inputs on a schedule" busy={busy === 'schedule'} disabled={!!busy && busy !== 'schedule'} onchange={(on) => schedule({ enabled: on })} />
        <label class="sr-only" for="sched-day">Day</label>
        <select id="sched-day" value={String(st.schedule.day)} disabled={!st.schedule.enabled || !!busy} onchange={(e) => schedule({ day: Number(e.currentTarget.value) })}>
          {#each scheduleDays as d (d.value)}<option value={String(d.value)}>{d.label}</option>{/each}
        </select>
        <span class="muted">at</span>
        <label class="sr-only" for="sched-hour">Hour</label>
        <select id="sched-hour" value={String(st.schedule.hour)} disabled={!st.schedule.enabled || !!busy} onchange={(e) => schedule({ hour: Number(e.currentTarget.value) })}>
          {#each hours as h (h)}<option value={String(h)}>{hourText(h)}</option>{/each}
        </select>
        <span class="muted">
          {st.zone}, the server's clock{#if st.next}; next {new Date(st.next).toLocaleString(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}{/if}
        </span>
      </div>

      {#if up && line}
        <div class="latest tone-{line.tone}" aria-live="polite">
          <div class="line">
            <span class="dot {line.tone === 'bad' ? 'err' : line.tone === 'muted' ? 'off' : line.tone}"></span>
            <strong>{line.title}</strong>
            <span class="muted">{up.scheduled ? 'Scheduled' : 'Started by hand'} {ago(up.started)}{#if up.commit}&nbsp;· <span class="mono">{up.commit.slice(0, 7)}</span> on flake-update{/if}</span>
          </div>
          {#if line.detail}
            {#if up.state === 'error'}<pre class="detail mono">{line.detail}</pre>{:else}<p class="detail">{line.detail}</p>{/if}
          {/if}
          <div class="actions">
            {#if up.state === 'ready' && !st.outdated}
              <button class="primary" onclick={merge} disabled={!!busy}><Icon name="merge" /> {busy === 'merge' ? 'Merging…' : `Merge into ${up.branch}`}</button>
            {/if}
            {#if up.state === 'ready' || up.state === 'failed' || up.state === 'error'}
              <button onclick={discard} disabled={!!busy}><Icon name="trash" /> Discard</button>
            {/if}
            {#if logJob}<button class="link" onclick={() => (logOpen = !logOpen)} aria-expanded={logOpen}>{logOpen ? 'Hide the log' : 'Show the log'}</button>{/if}
          </div>
          {#if logOpen && logJob}<div class="log">{#key logJob}<JobLog jobId={logJob} compact />{/key}</div>{/if}
          {#if up.commit && (up.state === 'ready' || up.state === 'failed')}
            <MachineImpact {diff} {computing} commit quiet="The update changes nothing any machine runs." />
          {/if}
        </div>
      {:else}
        <p class="muted">No update has run yet.</p>
      {/if}
    </div>
  {/if}
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}
</section>

<style>
  .head,
  .schedule,
  .line,
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .schedule {
    gap: 0 1ch;
  }
  .schedule select {
    width: auto;
  }
  .body,
  .latest {
    display: grid;
    gap: var(--lh) 0;
  }
  .body,
  .error-text {
    margin-top: var(--lh);
  }
  .line {
    gap: 0 1ch;
  }
  pre.detail {
    max-height: calc(var(--lh) * 9);
    overflow: auto;
    white-space: pre-wrap;
  }
  .log {
    display: flex;
    flex-direction: column;
    height: calc(var(--lh) * 12);
    overflow: hidden;
  }
</style>
