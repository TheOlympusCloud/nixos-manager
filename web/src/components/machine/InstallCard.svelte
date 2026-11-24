<script lang="ts">
  import { api, type SecretsStatus } from '../../lib/api'
  import { deployOf } from '../../lib/deployments.svelte'
  import { addressProblem, installCommand, installReads } from '../../lib/install'
  import { codeLink } from '../../lib/router.svelte'
  import { copyText } from '../../lib/util'
  import Icon from '../Icon.svelte'
  import HostKeyStep from './HostKeyStep.svelte'

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
  let copied = $state(false)

  const m = $derived(deployOf(host))
  const installed = $derived(!!m && (m.state !== 'never' || !!m.lastSeen))
  const problem = $derived(addressProblem(address.trim()))
  const ready = $derived(!!address.trim() && !problem)
  const command = $derived(installCommand(host, address.trim(), !!generate))
  const canCopy = $derived(ready && generate !== null)
  const secretsOn = $derived(!!secrets && !secrets.missing.length)

  $effect(() => {
    const h = host
    Promise.allSettled([api.file(`hosts/${h}/disk-config.nix`), api.file(`hosts/${h}/hardware-configuration.nix`), api.secrets()]).then(([d, hw, st]) => {
      if (h !== host) return
      ;({ disk, generate, secrets, errors: loadErrors } = installReads(d, hw, st))
    })
  })

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
        <HostKeyStep {host} bind:secrets bind:address />
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
            ><Icon name={copied ? 'tick' : 'copy'} /> {copied ? 'Copied' : 'Copy'}</button
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
  summary h2 {
    display: inline;
  }
  summary h2::before {
    content: none;
  }
  .lead {
    margin-top: var(--lh);
  }
  .steps {
    display: grid;
    grid-template-columns: minmax(0, 1fr); /* a long argument scrolls in its command, not the page */
    gap: var(--lh);
    margin-top: var(--lh);
    counter-reset: step;
  }
  .steps > li {
    position: relative;
    padding-left: 3ch;
    counter-increment: step;
  }
  .steps > li::before {
    position: absolute;
    left: 0;
    color: var(--accent);
    font-weight: 700;
    content: counter(step) '.';
  }
  .steps li > .muted {
    display: block;
  }
  .cmd {
    margin-top: var(--lh);
    display: flex;
    align-items: flex-start;
    gap: 0 2ch;
  }
  .cmd code {
    flex: 1;
    min-width: 0;
    overflow-x: auto;
    padding: 0 1ch;
    color: var(--fg-strong);
    background: var(--bg-input);
  }
  .nw {
    white-space: nowrap;
  }
</style>
