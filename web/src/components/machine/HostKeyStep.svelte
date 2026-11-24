<script module lang="ts">
  import type { HostKey as Key } from '../../lib/api'
  // The address and key per machine, while the console is open: committing the secrets in Changes, or a new
  // evaluation, unmounts the cards that show this step.
  const kept = new Map<string, { address: string; key: Key | null }>()
</script>

<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type HostKey, type SecretsStatus } from '../../lib/api'
  import { addressProblem } from '../../lib/install'
  import { go } from '../../lib/router.svelte'
  import { refresh } from '../../lib/state.svelte'
  import { confirmBox, toast } from '../../lib/ui.svelte'
  import Icon from '../Icon.svelte'

  /**
   * A booted installer's SSH host key, read on a click (the handshake only, never a login), and approved for the
   * machine's secrets with its report token: the install copies the key to the machine, which then opens them.
   */
  let { host, secrets = $bindable(), address = $bindable('') }: { host: string; secrets: SecretsStatus | null; address?: string } = $props()

  let key = $state<HostKey | null>(null)
  let busy = $state('')
  let error = $state('')

  const problem = $derived(addressProblem(address.trim()))
  const ready = $derived(!!address.trim() && !problem)
  const secretsOn = $derived(!!secrets && !secrets.missing.length)
  const approved = $derived(secrets?.machines.find((s) => s.name === host) ?? null)
  const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

  $effect(() => {
    const h = host
    untrack(() => {
      address = kept.get(h)?.address ?? ''
      key = kept.get(h)?.key ?? null
      error = ''
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
      error = message(err)
    } finally {
      busy = ''
    }
  }

  async function approve() {
    const k = key
    if (!k) return
    const ok = await confirmBox({
      title: `Approve ${host}'s key`,
      message: `${k.fingerprint}: is this the key of the system you booted on ${host}? Compare it with ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub there. The install copies it to ${host}, which decrypts its secrets with it, and whoever holds it can read them.${approved ? ` It replaces the key approved for ${host} now.` : ''}`,
      ok: 'Approve',
    })
    if (!ok) return
    busy = 'approve'
    error = ''
    try {
      await api.setMachineKey(host, k.key)
      // Its report token too, so that it reports to Deployments from its first start.
      let token = true
      await api.storeReportTokens([host]).catch((err) => {
        token = false
        error = `The key is approved, but ${host}'s report token was not stored: ${message(err)}`
      })
      toast(`${host}'s secrets${token ? ', with its report token,' : ''} are encrypted for this key. Commit and push them before you install.`, 'ok', undefined, {
        label: 'View changes',
        run: () => go('#/changes'),
      })
      const [st] = await Promise.all([api.secrets(), refresh()])
      secrets = st
    } catch (err) {
      error = message(err)
    } finally {
      busy = ''
    }
  }
</script>

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
      <span class="ok-text"><Icon name="tick" /> {host}'s secrets are encrypted for this key.</span>
    {:else if secretsOn}
      {#if approved}<span class="warn"><Icon name="alert" /> Not the key approved for {host}: its secrets do not open with this one.</span>{/if}
      <button class="small primary" onclick={approve} disabled={!!busy}>{busy === 'approve' ? 'Approving…' : 'Use it for secrets'}</button>
    {/if}
  </div>
{/if}
{#if error}<span class="error-text" role="alert">{error}</span>{/if}

<style>
  .error-text {
    display: block;
  }
  .fetch,
  .key {
    margin-top: var(--lh);
  }
  .fetch {
    display: flex;
    gap: 0 2ch;
  }
  .fetch input {
    flex: 1;
    min-width: 0;
  }
  .key {
    display: grid;
    justify-items: start;
  }
  .ok-text {
    color: var(--green);
  }
  .warn {
    color: var(--yellow);
  }
  .ok-text :global(.ic),
  .warn :global(.ic) {
    color: inherit;
  }
</style>
