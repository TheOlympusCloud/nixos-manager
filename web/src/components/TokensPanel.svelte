<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type Token, type TokenScope } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { expiresAt, expiry, lifetimes, scopes } from '../lib/tokens'
  import { confirmBox } from '../lib/ui.svelte'
  import { ago, copyText } from '../lib/util'
  import Icon from './Icon.svelte'

  let tokens = $state<Token[] | null>(null)
  let name = $state('')
  let scope = $state<TokenScope>('read')
  let days = $state(0)
  let created = $state<(Token & { token: string }) | null>(null)
  let busy = $state(false)
  let copied = $state('')
  // Shown inside the dialog: toasts would sit behind the modal backdrop.
  let error = $state('')
  const show = (e: unknown) => (error = e instanceof Error ? e.message : String(e))

  const endpoint = `${location.origin}/mcp`
  const config = $derived(
    JSON.stringify(
      { mcpServers: { 'nixos-manager': { type: 'http', url: endpoint, headers: { Authorization: `Bearer ${created?.token ?? '<token>'}` } } } },
      null,
      2,
    ),
  )

  async function load() {
    try {
      tokens = await api.tokens()
    } catch (e) {
      show(e)
    }
  }

  onMount(load)

  async function create(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      created = await api.createToken(name.trim(), scope, expiresAt(days))
      name = ''
      await load()
    } catch (err) {
      show(err)
    } finally {
      busy = false
    }
  }

  async function revoke(t: Token) {
    const ok = await confirmBox({ title: 'Revoke token', message: `Revoke "${t.name}"? Clients using it stop working immediately.`, ok: 'Revoke', danger: true })
    if (!ok) return
    error = ''
    try {
      await api.revokeToken(t.id)
      if (created?.id === t.id) created = null
    } catch (e) {
      show(e)
    }
    await load()
  }

  async function copy(text: string, what: string) {
    if (await copyText(text)) {
      copied = what
      setTimeout(() => copied === what && (copied = ''), 2000)
    } else error = 'Copy failed; select the text and copy it by hand.'
  }
</script>

<section class="tokens" aria-labelledby="tokens-title">
  <h2 id="tokens-title" class="sr-only">API tokens</h2>
  <p class="muted">
    Tokens let scripts and MCP clients use the MCP endpoint and the <code>/api/</code> routes (<code>Authorization: Bearer &lt;token&gt;</code>). Give each the least
    access it needs: only a full token can push, which deploys. Commits made with a token name it in a <code>Via:</code> line.
  </p>

  <form class="row" onsubmit={create}>
    <label for="token-name" class="label">New token name</label>
    <input id="token-name" bind:value={name} placeholder="e.g. backup-script" autocomplete="off" spellcheck="false" />
    <label for="token-scope" class="sr-only">Access</label>
    <select id="token-scope" bind:value={scope} aria-describedby="token-scope-hint">
      {#each scopes as s (s.value)}<option value={s.value}>{s.label}</option>{/each}
    </select>
    <label for="token-days" class="sr-only">Expires</label>
    <select id="token-days" bind:value={days}>
      {#each lifetimes as l (l.days)}<option value={l.days}>{l.label}</option>{/each}
    </select>
    <button class="primary" type="submit" disabled={!name.trim() || busy}>Create</button>
  </form>
  <p class="muted" id="token-scope-hint">{scopes.find((s) => s.value === scope)?.hint}</p>

  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  {#if created}
    <div class="secret" role="status" use:frame={{ animate: true }} data-tone="ok">
      <p>
        <strong>Token "{created.name}" created</strong> ({scopes.find((s) => s.value === created?.scope)?.label.toLowerCase()}, expires {expiry(created.expires)}). Copy
        it now; it is not shown again.
      </p>
      <div class="row">
        <input aria-label="New token secret" readonly value={created.token} onfocus={(e) => e.currentTarget.select()} />
        <button onclick={() => created && copy(created.token, 'token')}>{copied === 'token' ? 'Copied' : 'Copy'}</button>
        <button class="ghost" onclick={() => (created = null)}>Done</button>
      </div>
    </div>
  {/if}

  <table class="data">
    <thead><tr><th>Name</th><th>Access</th><th>Created</th><th>Expires</th><th>Last used</th><th><span class="sr-only">Actions</span></th></tr></thead>
    <tbody>
      {#if tokens === null}
        <tr><td colspan="6" class="muted"><span class="spinner"></span>Loading…</td></tr>
      {:else}
        {#each tokens as t (t.id)}
          {@const exp = expiry(t.expires)}
          <tr>
            <td><b>{t.name}</b></td>
            <td>{scopes.find((s) => s.value === t.scope)?.label ?? t.scope}</td>
            <td title={t.created}>{ago(t.created)}</td>
            <td title={t.expires} class:expired={exp === 'expired'}>{#if exp === 'expired'}✖ {/if}{exp}</td>
            <td title={t.last_used}>{t.last_used ? ago(t.last_used) : 'never'}</td>
            <td class="act"><button class="danger" onclick={() => revoke(t)} aria-label="Revoke {t.name}">Revoke</button></td>
          </tr>
        {:else}
          <tr><td colspan="6" class="muted">No tokens yet.</td></tr>
        {/each}
      {/if}
    </tbody>
  </table>

  <h3>MCP endpoint</h3>
  <div class="row">
    <code class="endpoint">{endpoint}</code>
    <button onclick={() => copy(endpoint, 'endpoint')}><Icon name={copied === 'endpoint' ? 'tick' : 'copy'} /> {copied === 'endpoint' ? 'Copied' : 'Copy'}</button>
  </div>
  <h3>Client configuration</h3>
  <pre>{config}</pre>
  <button onclick={() => copy(config, 'config')}><Icon name={copied === 'config' ? 'tick' : 'copy'} /> {copied === 'config' ? 'Copied' : 'Copy configuration'}</button>
</section>

<style>
  .tokens > * + * {
    margin-top: var(--lh);
  }
  .tokens > :is(h2 + p, #token-scope-hint, h3 + *) {
    margin-top: 0;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .row input {
    flex: 1;
    width: auto;
    min-width: 16ch;
  }
  form.row .label {
    white-space: nowrap;
  }
  form.row select {
    flex: none;
    width: auto;
  }
  .secret .row {
    margin-top: var(--lh);
  }
  .expired {
    color: var(--red);
  }
  td.act {
    width: 1%;
    text-align: right;
    white-space: nowrap;
  }
  .endpoint {
    flex: 1;
    padding: 0 1ch;
    background: var(--bg-input);
  }
  pre {
    padding: 0 1ch;
    overflow: auto;
    background: var(--bg-input);
  }
</style>
