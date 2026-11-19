<script lang="ts">
  import { api } from '../lib/api'
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
    inputs: 'Inputs',
    changes: 'Changes',
    history: 'History',
    code: 'Code',
    settings: 'Settings',
  }
  const crumb = $derived(route.view === 'machines' && route.parts[0] ? route.parts[0] : route.view === 'options' && route.parts[0] ? route.parts[0] : '')

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

  function onWindowClick(e: MouseEvent) {
    const el = menu === 'branch' ? branchEl : menu === 'user' ? userEl : null
    if (el && !el.contains(e.target as Node)) menu = ''
  }
</script>

<svelte:window onclick={onWindowClick} onkeydown={(e) => e.key === 'Escape' && menu && (menu = '')} />

<header class="topbar">
  <div class="where">
    <span class="title">{titles[route.view]}</span>
    {#if crumb}<Icon name="right" size={14} /><span class="crumb">{crumb}</span>{/if}
  </div>

  <span class="spacer"></span>

  <a class="badge model {model.cls}" href={model.href} title={model.title} aria-live="polite">
    <span class="dot {model.cls}"></span>{model.text}
  </a>

  <span class="sep" aria-hidden="true"></span>

  <div class="pop" bind:this={branchEl}>
    <button class="ghost branch" aria-haspopup="true" aria-expanded={menu === 'branch'} onclick={openBranches} disabled={!!busy} title="Switch, create or merge branches">
      <Icon name="branch" size={15} />
      <span class="mono">{current || '…'}</span>
      <Icon name="down" size={12} />
    </button>
    {#if menu === 'branch'}
      <div class="menu right branch-menu" role="menu">
        <div class="menu-title">Local branches</div>
        {#if branches === null}
          <div class="pad"><span class="skel" style:width="60%"></span></div>
        {:else}
          <ul>
            {#each branches as b (b)}
              <li class="b-row">
                <button class="ghost menu-item mono" role="menuitem" disabled={b === current || !!busy} aria-current={b === current} title={b === current ? 'The current branch' : `Switch to ${b}`} onclick={() => switchTo(b)}>
                  <span class="mark">{#if b === current}<Icon name="tick" size={14} />{/if}</span><span class="b-name">{b}</span>
                </button>
                {#if b !== current}
                  <button class="small merge" role="menuitem" disabled={!!busy} title="Stage everything {b} changes on {current}, as one change to review and commit" onclick={() => merge(b)}>
                    Merge into {current}
                  </button>
                {/if}
              </li>
            {/each}
          </ul>
          {#if branches.length > 1}<p class="menu-hint">Merging stages the branch as one change on {current}; you review, check and commit it in Changes.</p>{/if}
        {/if}
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

  {#if status?.upstream}
    <span class="sync num" title="Tracking {status.upstream}: {status.ahead} ahead, {status.behind} behind">
      <span><Icon name="up" size={13} />{status.ahead}</span>
      <span><Icon name="arrow-down" size={13} />{status.behind}</span>
    </span>
  {:else if status && !remoteMissing}
    <span class="muted small" title="This branch has no remote branch yet; Push creates it">Not pushed yet</span>
  {/if}

  <button class="ghost" class:off={!!remoteMissing} aria-disabled={!!remoteMissing || undefined} onclick={() => pullOrPush('pull')} disabled={!!busy} title={remoteMissing || 'Pull from the remote'}>
    <Icon name="pull" size={15} /><span class="lbl">{busy === 'pull' ? 'Pulling…' : 'Pull'}</span>
  </button>
  <button class="ghost" class:off={!!remoteMissing} aria-disabled={!!remoteMissing || undefined} onclick={() => pullOrPush('push')} disabled={!!busy} title={remoteMissing || 'Push to the remote; the machines deploy what is pushed'}>
    <Icon name="push" size={15} /><span class="lbl">{busy === 'push' ? 'Pushing…' : 'Push'}</span>
  </button>

  <span class="sep" aria-hidden="true"></span>

  <div class="pop" bind:this={userEl}>
    <button class="ghost user" aria-haspopup="true" aria-expanded={menu === 'user'} aria-label="Account menu" onclick={() => (menu = menu === 'user' ? '' : 'user')}>
      <span class="avatar">{initials}</span>
      <Icon name="down" size={12} />
    </button>
    {#if menu === 'user'}
      <div class="menu right" role="menu">
        {#if app.info?.author}<div class="menu-title who" title="Commits are made as this author">{app.info.author}</div>{/if}
        <a class="menu-item" role="menuitem" href="#/settings" onclick={() => (menu = '')}><Icon name="key" size={15} />Tokens and MCP</a>
        <button class="ghost menu-item" role="menuitem" onclick={() => ((menu = ''), logout())}><Icon name="logout" size={15} />Log out</button>
      </div>
    {/if}
  </div>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: var(--s2);
    height: 48px;
    padding: 0 var(--s3) 0 var(--s4);
    background: var(--bg-deep);
    border-bottom: 1px solid var(--border);
  }
  .where {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    color: var(--fg-muted);
  }
  .title {
    color: var(--fg-strong);
    font-size: var(--fs-lg);
    font-weight: 600;
  }
  .crumb {
    overflow: hidden;
    color: var(--fg);
    font-size: var(--fs-lg);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .model {
    height: 26px;
    padding: 0 10px;
    gap: 8px;
    font-size: var(--fs-sm);
    border-radius: 13px;
  }
  .model {
    text-decoration: none;
    transition:
      background var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease);
  }
  .model.ok {
    color: var(--fg);
    background: var(--bg-raised);
    border-color: var(--line);
  }
  .model.ok:hover {
    border-color: var(--line-strong);
  }
  .model.warn {
    color: var(--yellow);
    background: color-mix(in srgb, var(--yellow) 10%, transparent);
    border-color: color-mix(in srgb, var(--yellow) 32%, transparent);
  }
  .model.warn .dot {
    --c: var(--yellow);
  }
  .model.warn:hover,
  .model.err:hover {
    background: color-mix(in srgb, currentColor 16%, transparent);
  }
  .off {
    color: var(--fg-faint);
    cursor: not-allowed;
  }
  .off:hover:not(:disabled) {
    color: var(--fg-muted);
  }
  .sep {
    width: 1px;
    height: 20px;
    margin: 0 var(--s1);
    background: var(--line);
  }
  .pop {
    position: relative;
  }
  .branch .mono {
    font-size: 12.5px;
  }
  .sync {
    display: inline-flex;
    gap: 10px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .sync span {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .user {
    padding: 0 6px 0 4px;
  }
  .avatar {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    color: var(--fg-strong);
    font-size: var(--fs-xs);
    font-weight: 600;
    background: color-mix(in srgb, var(--accent-strong) 55%, var(--bg-active));
    border-radius: 50%;
  }
  .menu {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    z-index: 50;
    width: 340px;
    padding: 6px;
    background: var(--bg-raised);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-2);
    animation: drop var(--t) var(--ease);
  }
  .menu.right {
    right: 0;
    left: auto;
    width: 240px;
  }
  .menu.branch-menu {
    width: 340px;
  }
  @keyframes drop {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
  }
  .menu-title {
    padding: 6px 8px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .who {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    border-bottom: 1px solid var(--border);
    margin-bottom: 4px;
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
    justify-content: flex-start;
    gap: 8px;
    width: 100%;
    height: 30px;
    padding: 0 8px;
    color: var(--fg);
    font-size: var(--fs-md);
    font-weight: 400;
    text-align: left;
    text-decoration: none;
    border-radius: var(--r);
  }
  a.menu-item {
    align-items: center;
  }
  a.menu-item:hover {
    background: var(--bg-hover);
  }
  .menu-item.mono {
    font-size: 12.5px;
  }
  .menu-item:disabled {
    opacity: 1;
    color: var(--fg-strong);
  }
  .mark {
    display: inline-grid;
    width: 16px;
    color: var(--green);
  }
  .pad {
    padding: 10px 8px;
  }
  .b-row {
    display: flex;
    align-items: center;
    gap: 4px;
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
    margin-right: 2px;
    color: var(--fg-muted);
    font-weight: 500;
  }
  .merge:hover:not(:disabled) {
    color: var(--fg-strong);
  }
  .menu-hint {
    margin: 6px 8px 2px;
    color: var(--fg-faint);
    font-size: var(--fs-xs);
    line-height: 1.45;
  }
  .new {
    margin-top: 6px;
    padding: 10px 6px 4px;
    border-top: 1px solid var(--border);
  }
  .new label {
    display: block;
    margin-bottom: 6px;
  }
  .row {
    display: flex;
    gap: 6px;
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
</style>
