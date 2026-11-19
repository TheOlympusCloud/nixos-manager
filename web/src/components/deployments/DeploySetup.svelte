<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type DeployOverview } from '../../lib/api'
  import { installCommand, localOnly, TOKEN_FILE, urlLine } from '../../lib/deployments'
  import { loadDeployments } from '../../lib/deployments.svelte'
  import { fleet } from '../../lib/fleet.svelte'
  import { codeLink, machineLink } from '../../lib/router.svelte'
  import { confirmBox, report } from '../../lib/ui.svelte'
  import { copyText } from '../../lib/util'
  import Icon from '../Icon.svelte'

  /** How to turn reporting on: the address for the repository, the token for each machine. */
  let { ov }: { ov: DeployOverview | null } = $props()

  let token = $state('')
  let reveal = $state(false)
  let copied = $state('')
  onMount(() => {
    api.deployToken().then((t) => (token = t), report)
  })

  const url = location.origin
  const local = localOnly(location.hostname)
  // Every machine of the fleet, those that never reported first.
  const reported = $derived(new Set((ov?.machines ?? []).filter((m) => m.reports.length).map((m) => m.host)))
  const machines = $derived(
    [...new Set([...(fleet.model?.hosts ?? []).map((h) => h.name), ...(ov?.machines ?? []).map((m) => m.host)])].sort(
      (a, b) => Number(reported.has(a)) - Number(reported.has(b)) || a.localeCompare(b),
    ),
  )
  let picked = $state('')
  const target = $derived(picked || machines[0] || 'machine')
  const sshName = $derived(fleet.model?.hosts.find((h) => h.name === target)?.hostName || target)
  const masked = $derived(token ? token.slice(0, 4) + '•'.repeat(12) : '…')
  const command = (t: string) => installCommand(t, sshName)

  async function copy(text: string, what: string) {
    if (await copyText(text)) {
      copied = what
      setTimeout(() => copied === what && (copied = ''), 2000)
    }
  }
  async function regenerate() {
    const ok = await confirmBox({
      title: 'Regenerate the report token?',
      message: 'Machines with the current token stop reporting until they get the new one.',
      ok: 'Regenerate',
      danger: true,
    })
    if (!ok) return
    try {
      token = await api.regenerateDeployToken()
      reveal = true
      loadDeployments()
    } catch (e) {
      report(e, 'Could not regenerate the token')
    }
  }
</script>

{#snippet copyButton(text: string, what: string, label: string)}
  <button class="small" onclick={() => copy(text, what)} aria-label={label} disabled={!token && what !== 'url'}>
    <Icon name={copied === what ? 'tick' : 'copy'} size={13} />{copied === what ? 'Copied' : 'Copy'}
  </button>
{/snippet}

<ol class="steps">
  <li>
    <span class="n num">1</span>
    <div class="body">
      <h3>Tell the machines where this console is</h3>
      <p class="muted">In <a class="mono" href={codeLink('modules/common.nix')}>modules/common.nix</a>, then commit and push. comin installs the report hook on every machine.</p>
      <div class="code-row">
        <code>{urlLine(url)}</code>
        {@render copyButton(urlLine(url), 'url', 'Copy the line for common.nix')}
      </div>
      {#if local}
        <p class="warn small" role="note"><Icon name="alert" size={13} />This page is open on {location.hostname}, which the machines cannot reach. Use the LAN or Tailscale address they know this server by.</p>
      {:else}
        <p class="muted small">Use the address the machines reach this server at: its LAN name or its Tailscale name.</p>
      {/if}
    </div>
  </li>
  <li>
    <span class="n num">2</span>
    <div class="body">
      <h3>Put the report token on each machine</h3>
      <p class="muted">One token for the whole fleet, kept in a file on each machine and never in the repository. A machine without it reports nothing.</p>
      <div class="token-row">
        <label class="label" for="deploy-token">Report token</label>
        <input id="deploy-token" class="mono" readonly type={reveal ? 'text' : 'password'} value={token} onfocus={(e) => e.currentTarget.select()} />
        <button class="small ghost" onclick={() => (reveal = !reveal)} aria-pressed={reveal}>{reveal ? 'Hide' : 'Show'}</button>
        {@render copyButton(token, 'token', 'Copy the report token')}
        <button class="small ghost" onclick={regenerate}><Icon name="refresh" size={13} />Regenerate</button>
      </div>
      <div class="install">
        <label class="label" for="deploy-host">Install it on</label>
        <select id="deploy-host" value={target} onchange={(e) => (picked = e.currentTarget.value)}>
          {#each machines as h (h)}<option value={h}>{h}{reported.has(h) ? ' · reports' : ''}</option>{/each}
        </select>
      </div>
      <div class="code-row">
        <code>{command(reveal ? token : masked)}</code>
        {@render copyButton(command(token), 'cmd', `Copy the command for ${target}`)}
      </div>
      <p class="muted small">
        Logs in as root, and the token travels on stdin. With sudo instead, use your user and <span class="mono">sudo install</span> (sudo must not ask for a password). With sops-nix or agenix, set
        <span class="mono">fleet.deployReport.tokenFile</span> to the secret's path instead of <span class="mono">{TOKEN_FILE}</span>.
      </p>
    </div>
  </li>
  <li>
    <span class="n num">3</span>
    <div class="body">
      <h3>Deploy</h3>
      <p class="muted">The first report arrives with the next deployment after both are in place: comin runs the hook its running version knows, so push one more change if a machine stays silent.</p>
      {#if machines.length}
        <div class="who" aria-label="Machines that reported">
          {#each machines as h (h)}
            <a class="who-chip" class:on={reported.has(h)} href={machineLink(h)} title={reported.has(h) ? `${h} reports its deployments` : `${h} has not reported yet`}>
              <Icon name={reported.has(h) ? 'tick' : 'minus'} size={12} />{h}
            </a>
          {/each}
        </div>
      {/if}
    </div>
  </li>
</ol>

<style>
  .steps {
    display: grid;
    gap: var(--s4);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .steps > li {
    display: grid;
    grid-template-columns: 26px minmax(0, 1fr);
    gap: var(--s3);
  }
  .n {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    color: var(--accent);
    font-size: var(--fs-sm);
    font-weight: 600;
    background: color-mix(in srgb, var(--accent) 12%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    border-radius: 50%;
  }
  .body {
    display: grid;
    gap: var(--s2);
    min-width: 0;
  }
  .body h3 {
    font-size: var(--fs-md);
    font-weight: 600;
  }
  .body p {
    margin: 0;
    line-height: 1.55;
  }
  .code-row {
    display: flex;
    align-items: center;
    gap: var(--s2);
    min-width: 0;
    padding: 6px 6px 6px 12px;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .code-row code {
    flex: 1;
    min-width: 0;
    color: var(--fg-strong);
    font: 12px/1.6 var(--mono);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .token-row,
  .install {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
  }
  .token-row .label,
  .install .label {
    width: 96px;
  }
  .token-row input {
    flex: 1;
    min-width: 200px;
    max-width: 380px;
  }
  .install select {
    width: auto;
    min-width: 200px;
  }
  .warn {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--yellow);
  }
  .who {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .who-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 22px;
    padding: 0 8px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    text-decoration: none;
    border: 1px dashed var(--line);
    border-radius: 11px;
  }
  .who-chip.on {
    color: var(--fg-strong);
    background: color-mix(in srgb, var(--green) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--green) 30%, transparent);
  }
  .who-chip.on :global(svg) {
    color: var(--green);
  }
</style>
