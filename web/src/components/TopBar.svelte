<script lang="ts">
  import { api } from '../lib/api'
  import { frame, typed } from '../lib/ascii'
  import { fleet, healthy } from '../lib/fleet.svelte'
  import { link, machineLink, route } from '../lib/router.svelte'
  import { mergeBranch, noRemote, pullOrPush, repo } from '../lib/repo.svelte'
  import { app, fileTabs, logout, refresh, syncOpenTabs } from '../lib/state.svelte'
  import { report, toast } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  let menu = $state<'' | 'branch' | 'user'>('')
  let branches = $state<string[] | null>(null)
  let newBranch = $state('')
  let branchEl = $state<HTMLElement>()
  let userEl = $state<HTMLElement>()

  const status = $derived(app.status)
  const current = $derived(status?.branch || app.info?.branch || '')
  const busy = $derived(repo.busy)
  const titles: Record<string, string> = {
    fleet: 'Fleet',
    machines: 'Machines',
    packages: 'Packages',
    services: 'Services',
    options: 'Options',
    health: 'Health',
    deployments: 'Deployments',
    inputs: 'Inputs',
    changes: 'Changes',
    history: 'History',
    code: 'Code',
    settings: 'Settings',
  }
  const crumb = $derived(route.view === 'machines' && route.parts[0] ? route.parts[0] : route.view === 'options' && route.parts[0] ? route.parts[0] : '')
  // The title is typed into the bar as you arrive: λ Machines › workstation.
  const where = $derived((titles[route.view] ?? '') + (crumb ? ' › ' + crumb : ''))

  /** The evaluation state; a click goes where the problem is. */
  const model = $derived.by(() => {
    const m = fleet.model
    if (!m && fleet.loadError) return { cls: 'err', text: 'Model unavailable', title: fleet.loadError, href: '#/fleet' }
    if (!m) return { cls: 'busy', text: 'Evaluating…', title: 'Nix is evaluating every machine of the flake', href: '#/fleet' }
    if (m.error) return { cls: 'err', text: /is Nix installed|cannot run nix/i.test(m.error) ? 'Nix not installed' : 'Flake does not evaluate', title: m.error, href: '#/fleet' }
    const n = m.hosts.length
    const broken = m.hosts.filter((h) => h.error)
    const busy = fleet.computing ? ' · re-evaluating' : ''
    if (broken.length) {
      const one = broken.length === 1 ? broken[0].name : ''
      return {
        cls: 'warn',
        text: `${healthy().length}/${n} OK${busy}`,
        title: `${broken.map((h) => h.name).join(', ')} ${broken.length === 1 ? 'does' : 'do'} not evaluate. ${one ? `Open ${one}.` : 'Open Health.'}`,
        href: one ? machineLink(one) : link('health'),
      }
    }
    return { cls: fleet.computing ? 'busy' : 'ok', text: `${n}/${n} OK${busy}`, title: `Every machine evaluates (working copy tree ${m.tree.slice(0, 12)}). Open Health.`, href: link('health') }
  })

  const initials = $derived(
    (app.info?.author.replace(/<.*>/, '').trim() || 'User')
      .split(/\s+/)
      .slice(0, 2)
      .map((w) => w[0]?.toUpperCase())
      .join(''),
  )
  const remoteMissing = $derived(noRemote())

  async function openBranches() {
    menu = menu === 'branch' ? '' : 'branch'
    if (menu !== 'branch') return
    branches = null
    try {
      branches = (await api.branches()).branches
    } catch (e) {
      menu = ''
      report(e)
    }
  }

  async function afterBranchChange() {
    menu = ''
    await Promise.all([refresh(), syncOpenTabs()])
    app.logVersion++
  }

  async function switchTo(name: string) {
    const dirty = fileTabs().filter((t) => t.dirty)
    if (dirty.length) return toast(`Save or close unsaved files before switching branch (${dirty.map((t) => t.path).join(', ')}).`, 'error')
    repo.busy = 'switch'
    try {
      await api.switchBranch(name)
      toast(`Switched to ${name}`, 'ok')
      await afterBranchChange()
    } catch (e) {
      report(e, `Cannot switch to ${name}`)
    } finally {
      repo.busy = ''
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault()
    const name = newBranch.trim()
    if (!name) return
    repo.busy = 'branch'
    try {
      await api.createBranch(name)
      newBranch = ''
      toast(`Created and switched to ${name}`, 'ok')
      await afterBranchChange()
    } catch (err) {
      report(err, `Cannot create ${name}`)
    } finally {
      repo.busy = ''
    }
  }

  function merge(b: string) {
    menu = ''
    mergeBranch(b, current)
  }

  // Pushes made elsewhere (the Forgejo webhook fetches them) show up here without a reload.
  $effect(() => {
    const id = setInterval(poll, 15000)
    return () => clearInterval(id)
  })
  async function poll() {
    const before = app.status
    if (document.visibilityState !== 'visible' || busy || !before || app.needLogin) return
    const now = await api.status().catch(() => null)
    if (!now || app.status !== before) return // an action refreshed it meanwhile
    if (now.staged_tree !== before.staged_tree || now.branch !== before.branch) {
      await Promise.all([refresh(), syncOpenTabs()]) // the files moved: a fast-forward, or a client of the API
      app.logVersion++
    } else if (now.behind !== before.behind || now.ahead !== before.ahead || now.upstream !== before.upstream) app.status = now
  }
  const incoming = $derived(status?.upstream && status.behind > 0 ? status.behind : 0)

  function onWindowClick(e: MouseEvent) {
    const el = menu === 'branch' ? branchEl : menu === 'user' ? userEl : null
    if (el && !el.contains(e.target as Node)) menu = ''
  }
</script>

<svelte:window onclick={onWindowClick} onkeydown={(e) => e.key === 'Escape' && menu && (menu = '')} />

<header class="topbar" use:frame={{ animate: true }}>
  <div class="where">
    <span class="prompt" aria-hidden="true">λ</span>
    <span class="sr-only">{where}</span>
    <b class="title" aria-hidden="true" use:typed={where}></b>
  </div>

  <span class="spacer"></span>

  <a class="model {model.cls}" href={model.href} title={model.title} aria-live="polite">
    <span class="dot {model.cls}"></span> {model.text}
  </a>

  <i class="sep" aria-hidden="true">│</i>

  <div class="pop" bind:this={branchEl}>
    <button class="bare branch" aria-haspopup="true" aria-expanded={menu === 'branch'} onclick={openBranches} disabled={!!busy} title="Switch, create or merge branches">
      <Icon name="branch" />
      <span>{current || '…'}</span>
      <Icon name="down" />
    </button>
    {#if menu === 'branch'}
      <div class="menu right branch-menu" role="menu" use:frame={{ kind: 'square', shadow: true, animate: true, title: 'Local branches' }}>
        {#if branches === null}
          <span class="skel"></span>
        {:else}
          <ul>
            {#each branches as b (b)}
              <li class="b-row">
                <button class="bare menu-item" role="menuitem" disabled={b === current || !!busy} aria-current={b === current} title={b === current ? 'The current branch' : `Switch to ${b}`} onclick={() => switchTo(b)}>
                  <span class="mark">{#if b === current}<Icon name="tick" />{/if}</span><span class="b-name">{b}</span>
                </button>
                {#if b !== current}
                  <button class="merge" role="menuitem" disabled={!!busy} title="Stage everything {b} changes on {current}, as one change to review and commit" onclick={() => merge(b)}>
                    Merge into {current}
                  </button>
                {/if}
              </li>
            {/each}
          </ul>
          {#if branches.length > 1}<p class="menu-hint">Merging stages the branch as one change on {current}; you review, check and commit it in Changes.</p>{/if}
        {/if}
        <div class="rule" aria-hidden="true"></div>
        <form class="new" onsubmit={create}>
          <label for="new-branch" class="label">New branch from HEAD</label>
          <div class="row">
            <input id="new-branch" bind:value={newBranch} placeholder="branch-name" spellcheck="false" autocomplete="off" />
            <button type="submit" disabled={!newBranch.trim() || !!busy}>Create</button>
          </div>
        </form>
      </div>
    {/if}
  </div>

  {#if incoming}
    <button class="bare incoming" onclick={() => pullOrPush('pull')} disabled={!!busy} aria-label="{incoming} new on Forgejo: pull" title="Forgejo has {incoming} commit{incoming === 1 ? '' : 's'} this console does not have yet. Pull brings {incoming === 1 ? 'it' : 'them'} in.">
      <Icon name="arrow-down" /><span class="num">{incoming}</span> <span class="lbl">new on Forgejo</span>
    </button>
  {/if}
  {#if status?.upstream}
    <span class="sync num" class:quiet={incoming && !status.ahead} title="Tracking {status.upstream}: {status.ahead} ahead, {status.behind} behind">
      <span><Icon name="up" />{status.ahead}</span>
      <span><Icon name="arrow-down" />{status.behind}</span>
    </span>
  {:else if status && !remoteMissing}
    <span class="muted" title="This branch has no remote branch yet; Push creates it">Not pushed yet</span>
  {/if}

  <span class="pair">
    <button class:off={!!remoteMissing} class:twin={!!incoming} aria-disabled={!!remoteMissing || undefined} onclick={() => pullOrPush('pull')} disabled={!!busy} title={remoteMissing || 'Pull from the remote'}>
      <Icon name="pull" /> <span class="lbl">{busy === 'pull' ? 'Pulling…' : 'Pull'}</span>
    </button>
    <button class:off={!!remoteMissing} aria-disabled={!!remoteMissing || undefined} onclick={() => pullOrPush('push')} disabled={!!busy} title={remoteMissing || 'Push to the remote; the machines deploy what is pushed'}>
      <Icon name="push" /> <span class="lbl">{busy === 'push' ? 'Pushing…' : 'Push'}</span>
    </button>
  </span>

  <i class="sep" aria-hidden="true">│</i>

  <div class="pop" bind:this={userEl}>
    <button class="bare user" aria-haspopup="true" aria-expanded={menu === 'user'} aria-label="Account menu" onclick={() => (menu = menu === 'user' ? '' : 'user')}>
      <span class="count">{initials}</span>
      <Icon name="down" />
    </button>
    {#if menu === 'user'}
      <div class="menu right" role="menu" use:frame={{ kind: 'square', shadow: true, animate: true }}>
        {#if app.info?.author}<div class="menu-title who" title="Commits are made as this author">{app.info.author}</div><div class="rule" aria-hidden="true"></div>{/if}
        <a class="menu-item" role="menuitem" href="#/settings" onclick={() => (menu = '')}><Icon name="key" /> Tokens and MCP</a>
        <button class="bare menu-item" role="menuitem" onclick={() => ((menu = ''), logout())}><Icon name="logout" /> Log out</button>
      </div>
    {/if}
  </div>
</header>

<style>
  .topbar {
    position: relative;
    z-index: 5; /* its menus open over the page */
    display: flex;
    flex: none;
    align-items: baseline;
    gap: 2ch;
    white-space: nowrap;
  }
  .where {
    display: flex;
    gap: 1ch;
    min-width: 0;
    overflow: hidden;
  }
  .title {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .model {
    color: var(--fg);
  }
  .model.warn {
    color: var(--yellow);
  }
  .model.warn .dot {
    --c: var(--yellow);
  }
  .model.err {
    color: var(--red);
  }
  .model:hover,
  .model:hover .dot {
    color: var(--on-accent);
  }
  .sep {
    color: var(--line);
  }
  .pop {
    position: relative;
  }
  .branch,
  .user,
  .incoming {
    color: var(--fg);
    font-weight: 400;
  }
  .incoming {
    color: var(--accent);
  }
  .sync {
    display: inline-flex;
    gap: 1ch;
    color: var(--fg-muted);
  }
  .sync.quiet {
    display: none; /* "N new on Forgejo" already says it */
  }
  .pair {
    display: inline-flex;
    gap: 1ch;
  }
  .off {
    color: var(--fg-faint);
    cursor: not-allowed;
  }
  .user .count {
    min-width: 3ch;
  }
  .menu {
    position: absolute;
    top: calc(var(--lh) * 2);
    left: 0;
    z-index: 50;
    width: 40ch;
    white-space: normal;
    background: var(--bg-raised);
  }
  .menu.right {
    right: -2ch;
    left: auto;
    width: 32ch;
  }
  .menu.branch-menu {
    width: 48ch;
  }
  .menu-title {
    color: var(--fg-muted);
  }
  .who {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  ul {
    max-height: 40vh;
    margin: 0;
    padding: 0;
    overflow: auto;
    list-style: none;
  }
  .menu-item {
    display: flex;
    gap: 1ch;
    width: 100%;
    padding: 0 1ch;
    color: var(--fg);
    font-weight: 400;
    text-align: left;
  }
  .menu-item:hover:not(:disabled) {
    color: var(--fg-strong);
    background: var(--bg-hover);
  }
  .menu-item:disabled {
    color: var(--fg-strong);
    font-weight: 700;
    cursor: default;
  }
  .mark {
    display: inline-block;
    width: 1ch;
    color: var(--green);
  }
  .b-row {
    display: flex;
    gap: 1ch;
  }
  .b-row .menu-item {
    flex: 1;
    min-width: 0;
  }
  .b-name {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .merge {
    flex: none;
    color: var(--fg-muted);
    font-weight: 400;
  }
  .menu-hint {
    padding: 0 1ch;
    color: var(--fg-faint);
  }
  .new label {
    display: block;
  }
  .row {
    display: flex;
    gap: 1ch;
  }
  .row input {
    flex: 1;
    min-width: 0;
  }
  @media (max-width: 1180px) {
    .sync,
    .lbl {
      display: none;
    }
  }
  /* Phones: the rail already shows where you are. */
  @media (max-width: 640px) {
    .where,
    .sep {
      display: none;
    }
    .topbar {
      gap: 1ch;
    }
    .twin {
      display: none; /* the "new on Forgejo" button pulls too */
    }
  }
</style>
