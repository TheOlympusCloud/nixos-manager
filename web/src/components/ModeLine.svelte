<script lang="ts">
  import type { Job } from '../lib/api'
  import { on, still, typed } from '../lib/ascii'
  import { health } from '../lib/health.svelte'
  import { app } from '../lib/state.svelte'
  import { statusLine, toasts } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  // The mode line: where you are, what the fleet's findings are, a ticker of what the server last did, the keys
  // that move you around, and the time.
  const repoName = $derived.by(() => {
    const remote = app.info?.remote.replace(/\.git$/, '').replace(/\/+$/, '') ?? ''
    const fromRemote = remote.split(/[/:]/).slice(-2).join('/')
    return fromRemote || app.info?.root.split('/').filter(Boolean).pop() || 'nixos-manager'
  })
  const branch = $derived(app.status?.branch || app.info?.branch || '')
  const count = (s: string) => health.insights?.filter((i) => i.severity === s).length ?? 0
  const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)

  const KIND: Record<string, string> = {
    check: 'check',
    lock: 'lock the flake',
    update: 'update inputs',
    'update-branch': 'scheduled update',
    model: 'evaluate the fleet',
    options: 'index the options',
    packages: 'index the packages',
    values: 'read option values',
    iso: 'installer ISO of',
  }
  function jobLine(j: Job) {
    const what = (KIND[j.kind] ?? j.kind) + (j.commit ? ' ' + j.commit.slice(0, 7) : j.targets?.length ? ' ' + j.targets.join(', ') : j.kind === 'iso' ? ' ' + j.host : '')
    const built = j.summary?.evaluatedOnly?.length ? ` · evaluated ${j.summary.evaluatedOnly.length}` : j.summary ? ` · built ${j.summary.built.length}` : ''
    return j.state === 'running' || j.state === 'queued' ? `${what}: ${j.state}…` : `${what} ${j.state}${built}`
  }
  const lastJob = $derived(app.jobs.length ? app.jobs.reduce((a, b) => ((b.finished || b.started || '') > (a.finished || a.started || '') ? b : a)) : null)
  const lastToast = $derived(toasts.at(-1))

  // Whatever spoke last takes the ticker.
  let line = $state('')
  $effect(() => {
    if (lastJob) line = jobLine(lastJob)
  })
  $effect(() => {
    if (lastToast) line = lastToast.title ? `${lastToast.title}: ${lastToast.text}` : lastToast.text
  })
  $effect(() => {
    if (statusLine.text) line = statusLine.text
  })

  const hhmm = () => new Date().toTimeString().slice(0, 5)
  let time = $state(hhmm())
  $effect(() => {
    if (!still) return on((t) => void (t % 12 === 0 && (time = hhmm())))
    // No motion: the time still has to be right, so it follows the minute instead of the clock.
    let id = 0
    const next = () => (id = window.setTimeout(() => ((time = hhmm()), next()), 60000 - (Date.now() % 60000)))
    next()
    return () => clearTimeout(id)
  })
</script>

<footer class="modeline">
  <b class="mode">λ {repoName}</b>
  {#if branch}<span><Icon name="branch" /> {branch}</span>{/if}
  {#if health.insights}
    <a class="counts" href="#/health" title="Errors, warnings and suggestions on Health">
      <i class="err">✖</i> {count('error')} <i class="warn">▲</i> {count('warning')} <i class="info">◆</i> {count('info')}
    </a>
  {/if}
  <span class="ticker" role="status">{#if line}› <span use:typed={line}></span>{/if}</span>
  <span class="keys"><kbd>{mac ? '⌘' : '^'}K</kbd> palette <kbd>g</kbd>+letter go <kbd>/</kbd> search</span>
  <span class="time num">{time}</span>
</footer>

<style>
  .modeline {
    display: flex;
    flex: none;
    gap: 3ch;
    height: var(--lh);
    padding: 0 1ch;
    overflow: hidden;
    color: var(--fg-muted);
    white-space: nowrap;
    background: var(--bg-hover);
  }
  .mode {
    margin-left: -1ch;
    padding: 0 1ch;
    color: var(--on-accent);
    background: var(--accent);
  }
  .counts {
    color: inherit;
  }
  .ticker {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg);
    text-overflow: ellipsis;
  }
  kbd {
    padding: 0;
    color: var(--fg-strong);
    background: none;
  }
  @media (max-width: 1180px) {
    .keys {
      display: none;
    }
  }
</style>
