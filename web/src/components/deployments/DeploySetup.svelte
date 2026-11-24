<script lang="ts">
  import { api, type DeployOverview } from '../../lib/api'
  import { installCommand, localOnly, onSharedToken, TOKEN_FILE, urlLine } from '../../lib/deployments'
  import { loadDeployments } from '../../lib/deployments.svelte'
  import { fleet } from '../../lib/fleet.svelte'
  import { codeLink, go, link, machineLink } from '../../lib/router.svelte'
  import { confirmBox, report, toast } from '../../lib/ui.svelte'
  import { copyText } from '../../lib/util'
  import { frame } from '../../lib/ascii'
  import Icon from '../Icon.svelte'

  /** How to turn reporting on: the address for the repository, each machine's own token. */
  let { ov }: { ov: DeployOverview | null } = $props()

  let token = $state('')
  let reveal = $state(false)
  let copied = $state('')
  let rotated = $state(0) // bumped to fetch the token again after new ones were made

  const url = location.origin
  const local = localOnly(location.hostname)
  // Every machine of the fleet, those that never reported first.
  const reported = $derived(new Set((ov?.machines ?? []).filter((m) => m.reports.length || m.lastSeen).map((m) => m.host)))
  const machines = $derived(
    [...new Set([...(fleet.model?.hosts ?? []).map((h) => h.name), ...(ov?.machines ?? []).map((m) => m.host)])].sort(
      (a, b) => Number(reported.has(a)) - Number(reported.has(b)) || a.localeCompare(b),
    ),
  )
  const shared = $derived(onSharedToken(ov))
  let picked = $state('')
  const target = $derived(picked || machines[0] || '')
  const sshName = $derived(fleet.model?.hosts.find((h) => h.name === target)?.hostName || target)
  const masked = $derived(token ? token.slice(0, 4) + '•'.repeat(12) : '…')
  const command = (t: string) => installCommand(t, sshName || 'machine')

  // The token is the picked machine's: fetch it whenever the pick changes.
  $effect(() => {
    const host = target
    void rotated
    token = ''
    if (!host) return
    api.deployToken(host).then((t) => host === target && (token = t), report)
  })

  async function copy(text: string, what: string) {
    if (await copyText(text)) {
      copied = what
      setTimeout(() => copied === what && (copied = ''), 2000)
    }
  }
  async function regenerate() {
    const ok = await confirmBox({
      title: 'Make new report tokens?',
      message:
        'Every machine gets a new token. Machines that keep theirs in the secrets get the new one there, to commit and push; the others stop reporting until they get it.',
      ok: 'Make new tokens',
      danger: true,
    })
    if (!ok) return
    try {
      const stored = await api.regenerateDeployTokens()
      if (stored.length)
        toast(`New tokens are in the secrets of ${stored.join(', ')}: commit and push them, or ${stored.length === 1 ? 'it stops' : 'they stop'} reporting.`, 'ok', 'New tokens', {
          label: 'View changes',
          run: () => go('#/changes'),
        })
      rotated++
      reveal = true
      loadDeployments()
    } catch (e) {
      report(e, 'Could not make new tokens')
    }
  }
  async function dropShared() {
    const ok = await confirmBox({
      title: 'Stop accepting the shared token?',
      message: shared.length
        ? `${shared.join(', ')} still ${shared.length === 1 ? 'reports' : 'report'} with it and will stop reporting until ${shared.length === 1 ? 'it gets its' : 'they get their'} own token.`
        : 'No machine reported with it lately. Machines that still have it stop reporting until they get their own token.',
      ok: 'Stop accepting it',
      danger: shared.length > 0,
    })
    if (!ok) return
    try {
      await api.dropSharedDeployToken()
      loadDeployments()
    } catch (e) {
      report(e, 'Could not stop accepting the shared token')
    }
  }
</script>

