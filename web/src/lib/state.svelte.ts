import { tick } from 'svelte'
import type { Text } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'
import { ApiError, api, onUnauthorized, type Info, type Job, type JobRequest, type Status, type Target, type TreeEntry } from './api'
import { scheduleModel } from './fleet.svelte'
import { codeLink, go } from './router.svelte'
import { confirmBox, note, promptBox, report, toast } from './ui.svelte'
import { basename, isWithin, minimalChange } from './util'

type Base = { id: string; title: string }
export type FileTab = Base & { kind: 'file'; path: string; hash: string; initial: string; dirty: boolean; conflict: boolean }
/** `n` changes on every (re)open so the view remounts and fetches fresh data. */
export type ViewTab = Base & { n?: number } & (
    | { kind: 'diff'; staged: boolean; path?: string }
    | { kind: 'commit'; rev: string }
    | { kind: 'version' | 'compare'; path: string; rev: string; short: string }
    | { kind: 'job'; jobId: string }
  )
export type Tab = FileTab | ViewTab
/** What the Changes view shows next to the file lists. */
export type Inspect = { kind: 'diff'; staged: boolean; path?: string } | { kind: 'job'; id: string }

export const app = $state({
  needLogin: false,
  info: null as Info | null,
  status: null as Status | null,
  entries: [] as TreeEntry[],
  tabs: [] as Tab[],
  active: '',
  inspect: null as Inspect | null,
  jobs: [] as Job[],
  targets: null as Target[] | null,
  selectedTargets: JSON.parse(localStorage.getItem('nixmgr.targets') ?? '[]') as string[],
  logVersion: 0,
})

onUnauthorized(() => (app.needLogin = true))

// ---- refresh -------------------------------------------------------------

export async function refreshStatus() {
  try {
    app.status = await api.status()
  } catch (e) {
    return report(e)
  }
  scheduleModel() // every status refresh follows something that may have changed the working copy
}

export async function refreshTree() {
  try {
    app.entries = await api.tree()
  } catch (e) {
    report(e)
  }
}

export const refresh = () => Promise.all([refreshStatus(), refreshTree()])

/** Run an action, show its error, then refresh whatever it may have changed. Resolves to success. */
export async function act(fn: () => Promise<unknown>, after: { tree?: boolean; tabs?: boolean; log?: boolean } = {}) {
  let ok = true
  try {
    await fn()
  } catch (e) {
    report(e)
    ok = false
  }
  await Promise.all([refreshStatus(), after.tree && refreshTree(), after.tabs && syncOpenTabs()])
  if (after.log) app.logVersion++
  return ok
}

export async function init() {
  try {
    app.info = await api.info()
  } catch (e) {
    report(e)
    return
  }
  await Promise.all([refresh(), refreshJobs()])
}

export async function login(password: string) {
  await api.login(password)
  app.needLogin = false
  import('./lsp.svelte').then((m) => m.reconnectLsp())
  await init()
  await syncOpenTabs()
}

export async function logout() {
  const dirty = fileTabs().filter((t) => t.dirty)
  if (dirty.length && !(await confirmBox({ title: 'Log out', message: `Discard unsaved changes in ${dirty.map((t) => t.path).join(', ')}?`, ok: 'Discard and log out', danger: true })))
    return
  try {
    await api.logout()
  } catch (e) {
    return report(e)
  }
  app.tabs = []
  app.active = ''
  app.info = null
  app.needLogin = true
}

// ---- tabs ----------------------------------------------------------------

const editors = new Map<string, EditorView>()
const savedDocs = new Map<string, Text>()

export function attachEditor(tab: FileTab, view: EditorView) {
  editors.set(tab.path, view)
  savedDocs.set(tab.path, view.state.doc)
  tab.initial = '' // the editor owns the content from here on
}
export function detachEditor(path: string, view: EditorView) {
  if (editors.get(path) !== view) return
  editors.delete(path)
  savedDocs.delete(path)
}
export const editorFor = (path: string) => editors.get(path)

export function updateDirty(tab: FileTab) {
  const view = editors.get(tab.path)
  const saved = savedDocs.get(tab.path)
  tab.dirty = !!view && !!saved && !view.state.doc.eq(saved)
}

export const fileTabs = () => app.tabs.filter((t): t is FileTab => t.kind === 'file')
export const fileTab = (path: string) => fileTabs().find((t) => t.path === path)
export const activeTab = () => app.tabs.find((t) => t.id === app.active)
/** The file the user is looking at (an editor, or an old version of it). */
export function activePath() {
  const t = activeTab()
  return t && (t.kind === 'file' || t.kind === 'version' || t.kind === 'compare') ? t.path : ''
}

