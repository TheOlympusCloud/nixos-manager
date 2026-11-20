<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type ChannelStatus, type EmailSecurity } from '../lib/api'
  import { defaultPort, formOf, settingsOf, type NotifyForm } from '../lib/notifications'
  import { ago } from '../lib/util'

  /** Settings → Notifications: where nixos-manager tells about failed, stuck and silent machines. */
  let form = $state<NotifyForm | null>(null)
  let status = $state<ChannelStatus[]>([])
  let saved = $state('') // the form as last saved, to know when it changed
  let busy = $state<'' | 'save' | 'test'>('')
  let error = $state('')
  let done = $state('')
  const dirty = $derived(!!form && JSON.stringify(form) !== saved)
  const any = $derived(!!form && (form.ntfy.on || form.webhook.on || form.email.on))
  const show = (e: unknown) => (error = e instanceof Error ? e.message : String(e))
  const statusOf = (c: string) => status.find((s) => s.channel === c)

  function fill(f: NotifyForm) {
    form = f
    saved = JSON.stringify(f)
  }
  async function load() {
    try {
      const r = await api.notifications()
      fill(formOf(r.settings, location.origin))
      status = r.status
    } catch (e) {
      show(e)
    }
  }
  onMount(load)

  async function save(e: SubmitEvent) {
    e.preventDefault()
    if (!form) return
    busy = 'save'
    error = done = ''
    try {
      fill(formOf(await api.saveNotifications(settingsOf(form)), location.origin))
      const r = await api.notifications()
      status = r.status
      done = 'Saved.'
    } catch (err) {
      show(err)
    } finally {
      busy = ''
    }
  }
  async function test() {
    busy = 'test'
    error = done = ''
    try {
      status = await api.testNotifications()
      done = status.every((s) => s.ok) ? 'Test message sent: check that it arrived.' : ''
    } catch (err) {
      show(err)
    } finally {
      busy = ''
    }
  }
  function security(s: EmailSecurity) {
    if (!form) return
    if (form.email.port === defaultPort[form.email.security]) form.email.port = defaultPort[s]
    form.email.security = s
  }
</script>

