// App configs and keyring entries in homes (managed.nix 6): what the Home tab's
// Apps section shows and edits. The server keeps them in managed.json's
// home.<user>.configs and keyring, with secrets by name only.

export type ConfigFormat = 'json' | 'toml' | 'yaml' | 'ini' | 'properties'
export const CONFIG_FORMATS: ConfigFormat[] = ['json', 'toml', 'yaml', 'ini', 'properties']
/** An app's settings file in a home, which the app also writes; secrets has settings' shape, with secret names. */
export type HomeConfig = {
  format: ConfigFormat
  mode?: 'merge' | 'seed'
  settings?: Record<string, unknown>
  secrets?: Record<string, unknown>
  process?: string
  lock?: string
}
export type KeyringEntry = { label: string; attributes: Record<string, string>; secret: string }

/** One key of a config as a tree: an object of keys, or a value, or one of the machine's secrets. */
export type ConfigNode = { key: string; path: string[]; children?: ConfigNode[]; value?: unknown; secret?: string }

const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)

/** settings and secrets as one tree, keys in order, objects before values. */
export function configTree(settings: Record<string, unknown> = {}, secrets: Record<string, unknown> = {}, at: string[] = []): ConfigNode[] {
  const keys = [...new Set([...Object.keys(settings), ...Object.keys(secrets)])].sort((a, b) => a.localeCompare(b))
  const nodes = keys.map((key): ConfigNode => {
    const path = [...at, key]
    const v = settings[key]
    const s = secrets[key]
    if (isObject(v) || isObject(s)) return { key, path, children: configTree(isObject(v) ? v : {}, isObject(s) ? s : {}, path) }
    if (typeof s === 'string') return { key, path, secret: s }
    return { key, path, value: v }
  })
  return [...nodes.filter((n) => n.children), ...nodes.filter((n) => !n.children)]
}

/** How many values and secrets a config declares. */
export const countKeys = (nodes: ConfigNode[]): number => nodes.reduce((n, x) => n + (x.children ? countKeys(x.children) : 1), 0)

/**
 * A key as typed, into its parts: a.b.c; "a.b" for a key with dots; Servers[ID=8f2c].Hostname for one
 * item of a list. INI is section.key (the first dot), properties one key, dots and all.
 */
export function parseKey(text: string, format: ConfigFormat): string[] | string {
  const t = text.trim()
  if (!t) return 'Enter a key.'
  if (format === 'properties') return [t]
  const parts: string[] = []
  let cur = ''
  let depth = 0
  let quoted = false
  for (let i = 0; i < t.length; i++) {
    const c = t[i]
    if (quoted) {
      if (c === '"') quoted = false
      else cur += c
    } else if (c === '"' && !cur) quoted = true
    else if (c === '[') (depth++, (cur += c))
    else if (c === ']') (depth--, (cur += c))
    else if (c === '.' && depth === 0) (parts.push(cur), (cur = ''))
    else cur += c
  }
  if (quoted || depth !== 0) return 'An open quote or bracket.'
  parts.push(cur)
  if (parts.some((p) => !p)) return 'An empty part: two dots, or one at an end.'
  const bad = parts.find((p) => p.endsWith(']') && !/^.+\[[^=[\]]+=[^[\]]*\]$/.test(p))
  if (bad) return `${bad}: a list item is Name[Field=Value].`
  if (format === 'ini') {
    if (parts.length < 2) return 'An INI key is section.key.'
    return [parts[0], parts.slice(1).join('.')]
  }
  return parts
}

/** A key's parts as typed back: quotes around a part with dots, outside a list item. */
export const keyText = (path: string[], format: ConfigFormat) =>
  format === 'properties' ? path.join('.') : path.map((p, i) => (p.includes('.') && !(format === 'ini' && i === 1) && !p.endsWith(']') ? `"${p}"` : p)).join('.')

/** The kind of input a value gets, from the value itself. */
export type ValueKind = 'text' | 'number' | 'boolean' | 'list'
export const kindOf = (v: unknown): ValueKind => (typeof v === 'boolean' ? 'boolean' : typeof v === 'number' ? 'number' : Array.isArray(v) ? 'list' : 'text')

/** A typed value from an input's text, or a problem. */
export function parseValue(kind: ValueKind, text: string): { value: unknown } | { problem: string } {
  if (kind === 'text') return { value: text }
  if (kind === 'boolean') return { value: text === 'true' }
  if (kind === 'number') {
    const n = Number(text)
    return text.trim() === '' || !Number.isFinite(n) ? { problem: 'Enter a number.' } : { value: n }
  }
  try {
    const v = JSON.parse(text)
    return Array.isArray(v) ? { value: v } : { problem: 'Enter a JSON list, e.g. ["a", 1].' }
  } catch {
    return { problem: 'Enter a JSON list, e.g. ["a", 1].' }
  }
}

/** A path in ~ for a config: inside the home, no .. or //. */
export function configPathProblem(p: string, taken: string[]): string {
  const t = p.trim()
  if (!t) return ''
  if (t.startsWith('/') || t.startsWith('~')) return 'A path inside the home folder, such as .config/app/settings.json.'
  if (t.split('/').some((s) => s === '' || s === '.' || s === '..')) return 'No empty parts, . or .. in the path.'
  if (taken.includes(t)) return `~/${t} is already set here.`
  return ''
}

/** Keyring attributes typed one per line, key=value. */
export function parseAttributes(text: string): Record<string, string> | string {
  const out: Record<string, string> = {}
  for (const line of text.split('\n').map((l) => l.trim()).filter(Boolean)) {
    const at = line.indexOf('=')
    if (at < 1) return `${line}: write key=value.`
    out[line.slice(0, at).trim()] = line.slice(at + 1).trim()
  }
  return Object.keys(out).length ? out : 'Give the attributes the app looks its item up by, such as service=…'
}
export const attributesText = (a: Record<string, string>) =>
  Object.entries(a)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')

