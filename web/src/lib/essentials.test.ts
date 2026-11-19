import { describe, expect, it } from 'vitest'
import { essentialOptions, essentials, retentionDays, timeZones, withRetention } from './essentials'

describe('essentials', () => {
  it('fits in one values request, with no duplicates', () => {
    expect(essentialOptions.length).toBeLessThanOrEqual(40)
    expect(new Set(essentialOptions).size).toBe(essentialOptions.length)
    // Dependencies point at options of the page.
    for (const i of essentials.flatMap((g) => g.items)) if (i.when) expect(essentialOptions).toContain(i.when)
  })
  it('checks typed values', () => {
    const item = (o: string) => essentials.flatMap((g) => g.items).find((i) => i.option === o)!
    expect(item('i18n.defaultLocale').check!('en_US.UTF-8')).toBe('')
    expect(item('i18n.defaultLocale').check!('english')).toBe('')
    expect(item('i18n.defaultLocale').check!('en US')).not.toBe('')
    expect(item('time.timeZone').check!('Europe/Madrid')).toBe('')
    expect(item('time.timeZone').check!('Mars/Olympus')).not.toBe('')
    expect(timeZones()[0]).toBe('UTC')
  })
  it('reads and writes the retention in the gc options', () => {
    expect(retentionDays('--delete-older-than 14d')).toBe(14)
    expect(retentionDays('--max-freed 1G')).toBeNull()
    expect(retentionDays(undefined)).toBeNull()
    expect(withRetention('--delete-older-than 14d', 30)).toBe('--delete-older-than 30d')
    expect(withRetention('--max-freed 1G', 7)).toBe('--max-freed 1G --delete-older-than 7d')
    expect(withRetention('', 7)).toBe('--delete-older-than 7d')
  })
})
