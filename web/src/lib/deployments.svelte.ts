// What the machines report after comin deploys, kept fresh while a view shows it.
import { ApiError, api, type DeployOverview } from './api'
import { pollInterval } from './deployments'
import { loadInsights } from './health.svelte'

export const deploys = $state({
  overview: null as DeployOverview | null,
  error: '',
})

export const deployOf = (host: string) => deploys.overview?.machines.find((m) => m.host === host)

let signature = ''
export async function loadDeployments() {
  try {
    const ov = await api.deployments()
    deploys.overview = ov
    deploys.error = ''
    // A new failure or a machine catching up changes the Health findings too.
    const sig = ov.machines.map((m) => `${m.host}:${m.state}:${m.failure?.at ?? ''}`).join()
    if (signature && sig !== signature) loadInsights()
    signature = sig
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 401)) deploys.error = e instanceof Error ? e.message : String(e)
  }
}

let users = 0
let gen = 0
/** Poll while something on screen shows deployments, starting now; returns the stop function (for $effect). */
export function watchDeployments(): () => void {
  users++
  poll(++gen) // the previous loop ends when it wakes up
  return () => {
    if (--users === 0) gen++
  }
}

async function poll(g: number) {
  while (g === gen) {
    await loadDeployments()
    if (g !== gen) return
    await new Promise((r) => setTimeout(r, pollInterval(deploys.overview)))
  }
}