// ---- Known apps: what Pablo's desktop apps keep where (OLYMPUS-283's map) ----

/** A file an app also writes, to manage by its keys; process or lock keep the merge off while it runs. */
export type AppFile = { path: string; format: ConfigFormat; process?: string; lock?: string; note: string }
export type KnownApp = {
  name: string
  /** Package names (pname) it comes as. */
  packages: string[]
  files: AppFile[]
  /** Its password goes in the keyring: the attributes it looks it up by. */
  keyring?: { label: string; attributes: Record<string, string>; note: string }
  /** What stays in the app: logins in its own databases, rotating tokens, keys it made. */
  inApp: string[]
  note?: string
}

export const KNOWN_APPS: KnownApp[] = [
  {
    name: 'Supersonic',
    packages: ['supersonic', 'supersonic-wayland'],
    files: [{ path: '.config/supersonic/config.toml', format: 'toml', process: 'supersonic', note: 'Servers[ID=<id>].Hostname, Username…; Application.*' }],
    keyring: {
      label: "Password for '<server ID>' on 'supersonic'",
      attributes: { service: 'supersonic', username: '<server ID>' },
      note: "A server's password, by its ID in config.toml (go-keyring, login collection).",
    },
    inApp: ['The play queue (saved_queue.json…) and the cache in ~/.cache/supersonic', 'config.toml.bak, its own backup'],
  },
  {
    name: 'Vesktop',
    packages: ['vesktop'],
    files: [
      { path: '.config/vesktop/settings.json', format: 'json', process: 'vesktop', note: 'tray, minimizeToTray, arRPC, splash…' },
      { path: '.config/vesktop/settings/settings.json', format: 'json', process: 'vesktop', note: 'Vencord: plugins.<Name>.enabled, enabledThemes, themeLinks…' },
    ],
    inApp: ['The Discord login (Local Storage in sessionData, LevelDB)', "Vencord's cloud token (IndexedDB VencordData)", 'native-settings.json and sessionData/'],
  },
  {
    name: 'Brave',
    packages: ['brave'],
    files: [],
    inApp: [
      'Profile preferences (Preferences, Secure Preferences: protected by MACs)',
      'Saved passwords (Login Data, encrypted with its Brave Safe Storage key)',
      'The Brave Safe Storage key in the keyring, which Brave made',
      'Bookmarks, extensions and the rest of the profile',
    ],
    note: 'Its settings go through managed policies (/etc/brave/policies/managed), set for the machine, not the home.',
  },
  {
    name: 'Hydra Launcher',
    packages: ['hydralauncher', 'hydra-launcher'],
    files: [],
    inApp: ['Settings and the self-hosted server URL (hydra-db, LevelDB, locked while it runs)', 'The login (hydra-db auth) and API tokens', 'Its safeStorage key in the keyring'],
  },
  {
    name: 'unbraid',
    packages: ['unbraid'],
    files: [
      { path: '.local/share/unbraid/unbraid_ui.properties', format: 'properties', process: 'unbraid', note: 'terms, intro_seen, tour_pending…' },
      { path: '.local/share/unbraid/desktop.properties', format: 'properties', process: 'unbraid', note: 'theme, close_to_tray, remember_window…' },
    ],
    inApp: ['VPN logins and setup (accounts.bin, setup.bin, links.bin: sealed)', 'Its store key (keyring, or ~/.local/share/unbraid/key), which it made', 'The engine state in /var/lib/unbraid-engine'],
  },
  {
    name: 'Claude Code',
    packages: ['claude-code'],
    files: [
      { path: '.claude/settings.json', format: 'json', lock: '.claude/settings.json.lock', note: 'model, permissions, env, hooks, enabledPlugins…' },
      { path: '.claude.json', format: 'json', lock: '.claude.json.lock', note: 'mcpServers…' },
    ],
    inApp: ['The login (.claude/.credentials.json: OAuth tokens that rotate)', "~/.claude.json's primaryApiKey and account", 'Sessions, projects and history'],
  },
  {
    name: 'Multica',
    packages: ['multica-desktop'],
    // multica-desktop, its package: plain "multica" also matches the CLI's daemon, which runs all the time.
    files: [{ path: '.multica/desktop_prefs.json', format: 'json', process: 'multica-desktop', note: 'autoStart, autoStop' }],
    inApp: ['The login token (~/.multica/profiles/…/config.json and Local Storage)', 'Window state'],
  },
  {
    name: 'Steam',
    packages: ['steam'],
    files: [],
    inApp: ['The login (loginusers.vdf, registry.vdf, ssfn files)', 'Everything in ~/.local/share/Steam: library folders and Proton default are VDF, not editable here yet'],
  },
  {
    name: 'Xodus Nexus',
    packages: ['xodus-nexus'],
    files: [{ path: '.config/xodus-nexus/launcher-state.json', format: 'json', process: 'xodus-nexus', note: 'settings.externalXodusRoot, installRoot, closeToTray…' }],
    inApp: ['Xbox and Microsoft logins (keyring: service "Xodus Service")', 'The Xbox profile, window state and backends'],
  },
]

/** The known apps a home has: installed (by package name) or with one of its files managed. */
export const appsIn = (packages: string[], paths: string[]) =>
  KNOWN_APPS.filter((a) => a.packages.some((p) => packages.includes(p)) || a.files.some((f) => paths.includes(f.path)))
