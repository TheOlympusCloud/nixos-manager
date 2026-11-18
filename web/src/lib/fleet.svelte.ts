// The evaluated fleet (stale while a new evaluation runs) and editing by clicking.
import { ApiError, api, type FeatureGroup, type Host, type Managed, type SearchReply } from './api'
import { fillPath, optionSegments, pathName } from './options'
import { go } from './router.svelte'
import { refreshStatus } from './state.svelte'
import { report, toast } from './ui.svelte'

export const fleet = $state({
  model: null as import('./api').Model | null,
  /** A newer evaluation is running; `model` is the last one that finished. */
  computing: false,
  job: '',
  loadError: '',
  /** Machines edited since `model` was evaluated. */
  pending: [] as string[],
  managed: {} as Record<string, Managed>,
})

/** Resolves with the job's final state ('' when the stream broke) once it finishes. */
export function waitJob(id: string): Promise<string> {
  return new Promise((resolve) => {
    const es = new EventSource(api.jobEventsUrl(id))
    es.addEventListener('done', (e) => {
      es.close()
      resolve(JSON.parse((e as MessageEvent).data).state)
    })
    es.addEventListener('error', () => {
      if (es.readyState !== EventSource.CLOSED) return // the browser retries by itself
      resolve('')
    })
  })
}

/**
 * Run a search that answers 202 while its index builds: report the job, wait for it, ask again.
 * Null when the caller lost interest (`alive` turned false) or the index never became ready.
 */
export async function searchIndexed<T>(fn: () => Promise<SearchReply<T>>, onJob: (job: string) => void, alive = () => true): Promise<T[] | null> {
  for (let tries = 0; tries < 4 && alive(); tries++) {
    const r = await fn()
    if (r.state === 'ready') return r.results ?? []
    onJob(r.job)
    if ((await waitJob(r.job)) === 'failed') throw new Error('Building the search index failed; the log above says why.')
  }
  return null
}

let seq = 0
export async function loadModel() {
  const mine = ++seq
  let reply
  try {
    reply = await api.model()
  } catch (e) {
    if (mine !== seq) return
    if (!(e instanceof ApiError && e.status === 401)) fleet.loadError = e instanceof Error ? e.message : String(e)
    return
  }
  if (mine !== seq) return
  fleet.loadError = ''
  if (reply.state === 'ready') {
    fleet.model = reply.model
    fleet.computing = false
    fleet.job = ''
    fleet.pending = []
    return
  }
  fleet.computing = true
  fleet.job = reply.job
  const state = await waitJob(reply.job)
  if (mine !== seq) return
  if (!state) await new Promise((r) => setTimeout(r, 1500)) // lost the stream: do not spin
  if (mine === seq) loadModel()
}

/** Evaluate the same working copy again, after a failure outside the files (no network, Nix fixed). */
export async function evaluateAgain() {
  try {
    fleet.job = await api.refreshModel()
    fleet.computing = true
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 409)) fleet.loadError = e instanceof Error ? e.message : String(e)
  }
  await loadModel()
}

let timer: ReturnType<typeof setTimeout> | undefined
/** The working copy changed (or may have): ask for its model soon. */
export function scheduleModel() {
  clearTimeout(timer)
  timer = setTimeout(loadModel, 250)
}

export const hostByName = (name: string): Host | undefined => fleet.model?.hosts.find((h) => h.name === name)
export const healthy = () => (fleet.model?.hosts ?? []).filter((h) => !h.error)

export async function loadManaged(host: string) {
  try {
    fleet.managed[host] = await api.managed(host)
  } catch (e) {
    report(e, `Could not read ${host}'s managed settings`)
  }
}

/** A setting's current value in the machine's managed file, or undefined. */
export function managedValue(host: string, path: string[]): unknown {
  let v: unknown = fleet.managed[host]?.settings
  for (const p of path) v = v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>)[p] : undefined
  return v
}

const list = (xs: string[]) => (xs.length <= 3 ? xs.join(', ') : `${xs.slice(0, 2).join(', ')} and ${xs.length - 2} more`)

async function write(host: string, fn: () => Promise<Managed>, done: string): Promise<boolean> {
  try {
    fleet.managed[host] = await fn()
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      toast(`Someone changed ${host}'s managed file at the same time. It has been reloaded; check it and try again.`, 'error', 'Edit not applied')
      await loadManaged(host)
    } else report(e, `Could not change ${host}`)
    return false
  }
  if (!fleet.pending.includes(host)) fleet.pending.push(host)
  toast(done, 'ok', undefined, { label: 'View changes', run: () => go('#/changes') })
  await refreshStatus() // the working copy changed: this also re-evaluates the model
  return true
}

export const addPackages = (host: string, attrs: string[]) => write(host, () => api.editPackages(host, { add: attrs }), `Added ${list(attrs)} to ${host}`)
export const removePackages = (host: string, attrs: string[]) =>
  write(host, () => api.editPackages(host, { remove: attrs }), `Removed ${list(attrs)} from ${host}`)
/** Remove a package the model knows by name (a pname) from the machine's managed file, by its attribute path there. */
export async function removePackageByName(host: string, name: string) {
  if (!fleet.managed[host]) await loadManaged(host)
  const attrs = fleet.managed[host]?.packages ?? []
  const attr = attrs.find((a) => a === name) ?? attrs.find((a) => a.split('.').pop() === name)
  if (!attr) return toast(`${name} is not in ${host}'s managed packages any more.`, 'error'), false
  return removePackages(host, [attr])
}
/** The hash of the managed file this tab last saw, so a stale form cannot overwrite a newer edit. */
async function seenHash(host: string): Promise<string | undefined> {
  if (!fleet.managed[host]) await loadManaged(host)
  return fleet.managed[host]?.hash
}
export const setOption = (host: string, path: string[], value: unknown) =>
  write(host, async () => api.editSetting(host, { path, value, base_hash: await seenHash(host) }), `Set ${pathName(path)} on ${host}`)
export const unsetOption = (host: string, path: string[]) =>
  write(host, async () => api.editSetting(host, { path, unset: true, base_hash: await seenHash(host) }), `Reset ${pathName(path)} on ${host}`)

/** `services` + `desktopManager.plasma6` → services.desktopManager.plasma6.enable */
export const enablePath = (group: FeatureGroup | string, name: string) => [group, ...fillPath(optionSegments(name), []), 'enable']

export const toggleFeature = (host: string, group: FeatureGroup, name: string, on: boolean) =>
  on ? setOption(host, enablePath(group, name), true) : unsetOption(host, enablePath(group, name))

/** `/nix/store/…-source/hosts/pi/default.nix:12:3` in an evaluation error → the repo file. */
export function errorFile(error: string): string | null {
  return /\/nix\/store\/[^/\s]+-source\/([^:\s]+)/.exec(error)?.[1] ?? null
}