{#snippet copyButton(text: string, what: string, label: string)}
  <button class="small" onclick={() => copy(text, what)} aria-label={label} disabled={!token && what !== 'url'}>
    <Icon name={copied === what ? 'tick' : 'copy'} /> {copied === what ? 'Copied' : 'Copy'}
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
        <p class="warn small" role="note"><Icon name="alert" /> This page is open on {location.hostname}, which the machines cannot reach. Use the LAN or Tailscale address they know this server by.</p>
      {:else}
        <p class="muted small">Use the address the machines reach this server at: its LAN name or its Tailscale name.</p>
      {/if}
    </div>
  </li>
  <li>
    <span class="n num">2</span>
    <div class="body">
      <h3>Put each machine's report token on it</h3>
      <p class="muted">Every machine has its own token, kept in a file on that machine and never in the repository. A token only reports for its machine; a machine without one reports nothing.</p>
      {#if machines.length}
        <div class="install">
          <label class="label" for="deploy-host">Machine</label>
          <select id="deploy-host" value={target} onchange={(e) => (picked = e.currentTarget.value)}>
            {#each machines as h (h)}<option value={h}>{h}{reported.has(h) ? ' · reports' : ''}</option>{/each}
          </select>
        </div>
        <div class="token-row">
          <label class="label" for="deploy-token">Its token</label>
          <input id="deploy-token" class="mono" readonly type={reveal ? 'text' : 'password'} value={token} onfocus={(e) => e.currentTarget.select()} />
          <button class="small ghost" onclick={() => (reveal = !reveal)} aria-pressed={reveal}>{reveal ? 'Hide' : 'Show'}</button>
          {@render copyButton(token, 'token', `Copy the report token of ${target}`)}
        </div>
        <div class="code-row">
          <code>{command(reveal ? token : masked)}</code>
          {@render copyButton(command(token), 'cmd', `Copy the command for ${target}`)}
        </div>
      {:else}
        <p class="muted small">The machines show here once the fleet is evaluated.</p>
      {/if}
      <p class="muted small">
        Logs in as root, and the token travels on stdin. With sudo instead, use your user and <span class="mono">sudo install</span> (sudo must not ask for a password). Or, with no
        SSH at all, store every machine's token as its secret in <a href={link('settings')}>Settings → Secrets</a> (the starter fleet's sops-nix setup uses it); with agenix or
        your own sops-nix setup, set <span class="mono">fleet.deployReport.tokenFile</span> to the secret's path instead of <span class="mono">{TOKEN_FILE}</span>.
      </p>
      <div class="row-end"><button class="small ghost" onclick={regenerate}><Icon name="refresh" /> New tokens for every machine</button></div>
      {#if ov?.sharedToken}
        <div class="shared" role="note" data-tone="warn" use:frame>
          <Icon name="alert" />
          <div>
            <p>
              This console still accepts the shared fleet token machines were set up with before each had its own. Whoever holds it can report as any machine.
              {#if shared.length}Still on it: <strong>{shared.join(', ')}</strong>. Install their own tokens in the same file, then stop accepting it.{:else}No machine reported with it lately.{/if}
              The next release stops accepting it anyway.
            </p>
            <button class="small" onclick={dropShared}>Stop accepting the shared token</button>
          </div>
        </div>
      {/if}
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
              <Icon name={reported.has(h) ? 'tick' : 'minus'} /> {h}
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
    gap: calc(var(--lh) * 2) 0;
  }
  .steps > li {
    display: grid;
    grid-template-columns: 3ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  /* The step number in reverse video: ▐1▌ */
  .n {
    height: var(--lh);
    color: var(--on-accent);
    font-weight: 700;
    text-align: center;
    background: var(--accent);
  }
  .body {
    display: grid;
    gap: var(--lh) 0;
    min-width: 0;
  }
  .body > h3 + p {
    margin-top: calc(var(--lh) * -1);
  }
  .code-row {
    display: flex;
    align-items: baseline;
    gap: 0 2ch;
    min-width: 0;
    padding: 0 1ch;
    background: var(--bg-input);
  }
  .code-row code {
    flex: 1;
    min-width: 0;
    color: var(--fg-strong);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .token-row,
  .install {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .install + .token-row {
    margin-top: calc(var(--lh) * -1);
  }
  .token-row .label,
  .install .label {
    width: 12ch;
  }
  .token-row input {
    flex: 1;
    min-width: 24ch;
    max-width: 46ch;
  }
  .install select {
    width: auto;
    min-width: 24ch;
  }
  .row-end {
    display: flex;
    justify-content: flex-end;
  }
  .shared {
    display: flex;
    gap: 0 2ch;
  }
  .shared > :global(.ic) {
    color: var(--yellow);
  }
  .shared div {
    display: grid;
    gap: var(--lh) 0;
    justify-items: start;
  }
  .warn {
    color: var(--yellow);
  }
  .who {
    display: flex;
    flex-wrap: wrap;
    gap: 0 1ch;
  }
  /* A machine that reports is a green chip, one that does not a muted one. */
  .who-chip {
    padding: 0 1ch;
    color: var(--fg-muted);
    background: var(--bg-hover);
  }
  .who-chip.on {
    color: var(--green);
    background: color-mix(in srgb, var(--green) 14%, transparent);
  }
  .who-chip:hover {
    color: var(--on-accent);
    background: var(--accent);
  }
</style>
