import { describe, expect, it } from 'vitest'
import type { HostDiff } from './api'
import { impactGroups, impactLine, nixValue, revDistance, settingChanges, splitImpact, withSettings } from './impact'

const changed: HostDiff = {
  host: 'ws',
  status: 'changed',
  versionsChanged: [
    { name: 'curl', from: '8.16.0', to: '8.17.0' },
    { name: 'firefox', from: '145.0', to: '146.0' },
  ],
  packagesAdded: ['vlc'],
  packagesRemoved: ['nano', 'vim', 'htop'],
  featuresAdded: ['services.jellyfin'],
  portsAdded: ['tcp/8096'],
}

describe('impact', () => {
  it('says what changes on a machine in one line', () => {
    expect(impactLine(changed)).toBe('curl 8.16.0 → 8.17.0 and 1 more updated · vlc installed · 3 packages removed · jellyfin turned on · port tcp/8096 opened')
    expect(impactLine({ host: 'x', status: 'changed', versionsChanged: [{ name: 'git', from: '1', to: '2' }] })).toBe('git 1 → 2')
    expect(impactLine({ host: 'x', status: 'changed', usersAdded: ['alice', 'bob'] })).toBe('user alice and bob added')
    expect(impactLine({ host: 'n', status: 'added', packagesAdded: ['a', 'b'], featuresAdded: ['services.x'] })).toBe('New machine · 2 packages · 1 service or program')
    expect(impactLine({ host: 'x', status: 'unchanged' })).toBe('No change')
    expect(impactLine({ host: 'x', status: 'error', error: 'boom' })).toBe('Does not evaluate')
  })

  it('names a change the lists do not show, and a reboot first so it is never cut off', () => {
    expect(impactLine({ host: 'x', status: 'changed' })).toBe('Other configuration')
    expect(impactLine({ host: 'x', status: 'changed', reboot: true })).toBe('Reboot needed · other configuration')
    const kernel = { name: 'kernel', from: '6.12.1', to: '6.12.2' }
    const k = { host: 'x', status: 'changed' as const, reboot: true, kernel, versionsChanged: [{ name: 'git', from: '1', to: '2' }] }
    expect(impactLine(k)).toBe('Reboot needed · kernel 6.12.1 → 6.12.2 · git 1 → 2')
    expect(impactGroups(k)[0].items.map((i) => i.text)).toEqual(['kernel 6.12.1 → 6.12.2', 'git 1 → 2'])
  })

  it('groups the details and drops empty groups', () => {
    const g = impactGroups(changed)
    expect(g.map((x) => x.label)).toEqual(['Updated', 'Installed', 'Removed', 'Turned on', 'Ports'])
    expect(g[0].items[1]).toEqual({ text: 'firefox 145.0 → 146.0', kind: 'up' })
    expect(g[3].items[0]).toEqual({ text: 'jellyfin', title: 'services.jellyfin', kind: 'add' })
  })

  const MiB = 1024 * 1024
  const detailed: HostDiff = {
    host: 'web',
    status: 'changed',
    versionsChanged: [{ name: 'nginx', from: '1.26.0', to: '1.28.0' }],
    units: [
      { name: 'dhcpcd.service', action: 'keep' },
      { name: 'new.service', action: 'start' },
      { name: 'nginx.service', action: 'restart' },
      { name: 'old.service', action: 'stop' },
      { name: 'phpfpm.service', action: 'reload' },
    ],
    etc: [
      { path: '/etc/hosts', status: 'changed' },
      { path: '/etc/new.conf', status: 'added' },
      { path: '/etc/old.conf', status: 'removed' },
    ],
    closure: {
      sizeFrom: 1300 * MiB,
      sizeTo: 1318.4 * MiB,
      changes: [
        { name: 'glibc', delta: -12288 },
        { name: 'hello', to: '2.12.2', delta: 268902 },
        { name: 'nano', from: '8.6', delta: -2930176 },
        { name: 'nginx', from: '1.26.0', to: '1.28.0' },
      ],
    },
  }

  it('lists units, /etc files, dependencies and the size of a built system', () => {
    const g = impactGroups(detailed)
    expect(g.map((x) => x.label)).toEqual(['Updated', 'Restarts', 'Reloads', 'New units', 'Stops', 'Not restarted', '/etc', 'Dependencies', 'Size'])
    const by = (label: string) => g.find((x) => x.label === label)!.items
    expect(by('Restarts')).toEqual([{ text: 'nginx.service', kind: 'up' }])
    expect(by('New units')).toEqual([{ text: 'new.service', kind: 'add', title: expect.stringMatching(/running target wants it/) }])
    expect(by('Stops')).toEqual([{ text: 'old.service', kind: 'del' }])
    expect(by('Not restarted')[0].title).toMatch(/old version until/)
    expect(by('/etc').map((i) => [i.text, i.kind])).toEqual([
      ['/etc/hosts', 'up'],
      ['/etc/new.conf', 'add'],
      ['/etc/old.conf', 'del'],
    ])
    expect(by('Dependencies')).toEqual([
      { text: 'glibc', title: '−12.0 KiB', kind: 'up' },
      { text: 'hello 2.12.2', title: 'Added, +262.6 KiB', kind: 'add' },
      { text: 'nano 8.6', title: 'Removed, −2.8 MiB', kind: 'del' },
      { text: 'nginx 1.26.0 → 1.28.0', title: undefined, kind: 'up' },
    ])
    expect(by('Size')).toEqual([{ text: '1.27 GiB → 1.29 GiB (+18.4 MiB)', kind: 'up' }])
  })

  it('says which units restart or stop and which /etc files change, and the size change last', () => {
    expect(impactLine(detailed)).toBe('nginx 1.26.0 → 1.28.0 · nginx.service restarts · old.service stops · 3 /etc files change · +18.4 MiB')
    const two = { host: 'x', status: 'changed' as const, units: [{ name: 'a.service', action: 'restart' as const }, { name: 'b.service', action: 'restart' as const }] }
    expect(impactLine(two)).toBe('a.service and b.service restart')
    expect(impactLine({ host: 'x', status: 'changed', etc: [{ path: '/etc/hosts', status: 'changed' }] })).toBe('/etc/hosts changes')
    // Reloads and new units alone are not news in one line; the details list them.
    expect(impactLine({ host: 'x', status: 'changed', units: [{ name: 'a.service', action: 'reload' }] })).toBe('Other configuration')
    expect(impactLine({ host: 'x', status: 'changed', closure: { sizeFrom: 10, sizeTo: 10, changes: [] } })).toBe('Other configuration')
  })

  it('lists home-manager changes per user, and counts them in the line', () => {
    const h: HostDiff = {
      host: 'pc',
      status: 'changed',
      homePackagesAdded: ['pablo: ripgrep'],
      homePackagesRemoved: ['pablo: htop'],
      homeProgramsAdded: ['anna: starship'],
      homeFiles: [{ path: 'pablo: ~/.config/git/config', status: 'changed' }],
    }
    expect(impactLine(h)).toBe("anna and pablo's homes: 4 changes")
    expect(impactLine({ host: 'pc', status: 'changed', homePackagesAdded: ['pablo: fd'] })).toBe("pablo's home: 1 change")
    expect(impactGroups(h).map((g) => [g.label, g.items.map((i) => `${i.kind} ${i.text}`)])).toEqual([
      ['Home packages', ['add pablo: ripgrep', 'del pablo: htop']],
      ['Home programs', ['add anna: starship']],
      ['Home files', ['up pablo: ~/.config/git/config']],
    ])
  })

  it('keeps broken and unchanged machines apart from the changes', () => {
    const s = splitImpact([{ host: 'z', status: 'unchanged' }, changed, { host: 'b', status: 'error' }, { host: 'n', status: 'added' }, { host: 'r', status: 'removed' }])
    expect(s.changed.map((d) => d.host)).toEqual(['n', 'ws', 'r'])
    expect(s.broken.map((d) => d.host)).toEqual(['b'])
    expect(s.unchanged.map((d) => d.host)).toEqual(['z'])
  })

  it('measures how far an input moved', () => {
    expect(revDistance({ name: 'n', status: 'updated', fromDate: 0, toDate: 9 * 86400 })).toBe('')
    expect(revDistance({ name: 'n', status: 'updated', fromDate: 100, toDate: 100 + 9 * 86400 })).toBe('9 days newer')
    expect(revDistance({ name: 'n', status: 'updated', fromDate: 100 + 86400, toDate: 100 })).toBe('1 day older')
  })
})

