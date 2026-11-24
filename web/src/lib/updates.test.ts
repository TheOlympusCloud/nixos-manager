import { describe, expect, it } from 'vitest'
import type { Update } from './api'
import { hourText, scheduleDays, scheduleText, updateBusy, updateLine } from './updates'

const up = (u: Partial<Update>): Update => ({ state: 'ready', scheduled: true, started: '2026-11-23T04:00:00Z', branch: 'main', ...u })

describe('scheduled updates', () => {
  it('lists every day, then Monday to Sunday with their Go weekday numbers', () => {
    expect(scheduleDays.map((d) => d.value)).toEqual([-1, 1, 2, 3, 4, 5, 6, 0])
    expect(scheduleDays[7].label).toBe('Every Sunday')
  })

  it('says when the schedule runs, in the server time zone', () => {
    expect(scheduleText({ enabled: false, day: 1, hour: 4 }, 'CET')).toBe('Off')
    expect(scheduleText({ enabled: true, day: 1, hour: 4 }, 'CET')).toBe('Every Monday at 04:00 (CET)')
    expect(scheduleText({ enabled: true, day: -1, hour: 23 }, '')).toBe('Every day at 23:00')
    expect(hourText(0)).toBe('00:00')
  })

  it('says where the update stands', () => {
    expect(updateLine(up({ summary: { base: '', changed: ['pi'], built: ['pi', 'web'], toBuild: 0, toFetch: 0 } }))).toEqual({
      tone: 'ok',
      title: 'Ready to merge into main',
      detail: 'Passed its check, which built 2 machines: pi, web.',
    })
    expect(updateLine(up({ summary: { base: 'abc', changed: ['pi'], built: [], evaluatedOnly: ['pi'], toBuild: 0, toFetch: 0 } })).detail).toBe(
      'Passed its check, which evaluated 1 machine without building it here: pi.',
    )
    expect(updateLine(up({}), true)).toMatchObject({ tone: 'bad', title: 'main moved on since this update' })
    expect(updateLine(up({ state: 'failed', summary: { base: '', changed: ['pi'], built: [], failed: ['pi'], toBuild: 0, toFetch: 0 } })).detail).toBe(
      'pi does not build with it. The log says why.',
    )
    expect(updateLine(up({ state: 'error', error: 'no remote is configured' }))).toEqual({ tone: 'bad', title: 'The update could not run', detail: 'no remote is configured' })
    expect(updateLine(up({ state: 'updating', branch: undefined })).detail).toBe('nix flake update, on top of the fleet branch as it is on the remote.')
    expect(updateLine(up({ state: 'current' })).title).toBe('Every input is up to date')
    expect(updateBusy(up({ state: 'checking' }))).toBe(true)
    expect(updateBusy(up({ state: 'failed' }))).toBe(false)
    expect(updateBusy(undefined)).toBe(false)
  })
})
