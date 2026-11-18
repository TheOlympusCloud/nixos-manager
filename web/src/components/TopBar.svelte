<script lang="ts">
  import { ApiError, api } from '../lib/api'
  import { app, fileTabs, logout, refresh, syncOpenTabs } from '../lib/state.svelte'
  import { alertBox, report, toast } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  let menuOpen = $state(false)
  let branches = $state<string[] | null>(null)
  let newBranch = $state('')
  let busy = $state('')
  let menuEl = $state<HTMLElement>()

  const status = $derived(app.status)

  async function toggleMenu() {
    menuOpen = !menuOpen
    if (!menuOpen) return
    branches = null
    try {
      branches = (await api.branches()).branches
    } catch (e) {
      menuOpen = false
      report(e)
    }
  }

  async function afterBranchChange() {
    menuOpen = false
    await Promise.all([refresh(), syncOpenTabs()])
    app.logVersion++
  }

  async function switchTo(name: string) {
    const dirty = fileTabs().filter((t) => t.dirty)
    if (dirty.length) return toast(`Save or close unsaved tabs before switching branch (${dirty.map((t) => t.path).join(', ')}).`, 'error')
    busy = 'switch'
    try {
      await api.switchBranch(name)
      toast(`Switched to ${name}`, 'ok')
      await afterBranchChange()
    } catch (e) {
      report(e, `Cannot switch to ${name}`)
    } finally {
      busy = ''
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault()
    const name = newBranch.trim()
    if (!name) return
    busy = 'branch'
    try {
      await api.createBranch(name)
      newBranch = ''
      toast(`Created and switched to ${name}`, 'ok')
      await afterBranchChange()
    } catch (err) {
      report(err, `Cannot create ${name}`)
    } finally {
      busy = ''
    }
  }

  async function git(kind: 'pull' | 'push') {
    busy = kind
    try {
      const out = await api[kind]()
      await alertBox({ title: `git ${kind}`, pre: out.trim() || '(no output)' })
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) await alertBox({ title: `git ${kind} failed`, pre: e instanceof Error ? e.message : String(e) })
    } finally {
      busy = ''
      await Promise.all([refresh(), kind === 'pull' && syncOpenTabs()])
      app.logVersion++
    }
  }

  function onWindowClick(e: MouseEvent) {
    if (menuOpen && menuEl && !menuEl.contains(e.target as Node)) menuOpen = false
  }
</script>

<svelte:window onclick={onWindowClick} onkeydown={(e) => e.key === 'Escape' && menuOpen && (menuOpen = false)} />

<header class="topbar">
  <span class="brand">nixos-manager</span>

  <div class="branch" bind:this={menuEl}>
    <button class="ghost" aria-haspopup="true" aria-expanded={menuOpen} onclick={toggleMenu} disabled={!!busy} title="Switch or create a branch">
      <Icon name="branch" size={14} />
      {status?.branch || app.info?.branch || '…'}
      <Icon name="down" size={12} />
    </button>
    {#if menuOpen}
      <div class="menu">
        <div class="menu-title">Local branches</div>
        {#if branches === null}
          <div class="muted pad">Loading…</div>
        {:else}
          <ul>
            {#each branches as b (b)}
              <li>
                <button class="ghost item" disabled={b === status?.branch || !!busy} aria-current={b === status?.branch} onclick={() => switchTo(b)}>
                  <span class="mark">{b === status?.branch ? '✓' : ''}</span>{b}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <form class="new" onsubmit={create}>
          <label for="new-branch">New branch from HEAD</label>
          <div class="row">
            <input id="new-branch" bind:value={newBranch} placeholder="branch-name" spellcheck="false" autocomplete="off" />
            <button type="submit" disabled={!newBranch.trim() || !!busy}>Create</button>
          </div>
        </form>
      </div>
    {/if}
  </div>

  {#if status?.upstream}
    <span class="sync" title="Tracking {status.upstream}">
      <span aria-label="{status.ahead} commits ahead">↑{status.ahead}</span>
      <span aria-label="{status.behind} commits behind">↓{status.behind}</span>
    </span>
  {:else if status}
    <span class="muted small">no upstream</span>
  {/if}

  <button onclick={() => git('pull')} disabled={!!busy}>{busy === 'pull' ? 'Pulling…' : 'Pull'}</button>
  <button onclick={() => git('push')} disabled={!!busy}>{busy === 'push' ? 'Pushing…' : 'Push'}</button>

  <span class="spacer"></span>
  {#if app.info?.author}<span class="muted small author" title="Commit author">{app.info.author}</span>{/if}
  <button class="ghost" onclick={() => (app.tokensOpen = true)}>Tokens</button>
  <button class="ghost" onclick={logout}>Log out</button>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 38px;
    padding: 0 10px;
    background: var(--bg-deep);
    border-bottom: 1px solid var(--border);
  }
  .brand {
    margin-right: 6px;
    font-weight: 600;
    letter-spacing: 0.02em;
  }
  .branch {
    position: relative;
  }
  .branch > button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-family: var(--mono);
    font-size: 12.5px;
  }
  .menu {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 30;
    width: 280px;
    padding: 6px;
    background: var(--bg-raised);
    border: 1px solid var(--line);
    border-radius: 6px;
    box-shadow: 0 8px 28px rgb(0 0 0 / 0.45);
  }
  .menu-title {
    padding: 4px 6px;
    color: var(--fg-muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  ul {
    max-height: 40vh;
    margin: 0;
    padding: 0;
    overflow: auto;
    list-style: none;
  }
  .item {
    display: flex;
    width: 100%;
    gap: 6px;
    text-align: left;
    font-family: var(--mono);
    font-size: 12.5px;
  }
  .item:disabled {
    opacity: 1;
    color: var(--fg);
  }
  .mark {
    width: 1em;
    color: var(--green);
  }
  .pad {
    padding: 6px;
  }
  .new {
    margin-top: 6px;
    padding: 8px 6px 2px;
    border-top: 1px solid var(--line);
  }
  .new label {
    display: block;
    margin-bottom: 4px;
    color: var(--fg-muted);
    font-size: 12px;
  }
  .row {
    display: flex;
    gap: 6px;
  }
  .row input {
    flex: 1;
    min-width: 0;
  }
  .sync {
    display: inline-flex;
    gap: 8px;
    font-family: var(--mono);
    font-size: 12px;
    color: var(--fg-muted);
  }
  .spacer {
    flex: 1;
  }
  .small {
    font-size: 12px;
  }
  .author {
    max-width: 30ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
