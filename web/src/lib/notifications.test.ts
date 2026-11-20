import { describe, expect, it } from 'vitest'
import { formOf, settingsOf } from './notifications'

describe('notifications', () => {
  it('fills the form from saved settings, secrets left out', () => {
    const f = formOf(
      {
        ntfy: { url: 'https://ntfy.sh/fleet', hasToken: true },
        email: { host: 'smtp.example.org', port: 465, security: 'tls', username: 'pablo', hasPassword: true, from: 'fleet@example.org', to: ['a@example.org', 'b@example.org'] },
      },
      'http://manager.lan:8080',
    )
    expect(f.consoleUrl).toBe('http://manager.lan:8080')
    expect(f.ntfy).toEqual({ on: true, url: 'https://ntfy.sh/fleet', token: '', hasToken: true })
    expect(f.webhook.on).toBe(false)
    expect(f.email).toMatchObject({ on: true, port: 465, security: 'tls', password: '', hasPassword: true, to: 'a@example.org, b@example.org' })
    expect(formOf({}, 'http://x').email).toMatchObject({ on: false, port: 587, security: 'starttls' })
  })

  it('saves the channels turned on, and keeps secrets that were not retyped', () => {
    const f = formOf({ ntfy: { url: 'https://ntfy.sh/fleet', hasToken: true }, webhook: { url: 'https://hook' } }, 'http://x')
    f.webhook.on = false
    f.email = { ...f.email, on: true, host: ' smtp ', username: 'u', password: 'p', from: 'f@x', to: 'a@x; b@x\nc@x, ' }
    expect(settingsOf(f)).toEqual({
      consoleUrl: 'http://x',
      ntfy: { url: 'https://ntfy.sh/fleet', token: '', hasToken: true },
      email: { host: 'smtp', port: 587, security: 'starttls', username: 'u', password: 'p', hasPassword: false, from: 'f@x', to: ['a@x', 'b@x', 'c@x'] },
    })
    f.ntfy.hasToken = false // removed
    expect(settingsOf(f).ntfy).toEqual({ url: 'https://ntfy.sh/fleet', token: '', hasToken: false })
  })
})
