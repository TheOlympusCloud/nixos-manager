import { describe, expect, it } from 'vitest'
import type { ISO } from './api'
import { flashCommand, isoBuilding, isoState } from './iso'

const iso = (state: ISO['state']): ISO => ({ id: 'pc-abcdef0-1', host: 'pc', commit: 'abcdef0123', branch: 'main', state, started: '2026-11-24T08:00:00Z' })

describe('installer ISOs', () => {
  it('writes the file to a stick the person names', () => {
    expect(flashCommand('nixos-pc-abcdef0.iso')).toBe('sudo dd if=nixos-pc-abcdef0.iso of=/dev/sdX bs=4M status=progress conv=fsync')
  })
  it('knows when a build is under way', () => {
    expect(isoBuilding([iso('ready'), iso('failed')])).toBe(false)
    expect(isoBuilding([iso('ready'), iso('queued')])).toBe(true)
    expect(isoBuilding([iso('running')])).toBe(true)
  })
  it('names every state with a glyph and a word', () => {
    expect((['queued', 'running', 'failed', 'ready'] as const).map((s) => isoState(iso(s)))).toEqual([
      { dot: 'busy', text: 'Waiting' },
      { dot: 'busy', text: 'Building' },
      { dot: 'err', text: 'Failed' },
      { dot: 'ok', text: 'Ready' },
    ])
  })
})
