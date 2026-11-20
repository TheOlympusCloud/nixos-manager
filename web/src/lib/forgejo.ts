// Settings → Forgejo: the remote, the deploy key, the connection test, the push webhook, and links to Forgejo.
import { request } from './api'

export type DeployKey = { public: string; fingerprint: string }
export type Delivery = { time: string; event: string; ok: boolean; result: string }
export type StatusDelivery = { time: string; commit: string; state: string; ok: boolean; result: string }
/** Only the public key is ever sent; ssh says which identity git uses. */
export type ForgejoState = {
  remote: string
  env_remote: boolean
  kind: '' | 'ssh' | 'https' | 'http' | 'local'
  web: string
  web_base: string
  ssh: 'env' | 'app' | 'home' | 'none'
  key: DeployKey | null
  host_key: string
  known_hosts: string
  hook_secret: string
  last_delivery: Delivery | null
  /** Commit statuses: a token is set (it is never sent), the address their links open, where they go, the last one sent. */
  status_token: boolean
  status_console: string
  status_api: string
  last_status: StatusDelivery | null
}
export type CheckResult = 'connected' | 'empty' | 'readonly' | 'unreachable' | 'auth' | 'not_found' | 'host_key_changed' | 'no_remote' | 'error'
/** next: what the working copy should do now (push to an empty remote, adopt its history, or nothing possible). */
export type Check = { result: CheckResult; message: string; detail?: string; branches: string[]; host_key?: string; next?: '' | 'push' | 'adopt' | 'unrelated' }

const post = <T>(path: string, body: unknown = {}) => request<T>('POST', path, body)

export const forgejoApi = {
  state: () => request<ForgejoState>('GET', '/api/forgejo'),
  setRemote: (url: string) => post<ForgejoState>('/api/forgejo/remote', { url }),
  setWeb: (url: string) => post<ForgejoState>('/api/forgejo/web', { url }),
  createKey: () => post<ForgejoState>('/api/forgejo/key'),
  test: () => post<Check>('/api/forgejo/test'),
  adopt: () => post<{ output: string }>('/api/forgejo/adopt').then((r) => r.output),
  /** An empty token removes it. */
  setStatus: (token: string, console: string) => post<ForgejoState>('/api/forgejo/status', { token, console }),
}

const segments = (p: string) => p.split('/').map(encodeURIComponent).join('/')

/** A commit's page on Forgejo, or '' without a web address. */
export const commitUrl = (web: string, sha: string) => (web && sha ? `${web}/commit/${encodeURIComponent(sha)}` : '')

/** A file on a branch, as Forgejo shows it. */
export const fileUrl = (web: string, branch: string, path: string) => (web && branch && path ? `${web}/src/branch/${segments(branch)}/${segments(path)}` : '')

/** Commits in the log of the current branch that the remote has: all but the `ahead` newest. */
export const onRemote = (index: number, upstream: string | undefined, ahead: number | undefined) => !!upstream && index >= (ahead ?? 0)

/** The webhook URL to paste into Forgejo, and why Forgejo may not reach it. */
export function hookTarget(origin: string): { url: string; warning: string } {
  const url = `${origin}/api/hooks/forgejo`
  const host = new URL(origin).hostname
  if (host === 'localhost' || host === '[::1]' || host.startsWith('127.'))
    return { url, warning: 'This address only works on this server itself. Open the console through the address Forgejo can reach (its LAN name or IP) and copy the URL from there.' }
  return { url, warning: '' }
}

/** How each test result reads, and its tone. */
export const checkTitle: Record<CheckResult, [string, 'ok' | 'warn' | 'err' | 'off']> = {
  connected: ['Connected', 'ok'],
  empty: ['Connected · the repository is empty', 'ok'],
  readonly: ['Connected, but read-only', 'warn'],
  unreachable: ['Host unreachable', 'err'],
  auth: ['Authentication refused', 'err'],
  not_found: ['Repository not found', 'err'],
  host_key_changed: ['Host key changed', 'err'],
  no_remote: ['No remote yet', 'off'],
  error: ['Cannot connect', 'err'],
}

/** The command that forgets a changed host key, for the host:port of an SSH remote. */
export function forgetHostKey(remote: string, knownHosts: string): string {
  let host = ''
  let port = ''
  const m = remote.includes('://') ? /^ssh:\/\/(?:[^@/]+@)?(\[[^\]]+\]|[^:/]+)(?::(\d+))?\//.exec(remote) : /^(?:[^@:/]+@)?([^:/]+):/.exec(remote)
  if (m) [host, port] = [m[1], m[2] ?? '']
  if (!host || !knownHosts) return ''
  const name = port && port !== '22' ? `[${host.replace(/^\[|\]$/g, '')}]:${port}` : host
  return `ssh-keygen -R '${name}' -f '${knownHosts}'`
}
