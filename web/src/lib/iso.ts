// Installer ISOs (Machines → Installer ISO): what the list and the install steps say.
import type { ISO } from './api'

/** The command that writes an ISO to a USB stick: /dev/sdX is the stick, which it erases. */
export const flashCommand = (file: string) => `sudo dd if=${file} of=/dev/sdX bs=4M status=progress conv=fsync`

/** Whether a build is waiting or running. */
export const isoBuilding = (isos: ISO[]) => isos.some((i) => i.state === 'queued' || i.state === 'running')

/** A build's state as a status dot and a word. */
export function isoState(i: ISO): { dot: 'busy' | 'err' | 'ok'; text: string } {
  switch (i.state) {
    case 'queued':
      return { dot: 'busy', text: 'Waiting' }
    case 'running':
      return { dot: 'busy', text: 'Building' }
    case 'failed':
      return { dot: 'err', text: 'Failed' }
    default:
      return { dot: 'ok', text: 'Ready' }
  }
}
