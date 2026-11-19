import { ApiError } from './api'

// Toasts, the status-line message and the single modal dialog.

export type ToastAction = { label: string; run: () => void }
export type Toast = { id: number; kind: 'error' | 'info' | 'ok' | 'busy'; title?: string; text: string; actions: ToastAction[]; key?: string }
export const toasts = $state<Toast[]>([])
let nextToast = 1
const timers = new Map<number, ReturnType<typeof setTimeout>>()
let quiet = 0

/**
 * Show a toast. A toast with the `key` of one on screen replaces it in place (one toast per job).
 * Errors stay until dismissed; "busy" toasts until replaced.
 */
export function toast(text: string, kind: Toast['kind'] = 'info', title?: string, action?: ToastAction | ToastAction[], key?: string) {
  if (quiet && kind !== 'error') return
  const actions = action ? [action].flat() : []
  const old = key ? toasts.find((t) => t.key === key) : undefined
  const id = old?.id ?? nextToast++
  const t: Toast = { id, kind, title, text, actions, key }
  if (old) toasts[toasts.indexOf(old)] = t
  else {
    toasts.push(t)
    if (toasts.length > 4) toasts.splice(Math.max(0, toasts.findIndex((x) => x.kind !== 'busy')), 1)
  }
  clearTimeout(timers.get(id))
  if (kind !== 'error' && kind !== 'busy') timers.set(id, setTimeout(() => dismiss(id), actions.length ? 8000 : 5000))
}

export function dismiss(id: number) {
  clearTimeout(timers.get(id))
  timers.delete(id)
  const i = toasts.findIndex((t) => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
}

/** Run a batch of edits without their one-toast-each confirmations (errors still show). */
export async function quietly<T>(fn: () => Promise<T>): Promise<T> {
  quiet++
  try {
    return await fn()
  } finally {
    quiet--
  }
}

/** Show an API (or other) error. 401s are handled by the login screen instead. */
export function report(err: unknown, title?: string) {
  if (err instanceof ApiError && err.status === 401) return
  toast(err instanceof Error ? err.message : String(err), 'error', title)
}

export const statusLine = $state({ text: '' })
let statusTimer: ReturnType<typeof setTimeout> | undefined
export function note(text: string) {
  statusLine.text = text
  clearTimeout(statusTimer)
  statusTimer = setTimeout(() => (statusLine.text = ''), 4000)
}

export type DialogSpec = {
  title: string
  message?: string
  /** Monospace block, e.g. git output. */
  pre?: string
  ok?: string
  cancel?: string | null
  danger?: boolean
  /** When set the dialog asks for a value. */
  input?: { label: string; value: string }
}
export const dialog = $state<{ spec: DialogSpec | null; resolve: (v: string | null) => void }>({
  spec: null,
  resolve: () => {},
})

function open(spec: DialogSpec): Promise<string | null> {
  dialog.resolve(null) // a newer dialog cancels the previous one
  return new Promise((resolve) => {
    dialog.spec = spec
    dialog.resolve = (v) => {
      dialog.spec = null
      dialog.resolve = () => {}
      resolve(v)
    }
  })
}

export const confirmBox = async (spec: DialogSpec) => (await open(spec)) !== null
export const promptBox = (spec: DialogSpec & { input: { label: string; value: string } }) => open(spec)
export const alertBox = async (spec: DialogSpec) => void (await open({ cancel: null, ok: 'Close', ...spec }))
