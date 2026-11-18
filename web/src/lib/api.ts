// Typed client for the nixos-manager HTTP API.

export type Info = { root: string; branch: string; author: string; remote: string }
export type TreeEntry = { path: string; dir: boolean }
export type FileData = { path: string; content: string; hash: string }
export type Letter = '' | 'M' | 'A' | 'D' | 'R' | '?'
export type StatusFile = { path: string; staged: Letter; unstaged: Letter; from?: string }
export type Status = {
  branch: string
  upstream: string
  ahead: number
  behind: number
  files: StatusFile[]
  staged_tree: string
  staged_checked: boolean
}
export type Commit = { sha: string; short: string; author: string; email: string; date: string; subject: string }
export type CommitDetail = { sha: string; author: string; email: string; date: string; message: string; diff: string }
export type Target = { kind: 'nixos' | 'home' | 'darwin'; name: string; attr: string }
export type JobState = 'queued' | 'running' | 'passed' | 'failed'
export type Job = {
  id: string
  kind: 'check' | 'lock' | 'update' | 'model' | 'options' | 'packages'
  state: JobState
  targets?: string[] | null
  tree?: string
  started?: string
  finished?: string
  output?: string
}
export type JobRequest =
  | { kind: 'check'; targets: string[] }
  | { kind: 'lock' }
  | { kind: 'update'; inputs: string[] }
export type Token = { id: string; name: string; created: string; last_used: string }

// ---- fleet model (what Nix evaluates, machine by machine) ----

export type FeatureGroup = 'services' | 'programs' | 'virtualisation' | 'hardware'
export type Package = { name: string; version: string; files: string[] }
export type Feature = { group: FeatureGroup; name: string; files: string[]; description: string }
export type User = { name: string; description: string; groups: string[]; home: { packages: string[]; programs: string[] } | null }
export type Host = {
  name: string
  hostName: string
  system: string
  release: string
  stateVersion: string
  timeZone: string | null
  kernel: string | null
  /** "hosts/<name>/managed.json" when the machine loads it, else null (visual editing off). */
  managedFile: string | null
  packages: Package[]
  features: Feature[]
  users: User[]
  ports: { tcp: number[]; udp: number[]; firewall: boolean }
  fileSystems: { mountPoint: string; fsType: string; device: string }[]
  files: string[]
  error?: string | null
}
export type Home = { name: string; packages: string[]; programs: string[]; error?: string | null }
export type Input = { name: string; type: string; url: string; rev: string; lastModified: number }
export type Model = { tree: string; hosts: Host[]; homes: Home[]; inputs: Input[]; error?: string }
export type ModelReply = { state: 'ready'; model: Model } | { state: 'computing'; job: string }

export type DiffStatus = 'added' | 'removed' | 'changed' | 'unchanged' | 'error'
export type HostDiff = {
  host: string
  status: DiffStatus
  error?: string
  packagesAdded?: string[]
  packagesRemoved?: string[]
  featuresAdded?: string[]
  featuresRemoved?: string[]
  usersAdded?: string[]
  usersRemoved?: string[]
  portsAdded?: string[]
  portsRemoved?: string[]
}
export type ModelDiffReply = { state: 'ready'; hosts?: HostDiff[]; error?: string } | { state: 'computing'; jobs: string[] }

/** The machine's app-owned file (hosts/<host>/managed.json). */
export type Managed = { host?: string; path: string; exists: boolean; hash: string; packages: string[]; settings: Record<string, unknown> }
export type SettingEdit = { path: string[]; value: unknown } | { path: string[]; unset: true }

export type PackageHit = { attr: string; pname: string; version: string; description: string }
export type OptionInfo = { name: string; type: string; description: string; default: string | null; example: string | null; readOnly: boolean }
export type SearchReply<T> = { state: 'ready'; results: T[] | null } | { state: 'indexing'; job: string }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

let unauthorized = () => {}
/** Called whenever a request (other than login itself) answers 401. */
export function onUnauthorized(fn: () => void) {
  unauthorized = fn
}

