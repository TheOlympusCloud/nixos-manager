// Typed client for the nixos-manager HTTP API.

/** web: the repository's page on Forgejo, or '' when the remote has none. */
export type Info = { root: string; branch: string; author: string; remote: string; web: string }
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
  /** The check the staged tree passed: the tree of what is pushed it compared with ('' for none), and the machines it built. */
  staged_check?: CheckPass
}
export type CheckPass = { base: string; built: string[] }
/** What a check verified; Nix's figures come from a dry run before the builds. */
export type CheckSummary = {
  base: string
  changed: string[]
  built: string[]
  failed?: string[]
  reboot?: string[]
  toBuild: number
  toFetch: number
  download?: string
  unpacked?: string
}
export type Commit = { sha: string; short: string; author: string; email: string; date: string; subject: string }
export type CommitDetail = { sha: string; author: string; email: string; date: string; message: string; diff: string }
export type Target = { kind: 'nixos' | 'home' | 'darwin'; name: string; attr: string }
export type JobState = 'queued' | 'running' | 'passed' | 'failed'
export type Job = {
  id: string
  /** update-branch: nix flake update for a scheduled update, never in the working copy. */
  kind: 'check' | 'lock' | 'update' | 'update-branch' | 'model' | 'options' | 'packages'
  state: JobState
  targets?: string[] | null
  tree?: string
  /** A check of this commit, not of the staged changes; branch: where a push brought it. */
  commit?: string
  branch?: string
  started?: string
  finished?: string
  summary?: CheckSummary
  output?: string
}
export type JobRequest =
  | { kind: 'check'; targets: string[] }
  | { kind: 'check'; commit: string }
  | { kind: 'lock' }
  | { kind: 'update'; inputs: string[] }
export type TokenScope = 'read' | 'edit' | 'full'
export type Token = { id: string; name: string; scope: TokenScope; created: string; expires?: string; last_used: string }

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
  /** managed.json attribute -> package name (nodejs_22 -> nodejs), to remove a model package by attribute. */
  managedPackages?: Record<string, string>
  packages: Package[]
  features: Feature[]
  users: User[]
  ports: { tcp: number[]; udp: number[]; firewall: boolean }
  fileSystems: { mountPoint: string; fsType: string; device: string }[]
  files: string[]
  /** Security and maintenance facts the fleet insights look at. */
  checks?: Checks
  /** The sops-nix secrets it decrypts, never their values; null (or absent) without sops-nix. */
  secrets?: HostSecret[] | null
  error?: string | null
}
/** One of sops.secrets: where the machine finds it and the sops file it comes from ("nixos" outside the flake). */
export type HostSecret = { name: string; path: string; file: string; owner: string; group: string; mode: string }
export type Checks = {
  ssh: boolean
  /** null: the NixOS default, which allows passwords. */
  sshPasswordAuthentication: boolean | null
  /** Authorized SSH keys of normal users; null: unknown. */
  sshUserKeys: number | null
  sshPermitRootLogin: string | null
  fail2ban: boolean
  autoGC: boolean
  autoOptimise: boolean
  zram: boolean
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
  /** Packages kept but at another version, e.g. after a nixpkgs update. */
  versionsChanged?: VersionChange[]
  kernel?: VersionChange
  /** The kernel, its modules, the initrd or the kernel parameters change: only a reboot applies them. */
  reboot?: boolean
  /** What switching does to each changed unit, if it runs. */
  units?: UnitChange[]
  etc?: EtcChange[]
  /** Only when the server has built both systems. */
  closure?: ClosureDiff
}
export type VersionChange = { name: string; from: string; to: string }
/** keep: changed, but switching leaves it running the old version until it is restarted or the machine reboots. */
export type UnitChange = { name: string; action: 'restart' | 'reload' | 'start' | 'stop' | 'keep' }
export type EtcChange = { path: string; status: 'added' | 'removed' | 'changed' }
/** One line of nix store diff-closures: versions before and after ("" or absent: not there), size change in bytes. */
export type ClosureChange = { name: string; from?: string; to?: string; delta?: number }
export type ClosureDiff = { sizeFrom: number; sizeTo: number; changes: ClosureChange[] }
/** A flake input added, removed or moved to another revision; compareUrl for GitHub inputs. */
export type InputChange = {
  name: string
  status: 'added' | 'removed' | 'updated'
  fromRev?: string
  toRev?: string
  fromDate?: number
  toDate?: number
  compareUrl?: string
}
export type ModelDiffReply =
  | { state: 'ready'; hosts?: HostDiff[]; inputs?: InputChange[]; error?: string; headError?: string }
  | { state: 'computing'; jobs: string[] }

