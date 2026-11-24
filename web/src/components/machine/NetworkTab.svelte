<script lang="ts">
  import type { Host } from '../../lib/api'
  import { fleet, managedValue, setOption, unsetOption, useValues, valueOf } from '../../lib/fleet.svelte'
  import { codeLink } from '../../lib/router.svelte'
  import { effective } from '../../lib/values'
  import { frame } from '../../lib/ascii'
  import Icon from '../Icon.svelte'
  import SourceTag from '../SourceTag.svelte'

  let { host }: { host: Host } = $props()

  const name = 'networking.firewall.allowedTCPPorts'
  const path = ['networking', 'firewall', 'allowedTCPPorts']
  const vals = useValues(() => ({ names: [name], hosts: [host.name] }))
  const eff = $derived(effective(valueOf(vals.values, name, host.name), managedValue(host.name, path), host.managedFile, !!fleet.managed[host.name]))
  const ports = $derived(eff?.state === 'value' && Array.isArray(eff.value) ? (eff.value as number[]) : [])
  const editable = $derived(!!host.managedFile && !!fleet.managed[host.name])
  /** The list is written whole: only from a known, current value. */
  const canEdit = $derived(editable && eff?.state === 'value' && !eff.pending)

  // A number input binds a number, or null while it is empty.
  let port = $state<number | null>(null)
  let busy = $state(false)
  const portProblem = $derived.by(() => {
    if (port == null) return ''
    if (!Number.isInteger(port) || port < 1 || port > 65535) return 'A port is a whole number from 1 to 65535.'
    if (ports.includes(port)) return `${port} is already open.`
    return ''
  })

  // Lists are written whole: the list set here replaces every hand-written one, so it starts from the effective list.
  async function save(next: number[], done: string) {
    busy = true
    const ok = await setOption(host.name, path, [...new Set(next)].sort((a, b) => a - b), done)
    busy = false
    return ok
  }
  async function add(e: SubmitEvent) {
    e.preventDefault()
    if (port == null || portProblem) return
    if (await save([...ports, port], `Opened TCP port ${port} on ${host.name}`)) port = null
  }
  async function reset() {
    busy = true
    await unsetOption(host.name, path, `The allowed TCP ports on ${host.name} come from its Nix files again`)
    busy = false
  }
</script>

<div class="grid">
  <section class="card block" use:frame={{ title: 'Firewall' }}>
    <h2 class="sr-only"><Icon name="shield" /> Firewall</h2>
    <p class:off={!host.ports.firewall} class="fw">{host.ports.firewall ? 'On: only the ports below accept connections.' : 'Off: every port is reachable.'}</p>
    <div class="ports">
      <span class="label">TCP</span>
      <div class="chips">{#each host.ports.tcp as p (p)}<span class="chip mono num">{p}</span>{:else}<span class="muted small">none</span>{/each}</div>
      <span class="label">UDP</span>
      <div class="chips">{#each host.ports.udp as p (p)}<span class="chip mono num">{p}</span>{:else}<span class="muted small">none</span>{/each}</div>
    </div>
    <p class="muted small">Every open port as evaluated, whoever opened it: services with <span class="mono">openFirewall</span>, hand-written Nix, or the list below.</p>
  </section>

  <section class="card block" use:frame={{ title: 'Allowed TCP ports' }}>
    <div class="b-head">
      <h2 class="sr-only"><Icon name="globe" /> Allowed TCP ports</h2>
      <p class="muted small opt"><span class="mono">{name}</span></p>
      {#if eff?.state === 'value'}<SourceTag source={eff.source} pending={eff.pending} />{/if}
    </div>
    {#if vals.error}
      <p class="error-text small" role="alert">{vals.error}</p>
    {:else if !eff}
      <div class="skels" aria-busy="true"><span class="skel" style:width="30ch"></span><span class="skel" style:width="20ch"></span></div>
    {:else}
      <div class="chips">
        {#each ports as p (p)}
          <span class="chip mono num"
            >{p}{#if canEdit}<button class="x icon" aria-label="Close port {p}" title="Close {p}" disabled={busy} onclick={() => save(ports.filter((x) => x !== p), `Closed TCP port ${p} on ${host.name}`)}><Icon name="x" /></button>{/if}</span>
        {:else}
          <span class="muted small">No TCP ports in this list.</span>
        {/each}
      </div>
      {#if !host.managedFile}
        <p class="muted small">{host.name} is Code only: open ports in {#if eff.state === 'value' && eff.source.kind === 'code'}<a href={codeLink(eff.source.file)}>{eff.source.file}</a>{:else}its Nix files{/if}.</p>
      {:else if eff.state === 'value' && eff.pending}
        <p class="muted small note">Reset here: the list that applies now shows once {host.name} is evaluated again.</p>
      {:else if editable}
        <form class="add" onsubmit={add}>
          <b class="prompt" aria-hidden="true">›</b>
          <input class="mono num" type="number" min="1" max="65535" placeholder="Port, e.g. 8080" aria-label="TCP port to open" bind:value={port} aria-invalid={!!portProblem} />
          <button type="submit" disabled={port == null || !!portProblem || busy}>Open port</button>
          {#if eff.state === 'value' && eff.source.kind === 'here'}<button type="button" class="ghost" disabled={busy} onclick={reset} title="Remove the list from {host.managedFile}">Reset</button>{/if}
        </form>
        {#if portProblem}<p class="problem" role="alert">{portProblem}</p>{/if}
        {#if eff.state === 'value' && eff.source.kind !== 'here'}
          <p class="muted small note">Changing the list sets it here, whole: it keeps the ports above and wins over the hand-written list from then on.</p>
        {/if}
      {/if}
    {/if}
  </section>
</div>

<section class="section">
  <h2>File systems</h2>
  <table class="data">
    <thead><tr><th>Mount point</th><th>Type</th><th>Device</th></tr></thead>
    <tbody>
      {#each host.fileSystems as fs (fs.mountPoint)}
        <tr><td class="mono strong">{fs.mountPoint}</td><td>{fs.fsType}</td><td class="mono muted">{fs.device}</td></tr>
      {:else}
        <tr><td colspan="3" class="muted">No file systems declared.</td></tr>
      {/each}
    </tbody>
  </table>
</section>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(48ch, 1fr));
    gap: var(--lh) 2ch;
  }
  .block {
    display: grid;
    align-content: start;
    gap: var(--lh);
  }
  .b-head {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .opt {
    flex: 1;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  /* The state is a glyph and a word. */
  .fw {
    color: var(--green);
  }
  .fw::before {
    content: '● ';
  }
  .fw.off {
    color: var(--yellow);
  }
  .fw.off::before {
    content: '▲ ';
  }
  .ports {
    display: grid;
    grid-template-columns: 4ch 1fr;
    gap: 0 2ch;
  }
  .add {
    display: flex;
    align-items: baseline;
    gap: 0 2ch;
  }
  .add .prompt {
    margin-right: -1ch;
  }
  .add input {
    width: 20ch;
  }
  .problem {
    color: var(--red);
  }
  table.data th {
    top: calc(var(--lh) * 2); /* below the machine tabs */
  }
</style>