export async function openFile(path: string, focus = false) {
  const id = 'file:' + path
  if (!app.tabs.some((t) => t.id === id)) {
    let f
    try {
      f = await api.file(path)
    } catch (e) {
      return report(e)
    }
    if (!app.tabs.some((t) => t.id === id))
      app.tabs.push({ kind: 'file', id, title: basename(path), path, hash: f.hash, initial: f.content, dirty: false, conflict: false })
  }
  app.active = id
  if (focus) {
    await tick()
    editors.get(path)?.focus()
  }
}

let viewCounter = 0
export function openView(tab: ViewTab) {
  const full = { ...tab, n: ++viewCounter }
  const i = app.tabs.findIndex((t) => t.id === tab.id)
  if (i >= 0) app.tabs[i] = full
  else app.tabs.push(full)
  app.active = tab.id
}

function removeTab(id: string) {
  const i = app.tabs.findIndex((t) => t.id === id)
  if (i < 0) return
  app.tabs.splice(i, 1)
  if (app.active === id) app.active = app.tabs[Math.min(i, app.tabs.length - 1)]?.id ?? ''
}

export async function closeTab(id: string) {
  const tab = app.tabs.find((t) => t.id === id)
  if (tab?.kind === 'file' && tab.dirty) {
    const ok = await confirmBox({ title: 'Unsaved changes', message: `${tab.path} has unsaved changes. Close it and lose them?`, ok: 'Close without saving', danger: true })
    if (!ok) return
  }
  removeTab(id)
}

/** Replace the editor content (keeping the cursor where possible) and mark it saved. */
function setContent(tab: FileTab, content: string, hash: string) {
  const view = editors.get(tab.path)
  if (view) {
    const change = minimalChange(view.state.doc.toString(), content)
    if (change) view.dispatch({ changes: change })
    savedDocs.set(tab.path, view.state.doc)
  } else tab.initial = content
  tab.hash = hash
  tab.dirty = false
  tab.conflict = false
}

const saving = new Set<string>()

export async function saveTab(tab: FileTab, overwrite = false) {
  const view = editors.get(tab.path)
  // A second save racing the first would send a stale base_hash and report a false conflict.
  if (!view || saving.has(tab.path)) return
  saving.add(tab.path)
  const doc = view.state.doc
  try {
    let base = tab.hash
    if (overwrite) base = await api.file(tab.path).then((f) => f.hash, (e) => (e instanceof ApiError && e.status === 404 ? '' : Promise.reject(e)))
    tab.hash = (await api.saveFile(tab.path, doc.toString(), base)).hash
    savedDocs.set(tab.path, doc)
    tab.conflict = false
    updateDirty(tab)
    note(`Saved ${tab.path}`)
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) tab.conflict = true
    else report(e, `Saving ${tab.path} failed`)
  } finally {
    saving.delete(tab.path)
  }
  await Promise.all([refreshStatus(), overwrite && refreshTree()])
}

export async function reloadTab(tab: FileTab) {
  try {
    const f = await api.file(tab.path)
    setContent(tab, f.content, f.hash)
  } catch (e) {
    report(e, `Reloading ${tab.path} failed`)
  }
}

export async function formatTab(tab: FileTab) {
  const view = editors.get(tab.path)
  if (!view) return
  const before = view.state.doc
  try {
    const out = await api.format(before.toString())
    if (!view.state.doc.eq(before)) return toast('The file changed while formatting; try again.')
    const change = minimalChange(before.toString(), out)
    if (change) view.dispatch({ changes: change })
    note(change ? `Formatted ${tab.path}` : 'Already formatted')
  } catch (e) {
    report(e, e instanceof ApiError && e.status === 422 ? 'nixfmt could not format the file' : 'Format failed')
  }
}

/** After git or other outside changes: reload clean tabs, flag dirty ones, drop vanished files. */
export async function syncOpenTabs() {
  await Promise.all(
    fileTabs().map(async (tab) => {
      try {
        const f = await api.file(tab.path)
        if (f.hash === tab.hash) return
        if (tab.dirty) tab.conflict = true
        else setContent(tab, f.content, f.hash)
      } catch (e) {
        if (!(e instanceof ApiError && e.status === 404)) return report(e)
        if (tab.dirty) tab.conflict = true
        else removeTab(tab.id)
      }
    }),
  )
}

// ---- files ---------------------------------------------------------------

const cleanPath = (p: string | null) => (p ?? '').trim().replace(/^\/+|\/+$/g, '')

export async function newFile(folder: string) {
  const path = cleanPath(await promptBox({ title: 'New file', input: { label: 'Path', value: folder ? folder + '/' : '' }, ok: 'Create' }))
  if (path && (await act(() => api.saveFile(path, '', ''), { tree: true }))) await openFile(path, true)
}

export async function newFolder(folder: string) {
  const path = cleanPath(await promptBox({ title: 'New folder', input: { label: 'Path', value: folder ? folder + '/' : '' }, ok: 'Create' }))
  if (path) await act(() => api.mkdir(path), { tree: true })
}

