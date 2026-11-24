<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type ISO, type SecretsStatus } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { fleet } from '../lib/fleet.svelte'
  import { bytes } from '../lib/format'
  import { flashCommand, isoBuilding, isoState } from '../lib/iso'
  import { refreshJobs } from '../lib/state.svelte'
  import { ago, copyText } from '../lib/util'
  import Icon from './Icon.svelte'
  import JobLog from './JobLog.svelte'
  import HostKeyStep from './machine/HostKeyStep.svelte'

  /**
   * Machines → Installer ISO: a USB stick that installs one machine as the repository has it, built on the server from
   * what is pushed; then how to install it, with the step that approves its SSH host key for its secrets.
   */
  let { onnew }: { onnew: () => void } = $props()

  let isos = $state<ISO[] | null>(null)
  let host = $state('')
  let busy = $state(false)
  let error = $state('')
  let logOf = $state('') // the build whose log is open
  let chosen = $state('') // the ISO whose install steps are shown
  let secrets = $state<SecretsStatus | null>(null)
  let copied = $state('')

  const hosts = $derived(fleet.model?.hosts.map((h) => h.name) ?? [])
  const shown = $derived(isos?.find((i) => i.id === chosen && i.state === 'ready') ?? isos?.find((i) => i.state === 'ready') ?? null)
  const show = (e: unknown) => (error = e instanceof Error ? e.message : String(e))

  $effect(() => {
    if (!hosts.includes(host)) host = hosts[0] ?? ''
  })

  let alive = true
  let timer: ReturnType<typeof setTimeout> | undefined
  async function load() {
    clearTimeout(timer)
    try {
      isos = await api.isos()
      await refreshJobs() // its toast says when a build ends
    } catch (e) {
      show(e)
    }
    if (alive && isos && isoBuilding(isos)) timer = setTimeout(load, 3000)
  }
  onMount(() => {
    load()
    api.secrets().then((st) => (secrets = st), () => (secrets = null)) // off on this server: the key step says so
    return () => {
      alive = false
      clearTimeout(timer)
    }
  })

  async function build(e: SubmitEvent) {
    e.preventDefault()
    if (!host) return
    busy = true
    error = ''
    try {
      const iso = await api.buildISO(host)
      logOf = iso.id
    } catch (err) {
      show(err)
    } finally {
      busy = false
    }
    load()
  }

  async function copy(text: string) {
    copied = (await copyText(text)) ? text : ''
    setTimeout(() => (copied = ''), 2000)
  }
</script>

