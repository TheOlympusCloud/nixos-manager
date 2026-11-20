<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../lib/api'
  import { checkTitle, forgejoApi, forgetHostKey, hookTarget, type Check, type ForgejoState } from '../lib/forgejo'
  import { pullOrPush, repo } from '../lib/repo.svelte'
  import { app, refresh, syncOpenTabs } from '../lib/state.svelte'
  import { alertBox, confirmBox, report, toast } from '../lib/ui.svelte'
  import { ago, copyText } from '../lib/util'
  import Icon from './Icon.svelte'

  /** Settings → Forgejo: the remote, the deploy key, the connection test and the push webhook, as numbered steps. */
  let st = $state<ForgejoState | null>(null)
  let loadError = $state('')
  let url = $state('')
  let urlError = $state('')
  let check = $state<Check | null>(null)
  let busy = $state<'' | 'remote' | 'test' | 'key' | 'adopt' | 'web' | 'status'>('')
  let copied = $state('')
  let showSecret = $state(false)
  let editWeb = $state(false)
  let webBase = $state('')
  let webError = $state('')
  let token = $state('')
  let tokenError = $state('')

  const hook = hookTarget(location.origin)
  const ssh = $derived(st?.kind === 'ssh' || (!st?.remote && st?.kind === ''))
  const tone = $derived(check?.next === 'unrelated' ? 'warn' : check ? checkTitle[check.result][1] : 'off')
  const title = $derived(check?.next === 'unrelated' ? 'Connected · unrelated history' : check ? checkTitle[check.result][0] : '')
  const forget = $derived(st && check?.result === 'host_key_changed' ? forgetHostKey(st.remote, st.known_hosts) : '')

  async function load() {
    try {
      st = await forgejoApi.state()
      loadError = ''
      url ||= st.remote
    } catch (e) {
      loadError = e instanceof Error ? e.message : String(e)
    }
  }

  async function test() {
    busy = 'test'
    try {
      check = await forgejoApi.test()
      if (st && check.host_key) st.host_key = check.host_key
    } catch (e) {
      report(e, 'Test failed')
    } finally {
      busy = ''
    }
    refresh() // the test fetches: ahead and behind may have moved
  }

  onMount(async () => {
    await load()
    if (st?.remote) test()
  })

  async function saveRemote(e: SubmitEvent) {
    e.preventDefault()
    busy = 'remote'
    urlError = ''
    try {
      st = await forgejoApi.setRemote(url.trim())
      url = st.remote
      app.info = await api.info()
    } catch (err) {
      urlError = err instanceof Error ? err.message : String(err)
      return
    } finally {
      busy = ''
    }
    check = null
    test()
  }

  async function createKey() {
    busy = 'key'
    try {
      st = await forgejoApi.createKey()
    } catch (e) {
      report(e, 'Cannot create the key')
    } finally {
      busy = ''
    }
  }

  async function adopt() {
    const ok = await confirmBox({
      title: 'Use the Forgejo repository?',
      message:
        'The starter files in this working copy are replaced by the repository on Forgejo. Nothing was committed here, so no history is lost; the replaced files are kept in git (refs/nixmgr/starter-backup) in case you need them.',
      ok: 'Use the Forgejo repository',
    })
    if (!ok) return
    busy = 'adopt'
    try {
      const out = await forgejoApi.adopt()
      toast(out, 'ok', 'Connected to Forgejo')
    } catch (e) {
      await alertBox({ title: 'Nothing was replaced', message: e instanceof Error ? e.message : String(e) })
    } finally {
      busy = ''
      app.info = await api.info().catch(() => app.info)
      await Promise.all([refresh(), syncOpenTabs()])
      app.logVersion++
    }
    test()
  }

  async function push() {
    await pullOrPush('push')
    test()
  }

  async function saveWeb(e: SubmitEvent) {
    e.preventDefault()
    busy = 'web'
    webError = ''
    try {
      st = await forgejoApi.setWeb(webBase.trim())
      app.info = await api.info()
      editWeb = false
    } catch (err) {
      webError = err instanceof Error ? err.message : String(err)
    } finally {
      busy = ''
    }
  }

  /** Save (or with '' remove) the status token; the links in Forgejo open this console's address. */
  async function saveToken(value: string) {
    busy = 'status'
    tokenError = ''
    try {
      st = await forgejoApi.setStatus(value, value ? location.origin : '')
      token = ''
    } catch (err) {
      tokenError = err instanceof Error ? err.message : String(err)
    } finally {
      busy = ''
    }
  }

  async function copy(text: string, what: string) {
    if (await copyText(text)) {
      copied = what
      setTimeout(() => copied === what && (copied = ''), 2000)
    } else toast('Copy failed; select the text and copy it by hand.', 'error')
  }
