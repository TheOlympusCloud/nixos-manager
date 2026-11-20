// Settings → Notifications: the form, and the settings it saves.
import type { EmailSecurity, NotifySettings } from './api'

export type NotifyForm = {
  consoleUrl: string
  ntfy: { on: boolean; url: string; token: string; hasToken: boolean }
  webhook: { on: boolean; url: string }
  email: { on: boolean; host: string; port: number; security: EmailSecurity; username: string; password: string; hasPassword: boolean; from: string; to: string }
}

export const defaultPort: Record<EmailSecurity, number> = { starttls: 587, tls: 465, none: 25 }

/** The form for saved settings; the console address defaults to the page's. */
export function formOf(s: NotifySettings, origin: string): NotifyForm {
  const e = s.email
  return {
    consoleUrl: s.consoleUrl || origin,
    ntfy: { on: !!s.ntfy, url: s.ntfy?.url ?? '', token: '', hasToken: !!s.ntfy?.hasToken },
    webhook: { on: !!s.webhook, url: s.webhook?.url ?? '' },
    email: {
      on: !!e,
      host: e?.host ?? '',
      port: e?.port ?? defaultPort.starttls,
      security: e?.security ?? 'starttls',
      username: e?.username ?? '',
      password: '',
      hasPassword: !!e?.hasPassword,
      from: e?.from ?? '',
      to: (e?.to ?? []).join(', '),
    },
  }
}

/** The settings a form saves: only the channels turned on, keeping saved secrets that were not retyped. */
export function settingsOf(f: NotifyForm): NotifySettings {
  const out: NotifySettings = { consoleUrl: f.consoleUrl.trim() }
  if (f.ntfy.on) out.ntfy = { url: f.ntfy.url.trim(), token: f.ntfy.token.trim(), hasToken: f.ntfy.hasToken }
  if (f.webhook.on) out.webhook = { url: f.webhook.url.trim() }
  if (f.email.on) {
    const e = f.email
    out.email = {
      host: e.host.trim(),
      port: Number(e.port),
      security: e.security,
      username: e.username.trim(),
      password: e.password,
      hasPassword: e.hasPassword,
      from: e.from.trim(),
      to: e.to
        .split(/[,;\s]+/)
        .map((a) => a.trim())
        .filter(Boolean),
    }
  }
  return out
}