/** Pull a readable message out of an error response: `{"error": "..."}`, else the raw text. */
export async function errorMessage(res: Response): Promise<string> {
  const text = (await res.text().catch(() => '')).trim()
  try {
    const body = JSON.parse(text)
    if (body && typeof body.error === 'string' && body.error) return body.error
  } catch {
    // not JSON
  }
  return text || `${res.status} ${res.statusText}`.trim()
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin' }
  if (method !== 'GET') {
    // The server rejects non-JSON writes (CSRF guard), so always send a JSON body.
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(body ?? {})
  }
  let res: Response
  try {
    res = await fetch(path, init)
  } catch {
    throw new ApiError(0, 'Cannot reach the server')
  }
  if (res.status === 401 && path !== '/api/login') unauthorized()
  if (!res.ok) throw new ApiError(res.status, await errorMessage(res))
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function withQuery(path: string, params: Record<string, string | number | boolean | undefined>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') q.set(k, String(v))
  const s = q.toString()
  return s ? `${path}?${s}` : path
}

const get = <T>(path: string, params: Record<string, string | number | boolean | undefined> = {}) =>
  request<T>('GET', withQuery(path, params))
const post = <T = void>(path: string, body: unknown = {}) => request<T>('POST', path, body)

export const api = {
  login: (password: string) => post('/api/login', { password }),
  logout: () => post('/api/logout'),
  info: () => get<Info>('/api/info'),

  tree: () => get<{ entries: TreeEntry[] }>('/api/tree').then((r) => r.entries ?? []),
  file: (path: string) => get<FileData>('/api/file', { path }),
  saveFile: (path: string, content: string, base_hash: string) =>
    request<{ hash: string }>('PUT', '/api/file', { path, content, base_hash }),
  mkdir: (path: string) => post('/api/mkdir', { path }),
  move: (from: string, to: string) => post('/api/move', { from, to }),
  remove: (path: string) => post('/api/delete', { path }),
  format: (content: string) => post<{ content: string }>('/api/format', { content }).then((r) => r.content),

  status: () => get<Status>('/api/status'),
  diff: (staged: boolean, path?: string) => get<{ diff: string }>('/api/diff', { staged, path }).then((r) => r.diff),
  stage: (paths: string[]) => post('/api/stage', { paths }),
  unstage: (paths: string[]) => post('/api/unstage', { paths }),
  discard: (paths: string[]) => post('/api/discard', { paths }),
  commit: (message: string, force: boolean) => post<{ sha: string }>('/api/commit', { message, force }),
  log: (opts: { path?: string; limit?: number; rev?: string } = {}) =>
    get<{ commits: Commit[] }>('/api/log', opts).then((r) => r.commits ?? []),
  commitDetail: (rev: string) => get<CommitDetail>('/api/commit', { rev }),
  fileAt: (rev: string, path: string) => get<{ content: string }>('/api/file-at', { rev, path }).then((r) => r.content),
  restore: (path: string, rev: string, base_hash: string) => post('/api/restore', { path, rev, base_hash }),
  branches: () => get<{ current: string; branches: string[] }>('/api/branches'),
  createBranch: (name: string) => post('/api/branches', { name, from: '' }),
  switchBranch: (name: string) => post('/api/switch', { name }),
  push: (force = false) => post<{ output: string }>('/api/push', { force }).then((r) => r.output),
  pull: () => post<{ output: string }>('/api/pull').then((r) => r.output),

  targets: () => get<{ targets: Target[] }>('/api/targets').then((r) => r.targets ?? []),
  createJob: (job: JobRequest) => post<{ id: string }>('/api/jobs', job).then((r) => r.id),
  jobs: () => get<{ jobs: Job[] }>('/api/jobs').then((r) => r.jobs ?? []),
  job: (id: string) => get<Job>(`/api/jobs/${encodeURIComponent(id)}`),
  jobEventsUrl: (id: string) => `/api/jobs/${encodeURIComponent(id)}/events`,

  // These answer 202 while a job computes the result; the body's `state` says which.
  model: () => get<ModelReply>('/api/model', { at: 'working' }),
  modelDiff: () => get<ModelDiffReply>('/api/model/diff'),
  refreshModel: () => post<{ job: string }>('/api/model/refresh').then((r) => r.job),
  managed: (host: string) => get<Managed>(`/api/hosts/${encodeURIComponent(host)}/managed`),
  editPackages: (host: string, edit: { add?: string[]; remove?: string[] }) => post<Managed>(`/api/hosts/${encodeURIComponent(host)}/packages`, edit),
  editSetting: (host: string, edit: SettingEdit) => post<Managed>(`/api/hosts/${encodeURIComponent(host)}/settings`, edit),
  searchPackages: (q: string, limit = 50) => get<SearchReply<PackageHit>>('/api/search/packages', { q, limit }),
  searchOptions: (host: string, q: string, opts: { limit?: number; exact?: boolean } = {}) =>
    get<SearchReply<OptionInfo>>('/api/search/options', { host, q, limit: opts.limit, exact: opts.exact ? 1 : undefined }),

  tokens: () => get<{ tokens: Token[] }>('/api/tokens').then((r) => r.tokens ?? []),
  createToken: (name: string) => post<{ id: string; name: string; token: string }>('/api/tokens', { name }),
  revokeToken: (id: string) => request<void>('DELETE', `/api/tokens/${encodeURIComponent(id)}`, {}),
}
