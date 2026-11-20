// The model comparison (working copy, one commit or what a push changes), waiting while either side evaluates.
import { api, type ModelDiffReply } from './api'
import { waitJob } from './fleet.svelte'
import { settingChanges, type SettingChange } from './impact'

export type ModelDiff = Extract<ModelDiffReply, { state: 'ready' }>

/** Resolves with the comparison, or null when the caller lost interest (`alive` turned false). */
export async function fetchModelDiff(which: Parameters<typeof api.modelDiff>[0], onComputing: () => void, alive: () => boolean): Promise<ModelDiff | null> {
  for (let tries = 0; tries < 6 && alive(); tries++) {
    const r = await api.modelDiff(which)
    if (r.state === 'ready') return alive() ? r : null
    onComputing()
    const states = await Promise.all(r.jobs.map(waitJob))
    if (states.some((s) => !s)) await new Promise((res) => setTimeout(res, 1500)) // lost a stream: do not spin
  }
  return null
}

const MANAGED = /^hosts\/([^/]+)\/managed\.json$/
/** The managed.json files among `paths`. */
export const managedFiles = (paths: string[]) => paths.filter((p) => MANAGED.test(p))

/**
 * What each changed managed.json sets differently, per machine: `before` is a revision,
 * `after` a revision or null for the working copy. A missing or unreadable file counts as empty.
 */
export async function managedSettings(
  paths: string[],
  before: string,
  after: string | null,
  /** Files known to be absent on one side (new or deleted), so they are not asked for. */
  absent: { before?: string[]; after?: string[] } = {},
): Promise<Record<string, SettingChange[]>> {
  const settingsOf = (text: string) => {
    try {
      return (JSON.parse(text) as { settings?: unknown }).settings ?? {}
    } catch {
      return {}
    }
  }
  const read = (rev: string | null, path: string) => (rev ? api.fileAt(rev, path) : api.file(path).then((f) => f.content)).catch(() => '{}')
  const out: Record<string, SettingChange[]> = {}
  await Promise.all(
    managedFiles(paths).map(async (path) => {
      const [a, b] = await Promise.all([absent.before?.includes(path) ? '{}' : read(before, path), absent.after?.includes(path) ? '{}' : read(after, path)])
      const changes = settingChanges(settingsOf(a), settingsOf(b))
      if (changes.length) out[MANAGED.exec(path)![1]] = changes
    }),
  )
  return out
}