/** A health or consistency finding about the fleet. */
export type Insight = {
  id: string
  severity: 'error' | 'warning' | 'info'
  title: string
  detail: string
  hosts: string[]
  /** settings: managed.json values that resolve it on each affected machine; updateInput: a flake input to update; check: a commit to check. */
  fix?: { label: string; settings?: { path: string[]; value: unknown }[]; updateInput?: string; check?: string }
  /** A console route with more about it, e.g. #/deployments. */
  link?: string
}
export type InsightsReply = { state: 'ready'; insights: Insight[] } | { state: 'computing'; job: string }

/** An option's effective value on one machine. files: repo files that define it ("nixos" = NixOS or an input). */
export type HostValue = { host: string; exists: boolean; value: unknown; files: string[]; type: string; error?: string }
export type OptionValues = { name: string; path: string[]; hosts: HostValue[] }
export type ValuesReply = { state: 'ready'; values: OptionValues[] } | { state: 'computing'; job: string }

export type MachineRole = 'minimal' | 'server' | 'desktop'
export type MachinePlatform = 'hardware' | 'proxmox-lxc' | 'proxmox-vm'
/** A disko disk layout, for a machine nixos-anywhere installs. */
export type MachineDisk = 'ext4' | 'btrfs'
/** disk and device go together: the layout and the disk nixos-anywhere erases (then hardware stays empty). */
export type MachineSpec = {
  name: string
  system: 'x86_64-linux' | 'aarch64-linux'
  role: MachineRole
  hardware?: string
  platform?: MachinePlatform
  disk?: MachineDisk
  device?: string
}
/** registered: the flake picks hosts/<name>/ up by itself; otherwise snippet is the line to add to nixosConfigurations. inputSnippet: the disko input flake.nix lacks. */
export type MachineCreated = { name: string; files: string[]; registered: boolean; snippet?: string; inputSnippet?: string }
/** An SSH ed25519 host key and its SHA256 fingerprint. */
export type HostKey = { key: string; fingerprint: string }

/** The machine's app-owned file (hosts/<host>/managed.json). */
export type Managed = { host?: string; path: string; exists: boolean; hash: string; packages: string[]; settings: Record<string, unknown> }
/** base_hash: the managed file the edit was based on; the server answers 409 if it changed since. */
export type SettingEdit = ({ path: string[]; value: unknown } | { path: string[]; unset: true }) & { base_hash?: string }

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

type QueryValue = string | number | boolean | undefined | string[]

/** Arrays become repeated parameters (name=a&name=b); empty values are left out. */
export function withQuery(path: string, params: Record<string, QueryValue>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (Array.isArray(v)) for (const item of v) q.append(k, item)
    else if (v !== undefined && v !== '') q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `${path}?${s}` : path
}