describe('settings set here', () => {
  it('lists the leaf settings that change, lists as leaves', () => {
    const before = { services: { openssh: { settings: { PasswordAuthentication: true } } }, nix: { gc: { automatic: true } } }
    const after = { services: { openssh: { settings: { PasswordAuthentication: false } }, fail2ban: { enable: true } }, users: { pablo: { keys: ['a', 'b'] } } }
    expect(settingChanges(before, after)).toEqual([
      { path: ['nix', 'gc', 'automatic'], from: true, to: undefined },
      { path: ['services', 'fail2ban', 'enable'], from: undefined, to: true },
      { path: ['services', 'openssh', 'settings', 'PasswordAuthentication'], from: true, to: false },
      { path: ['users', 'pablo', 'keys'], from: undefined, to: ['a', 'b'] },
    ])
    expect(settingChanges({ a: [1] }, { a: [1] })).toEqual([])
    expect(settingChanges(undefined, {})).toEqual([])
  })

  it('turns an unchanged machine with new settings into a change, in few words', () => {
    const [d] = withSettings([{ host: 'ws', status: 'unchanged' }], { ws: [{ path: ['services', 'fail2ban', 'enable'], to: true }] })
    expect(d.status).toBe('changed')
    expect(impactLine(d)).toBe('fail2ban turned on')
    expect(impactGroups(d)[0]).toEqual({ label: 'Settings', items: [{ text: 'services.fail2ban.enable = true', title: 'Was not set here', kind: 'up' }] })
    const [e] = withSettings([{ host: 'ws', status: 'changed' }], { ws: [{ path: ['a', 'b'], from: 1 }, { path: ['nix', 'gc', 'dates'], to: 'weekly' }] })
    expect(impactLine(e)).toBe('2 settings changed')
    expect(impactGroups(e)[0].items.map((i) => i.text)).toEqual(['a.b reset', 'nix.gc.dates = "weekly"'])
    expect(withSettings([{ host: 'x', status: 'error' }], { x: [{ path: ['a'], to: 1 }] })[0].status).toBe('error')
  })

  it('shows values the way Nix writes them, shortened', () => {
    expect(nixValue(['a', 'b'])).toBe('[ "a" "b" ]')
    expect(nixValue('x'.repeat(60))).toHaveLength(48)
  })
})
