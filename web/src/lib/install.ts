// Installing a machine that has a disk layout: nixos-anywhere, run by a person from a clone of the repository.

/** Problem with the machine's address ('' when fine). It goes into a shell command, so only a host name or IP address passes. */
export function addressProblem(address: string): string {
  if (!address) return ''
  const name = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$/
  const ipv6 = /^[0-9A-Fa-f:]*:[0-9A-Fa-f:]*:[0-9A-Fa-f.:]*$/
  return name.test(address) || ipv6.test(address) ? '' : 'A host name or IP address, such as 192.168.1.50 or nas.lan (SSH on port 22).'
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