{#snippet last(channel: string)}
  {@const s = statusOf(channel)}
  {#if s}
    <p class="small last" class:bad={!s.ok} role="status">
      {s.ok ? `Last message sent ${ago(s.at)}.` : `The last send failed ${ago(s.at)}: ${s.error}. It is tried again, waiting longer each time.`}
    </p>
  {/if}
{/snippet}

<section class="card block" aria-labelledby="set-notify">
  <h2 id="set-notify">Notifications</h2>
  <p class="muted">
    Get a message when a deployment fails or a machine is stuck behind for over an hour, when a machine stops reporting, when the branch comin deploys does not evaluate,
    when a NixOS release or a flake input reaches its end of life, and when a scheduled update of the inputs is ready to merge or fails. Each event is sent once per channel; it is sent again only if it ends and comes back. A channel only
    gets what starts after it was set up: Health shows what is already wrong.
  </p>
  {#if !form}
    {#if error}<p class="error" role="alert">{error}</p>{:else}<span class="skel" style:width="60%"></span>{/if}
  {:else}
    <form onsubmit={save}>
      <div class="field">
        <label class="label" for="notify-console">Console address</label>
        <input id="notify-console" class="mono" bind:value={form.consoleUrl} placeholder="http://nixos-manager.home.lan:8080" spellcheck="false" autocomplete="off" />
        <span class="muted small">Messages link here; use the address you open the console at.</span>
      </div>

      <fieldset>
        <label class="toggle"><input type="checkbox" bind:checked={form.ntfy.on} /><strong>ntfy</strong><span class="muted small">push to your phone</span></label>
        {#if form.ntfy.on}
          <div class="field">
            <label class="label" for="notify-ntfy-url">Topic address</label>
            <input id="notify-ntfy-url" class="mono" bind:value={form.ntfy.url} placeholder="https://ntfy.sh/my-fleet-8f3k" required spellcheck="false" autocomplete="off" />
            <span class="muted small">On ntfy.sh anyone who knows the topic can read it: pick a name nobody guesses, or a server of your own.</span>
          </div>
          <div class="field">
            <label class="label" for="notify-ntfy-token">Access token</label>
            <div class="secret-row">
              <input id="notify-ntfy-token" type="password" bind:value={form.ntfy.token} placeholder={form.ntfy.hasToken ? 'saved: type to replace' : 'for a protected topic (optional)'} autocomplete="off" />
              {#if form.ntfy.hasToken && !form.ntfy.token}<button type="button" class="small ghost" onclick={() => form && (form.ntfy.hasToken = false)}>Remove</button>{/if}
            </div>
          </div>
          {@render last('ntfy')}
        {/if}
      </fieldset>

      <fieldset>
        <label class="toggle"><input type="checkbox" bind:checked={form.webhook.on} /><strong>Webhook</strong><span class="muted small">a JSON POST, for Home Assistant, n8n, chat bots</span></label>
        {#if form.webhook.on}
          <div class="field">
            <label class="label" for="notify-hook">Address</label>
            <input id="notify-hook" class="mono" bind:value={form.webhook.url} placeholder="https://home.lan/api/webhook/fleet" required spellcheck="false" autocomplete="off" />
            <span class="muted small">Gets <span class="mono">key, kind, severity, title, message, hosts, url, at</span>.</span>
          </div>
          {@render last('webhook')}
        {/if}
      </fieldset>

      <fieldset>
        <label class="toggle"><input type="checkbox" bind:checked={form.email.on} /><strong>Email</strong><span class="muted small">over SMTP</span></label>
        {#if form.email.on}
          <div class="grid">
            <div class="field">
              <label class="label" for="notify-smtp-host">SMTP server</label>
              <input id="notify-smtp-host" bind:value={form.email.host} placeholder="smtp.example.org" required spellcheck="false" autocomplete="off" />
            </div>
            <div class="field">
              <label class="label" for="notify-smtp-port">Port</label>
              <input id="notify-smtp-port" type="number" min="1" max="65535" bind:value={form.email.port} required />
            </div>
            <div class="field">
              <label class="label" for="notify-smtp-sec">Security</label>
              <select id="notify-smtp-sec" value={form.email.security} onchange={(e) => security(e.currentTarget.value as EmailSecurity)}>
                <option value="starttls">STARTTLS</option>
                <option value="tls">TLS</option>
                <option value="none">None (local relay)</option>
              </select>
            </div>
            <div class="field">
              <label class="label" for="notify-smtp-user">User name</label>
              <input id="notify-smtp-user" bind:value={form.email.username} placeholder="optional" autocomplete="off" spellcheck="false" />
            </div>
            <div class="field">
              <label class="label" for="notify-smtp-pass">Password</label>
              <input id="notify-smtp-pass" type="password" bind:value={form.email.password} placeholder={form.email.hasPassword ? 'saved: type to replace' : ''} autocomplete="off" />
            </div>
            <div class="field">
              <label class="label" for="notify-smtp-from">From</label>
              <input id="notify-smtp-from" bind:value={form.email.from} placeholder="nixos-manager <fleet@example.org>" required autocomplete="off" spellcheck="false" />
            </div>
            <div class="field">
              <label class="label" for="notify-smtp-to">To</label>
              <input id="notify-smtp-to" bind:value={form.email.to} placeholder="you@example.org, other@example.org" required autocomplete="off" spellcheck="false" />
            </div>
          </div>
          {@render last('email')}
        {/if}
      </fieldset>

      {#if error}<p class="error" role="alert">{error}</p>{/if}
      {#if done && !error}<p class="ok small" role="status">{done}</p>{/if}
      <div class="actions">
        <button class="primary" type="submit" disabled={!!busy || !dirty}>{busy === 'save' ? 'Saving…' : 'Save'}</button>
        <button type="button" onclick={test} disabled={!!busy || dirty || !any} title={dirty ? 'Save first: the test uses the saved channels' : ''}>{busy === 'test' ? 'Sending…' : 'Send a test message'}</button>
      </div>
    </form>
  {/if}
</section>

<style>
  .block {
    display: grid;
    min-width: 0;
    gap: 12px;
    padding: var(--s4) var(--s5) var(--s5);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  p {
    margin: 0;
    line-height: 1.55;
  }
  form {
    display: grid;
    gap: var(--s3);
  }
  fieldset {
    display: grid;
    gap: var(--s2);
    margin: 0;
    padding: var(--s3);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .toggle {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .field {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--s2) var(--s3);
  }
  .secret-row {
    display: flex;
    gap: 8px;
  }
  .secret-row input {
    flex: 1;
    min-width: 0;
  }
  .last {
    color: var(--fg-muted);
  }
  .last.bad,
  .error {
    color: var(--red);
  }
  .ok {
    color: var(--green);
  }
  .actions {
    display: flex;
    gap: var(--s2);
  }
</style>