{#snippet command(text: string)}
  <span class="cmd">
    <code>{text}</code>
    <button type="button" class="small" onclick={() => copy(text)}><Icon name={copied === text ? 'tick' : 'copy'} /> {copied === text ? 'Copied' : 'Copy'}</button>
  </span>
{/snippet}

<section class="section iso" aria-labelledby="iso-head" use:frame={{ title: 'Installer ISO' }}>
  <h2 id="iso-head" class="sr-only">Installer ISO</h2>
  <p class="muted">
    A USB stick that installs one machine as the repository has it: the NixOS installer from the fleet's nixpkgs, and <span class="mono">sudo fleet-install</span>,
    which fetches the repository, partitions the disk with the machine's disk layout and installs it, already enrolled: comin pulls from the repository and the machine
    reports to Deployments. Nothing secret is on it.
  </p>
  <form class="build" onsubmit={build}>
    <label for="iso-host">Machine</label>
    <select id="iso-host" bind:value={host} disabled={!hosts.length}>
      {#each hosts as h (h)}<option value={h}>{h}</option>{/each}
    </select>
    <button class="primary" type="submit" disabled={!host || busy}><Icon name="disk" /> {busy ? 'Starting…' : 'Build ISO'}</button>
    <span class="muted">or</span>
    <button type="button" onclick={onnew}><Icon name="plus" /> New machine</button>
  </form>
  <p class="muted small">
    It installs the machine as it is pushed: commit and push a new one in <a href="#/changes">Changes</a> before you build its ISO. A build runs apart from the checks; the
    last 3 ISOs are kept.
  </p>
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  {#if isos?.length}
    <table class="data">
      <thead><tr><th>Machine</th><th>Commit</th><th>State</th><th>Size</th><th>SHA-256</th><th><span class="sr-only">Actions</span></th></tr></thead>
      <tbody>
        {#each isos as i (i.id)}
          {@const st = isoState(i)}
          <tr>
            <td><strong>{i.host}</strong></td>
            <td><span class="mono">{i.commit.slice(0, 7)}</span> <span class="muted">{i.branch}</span></td>
            <td><span class="dot {st.dot}"></span> {st.text} <span class="muted">{ago(i.finished ?? i.started)}</span></td>
            <td class="num">{i.size ? bytes(i.size) : ''}</td>
            <td class="mono sha" title={i.sha256}>{i.sha256 ? `${i.sha256.slice(0, 12)}…` : ''}</td>
            <td class="act">
              {#if i.state === 'ready'}
                <a class="btn small" href={api.isoUrl(i.id)} download={i.file}><Icon name="arrow-down" /> Download</a>
                <button class="small" onclick={() => (chosen = i.id)} aria-pressed={shown?.id === i.id}>Install steps</button>
              {:else if i.job}
                <button class="link" onclick={() => (logOf = logOf === i.id ? '' : i.id)} aria-expanded={logOf === i.id}>{logOf === i.id ? 'Hide the log' : 'Show the log'}</button>
              {/if}
            </td>
          </tr>
          {#if logOf === i.id && i.job && i.state !== 'ready'}
            <tr class="log-row"><td colspan="6">{#key i.job}<JobLog jobId={i.job} compact />{/key}</td></tr>
          {/if}
        {/each}
      </tbody>
    </table>
  {/if}

  {#if shown}
    <div class="install">
      <h3>Installing {shown.host} from <span class="mono">{shown.file}</span></h3>
      <ol class="steps">
        <li>
          Check the download: <span class="mono">sha256sum {shown.file}</span> prints <span class="mono sum">{shown.sha256}</span>. Then write it to a USB stick, which
          it erases (<span class="mono">lsblk</span> names the stick):
          {@render command(flashCommand(shown.file ?? ''))}
        </li>
        <li>
          Boot {shown.host} from the stick and run <span class="mono">sudo fleet-install</span>. It shows the machine's addresses and the SSH host key {shown.host} will
          keep, and waits.
        </li>
        <li>
          Its key, so that {shown.host} opens its secrets and its report token from its first start:
          <HostKeyStep host={shown.host} bind:secrets />
          Then commit and push the secrets in <a href="#/changes">Changes</a>.
        </li>
        <li>
          Back on {shown.host}, press Enter. fleet-install fetches the repository (a private one asks for a Forgejo user and a token), erases the disk once you type the
          machine's name, installs it, and pushes the hardware configuration it wrote. Take out the stick and reboot.
        </li>
      </ol>
    </div>
  {/if}
</section>

<style>
  .build {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
    margin-top: var(--lh);
  }
  .build select {
    width: 32ch;
  }
  .build + p,
  .error-text,
  .data,
  .install {
    margin-top: var(--lh);
  }
  .act {
    white-space: nowrap;
    text-align: right;
  }
  .log-row td {
    padding: 0;
  }
  .install h3 {
    font-weight: 700;
  }
  .steps {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
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
  .sum {
    word-break: break-all;
  }
  .cmd {
    display: flex;
    align-items: flex-start;
    gap: 0 2ch;
    margin-top: var(--lh);
  }
  .cmd code {
    flex: 1;
    min-width: 0;
    padding: 0 1ch;
    color: var(--fg-strong);
    background: var(--bg-input);
  }
</style>
