<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type Token, type TokenScope } from '../lib/api'
  import { expiresAt, expiry, lifetimes, scopes } from '../lib/tokens'
  import { confirmBox } from '../lib/ui.svelte'
  import { ago, copyText } from '../lib/util'

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
  <h2 id="tokens-title">API tokens</h2>
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
  <p class="muted hint" id="token-scope-hint">{scopes.find((s) => s.value === scope)?.hint}</p>

  {#if error}<p class="error" role="alert">{error}</p>{/if}

  {#if created}
    <div class="secret" role="status">
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

  <table>
    <thead><tr><th>Name</th><th>Access</th><th>Created</th><th>Expires</th><th>Last used</th><th><span class="sr-only">Actions</span></th></tr></thead>
    <tbody>
      {#if tokens === null}
        <tr><td colspan="6" class="muted">Loading…</td></tr>
      {:else}
        {#each tokens as t (t.id)}
          {@const exp = expiry(t.expires)}
          <tr>
            <td>{t.name}</td>
            <td>{scopes.find((s) => s.value === t.scope)?.label ?? t.scope}</td>
            <td title={t.created}>{ago(t.created)}</td>
            <td title={t.expires} class:expired={exp === 'expired'}>{exp}</td>
            <td title={t.last_used}>{t.last_used ? ago(t.last_used) : 'never'}</td>
            <td class="act"><button class="small revoke" onclick={() => revoke(t)} aria-label="Revoke {t.name}">Revoke</button></td>
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
    <button onclick={() => copy(endpoint, 'endpoint')}>{copied === 'endpoint' ? 'Copied' : 'Copy'}</button>
  </div>
  <h3>Client configuration</h3>
  <pre>{config}</pre>
  <button onclick={() => copy(config, 'config')}>{copied === 'config' ? 'Copied' : 'Copy configuration'}</button>
</section>

<style>
  h2 {
    font-size: var(--fs-lg);
  }
  h3 {
    margin: 18px 0 6px;
    font-size: 13px;
  }
  p {
    margin: 6px 0 12px;
  }
  .row {
    display: flex;
    max-width: 640px;
    align-items: center;
    gap: 8px;
  }
  .row input {
    flex: 1;
    min-width: 0;
  }
  form.row {
    max-width: 820px;
  }
  form.row .label {
    white-space: nowrap;
  }
  form.row input {
    min-width: 140px;
  }
  form.row select {
    flex: none;
    width: auto;
  }
  .error,
  .expired {
    color: var(--red);
  }
  .hint {
    margin-top: 4px;
    font-size: 12px;
  }
  .secret {
    margin: 12px 0;
    padding: 10px 12px;
    background: color-mix(in srgb, var(--green) 10%, var(--bg-side));
    border: 1px solid color-mix(in srgb, var(--green) 40%, transparent);
    border-radius: 6px;
  }
  .secret p {
    margin: 0 0 8px;
  }
  .secret input {
    font-family: var(--mono);
  }
  table {
    width: 100%;
    margin-top: 14px;
    border-collapse: collapse;
  }
  th,
  td {
    padding: 5px 6px;
    text-align: left;
    border-bottom: 1px solid var(--line);
  }
  th {
    color: var(--fg-muted);
    font-size: 12px;
    font-weight: 500;
  }
  td.act {
    text-align: right;
  }
  .revoke {
    color: var(--fg-muted);
    background: none;
    border-color: var(--line);
  }
  .revoke:hover:not(:disabled) {
    color: var(--red);
    background: color-mix(in srgb, var(--red) 10%, transparent);
    border-color: color-mix(in srgb, var(--red) 40%, transparent);
  }
  .endpoint {
    flex: 1;
    padding: 5px 8px;
    background: var(--bg-deep);
    border-radius: 4px;
  }
  pre {
    margin: 0 0 8px;
    padding: 10px;
    overflow: auto;
    font: 12px/1.5 var(--mono);
    background: var(--bg-deep);
    border-radius: 4px;
  }
</style>