export async function movePath(from: string) {
  const affected = fileTabs().filter((t) => isWithin(t.path, from))
  const dirty = affected.find((t) => t.dirty)
  if (dirty) return toast(`Save or close ${dirty.path} before moving it.`, 'error')
  const to = cleanPath(await promptBox({ title: 'Rename or move', input: { label: 'New path', value: from }, ok: 'Move' }))
  if (!to || to === from) return
  if (!(await act(() => api.move(from, to), { tree: true }))) return
  const wasActive = app.active
  for (const t of affected) removeTab(t.id)
  for (const t of affected) await openFile(to + t.path.slice(from.length))
  const moved = affected.find((t) => t.id === wasActive)
  if (moved) app.active = 'file:' + to + moved.path.slice(from.length)
}

export async function deletePath(path: string, dir: boolean) {
  const affected = fileTabs().filter((t) => isWithin(t.path, path))
  const unsaved = affected.some((t) => t.dirty) ? ' Unsaved changes in open tabs will be lost.' : ''
  const ok = await confirmBox({
    title: dir ? 'Delete folder' : 'Delete file',
    message: `Delete ${path}${dir ? ' and everything in it' : ''}?${unsaved}`,
    ok: 'Delete',
    danger: true,
  })
  if (ok && (await act(() => api.remove(path), { tree: true }))) for (const t of affected) removeTab(t.id)
}

export async function restoreVersion(path: string, rev: string, short: string) {
  const tab = fileTab(path)
  const ok = await confirmBox({
    title: 'Restore this version',
    message: `Replace ${path} in the working copy with its version from ${short}?${tab?.dirty ? ' Unsaved edits in the open tab will be lost.' : ''} The result is an uncommitted change.`,
    ok: 'Restore',
    danger: true,
  })
  // The server only replaces the version we saw: the open tab's, or what is on disk now ('' if absent).
  const seen = async (): Promise<string> => {
    if (tab) return tab.hash
    try {
      return (await api.file(path)).hash
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return ''
      throw e
    }
  }
  if (!ok || !(await act(async () => api.restore(path, rev, await seen()), { tree: true }))) return
  if (tab) await reloadTab(tab)
  await openFile(path)
  go(codeLink(path))
  note(`Restored ${path} from ${short}`)
}

// ---- jobs ----------------------------------------------------------------

let jobTimer: ReturnType<typeof setTimeout> | undefined
const isActive = (state: string | undefined) => state === 'queued' || state === 'running'

const seenStates = new Map<string, string>()

export async function refreshJobs() {
  clearTimeout(jobTimer)
  try {
    app.jobs = await api.jobs()
  } catch (e) {
    return report(e)
  }
  // Compare with what any earlier call saw, so overlapping refreshes announce a finish only once.
  const finished = app.jobs.filter((j) => isActive(seenStates.get(j.id)) && !isActive(j.state))
  for (const j of app.jobs) seenStates.set(j.id, j.state)
  // Model and search-index jobs report through their own views.
  for (const j of finished.filter((j) => j.kind in jobDone)) {
    const log = { label: 'Show log', run: () => showJob(j.id, j.kind) }
    const ok = j.state === 'passed'
    const more = ok && j.kind === 'update' ? [{ label: 'See what changed', run: () => go('#/inputs') }, log] : log
    toast(jobDone[j.kind][ok ? 0 : 1], ok ? 'ok' : 'error', undefined, more, 'job:' + j.id)
  }
  if (finished.length) await Promise.all([refresh(), syncOpenTabs()])
  if (app.jobs.some((j) => isActive(j.state))) jobTimer = setTimeout(refreshJobs, 2000)
}

const jobRunning: Record<string, string> = { check: 'Running the checks…', lock: 'Locking the flake inputs…', update: 'Updating the flake inputs…' }
const jobDone: Record<string, [string, string]> = {
  check: ['Checks passed', 'Checks failed'],
  lock: ['Flake inputs locked', 'Locking the flake inputs failed'],
  update: ['Flake inputs updated', 'Updating the flake inputs failed'],
}

/** Show a job's log: checks in Changes, input updates and locks in Inputs. */
export function showJob(id: string, kind = 'check') {
  app.inspect = { kind: 'job', id }
  go(kind === 'check' ? '#/changes' : '#/inputs')
}

/** Start a job; one toast follows it from start to finish. Resolves to the job id, '' on failure. */
export async function startJob(req: JobRequest): Promise<string> {
  try {
    const id = await api.createJob(req)
    app.inspect = { kind: 'job', id }
    toast(jobRunning[req.kind], 'busy', undefined, { label: 'Show log', run: () => showJob(id, req.kind) }, 'job:' + id)
    await refreshJobs()
    return id
  } catch (e) {
    report(e, `Could not start the ${req.kind === 'check' ? 'checks' : req.kind}`)
    return ''
  }
}

export const checkStaged = () => startJob({ kind: 'check', targets: [...app.selectedTargets] })
