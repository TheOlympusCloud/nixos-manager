// Secrets kept with sops-nix (Settings → Secrets): the rules the server applies,
// and where an option form can use one.
import { controlFor } from './options'

/** The secret comin pulls a private repository with, and each machine's report token. */
export const cominSecret = 'comin-token'
export const reportSecret = 'nixos-manager-report'

/** Why the server would refuse this name, or '' (it becomes /run/secrets/<name>). */
export function secretNameProblem(name: string): string {
  if (!name) return ''
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$/.test(name)) return 'Letters, digits, ".", "_" and "-", starting with a letter or digit (at most 100).'
  if (name === 'sops') return '"sops" is the file’s own metadata.'
  if (name.endsWith('_unencrypted')) return 'sops would keep a value named …_unencrypted in plain text.'
  return ''
}

/** Whether an option takes the path of a file, such as a password or token file: a secret fits there. */
export function takesSecretFile(path: string[], type: string): boolean {
  return /(File|_file)$/.test(path.at(-1) ?? '') && controlFor(type).kind === 'text'
}

/** A user's password hash: sops-nix must decrypt it before users are made (neededForUsers). */
export const forUsers = (path: string[]) => path.length === 4 && path[0] === 'users' && path[1] === 'users' && path[3] === 'hashedPasswordFile'

/** Where sops-nix puts a secret on the machine, by default. */
export const secretPath = (name: string, users: boolean) => `${users ? '/run/secrets-for-users' : '/run/secrets'}/${name}`
