// The evaluated fleet (stale while a new evaluation runs) and editing by clicking.
import { ApiError, api, type FeatureGroup, type Host, type MachineCreated, type MachineSpec, type Managed, type OptionValues, type PackageEdit, type PackageOverride, type SearchReply } from './api'
import { fillPath, optionSegments, pathName } from './options'
import { codeLink, go } from './router.svelte'
import { refresh, refreshStatus } from './state.svelte'
import { confirmBox, report, toast, type ToastAction } from './ui.svelte'

export const fleet = $state({
  model: null as import('./api').Model | null,
  /** A newer evaluation is running; `model` is the last one that finished. */
  computing: false,
  job: '',
  loadError: '',
  /** Machines edited since `model` was evaluated. */
  pending: [] as string[],
  managed: {} as Record<string, Managed>,
  /** Machines created by the wizard in this session, until the evaluation lists them. */
  created: {} as Record<string, MachineCreated>,
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

const loadingManaged = new Map<string, Promise<void>>()
/** Read a machine's managed file; callers asking while a read is in flight share it. */
export function loadManaged(host: string): Promise<void> {
  let p = loadingManaged.get(host)
  if (!p) {
    p = api.managed(host).then(
      (m) => {
        fleet.managed[host] = m
      },
      (e) => report(e, `Could not read ${host}'s managed settings`),
    )
    loadingManaged.set(host, p)
    p.finally(() => loadingManaged.delete(host))
  }
  return p
}

/** A setting's current value in the machine's managed file, or undefined. */
export function managedValue(host: string, path: string[]): unknown {
  let v: unknown = fleet.managed[host]?.settings
  for (const p of path) v = v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>)[p] : undefined
  return v
}

const list = (xs: string[]) => (xs.length <= 3 ? xs.join(', ') : `${xs.slice(0, 2).join(', ')} and ${xs.length - 2} more`)

const writing = new Map<string, Promise<boolean>>()
/** One write per machine at a time, so each starts from the hash the previous one left. */
function write(host: string, fn: () => Promise<Managed>, done: string, action?: ToastAction): Promise<boolean> {
  const next = (writing.get(host) ?? Promise.resolve(true)).then(() => writeNow(host, fn, done, action))
  writing.set(host, next)
  return next
}

async function writeNow(host: string, fn: () => Promise<Managed>, done: string, action?: ToastAction): Promise<boolean> {
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
  toast(done, 'ok', undefined, action ?? { label: 'View changes', run: () => go('#/changes') })
  await refreshStatus() // the working copy changed: this also re-evaluates the model
  return true
}

/** Adding a removed name clears its removal (EditPackages), so its other copies come back: say so. */
function added(host: string, attrs: string[], user?: string) {
  const removed = user ? fleet.managed[host]?.home?.[user]?.removedPackages : fleet.managed[host]?.removedPackages
  const back = attrs.filter((a) => removed?.includes(a))
  const whose = user ? `${user}'s home on ${host}` : host
  return `Added ${list(attrs)} to ${whose}` + (back.length ? `. ${list(back)} ${back.length > 1 ? 'are' : 'is'} no longer removed by name, so what else adds ${back.length > 1 ? 'them' : 'it'} comes back too` : '')
}
export const addPackages = (host: string, attrs: string[]) => write(host, () => api.editPackages(host, { add: attrs }), added(host, attrs))
/** Removing is one click, so the toast offers to put the packages back. */
export const removePackages = (host: string, attrs: string[]) =>
  write(host, () => api.editPackages(host, { remove: attrs }), `Removed ${list(attrs)} from ${host}`, { label: 'Undo', run: () => addPackages(host, attrs) })

/** The managed.json attribute that installs a package the model knows by name: nodejs → nodejs_22. */
export function managedAttr(host: string, name: string): string | undefined {
  const attrs = fleet.managed[host]?.packages ?? []
  const byModel = Object.entries(hostByName(host)?.managedPackages ?? {}).find(([a, pname]) => pname === name && attrs.includes(a))?.[0]
  return attrs.find((a) => a === name) ?? byModel ?? attrs.find((a) => a.split('.').pop() === name)
}
/** Remove a package the model knows by name (a pname) from the machine's managed file, by its attribute there. */
export async function removePackageByName(host: string, name: string) {
  if (!fleet.managed[host]) await loadManaged(host)
  const attr = managedAttr(host, name)
  if (!attr) return toast(`${name} is not in ${host}'s managed packages any more.`, 'error'), false
  return removePackages(host, [attr])
}
/** Where a package edit lands: the machine, or a home-manager user on it. */
const where = (host: string, user?: string) => (user ? `${user}'s home on ${host}` : host)
/** One write for any package edit; done is the toast. */
export const editPackages = (host: string, edit: PackageEdit, done: string, action?: ToastAction) => write(host, () => api.editPackages(host, edit), done, action)
/** Takes packages (by name) out of the machine, or a user's home, wherever they are added. */
export const excludePackages = (host: string, names: string[], user?: string): Promise<boolean> =>
  editPackages(host, { user, exclude: names }, `Removed ${list(names)} from ${where(host, user)}`, { label: 'Undo', run: () => restorePackages(host, names, user) })
/** Gives back packages taken out by name: what NixOS or the Nix files add comes back. */
export const restorePackages = (host: string, names: string[], user?: string): Promise<boolean> =>
  editPackages(host, { user, restore: names }, `Put ${list(names)} back on ${where(host, user)}`)
export const addHomePackages = (host: string, user: string, attrs: string[]) => editPackages(host, { user, add: attrs }, added(host, attrs, user))
export const removeHomePackages = (host: string, user: string, attrs: string[]) =>
  editPackages(host, { user, remove: attrs }, `Removed ${list(attrs)} from ${where(host, user)}`, { label: 'Undo', run: () => addHomePackages(host, user, attrs) })
/** How a package is changed on the machine; null puts it back as nixpkgs has it. */
export const setPackageOverride = (host: string, attr: string, o: PackageOverride | null) =>
  write(host, async () => api.setPackageOverride(host, attr, o, await seenHash(host)), o ? `Changed how ${attr} is built on ${host}` : `${attr} is back to nixpkgs' own on ${host}`)
/** Turns home-manager on (stateVersion for a user without it yet) or off for a user. */
export const setHome = (host: string, user: string, enable: boolean, stateVersion?: string) =>
  write(host, () => api.editHome(host, { user, enable, stateVersion }), enable ? `home-manager is on for ${user} on ${host}` : `${user}'s home on ${host} is no longer set here`)

/**
 * Opens a file next to the machine's managed.json in Code, written from
 * `template` first when it does not exist yet. Resolves false when it could not.
 */
export async function openOrCreate(path: string, template: string): Promise<boolean> {
  try {
    await api.file(path)
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 404)) return report(e, `Could not open ${path}`), false
    try {
      await api.saveFile(path, template, '')
    } catch (e) {
      return report(e, `Could not create ${path}`), false
    }
    await refresh()
    toast(`Created ${path}`, 'ok', undefined, { label: 'View changes', run: () => go('#/changes') })
  }
  go(codeLink(path))
  return true
}

