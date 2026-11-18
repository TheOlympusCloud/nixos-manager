<script lang="ts">
  import type { Host } from '../../lib/api'
  import { fleet, managedValue, setOption, unsetOption } from '../../lib/fleet.svelte'
  import Icon from '../Icon.svelte'

  let { host }: { host: Host } = $props()

  const path = ['networking', 'firewall', 'allowedTCPPorts']
  const managedPorts = $derived.by(() => {
    const v = managedValue(host.name, path)
    return Array.isArray(v) ? (v as number[]) : []
  })
  let port = $state('')
  let busy = $state(false)
  const portProblem = $derived.by(() => {
    if (!port.trim()) return ''
    const n = Number(port)
    if (!Number.isInteger(n) || n < 1 || n > 65535) return 'A port is a whole number from 1 to 65535.'
    if (managedPorts.includes(n)) return `${n} is already in the list.`
    return ''
  })

  async function save(ports: number[]) {
    busy = true
    if (ports.length) await setOption(host.name, path, [...ports].sort((a, b) => a - b))
    else await unsetOption(host.name, path)
    busy = false
  }
  async function add(e: SubmitEvent) {
    e.preventDefault()
    if (!port.trim() || portProblem) return
    await save([...managedPorts, Number(port)])
    port = ''
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
    <p class="muted small">All open ports, whoever opened them (services with <span class="mono">openFirewall</span>, hand-written Nix, or the list set here).</p>
  </section>

  <section class="card block">
    <h2><Icon name="globe" size={16} />Allowed TCP ports set here</h2>
    {#if !host.managedFile}
      <p class="muted">Visual editing is off on {host.name}; open ports in Code.</p>
    {:else if !fleet.managed[host.name]}
      <span class="skel" style:width="50%"></span>
    {:else}
      <p class="muted small"><span class="mono">networking.firewall.allowedTCPPorts</span> in <span class="mono">{host.managedFile}</span>.</p>
      <div class="chips">
        {#each managedPorts as p (p)}
          <span class="chip mono num"
            >{p}<button class="x" aria-label="Close port {p}" title="Remove {p}" disabled={busy} onclick={() => save(managedPorts.filter((x) => x !== p))}><Icon name="x" size={11} /></button></span>
        {:else}
          <span class="muted small">No ports opened here yet.</span>
        {/each}
      </div>
      <form class="add" onsubmit={add}>
        <input class="mono num" type="number" min="1" max="65535" placeholder="Port, e.g. 8080" aria-label="TCP port to open" bind:value={port} aria-invalid={!!portProblem} />
        <button type="submit" disabled={!port.trim() || !!portProblem || busy}>Open port</button>
      </form>
      {#if portProblem}<p class="problem" role="alert">{portProblem}</p>{/if}
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
  h2 {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: var(--fs-md);
  }
  h2 :global(svg) {
    color: var(--fg-muted);
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