const get = <T>(path: string, params: Record<string, QueryValue> = {}) =>
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
  /** The last commit against the working copy, or with rev: that commit against its parent. */
  /** The working copy against the last commit; with rev that commit against its parent; against pushed: the last commit against what is pushed. */
  modelDiff: (which: { rev?: string; against?: 'pushed' } = {}) => get<ModelDiffReply>('/api/model/diff', which),
  insights: () => get<InsightsReply>('/api/insights'),
  /** Effective values of options (full names, quoted segments allowed) on all machines, or the given ones. */
  optionValues: (names: string[], hosts: string[] = []) =>
    get<ValuesReply>('/api/fleet/values', { name: names, host: hosts }),
  createMachine: (spec: MachineSpec) => post<MachineCreated>('/api/machines', spec),
  /** The SSH host key of the machine at this address: the server starts the handshake and stops at the key, never logs in. */
  fetchHostKey: (address: string) => post<HostKey>('/api/hostkey', { address }),
  /** Stages the inverse of a commit, or with since of every commit after since up to rev (nothing is committed). 409: local changes, not on this branch, or a conflict. */
  revert: (rev: string, since = '') => post('/api/revert', { rev, since }),
  /** Commits the staged changes (or takes the unpushed commits) on the machine's testing branch and pushes it; the working copy moves there. */
  tryOn: (host: string, message: string, replace = false) => post<Trial>('/api/try', { host, message, replace }),
  /** Points the machine's testing branch back at its main branch's last commit. */
  endTrial: (host: string) => post<{ message: string }>('/api/try/end', { host }).then((r) => r.message),
  /** Stages a branch squashed into the current one; returns a suggested commit message. 409 on conflict. */
  merge: (branch: string) => post<{ message: string }>('/api/merge', { branch }).then((r) => r.message),
  refreshModel: () => post<{ job: string }>('/api/model/refresh').then((r) => r.job),
  managed: (host: string) => get<Managed>(`/api/hosts/${encodeURIComponent(host)}/managed`),
  editPackages: (host: string, edit: { add?: string[]; remove?: string[] }) => post<Managed>(`/api/hosts/${encodeURIComponent(host)}/packages`, edit),
  editSetting: (host: string, edit: SettingEdit) => post<Managed>(`/api/hosts/${encodeURIComponent(host)}/settings`, edit),
  searchPackages: (q: string, limit = 50) => get<SearchReply<PackageHit>>('/api/search/packages', { q, limit }),
  searchOptions: (host: string, q: string, opts: { limit?: number; exact?: boolean } = {}) =>
    get<SearchReply<OptionInfo>>('/api/search/options', { host, q, limit: opts.limit, exact: opts.exact ? 1 : undefined }),

  tokens: () => get<{ tokens: Token[] }>('/api/tokens').then((r) => r.tokens ?? []),
  createToken: (name: string, scope: TokenScope, expires?: string) =>
    post<Token & { token: string }>('/api/tokens', expires ? { name, scope, expires } : { name, scope }),
  revokeToken: (id: string) => request<void>('DELETE', `/api/tokens/${encodeURIComponent(id)}`, {}),

  deployments: () => get<DeployOverview>('/api/deployments'),
  /** One machine's own report token. */
  deployToken: (host: string) => get<{ token: string }>('/api/deployments/token', { host }).then((r) => r.token),
  /** New tokens for every machine: those with an old one stop reporting until they get the new one. */
  regenerateDeployTokens: () => post<void>('/api/deployments/token'),
  /** Stops accepting the shared fleet token of an install from before machine tokens. */
  dropSharedDeployToken: () => request<void>('DELETE', '/api/deployments/shared-token', {}),
  updates: () => get<UpdateStatus>('/api/updates'),
  setUpdateSchedule: (s: UpdateSchedule) => post<UpdateStatus>('/api/updates/schedule', s),
  /** Pushes the update's branch: a full token's, or the web login's. */
  runUpdate: () => post<UpdateStatus>('/api/updates/run'),
  /** Fast-forwards the fleet's branch to the update, which deploys it. */
  mergeUpdate: (commit: string) => post<{ message: string }>('/api/updates/merge', { commit }).then((r) => r.message),
  discardUpdate: () => post<UpdateStatus>('/api/updates/discard'),
  notifications: () => get<{ settings: NotifySettings; status: ChannelStatus[] }>('/api/notifications'),
  saveNotifications: (settings: NotifySettings) => post<{ settings: NotifySettings }>('/api/notifications', settings).then((r) => r.settings),
  /** Sends a test message to every saved channel. */
  testNotifications: () => post<{ status: ChannelStatus[] }>('/api/notifications/test').then((r) => r.status),
  /** Deletes a machine's deployment reports (a machine that left the fleet, say). */
  forgetMachine: (host: string) => request<void>('DELETE', `/api/deployments/machines/${encodeURIComponent(host)}`, {}),

  secrets: () => get<SecretsStatus>('/api/secrets'),
  /** Approves a machine's SSH host key, new or replacing its last one (the .pub file, or an ssh-keyscan line); needs full access. */
  setMachineKey: (host: string, key: string) => request<SecretMachine>('PUT', `/api/secrets/machines/${encodeURIComponent(host)}`, { key }),
  /** Removes a machine's key and its own secrets; the shared ones are encrypted again without it. */
  removeMachineKey: (host: string) => request<void>('DELETE', `/api/secrets/machines/${encodeURIComponent(host)}`, {}),
  /** host "": for every machine with a key. */
  setSecret: (host: string, name: string, value: string) => request<void>('PUT', '/api/secrets', { host, name, value }),
  deleteSecret: (host: string, name: string) => post<void>('/api/secrets/delete', { host, name }),
  /** The value in plain text: only with full access, when the person asks. */
  revealSecret: (host: string, name: string) => post<{ value: string }>('/api/secrets/reveal', { host, name }).then((r) => r.value),
  reencryptSecrets: () => post<void>('/api/secrets/reencrypt'),
  /** Stores every machine's own report token as its secret; returns the machines. */
  storeReportTokens: () => post<{ hosts: string[] }>('/api/secrets/report-tokens').then((r) => r.hosts),
}

// ---- secrets (Settings) ----

