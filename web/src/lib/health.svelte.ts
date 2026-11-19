// Fleet insights and their one-click fixes.
import { ApiError, api, type Insight } from './api'
import { fleet, setOption, waitJob } from './fleet.svelte'
import { fixPlan } from './insights'
import { go } from './router.svelte'
import { startJob } from './state.svelte'
import { quietly, toast } from './ui.svelte'

/** checked: an evaluation after the fix still reports the finding. */
export type FixState = { done: number; total: number; host: string; error: string; job: string; finished: boolean; checked?: boolean }

export const health = $state({
  insights: null as Insight[] | null,
  error: '',
  /** The insights are being computed (the model is evaluating). */
  computing: false,
  /** Per insight id: a fix in progress or just applied (until the next evaluation). */
  fixing: {} as Record<string, FixState>,
})

let seq = 0
export async function loadInsights() {
  const mine = ++seq
  for (let tries = 0; tries < 5; tries++) {
    let r
    try {
      r = await api.insights()
    } catch (e) {
      if (mine === seq && !(e instanceof ApiError && e.status === 401)) health.error = e instanceof Error ? e.message : String(e)
      if (mine === seq) health.computing = false
      return
    }
    if (mine !== seq) return
    if (r.state === 'ready') {
      health.insights = r.insights ?? []
      health.error = ''
      health.computing = false
      // This evaluation confirms applied fixes (the finding is gone) or not (it is still there).
      const ids = new Set(health.insights.map((i) => i.id))
      for (const [id, f] of Object.entries(health.fixing)) {
        if (!f.finished) continue
        if (f.error || !ids.has(id)) delete health.fixing[id]
        else f.checked = !fleet.computing
      }
      return
    }
    health.computing = true
    if (!(await waitJob(r.job))) await new Promise((res) => setTimeout(res, 1500))
    if (mine !== seq) return
  }
  health.computing = false
}

/** Write the fix's settings to each affected machine that is Editable here, one after the other. */
export async function applyFix(i: Insight) {
  const plan = fixPlan(i, fleet.model?.hosts ?? [])
  if (!plan.steps.length || health.fixing[i.id]?.total) return
  const st: FixState = { done: 0, total: plan.steps.length, host: plan.steps[0].host, error: '', job: '', finished: false }
  health.fixing[i.id] = st
  const ok = await quietly(async () => {
    for (const s of plan.steps) {
      health.fixing[i.id].host = s.host
      if (!(await setOption(s.host, s.path, s.value))) return false
      health.fixing[i.id].done++
    }
    return true
  })
  const f = health.fixing[i.id]
  if (!ok) {
    f.error = `Stopped on ${f.host}; ${f.done} of ${f.total} settings were written.`
    f.finished = true
    return
  }
  f.finished = true
  toast(`${i.fix?.label} on ${plan.editable.join(', ')}. The machines are evaluated again; review and commit in Changes.`, 'ok', 'Fix applied', {
    label: 'View changes',
    run: () => go('#/changes'),
  })
}

/** Start the flake update a fix asks for; its toast and the Inputs view follow it. */
export async function updateInputFix(i: Insight) {
  const name = i.fix?.updateInput
  if (!name || health.fixing[i.id]?.job) return
  const job = await startJob({ kind: 'update', inputs: [name] })
  if (!job) return
  health.fixing[i.id] = { done: 0, total: 0, host: '', error: '', job, finished: false }
  const state = await waitJob(job)
  if (health.fixing[i.id]?.job !== job) return
  health.fixing[i.id].finished = true
  if (state === 'failed') health.fixing[i.id].error = `Updating ${name} failed; the log says why.`
}
