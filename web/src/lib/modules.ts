// Fleet modules: modules/fleet/<name>/managed.json, a NixOS module of its own that
// machines turn on with fleet.modules.<name>.enable and set its options per machine.
import type { Host, HostModule, ModuleOptionType } from './api'
import { SHARED_FILE } from './packages'

/** Stands for a module where a machine name goes, as SHARED and HOME do: module:media. */
export const MODULE = 'module:'
export const MODULE_FOLDER = 'modules/fleet'
export const moduleTypes: ModuleOptionType[] = ['bool', 'int', 'port', 'str', 'listOf str', 'anything']

export const moduleNameOk = (name: string) => /^[A-Za-z_][A-Za-z0-9_-]{0,62}$/.test(name)
/** enable is the module's own switch. */
export const optionNameOk = (name: string) => moduleNameOk(name) && name !== 'enable'
export const moduleFile = (name: string) => `${MODULE_FOLDER}/${name}/managed.json`
export const moduleNixFile = (name: string) => `${MODULE_FOLDER}/${name}/custom.nix`
/** fleet.modules.<name>[.<rest>] as an option path. */
export const modulePath = (name: string, ...rest: string[]) => ['fleet', 'modules', name, ...rest]
/** In a module's settings, stands for that option's value on each machine. */
export const optionRef = (option: string) => `@option:${option}@`
/** Its managed.nix applies fleet modules (version 7). */
export const usesModules = (host: Host) => !!host.managedFile && (host.managedVersion ?? 1) >= 7

export const moduleNixTemplate = (name: string) => `# The NixOS settings of the fleet module ${name} that its managed.json cannot hold.
# It is loaded on every machine with managed.nix: keep what it sets under mkIf enable.
{ config, lib, pkgs, ... }:
let
  cfg = config.fleet.modules.${name};
in
{
  config = lib.mkIf cfg.enable {
  };
}
`

/** Where a module's switch or option on a machine is set: its managed.json, every machine's, hand-written Nix, or nowhere. */
export type ModuleSource = { kind: 'here' } | { kind: 'shared' } | { kind: 'code'; file: string } | { kind: 'default' }

/** files: as the model reports them; at its default, the module's own managed.json declares the value. */
export function moduleSource(files: string[], name: string, managedFile: string | null): ModuleSource {
  if (managedFile && files.includes(managedFile)) return { kind: 'here' }
  if (files.includes(SHARED_FILE)) return { kind: 'shared' }
  const code = files.filter((f) => f !== 'nixos' && f !== moduleFile(name))
  return code.length ? { kind: 'code', file: code[0] } : { kind: 'default' }
}

/**
 * Whether the module is on: a value set in the machine's managed.json wins (it applies at
 * priority 90) and shows at once; one turned on only there and since reset is off.
 */
export function moduleOn(hm: HostModule | undefined, setHere: unknown, name: string, managedFile: string | null): boolean {
  if (typeof setHere === 'boolean') return setHere
  return !!hm?.enable && moduleSource(hm.files, name, managedFile).kind !== 'here'
}

/** Turning it off: reset what is set here, or set false over every machine's or hand-written Nix. */
export function offValue(hm: HostModule | undefined, name: string, managedFile: string | null): false | undefined {
  const k = hm?.enable ? moduleSource(hm.files, name, managedFile).kind : 'default'
  return k === 'shared' || k === 'code' ? false : undefined
}

/** A value as a form shows it for its type. */
export function valueText(type: ModuleOptionType, v: unknown): string {
  if (v === undefined || v === null) return ''
  if (type === 'str' && typeof v === 'string') return v
  if (type === 'listOf str' && Array.isArray(v)) return v.join('\n')
  return JSON.stringify(v)
}

/** A form's text as a value of the type: a list of text is one item per line, anything is JSON. */
export function parseValue(type: ModuleOptionType, text: string): { value: unknown } | { problem: string } {
  const t = text.trim()
  switch (type) {
    case 'bool':
      return t === 'true' || t === 'false' ? { value: t === 'true' } : { problem: 'true or false' }
    case 'int':
    case 'port': {
      if (!/^-?\d+$/.test(t)) return { problem: 'a whole number' }
      const n = Number(t)
      return type === 'port' && (n < 0 || n > 65535) ? { problem: 'a port, from 0 to 65535' } : { value: n }
    }
    case 'str':
      return { value: text }
    case 'listOf str':
      return {
        value: text
          .split('\n')
          .map((x) => x.trim())
          .filter(Boolean),
      }
    default:
      try {
        return { value: JSON.parse(t) }
      } catch {
        return { problem: 'a JSON value: true, 8080, "text", [ … ] or { … }' }
      }
  }
}

/** A setting's value from a form: JSON, or else the text itself (Europe/Madrid, @option:port@). */
export function settingValue(text: string): { value: unknown } | { problem: string } {
  const t = text.trim()
  if (!t) return { problem: 'Write a value' }
  try {
    return { value: JSON.parse(t) }
  } catch {
    return /^[[{"]/.test(t) ? { problem: 'This is not valid JSON' } : { value: t }
  }
}

/** The machines where the last evaluation has the module on. */
export const machinesWith = (hosts: Host[], name: string) => hosts.filter((h) => h.modules?.some((m) => m.name === name && m.enable)).map((h) => h.name)
