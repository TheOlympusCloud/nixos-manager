<script module lang="ts">
  import type { HostKey as Key } from '../../lib/api'
  // The address and key per machine, while the console is open: committing the secrets in Changes, or a new
  // evaluation, unmounts this card.
  const kept = new Map<string, { address: string; key: Key | null }>()
</script>

<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type HostKey, type SecretsStatus } from '../../lib/api'
  import { deployOf } from '../../lib/deployments.svelte'
  import { addressProblem, installCommand, installReads } from '../../lib/install'
  import { codeLink, go } from '../../lib/router.svelte'
  import { refresh } from '../../lib/state.svelte'
  import { confirmBox, toast } from '../../lib/ui.svelte'
  import { copyText } from '../../lib/util'
  import Icon from '../Icon.svelte'

  /**
   * Installing a machine that has a disk layout (hosts/<host>/disk-config.nix) with nixos-anywhere. The console only
   * reads the installer's SSH host key, on a click; the install is a command a person runs.
   */
  let { host }: { host: string } = $props()

  const dir = $derived(`hosts/${host}`)
  let disk = $state('')
  let generate = $state<boolean | null>(true) // hardware-configuration.nix is still the placeholder; null: unknown
  let secrets = $state<SecretsStatus | null>(null)
  let loadErrors = $state<string[]>([])
  let address = $state('')
  let key = $state<HostKey | null>(null)
  let busy = $state('')
  let error = $state('')
  let copied = $state(false)

  const m = $derived(deployOf(host))
  const installed = $derived(!!m && (m.state !== 'never' || !!m.lastSeen))
  const problem = $derived(addressProblem(address.trim()))
  const ready = $derived(!!address.trim() && !problem)
  const command = $derived(installCommand(host, address.trim(), !!generate))
  const canCopy = $derived(ready && generate !== null)
  const secretsOn = $derived(!!secrets && !secrets.missing.length)
  const approved = $derived(secrets?.machines.find((s) => s.name === host) ?? null)
  const show = (e: unknown) => (error = e instanceof Error ? e.message : String(e))

  $effect(() => {
    const h = host
    untrack(() => {
      address = kept.get(h)?.address ?? ''
      key = kept.get(h)?.key ?? null
      error = ''
    })
    Promise.allSettled([api.file(`hosts/${h}/disk-config.nix`), api.file(`hosts/${h}/hardware-configuration.nix`), api.secrets()]).then(([d, hw, st]) => {
      if (h !== host) return
      ;({ disk, generate, secrets, errors: loadErrors } = installReads(d, hw, st))
    })
  })

  $effect(() => {
    kept.set(untrack(() => host), { address, key })
  })

  async function fetchKey(e: SubmitEvent) {
    e.preventDefault()
    if (!ready) return
    busy = 'fetch'
    error = ''
    key = null
    try {
      key = await api.fetchHostKey(address.trim())
    } catch (err) {
      show(err)
    } finally {
      busy = ''
    }
  }

  async function approve() {
    const k = key
    if (!k) return
    const ok = await confirmBox({
      title: `Approve ${host}'s key`,
      message: `${k.fingerprint}: is this the key of the system you booted on ${host}? Compare it with ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub there. nixos-anywhere copies it to ${host}, which decrypts its secrets with it, and whoever holds it can read them.${approved ? ` It replaces the key approved for ${host} now.` : ''}`,
      ok: 'Approve',
    })
    if (!ok) return
    busy = 'approve'
    error = ''
    try {
      await api.setMachineKey(host, k.key)
      toast(`${host}'s secrets are encrypted for this key. Commit and push them before you install.`, 'ok', undefined, { label: 'View changes', run: () => go('#/changes') })
      const [st] = await Promise.all([api.secrets(), refresh()])
      secrets = st
    } catch (err) {
      show(err)
    } finally {
      busy = ''
    }
  }

  async function copy() {
    copied = await copyText(command)
    setTimeout(() => (copied = false), 2000)
  }
</script>

