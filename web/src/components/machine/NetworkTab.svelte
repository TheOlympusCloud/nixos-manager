<script lang="ts">
  import type { Host } from '../../lib/api'
  import { fleet, managedValue, setOption, unsetOption, useValues, valueOf } from '../../lib/fleet.svelte'
  import { codeLink } from '../../lib/router.svelte'
  import { effective } from '../../lib/values'
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
  <section class="card block">
    <h2><Icon name="shield" size={16} />Firewall</h2>
    <p class:off={!host.ports.firewall} class="fw">{host.ports.firewall ? 'On: only the ports below accept connections.' : 'Off: every port is reachable.'}</p>
    <div class="ports">
      <span class="label">TCP</span>
      <div class="chips">{#each host.ports.tcp as p (p)}<span class="chip mono num">{p}</span>{:else}<span class="muted small">none</span>{/each}</div>
      <span class="label">UDP</span>
      <div class="chips">{#each host.ports.udp as p (p)}<span class="chip mono num">{p}</span>{:else}<span class="muted small">none</span>{/each}</div>
    </div>
    <p class="muted small">Every open port as evaluated, whoever opened it: services with <span class="mono">openFirewall</span>, hand-written Nix, or the list below.</p>
  </section>

  <section class="card block">
    <div class="b-head">
      <h2><Icon name="globe" size={16} />Allowed TCP ports</h2>
      {#if eff?.state === 'value'}<SourceTag source={eff.source} pending={eff.pending} />{/if}
    </div>
    <p class="muted small"><span class="mono">{name}</span></p>
    {#if vals.error}
      <p class="error-text small" role="alert">{vals.error}</p>
    {:else if !eff}
      <div class="skels" aria-busy="true"><span class="skel" style:width="55%"></span><span class="skel" style:width="35%"></span></div>
    {:else}
      <div class="chips">
        {#each ports as p (p)}
          <span class="chip mono num"
            >{p}{#if canEdit}<button class="x" aria-label="Close port {p}" title="Close {p}" disabled={busy} onclick={() => save(ports.filter((x) => x !== p), `Closed TCP port ${p} on ${host.name}`)}><Icon name="x" size={11} /></button>{/if}</span>
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
  <div class="table-wrap">
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
  </div>
</section>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
    gap: var(--s3);
  }
  .block {
    display: grid;
    align-content: start;
    gap: 12px;
    padding: 14px 16px;
  }
  .b-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  h2 {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: var(--fs-md);
  }
  h2 :global(svg) {
    color: var(--fg-muted);
  }
  .block > .muted.small:first-of-type {
    margin-top: -6px;
  }
  .fw {
    color: var(--green);
  }
  .fw.off {
    color: var(--yellow);
  }
  .ports {
    display: grid;
    grid-template-columns: 44px 1fr;
    gap: 10px;
    align-items: center;
  }
  .skels {
    display: grid;
    gap: 10px;
  }
  .add {
    display: flex;
    gap: 8px;
  }
  .add input {
    width: 180px;
  }
  .problem {
    color: var(--red);
    font-size: var(--fs-sm);
  }
  .note {
    line-height: 1.5;
  }
  .table-wrap {
    overflow: clip;
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  table.data th {
    top: 41px;
  }
  tbody tr:last-child td {
    border-bottom: 0;
  }
  .strong {
    color: var(--fg-strong);
  }
</style>