/**
 * Writes this server's modules/managed.nix over the repository's (an older one lacks
 * features), or creates it, once confirmed.
 */
export async function updateManagedModule(): Promise<boolean> {
  const path = 'modules/managed.nix'
  let base: string
  try {
    base = await api.file(path).then(
      (f) => f.hash,
      (e) => (e instanceof ApiError && e.status === 404 ? '' : Promise.reject(e)),
    )
  } catch (e) {
    return report(e, `Could not read ${path}`), false
  }
  const ok = await confirmBox(
    base
      ? {
          title: `Update ${path}?`,
          message:
            "It is replaced with this nixos-manager's version, which also applies removed and changed packages, home-manager users and the Nix files next to managed.json. Changes made to it by hand are replaced too: the diff shows in Changes before you commit.",
          ok: 'Update it',
        }
      : { title: `Create ${path}?`, message: "It is this nixos-manager's module that applies each machine's managed.json. Review it in Changes before you commit.", ok: 'Create it' },
  )
  if (!ok) return false
  try {
    await api.updateManagedModule(base)
  } catch (e) {
    return report(e, `Could not write ${path}`), false
  }
  toast(`${base ? 'Updated' : 'Created'} ${path}: review it in Changes`, 'ok', undefined, { label: 'View changes', run: () => go('#/changes') })
  await refresh()
  return true
}

