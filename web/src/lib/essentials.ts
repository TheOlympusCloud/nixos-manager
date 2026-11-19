// The settings most machines need, as one curated page.

export type EssentialKind = 'switch' | 'tristate' | 'select' | 'text' | 'retention'
export type Essential = {
  option: string
  label: string
  help: string
  kind: EssentialKind
  choices?: string[]
  /** Offered in the input's list; other values are allowed. */
  suggest?: () => string[]
  /** For text: a check of the typed value ('' when fine). */
  check?: (v: string) => string
  /** Shown only when this other option is on. */
  when?: string
  /** The opposite reads better (PasswordAuthentication false = "off" is safer). */
  danger?: unknown
}

/** IANA time zones from the browser, UTC first. */
export function timeZones(): string[] {
  const all = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []
  return ['UTC', ...all.filter((z) => z !== 'UTC')]
}

const locales = ['en_US.UTF-8', 'en_GB.UTF-8', 'es_ES.UTF-8', 'ca_ES.UTF-8', 'de_DE.UTF-8', 'fr_FR.UTF-8', 'it_IT.UTF-8', 'nl_NL.UTF-8', 'pt_PT.UTF-8', 'pt_BR.UTF-8', 'pl_PL.UTF-8', 'sv_SE.UTF-8', 'ja_JP.UTF-8', 'C.UTF-8']
const keymaps = ['us', 'uk', 'es', 'de', 'de-latin1', 'fr', 'it', 'pt-latin1', 'br-abnt2', 'dvorak', 'colemak', 'sv-latin1', 'no', 'dk', 'fi', 'pl']
const schedules = ['daily', 'weekly', 'monthly', 'Sun 03:00', '*-*-* 04:00']

const noSpaces = (v: string) => (/\s/.test(v.trim()) ? 'No spaces.' : '')

export const essentials: { title: string; note: string; items: Essential[] }[] = [
  {
    title: 'Region',
    note: 'Clock, language and keyboard.',
    items: [
      {
        option: 'time.timeZone',
        label: 'Time zone',
        help: 'Used by the clock, logs and timers.',
        kind: 'text',
        suggest: timeZones,
        check: (v) => {
          const zones = timeZones()
          return zones.length > 1 && !zones.includes(v.trim()) ? 'Pick a zone from the list, e.g. Europe/Madrid.' : noSpaces(v)
        },
      },
      {
        option: 'i18n.defaultLocale',
        label: 'Language and formats',
        help: 'The default locale: messages, dates and numbers.',
        kind: 'text',
        suggest: () => locales,
        check: (v) => (/^[A-Za-z]+(_[A-Za-z]+)?(\.[A-Za-z0-9-]+)?(@\w+)?$/.test(v.trim()) ? '' : 'A locale looks like en_US.UTF-8.'),
      },
      { option: 'console.keyMap', label: 'Console keyboard', help: 'The keyboard layout on the text console.', kind: 'text', suggest: () => keymaps, check: noSpaces },
    ],
  },
  {
    title: 'Maintenance',
    note: 'Keep the Nix store from filling the disk.',
    items: [
      { option: 'nix.gc.automatic', label: 'Automatic garbage collection', help: 'Delete store paths no generation uses any more.', kind: 'switch' },
      { option: 'nix.gc.dates', label: 'Schedule', help: 'When it runs: a systemd calendar time.', kind: 'text', suggest: () => schedules, when: 'nix.gc.automatic', check: (v) => (v.trim() ? '' : 'Enter a schedule, e.g. weekly.') },
      { option: 'nix.gc.options', label: 'Keep generations for', help: 'Older system generations are deleted, so rollback reaches back this far.', kind: 'retention', when: 'nix.gc.automatic' },
      { option: 'nix.optimise.automatic', label: 'Store optimisation', help: 'Deduplicate identical files in the store on a schedule.', kind: 'switch' },
    ],
  },
  {
    title: 'Memory and network',
    note: 'Swap in RAM and the firewall.',
    items: [
      { option: 'zramSwap.enable', label: 'Compressed swap in RAM (zram)', help: 'More usable memory for little CPU; good on small machines and desktops.', kind: 'switch' },
      { option: 'networking.firewall.enable', label: 'Firewall', help: 'Only listed ports accept connections. See Network for the open ports.', kind: 'switch', danger: false },
    ],
  },
  {
    title: 'SSH',
    note: 'Remote logins.',
    items: [
      { option: 'services.openssh.enable', label: 'SSH server', help: 'Accept remote logins with OpenSSH.', kind: 'switch' },
      {
        option: 'services.openssh.settings.PasswordAuthentication',
        label: 'Password logins',
        help: 'Off means keys only, which stops password guessing. Add your key in Users first.',
        kind: 'tristate',
        when: 'services.openssh.enable',
        danger: true,
      },
      {
        option: 'services.openssh.settings.PermitRootLogin',
        label: 'Root login',
        help: 'Whether root can log in over SSH.',
        kind: 'select',
        choices: ['no', 'prohibit-password', 'forced-commands-only', 'without-password', 'yes'],
        when: 'services.openssh.enable',
        danger: 'yes',
      },
    ],
  },
  {
    title: 'Protection and VPN',
    note: 'Ban attackers and join your tailnet.',
    items: [
      { option: 'services.fail2ban.enable', label: 'fail2ban', help: 'Ban addresses after repeated failed logins.', kind: 'switch' },
      { option: 'services.tailscale.enable', label: 'Tailscale', help: 'Join your tailnet; log in on the machine once with tailscale up.', kind: 'switch' },
    ],
  },
]

export const essentialOptions = essentials.flatMap((g) => g.items.map((i) => i.option))

/** "--delete-older-than 14d" → 14; null when the options say something else. */
export function retentionDays(options: unknown): number | null {
  if (typeof options !== 'string') return null
  const m = /--delete-older-than\s+(\d+)d\b/.exec(options)
  return m ? Number(m[1]) : null
}

/** The gc options with the retention set to `days`, other flags kept. */
export function withRetention(options: unknown, days: number): string {
  const flag = `--delete-older-than ${days}d`
  const s = typeof options === 'string' ? options.trim() : ''
  return /--delete-older-than\s+\S+/.test(s) ? s.replace(/--delete-older-than\s+\S+/, flag) : [s, flag].filter(Boolean).join(' ')
}
