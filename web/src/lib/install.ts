// Installing a machine that has a disk layout: nixos-anywhere, run by a person from a clone of the repository.
import { ApiError, type SecretsStatus } from './api'

/**
 * Problem with the machine's address ('' when fine). It goes into a shell command, so only a host name or IPv4 address
 * passes: nixos-anywhere also gives it to nix copy as ssh://root@<address>, where an IPv6 address needs brackets that ssh
 * itself does not take.
 */
export function addressProblem(address: string): string {
  if (!address) return ''
  const name = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/
  if (address.includes(':')) return 'An IPv6 address does not work in the command: use the machine’s host name.'
  return name.test(address) ? '' : 'A host name or IPv4 address, such as 192.168.1.50 or nas.lan (SSH on port 22).'
}

/**
 * The command that installs name on the machine at address (validated, or <address>): it pulls, runs nixos-anywhere
 * with the host key the installer has, and with generateHardware writes hardware-configuration.nix, then commits and
 * pushes it so comin deploys the same system.
 */
export function installCommand(name: string, address: string, generateHardware: boolean): string {
  const target = address && !addressProblem(address) ? address : '<address>'
  const hw = `hosts/${name}/hardware-configuration.nix`
  return [
    'git pull --ff-only &&',
    `nix run github:nix-community/nixos-anywhere -- --flake .#${name} --target-host root@${target} --copy-host-keys`,
    ...(generateHardware
      ? [`--generate-hardware-config nixos-generate-config ./${hw} &&`, `git commit -m "${name}: hardware configuration from nixos-anywhere" -- ${hw} &&`, 'git push']
      : []),
  ].join(' ')
}

/** The disk a disk-config.nix installs on ('' when it does not say plainly). */
export function diskOf(diskConfig: string): string {
  return /^\s*device\s*=\s*"(\/dev\/[^"]+)";/m.exec(diskConfig)?.[1] ?? ''
}

/** Whether hardware-configuration.nix is still the placeholder the wizard wrote. */
export const isPlaceholder = (hardware: string) => hardware.startsWith('# Placeholder')

const absent = (r: PromiseSettledResult<unknown>) => r.status === 'rejected' && r.reason instanceof ApiError && r.reason.status === 404
const why = (r: PromiseRejectedResult) => (r.reason instanceof Error ? r.reason.message : String(r.reason))

/**
 * What the install card makes of its reads of disk-config.nix, hardware-configuration.nix and the secrets. Only a 404
 * means a file is absent; any other failure is an error to show. generate is null while the hardware configuration is
 * unknown: --generate-hardware-config would overwrite a real one.
 */
export function installReads(
  disk: PromiseSettledResult<{ content: string }>,
  hardware: PromiseSettledResult<{ content: string }>,
  secrets: PromiseSettledResult<SecretsStatus>,
): { disk: string; generate: boolean | null; secrets: SecretsStatus | null; errors: string[] } {
  const errors: string[] = []
  if (disk.status === 'rejected' && !absent(disk)) errors.push(`Cannot read disk-config.nix: ${why(disk)}`)
  if (hardware.status === 'rejected' && !absent(hardware)) errors.push(`Cannot read hardware-configuration.nix: ${why(hardware)}`)
  if (secrets.status === 'rejected') errors.push(`Cannot load the secrets: ${why(secrets)}`)
  return {
    disk: disk.status === 'fulfilled' ? diskOf(disk.value.content) : '',
    generate: hardware.status === 'fulfilled' ? isPlaceholder(hardware.value.content) : absent(hardware) ? true : null,
    secrets: secrets.status === 'fulfilled' ? secrets.value : null,
    errors,
  }
}
