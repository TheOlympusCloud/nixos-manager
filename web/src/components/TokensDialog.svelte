<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type Token } from '../lib/api'
  import { app } from '../lib/state.svelte'
  import { confirmBox } from '../lib/ui.svelte'
  import { ago, copyText } from '../lib/util'

  let el: HTMLDialogElement
  let tokens = $state<Token[] | null>(null)
  let name = $state('')
  let created = $state<{ id: string; name: string; token: string } | null>(null)
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

  onMount(() => {
    el.showModal()
    load()
  })

  async function create(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      created = await api.createToken(name.trim())
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

<dialog bind:this={el} onclose={() => (app.tokensOpen = false)} aria-labelledby="tokens-title">
  <div class="head">
    <h2 id="tokens-title">API tokens</h2>
    <button class="icon" aria-label="Close" onclick={() => el.close()}>×</button>
  </div>
  <p class="muted">Tokens let agents and scripts use the MCP endpoint. Each token has full access to the repository.</p>

  <form class="row" onsubmit={create}>
    <label for="token-name" class="label">New token name</label>
    <input id="token-name" bind:value={name} placeholder="e.g. laptop-agent" autocomplete="off" spellcheck="false" />
    <button class="primary" type="submit" disabled={!name.trim() || busy}>Create</button>
  </form>

  {#if error}<p class="error" role="alert">{error}</p>{/if}

  {#if created}
    <div class="secret" role="status">
      <p><strong>Token "{created.name}" created.</strong> Copy it now; it is not shown again.</p>
      <div class="row">
        <input aria-label="New token secret" readonly value={created.token} onfocus={(e) => e.currentTarget.select()} />
        <button onclick={() => created && copy(created.token, 'token')}>{copied === 'token' ? 'Copied' : 'Copy'}</button>
        <button class="ghost" onclick={() => (created = null)}>Done</button>
      </div>
    </div>
  {/if}

  <table>
    <thead><tr><th>Name</th><th>Created</th><th>Last used</th><th><span class="sr-only">Actions</span></th></tr></thead>
    <tbody>
      {#if tokens === null}
        <tr><td colspan="4" class="muted">Loading…</td></tr>
      {:else}
        {#each tokens as t (t.id)}
          <tr>
            <td>{t.name}</td>
            <td title={t.created}>{ago(t.created)}</td>
            <td title={t.last_used}>{t.last_used ? ago(t.last_used) : 'never'}</td>
            <td><button class="danger" onclick={() => revoke(t)}>Revoke</button></td>
          </tr>
        {:else}
          <tr><td colspan="4" class="muted">No tokens yet.</td></tr>
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
</dialog>

<style>
  dialog {
    width: min(640px, 94vw);
    max-height: 90vh;
    padding: 18px 20px;
    color: var(--fg);
    background: var(--bg-side);
    border: 1px solid var(--line);
    border-radius: 8px;
    box-shadow: 0 12px 40px rgb(0 0 0 / 0.5);
  }
  dialog::backdrop {
    background: rgb(0 0 0 / 0.45);
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  h2 {
    margin: 0;
    font-size: 16px;
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
    align-items: center;
    gap: 8px;
  }
  .row input {
    flex: 1;
    min-width: 0;
  }
  .error {
    color: var(--red);
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
  td:last-child {
    text-align: right;
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
