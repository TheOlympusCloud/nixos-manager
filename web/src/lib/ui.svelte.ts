import { ApiError } from './api'

// Toasts, the status-line message and the single modal dialog.

export type Toast = { id: number; kind: 'error' | 'info' | 'ok'; title?: string; text: string }
export const toasts = $state<Toast[]>([])
let nextToast = 1

export function toast(text: string, kind: Toast['kind'] = 'info', title?: string) {
  const id = nextToast++
  toasts.push({ id, kind, title, text })
  if (toasts.length > 4) toasts.shift()
  if (kind !== 'error') setTimeout(() => dismiss(id), 5000)
}

export function dismiss(id: number) {
  const i = toasts.findIndex((t) => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
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