<section class="section install" aria-labelledby="install-title">
  <details open={!installed}>
    <summary><h2 id="install-title">Install with nixos-anywhere</h2></summary>
    <p class="muted small lead">
      {host} has a disk layout, <a class="mono" href={codeLink(`${dir}/disk-config.nix`)}>disk-config.nix</a>: nixos-anywhere erases
      {#if disk}<span class="mono">{disk}</span>{:else}the disk it names{/if}, partitions it and installs {host} on it. This console never starts an install: you
      run it.
    </p>
    {#each loadErrors as e (e)}<p class="error-text" role="alert">{e}</p>{/each}
    <ol class="steps">
      <li>Commit and push <span class="mono">{dir}/</span> in <a href="#/changes">Changes</a>: nixos-anywhere installs what the repository has.</li>
      <li>
        Boot {host} from the NixOS installer (or any Linux reachable as root over SSH: nixos-anywhere replaces it) and let root in over SSH: <span class="mono"
          >sudo passwd</span
        > on the installer, or your key in <span class="mono">/root/.ssh/authorized_keys</span>.
      </li>
      <li>
        Its SSH host key. nixos-anywhere copies the installer's key to {host}{#if secretsOn}, which decrypts its secrets with it{/if}.
        <form class="fetch" onsubmit={fetchKey}>
          <input class="mono" bind:value={address} oninput={() => ((key = null), (error = ''))} placeholder="192.168.1.50" aria-label="Address of {host}" aria-invalid={!!problem} spellcheck="false" autocomplete="off" />
          <button type="submit" disabled={!ready || !!busy}>{busy === 'fetch' ? 'Fetching…' : 'Fetch host key'}</button>
        </form>
        {#if problem}<span class="error-text">{problem}</span>{/if}
        {#if key}
          <div class="key" role="status">
            <span class="mono">{key.fingerprint}</span>
            <span class="muted small">Compare it with <span class="mono">ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub</span> on the machine.</span>
            {#if secrets && !secretsOn}
              <span class="muted small">Secrets are off on this server ({secrets.missing.join(', ')} not found).</span>
            {:else if secretsOn && approved?.key === key.key}
              <span class="ok-text"><Icon name="tick" size={13} />{host}'s secrets are encrypted for this key.</span>
            {:else if secretsOn}
              {#if approved}<span class="warn"><Icon name="alert" size={13} />Not the key approved for {host}: its secrets do not open with this one.</span>{/if}
              <button class="small primary" onclick={approve} disabled={!!busy}>{busy === 'approve' ? 'Approving…' : 'Use it for secrets'}</button>
            {/if}
          </div>
        {/if}
        {#if error}<span class="error-text" role="alert">{error}</span>{/if}
      </li>
      <li>
        {secretsOn ? 'Commit and push the secrets. Then, in' : 'In'} a clone of the repository, on a computer with Nix:
        <div class="cmd">
          <!-- Wrap between arguments only. -->
          <code>{#each command.split(' ') as arg, i (i)}<span class="nw">{arg}</span>{' '}{/each}</code>
          <button
            type="button"
            class="small"
            onclick={copy}
            disabled={!canCopy}
            title={!ready ? 'Enter its address first' : generate === null ? 'hardware-configuration.nix could not be read' : undefined}
            ><Icon name={copied ? 'tick' : 'copy'} size={13} />{copied ? 'Copied' : 'Copy'}</button
          >
        </div>
        <span class="muted small">
          {#if generate === null}
            Until hardware-configuration.nix can be read, this command is not known.
          {:else if generate}
            It erases the disk, installs {host} and reboots it, then commits and pushes the hardware configuration nixos-anywhere wrote, so that comin deploys the same
            system.
          {:else}
            It erases the disk, installs {host} with the hardware configuration in the repository, and reboots it.
          {/if}
        </span>
      </li>
    </ol>
  </details>
</section>

<style>
  summary {
    list-style-position: outside;
  }
  summary h2 {
    display: inline;
    font-size: var(--fs-lg);
  }
  .lead {
    margin: var(--s3) 0 0;
    line-height: 1.55;
  }
  .steps {
    display: grid;
    gap: var(--s3);
    margin: var(--s3) 0 0;
    padding-left: 20px;
    line-height: 1.55;
  }
  .fetch,
  .key,
  .cmd,
  .steps .error-text,
  .steps li > .muted {
    margin-top: 6px;
  }
  .steps .error-text,
  .steps li > .muted {
    display: block;
  }
  .fetch {
    display: flex;
    gap: 8px;
  }
  .fetch input {
    flex: 1;
    min-width: 0;
  }
  .key {
    display: grid;
    justify-items: start;
    gap: 4px;
  }
  .ok-text,
  .warn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--green);
    font-size: var(--fs-sm);
  }
  .warn {
    color: var(--yellow);
  }
  .cmd {
    display: flex;
    align-items: flex-start;
    gap: 8px;
  }
  .cmd code {
    flex: 1;
    min-width: 0;
    padding: 6px 10px;
    color: var(--fg-strong);
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .nw {
    white-space: nowrap;
  }
</style>