</script>

{#snippet copyButton(text: string, what: string, label: string)}
  <button class="icon" aria-label="Copy the {label}" title={copied === what ? 'Copied' : `Copy the ${label}`} onclick={() => copy(text, what)}>
    <Icon name={copied === what ? 'tick' : 'copy'} size={14} />
  </button>
{/snippet}

<section class="card fj" aria-labelledby="set-forgejo">
  <div class="head">
    <h2 id="set-forgejo">Forgejo</h2>
    {#if busy === 'test'}
      <span class="badge busy"><span class="dot busy"></span>Testing…</span>
    {:else if check}
      <span class="badge {tone === 'ok' ? 'ok' : tone === 'err' ? 'err' : tone === 'warn' ? 'warn' : ''}"><span class="dot {tone === 'warn' ? 'off' : tone}"></span>{title}</span>
    {/if}
    <span class="spacer"></span>
    {#if st?.web}<a class="btn" href={st.web} target="_blank" rel="noopener noreferrer"><Icon name="external" size={14} />Open in Forgejo</a>{/if}
  </div>
  <p class="muted lead">nixos-manager clones, commits and pushes to this repository, and every machine's comin pulls from it: a push here is a deploy.</p>

  {#if loadError}
    <p class="error-text" role="alert">{loadError}</p>
  {:else if !st}
    <span class="skel" style:width="60%"></span>
  {:else}
    <ol class="steps">
      <li class="step">
        <h3>Repository</h3>
        <form class="row" onsubmit={saveRemote}>
          <input
            class="mono"
            bind:value={url}
            placeholder="ssh://git@forgejo.lan:2222/you/nixos-config.git"
            aria-label="Repository URL"
            aria-invalid={!!urlError}
            aria-describedby="fj-url-help"
            spellcheck="false"
            autocomplete="off" />
          <button type="submit" disabled={!url.trim() || url.trim() === st.remote || !!busy}>{busy === 'remote' ? 'Saving…' : st.remote ? 'Change' : 'Save'}</button>
        </form>
        {#if urlError}<p class="error-text" role="alert">{urlError}</p>{/if}
        <p id="fj-url-help" class="hint">
          SSH, recommended: <span class="mono">ssh://git@host:2222/owner/repo.git</span> or <span class="mono">git@host:owner/repo.git</span>. HTTPS:
          <span class="mono">https://host/owner/repo.git</span>, with the token in a git credential helper, never in the URL.
        </p>
        {#if st.env_remote}
          <p class="note"><Icon name="info" size={14} /><span><span class="mono">NIXMGR_REMOTE</span> sets the remote at every start. Change it there as well, or a change made here lasts until the next restart.</span></p>
        {/if}
      </li>

      <li class="step">
        <h3>Deploy key</h3>
        {#if st.kind === 'local'}
          <p class="muted">This remote is a folder on this server, so git needs no key.</p>
        {:else if !ssh}
          <p class="muted">This remote does not use SSH: git takes the credentials from its credential helper (see the README). The deploy key is for SSH remotes.</p>
        {:else if st.ssh === 'env'}
          <p class="muted"><span class="mono">GIT_SSH_COMMAND</span> is set on the server, so git uses that command and its key; a deploy key managed here would not be used.</p>
        {:else if st.ssh === 'home' && !st.key}
          <p class="muted">git uses the SSH key in <span class="mono">~/.ssh</span> on this server. To manage the key here instead:</p>
          <button class="self-start" onclick={createKey} disabled={!!busy}><Icon name="key" size={14} />{busy === 'key' ? 'Creating…' : 'Create a deploy key'}</button>
        {:else if st.key}
          <div class="code-row">
            <code class="key" title={st.key.public}>{st.key.public}</code>
            {@render copyButton(st.key.public, 'key', 'public key')}
          </div>
          <p class="small muted">Fingerprint <span class="mono">{st.key.fingerprint}</span>. Only this public half leaves the server.</p>
          <ol class="how">
            <li>In Forgejo, open the repository → <b>Settings</b> → <b>Deploy keys</b> → <b>Add deploy key</b>.</li>
            <li>Paste the key, name it <i>nixos-manager</i>, tick <b>Enable write access</b>, and add it.</li>
          </ol>
        {/if}
      </li>

      <li class="step">
        <h3>Connection</h3>
        <div class="row">
          <button class="self-start" onclick={test} disabled={!st.remote || !!busy}><Icon name="refresh" size={14} />{busy === 'test' ? 'Testing…' : 'Test connection'}</button>
          {#if !st.remote}<span class="muted small">Save the repository URL first.</span>{/if}
        </div>
        {#if check && busy !== 'test'}
          <div class="result {tone}" role="status">
            <p>{check.message}</p>
            {#if check.branches.length}
              <p class="branches">
                <span class="muted small">Branches</span>{#each check.branches as b (b)}<span class="chip mono">{b}</span>{/each}
              </p>
            {/if}
            {#if forget}
              <div class="code-row"><code>{forget}</code>{@render copyButton(forget, 'forget', 'command')}</div>
            {/if}
            {#if check.next === 'adopt'}
              <div class="row">
                <button class="primary" onclick={adopt} disabled={!!busy}>{busy === 'adopt' ? 'Replacing…' : 'Use the Forgejo repository'}</button>
                <span class="muted small">Replaces the starter files; they stay in git as a backup.</span>
              </div>
            {:else if check.next === 'push'}
              <div class="row">
                <button class="primary" onclick={push} disabled={!!repo.busy || !!busy}><Icon name="push" size={14} />{repo.busy === 'push' ? 'Pushing…' : 'Push'}</button>
                <a class="small" href="#/changes">Commit first in Changes</a>
              </div>
            {/if}
            {#if check.detail}
              <details><summary class="small">What git said</summary><pre>{check.detail}</pre></details>
            {/if}
          </div>
        {/if}
        {#if st.host_key}
          <p class="small muted">Host key recorded on first contact: <span class="mono">{st.host_key}</span></p>
        {/if}
      </li>

      <li class="step">
        <h3>Push webhook <span class="opt">optional</span></h3>
        <p class="muted">
          Forgejo tells this console about every push, so a commit pushed from a laptop or Forgejo's web editor shows here within seconds instead of at the next Pull, and is
          checked: every machine it changes is built. Its result shows in Changes → Checks, in Health, and with the step below, in Forgejo.
        </p>
        <dl class="pairs">
          <dt>Target URL</dt>
          <dd class="code-row"><code>{hook.url}</code>{@render copyButton(hook.url, 'url', 'webhook URL')}</dd>
          <dt>Secret</dt>
          <dd class="code-row">
            <code class:masked={!showSecret}>{showSecret ? st.hook_secret : '•'.repeat(24)}</code>
            <button class="small ghost" aria-pressed={showSecret} onclick={() => (showSecret = !showSecret)}>{showSecret ? 'Hide' : 'Show'}</button>
            {@render copyButton(st.hook_secret, 'secret', 'secret')}
          </dd>
        </dl>
        {#if hook.warning}<p class="note warn"><Icon name="alert" size={14} /><span>{hook.warning}</span></p>{/if}
        <ol class="how">
          <li>In Forgejo, open the repository → <b>Settings</b> → <b>Webhooks</b> → <b>Add webhook</b> → <b>Forgejo</b>.</li>
          <li>Target URL and Secret: the two above. HTTP method <b>POST</b>, POST content type <b>application/json</b>.</li>
          <li>Trigger on <b>Push events</b>, keep it active, and add the webhook. Its <i>Test delivery</i> button sends one now.</li>
        </ol>
        <div class="delivery small" role="status">
          <span class="dot {st.last_delivery ? (st.last_delivery.ok ? 'ok' : 'err') : 'off'}"></span>
          {#if st.last_delivery}
            <span><span class="when">Last delivery <time datetime={st.last_delivery.time} title={new Date(st.last_delivery.time).toLocaleString()}>{ago(st.last_delivery.time)}</time></span> · {st.last_delivery.result}</span>
          {:else}
            <span class="muted">No delivery since the server started. If Forgejo's delivery log shows 401, its secret differs from the one above.</span>
          {/if}
          <button class="link small" onclick={load}>Refresh</button>
        </div>
      </li>

      <li class="step">
        <h3>Web address</h3>
        {#if editWeb}
          <form class="row" onsubmit={saveWeb}>
            <input class="mono" bind:value={webBase} placeholder="https://forgejo.lan" aria-label="Forgejo web address" aria-invalid={!!webError} spellcheck="false" autocomplete="off" />
            <button type="submit" disabled={!!busy}>Save</button>
            <button type="button" class="ghost" onclick={() => ((editWeb = false), (webError = ''))}>Cancel</button>
          </form>
          {#if webError}<p class="error-text" role="alert">{webError}</p>{/if}
          <p class="hint">Where you open Forgejo in a browser, if it is not https on the SSH host: another host, a port like <span class="mono">:3000</span>, or http. Leave empty to derive it from the remote.</p>
        {:else}
          <p class="row">
            {#if st.web}<a class="mono" href={st.web} target="_blank" rel="noopener noreferrer">{st.web}</a>{:else}<span class="muted">None: the remote has no web page.</span>{/if}
            <span class="muted small">{st.web_base ? 'set here' : 'derived from the remote'}</span>
            <button class="link small" onclick={() => ((webBase = st?.web_base ?? ''), (editWeb = true))}>Change</button>
          </p>
          <p class="hint">Commits in History and files in Code link to Forgejo through this address.</p>
        {/if}
      </li>

      <li class="step">
        <h3>Commit statuses <span class="opt">optional</span></h3>
        <p class="muted">The check of each pushed commit shows next to it in Forgejo, on branches and pull requests, with a link to its log here. It needs the push webhook.</p>
        {#if !st.status_api}
          <p class="muted">The remote needs a Forgejo web address first.</p>
        {:else}
          {#if st.status_token}
            <p class="row">
              <span class="ok-text"><Icon name="tick" size={14} />Token saved</span>
              <span class="muted small">{st.status_console ? `links open ${st.status_console}` : 'no links'}</span>
              <button class="link small" onclick={() => saveToken('')} disabled={!!busy}>Remove</button>
            </p>
          {/if}
          <form class="row" onsubmit={(e) => (e.preventDefault(), saveToken(token.trim()))}>
            <input
              class="mono"
              type="password"
              bind:value={token}
              placeholder={st.status_token ? 'Paste a new token to replace it' : 'Access token'}
              aria-label="Forgejo access token for commit statuses"
              aria-invalid={!!tokenError}
              spellcheck="false"
              autocomplete="off" />
            <button type="submit" disabled={!token.trim() || !!busy}>{busy === 'status' ? 'Trying…' : 'Save'}</button>
          </form>
          {#if tokenError}<p class="error-text" role="alert">{tokenError}</p>{/if}
          <ol class="how">
            <li>In Forgejo, as a user who can write to the repository: <b>Settings</b> → <b>Applications</b> → <b>Generate new token</b>.</li>
            <li>Give it the <b>repository</b> permission <b>Read and write</b>, generate it, and paste it here. It is tried on the repository, then kept on this server only.</li>
          </ol>
          <div class="delivery small" role="status">
            <span class="dot {st.last_status ? (st.last_status.ok ? 'ok' : 'err') : 'off'}"></span>
            {#if st.last_status}
              <span><span class="when">Last status <time datetime={st.last_status.time} title={new Date(st.last_status.time).toLocaleString()}>{ago(st.last_status.time)}</time></span> · {st.last_status.result}</span>
            {:else}
              <span class="muted">No status sent since the server started.</span>
            {/if}
            <button class="link small" onclick={load}>Refresh</button>
          </div>
        {/if}
      </li>
    </ol>
  {/if}
</section>

<style>
  .fj {
    display: grid;
    gap: 10px;
    min-width: 0;
    padding: var(--s4) var(--s5) var(--s5);
    overflow-wrap: anywhere;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
  h2 {
    flex: none;
    font-size: var(--fs-lg);
  }
  .badge.warn {
    color: var(--yellow);
    background: color-mix(in srgb, var(--yellow) 10%, transparent);
    border-color: color-mix(in srgb, var(--yellow) 30%, transparent);
  }
  .badge.warn .dot {
    --c: var(--yellow);
  }
  .lead {
    line-height: 1.5;
  }
  .steps {
    display: grid;
    gap: 0;
    margin: 4px 0 0;
    padding: 0;
    list-style: none;
    counter-reset: step;
  }
  .step {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
    min-width: 0;
    padding: 2px 0 18px 36px;
    counter-increment: step;
  }
  .step::before {
    position: absolute;
    top: 0;
    left: 0;
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    color: var(--fg-strong);
    font: 600 var(--fs-xs) var(--sans);
    content: counter(step);
    background: var(--bg-active);
    border: 1px solid var(--line);
    border-radius: 50%;
  }
  /* The line joining the step numbers. */
  .step:not(:last-child)::after {
    position: absolute;
    top: 26px;
    bottom: 4px;
    left: 11px;
    width: 1px;
    content: '';
    background: var(--line);
  }
  .step:last-child {
    padding-bottom: 0;
  }
  h3 {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin: 2px 0 0;
    color: var(--fg-strong);
    font-size: var(--fs-md);
  }
  .opt {
    color: var(--fg-faint);
    font-size: var(--fs-xs);
    font-weight: 500;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin: 0;
  }
  .row input {
    flex: 1;
    min-width: 220px;
  }
  .self-start {
    justify-self: start;
  }
  .ok-text {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--green);
  }
  .hint {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.5;
  }
  .note {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    color: var(--fg);
    font-size: var(--fs-sm);
    line-height: 1.5;
  }
  .note :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--accent);
  }
  .note.warn :global(svg) {
    color: var(--yellow);
  }
  .code-row {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    margin: 0;
  }
  .code-row code {
    flex: 1;
    min-width: 0;
    padding: 6px 10px;
    overflow: hidden;
    font-size: 12px;
    white-space: nowrap;
    text-overflow: ellipsis;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .code-row code.masked {
    color: var(--fg-muted);
    letter-spacing: 0.1em;
  }
  .how {
    display: grid;
    gap: 3px;
    margin: 0;
    padding-left: 18px;
    color: var(--fg);
    line-height: 1.5;
  }
  .how b {
    color: var(--fg-strong);
    font-weight: 600;
  }
  .pairs {
    display: grid;
    grid-template-columns: 84px minmax(0, 1fr);
    gap: 6px 10px;
    align-items: center;
    margin: 0;
  }
  .pairs dt {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .result {
    display: grid;
    gap: 8px;
    padding: 10px 14px;
    line-height: 1.5;
    background: color-mix(in srgb, var(--c, var(--line-strong)) 6%, var(--bg-side));
    border: 1px solid color-mix(in srgb, var(--c, var(--line-strong)) 35%, var(--line));
    border-radius: var(--r);
  }
  .result.ok {
    --c: var(--green);
  }
  .result.warn {
    --c: var(--yellow);
  }
  .result.err {
    --c: var(--red);
  }
  .branches {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  details summary {
    color: var(--fg-muted);
    cursor: pointer;
  }
  details pre {
    margin: 6px 0 0;
    padding: 8px 10px;
    overflow: auto;
    font: 12px/1.5 var(--mono);
    white-space: pre-wrap;
    background: var(--bg-deep);
    border-radius: var(--r-sm);
  }
  .delivery {
    display: grid;
    grid-template-columns: 8px minmax(0, 1fr) auto;
    align-items: start;
    gap: 10px;
    padding: 8px 12px;
    line-height: 1.5;
    background: var(--bg-side);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .delivery .dot {
    margin-top: 6px;
  }
  .when {
    color: var(--fg-strong);
  }
  a.small {
    font-size: var(--fs-sm);
  }
</style>