export type SecretMachine = { name: string; key: string; fingerprint: string; recipient: string; error?: string }
export type SecretFile = { path: string; host: string; names: string[]; stale: boolean; locked: boolean; error?: string }
export type SecretsStatus = {
  /** Tools not found on the server. */
  missing: string[]
  /** This server's age key (public half), and where its private half is. */
  recipient: string
  keyFile: string
  /** Machines with an approved key: only these can decrypt. */
  machines: SecretMachine[]
  /** Keys in secrets/hosts/*.pub not approved here (a pull, an edit): nothing is encrypted for them. */
  proposed: SecretMachine[]
  /** secrets/common.json first, then each machine's file. */
  files: SecretFile[]
  /** A file or .sops.yaml is not encrypted for exactly the machines' keys: re-encrypt. */
  stale: boolean
}

// ---- notifications (Settings) ----

export type EmailSecurity = 'starttls' | 'tls' | 'none'
/** Channels as saved. Secrets are write-only: read back empty with has* set; sent back empty with has* set, the saved one stays. */
export type NotifySettings = {
  consoleUrl?: string
  ntfy?: { url: string; token?: string; hasToken?: boolean }
  webhook?: { url: string }
  email?: { host: string; port: number; security: EmailSecurity; username?: string; password?: string; hasPassword?: boolean; from: string; to: string[] }
}
/** How the last send to a channel went. */
export type ChannelStatus = { channel: 'ntfy' | 'webhook' | 'email'; ok: boolean; error?: string; at: string }

// ---- scheduled updates of the flake inputs (Inputs) ----

/** day: -1 every day, else 0 (Sunday) to 6; hour: 0 to 23 in the server's time zone. */
export type UpdateSchedule = { enabled: boolean; day: number; hour: number }
export type UpdateState = 'updating' | 'checking' | 'ready' | 'failed' | 'current' | 'error' | 'merged' | 'discarded'
/** The latest update: every input on top of the fleet's branch on the remote, pushed to flake-update. */
export type Update = {
  state: UpdateState
  scheduled: boolean
  started: string
  finished?: string
  branch?: string
  base?: string
  commit?: string
  inputs?: InputChange[]
  job?: string
  check?: string
  summary?: CheckSummary
  changed?: string[]
  error?: string
  merged?: string
}
/** outdated: the branch moved on since the update was made, so it cannot be merged. */
export type UpdateStatus = { schedule: UpdateSchedule; next?: string; zone: string; update?: Update; outdated?: boolean }

// ---- deployments (what comin reports from each machine) ----

/** One report: comin deployed (phase deploy: done, failed) or built (phase build: built, already built, failed) a commit. */
export type DeployReport = { at: string; commit: string; ref?: string; phase: 'deploy' | 'build'; status: string; error?: string; generation?: string }
/** never: no deployment reported; unknown: its commit is not in the working copy or not on the branch. */
export type DeployState = 'current' | 'behind' | 'failed' | 'unknown' | 'never'
/** What a machine last said it runs (/run/current-system), booted, and boots next (the system profile). */
export type SystemState = { at: string; current: string; booted?: string; profile?: string; reboot: boolean }
export type MachineDeployment = {
  host: string
  state: DeployState
  /** No report for more than silentAfter hours (0: never flagged), whatever the state. */
  silent: boolean
  silentAfter: number
  /** It sends heartbeats (a report module from before them does not). */
  heartbeats: boolean
  /** Its last report used the shared fleet token, not its own. */
  sharedToken?: boolean
  system?: SystemState
  /** The system its commit builds, as comin evaluates it, once the console evaluated it. */
  expected?: string
  /** manual: it runs a system no deployment made; unfinished: a switch stopped or failed partway. */
  drift?: 'manual' | 'unfinished'
  /** The main branch its comin follows, or its testing branch when that is on top of it. */
  branch: string
  /** The two branches its comin follows; testing is absent when its settings name none (or are unknown). */
  main: string
  testing?: string
  /** After a failure on its main branch: the commits since the one it runs, newest first, to revert. */
  culprits?: Commit[]
  target?: string
  /** The last commit it deployed (or built and found unchanged). */
  commit?: string
  subject?: string
  deployedAt?: string
  lastSeen?: string
  behind: number
  behindSince?: string
  /** The last report, when it is a failure. */
  failure?: DeployReport
  reports: DeployReport[]
}
/** sharedToken: the shared fleet token of an install from before machine tokens is still accepted. */
/** A change tried on one machine: the commit pushed to its testing branch. */
export type Trial = { host: string; branch: string; main: string; commit: string }
export type DeployOverview = { reporting: boolean; sharedToken: boolean; branch: string; commits: Commit[]; machines: MachineDeployment[] }