/** The hash of the managed file this tab last saw, so a stale form cannot overwrite a newer edit. */
async function seenHash(host: string): Promise<string> {
  if (!fleet.managed[host]) await loadManaged(host)
  const hash = fleet.managed[host]?.hash
  // Without it the server could not refuse to overwrite an edit made elsewhere.
  if (hash === undefined) throw new Error(`${host}'s managed file could not be read, so nothing was changed. Reload and try again.`)
  return hash
}
/** `done`: the confirmation in words ("Opened port 8080 on pi"); by default the option's name. */
export const setOption = (host: string, path: string[], value: unknown, done = `Set ${pathName(path)} on ${host}`) =>
  write(host, async () => api.editSetting(host, { path, value, base_hash: await seenHash(host) }), done)
export const unsetOption = (host: string, path: string[], done = `Reset ${pathName(path)} on ${host}`) =>
  write(host, async () => api.editSetting(host, { path, unset: true, base_hash: await seenHash(host) }), done)

/** `services` + `desktopManager.plasma6` → services.desktopManager.plasma6.enable */
export const enablePath = (group: FeatureGroup | string, name: string) => [group, ...fillPath(optionSegments(name), []), 'enable']

export const toggleFeature = (host: string, group: FeatureGroup, name: string, on: boolean) =>
  on ? setOption(host, enablePath(group, name), true) : unsetOption(host, enablePath(group, name))

/** `/nix/store/…-source/hosts/pi/default.nix:12:3` in an evaluation error → the repo file. */
export function errorFile(error: string): string | null {
  return /\/nix\/store\/[^/\s]+-source\/([^:\s]+)/.exec(error)?.[1] ?? null
}

// ---- effective values ----

const valueCache = new Map<string, Promise<OptionValues[]>>()

async function jobProblem(id: string): Promise<string> {
  const out = await api.job(id).then((j) => j.output ?? '', () => '')
  return out.trim().split('\n').filter(Boolean).at(-1) ?? ''
}

/**
 * Effective values of options (full names) on these machines (all when empty), once computed.
 * Callers asking the same of the same evaluation share one request.
 */
export function optionValues(names: string[], hosts: string[] = [], onJob?: (job: string) => void): Promise<OptionValues[]> {
  const key = [fleet.model?.tree ?? '', [...names].sort().join('\0'), [...hosts].sort().join(',')].join('|')
  let p = valueCache.get(key)
  if (!p) {
    p = (async () => {
      for (let tries = 0; tries < 5; tries++) {
        const r = await api.optionValues(names, hosts)
        if (r.state === 'ready') return r.values
        onJob?.(r.job)
        const state = await waitJob(r.job)
        if (state === 'failed') throw new Error((await jobProblem(r.job)) || 'Nix could not compute the values.')
        if (!state) await new Promise((r) => setTimeout(r, 1500)) // lost the stream: do not spin
      }
      throw new Error('The values are still being computed; try again in a moment.')
    })()
    valueCache.set(key, p)
    p.catch(() => valueCache.delete(key))
    if (valueCache.size > 40) valueCache.delete(valueCache.keys().next().value!)
  }
  return p
}

/**
 * Values for a component, fetched again when the question or the evaluation changes.
 * Call during component setup; `query` returning null means "nothing to ask".
 */
export function useValues(query: () => { names: string[]; hosts?: string[] } | null) {
  const s = $state({ values: null as OptionValues[] | null, job: '', error: '', loading: false })
  let lastKey = ''
  $effect(() => {
    const q = query()
    void fleet.model?.tree // a new evaluation: new values
    const key = q ? JSON.stringify([q.names, q.hosts ?? []]) : ''
    if (key !== lastKey) s.values = null // another question: old answers would mislead
    lastKey = key
    s.error = ''
    if (!q || !q.names.length) return void (s.loading = false)
    let alive = true
    s.loading = true
    optionValues(q.names, q.hosts ?? [], (job) => alive && (s.job = job)).then(
      (v) => alive && Object.assign(s, { values: v, job: '', loading: false }),
      (e) => alive && Object.assign(s, { error: e instanceof Error ? e.message : String(e), job: '', loading: false }),
    )
    return () => (alive = false)
  })
  return s
}

/** One machine's answer for one option. */
export const valueOf = (values: OptionValues[] | null, name: string, host: string) => values?.find((v) => v.name === name)?.hosts.find((h) => h.host === host)

// ---- new machines ----

export async function createMachine(spec: MachineSpec): Promise<MachineCreated | null> {
  try {
    const res = await api.createMachine(spec)
    fleet.created[res.name] = res
    await refresh() // new files: the tree, the status, and a new evaluation
    return res
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) toast(`A machine called ${spec.name} already exists in hosts/.`, 'error', 'Not created')
    else report(e, 'Could not create the machine')
    return null
  }
}
