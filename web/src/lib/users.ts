// Users and SSH keys: checks before they reach managed.json.
import { copyable, type Effective } from './values'

const KEY_TYPES = [
  'ssh-ed25519',
  'ssh-rsa',
  'ecdsa-sha2-nistp256',
  'ecdsa-sha2-nistp384',
  'ecdsa-sha2-nistp521',
  'sk-ssh-ed25519@openssh.com',
  'sk-ecdsa-sha2-nistp256@openssh.com',
]

export type SSHKey = { type: string; data: string; comment: string }

/** One authorized_keys line ("ssh-ed25519 AAAA… me@laptop"), or the reason it is not one. */
export function parseSSHKey(line: string): SSHKey | { problem: string } {
  const [type = '', data = '', ...rest] = line.trim().split(/\s+/)
  if (!type) return { problem: 'Paste a public key, e.g. the contents of ~/.ssh/id_ed25519.pub.' }
  if (type === 'ssh-dss') return { problem: 'DSA keys are no longer accepted by OpenSSH; use an ed25519 key.' }
  if (!KEY_TYPES.includes(type)) {
    if (/PRIVATE KEY/.test(line)) return { problem: 'This is a private key. Never share it; paste the .pub file instead.' }
    return { problem: 'A public key starts with its type, e.g. ssh-ed25519 or ssh-rsa, then the key data.' }
  }
  if (!data) return { problem: 'The key data after the type is missing.' }
  let bytes: string
  try {
    if (data.length % 4 || !/^[A-Za-z0-9+/]+={0,2}$/.test(data)) throw new Error()
    bytes = atob(data)
  } catch {
    return { problem: 'The key data is not valid; copy the whole line from the .pub file.' }
  }
  // The data starts with its own type: a 4-byte length, then the name.
  const n = ((bytes.charCodeAt(0) << 24) | (bytes.charCodeAt(1) << 16) | (bytes.charCodeAt(2) << 8) | bytes.charCodeAt(3)) >>> 0
  const inner = bytes.slice(4, 4 + n)
  if (inner !== type) return { problem: `The key data is for ${inner || 'another type'}, not ${type}.` }
  return { type, data, comment: rest.join(' ') }
}

/**
 * A list option to edit, or null while its value is not known: a list set here replaces
 * the whole option, so editing from a guess would drop entries written in code.
 * `fresh`: a user created here that no evaluation has seen yet, whose lists start empty.
 */
export function editableList(e: Effective | null, fresh = false): string[] | null {
  if (e?.state === 'value') return !e.pending && Array.isArray(e.value) && copyable(e.value) ? (e.value as string[]) : null
  return fresh ? [] : null
}

/** Every key in pasted text, one per line, or the first line's problem. */
export function parseSSHKeys(text: string): { keys: string[] } | { problem: string } {
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
  const keys: string[] = []
  for (const [i, l] of lines.entries()) {
    const k = parseSSHKey(l)
    if ('problem' in k) return { problem: lines.length > 1 ? `Key ${i + 1}: ${k.problem}` : k.problem }
    keys.push([k.type, k.data, k.comment].filter(Boolean).join(' '))
  }
  return keys.length ? { keys } : { problem: 'Paste a public key, e.g. the contents of ~/.ssh/id_ed25519.pub.' }
}

/** "ssh-ed25519 AAAA…xyz me@laptop" → what to show: the type, the comment and the end of the data. */
export function keySummary(line: string): { type: string; comment: string; tail: string } {
  const [type = '', data = '', ...rest] = line.trim().split(/\s+/)
  return { type: type.replace(/^ssh-|@openssh\.com$/g, '').replace(/^ecdsa-sha2-/, 'ecdsa '), comment: rest.join(' '), tail: data.slice(-10) }
}

const RESERVED = new Set(['root', 'nobody', 'daemon', 'bin', 'sys', 'sshd', 'messagebus', 'systemd-network', 'nixbld'])

/** Problem with a new user name, or '' when it is fine. */
export function userNameProblem(name: string, existing: string[]): string {
  if (!name) return ''
  if (!/^[a-z_][a-z0-9_-]*$/.test(name)) return 'Lowercase letters, digits, - and _, starting with a letter.'
  if (name.length > 32) return 'At most 32 characters.'
  if (RESERVED.has(name)) return `${name} is a system account.`
  if (existing.includes(name)) return `${name} already exists on this machine.`
  return ''
}

/** Groups offered when adding users, with what they give. */
export const commonGroups: { name: string; note: string }[] = [
  { name: 'wheel', note: 'administrator: can use sudo' },
  { name: 'networkmanager', note: 'change network settings' },
  { name: 'docker', note: 'run containers' },
  { name: 'video', note: 'cameras and GPU access' },
  { name: 'audio', note: 'sound devices' },
  { name: 'dialout', note: 'serial ports' },
  { name: 'input', note: 'input devices' },
]
