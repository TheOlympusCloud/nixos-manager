<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import MachineCard from '../components/MachineCard.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import NewMachine from '../components/NewMachine.svelte'
  import { fleet } from '../lib/fleet.svelte'
  import { age, plural } from '../lib/format'
  import { spotlight } from '../lib/motion'
  import { machineLink, replace, route } from '../lib/router.svelte'
  import { app, startJob } from '../lib/state.svelte'

  let filter = $state('')
  const hosts = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    const all = fleet.model?.hosts ?? []
    return q ? all.filter((h) => [h.name, h.hostName, h.system, ...h.features.map((f) => f.name)].some((s) => s.toLowerCase().includes(q))) : all
  })
  const updating = $derived(app.jobs.some((j) => j.kind === 'update' && (j.state === 'running' || j.state === 'queued')))
  // #/machines?new=1 opens the wizard (the command palette links there).
  let wizard = $state(false)
  const wizardOpen = $derived(wizard || route.query.new === '1')
  function closeWizard() {
    wizard = false
    if (/[?&]new=1/.test(location.hash)) replace('#/machines') // not when the wizard already moved on to the new machine
  }
</script>

{#if wizardOpen}<NewMachine onclose={closeWizard} />{/if}

<div class="page">
  <div class="page-inner">
    <div class="page-head">
      <h1>Machines</h1>
      {#if fleet.model?.hosts.length}<span class="sub num">{plural(fleet.model.hosts.length, 'machine')} in <span class="mono">nixosConfigurations</span></span>{/if}
      <span class="spacer"></span>
      {#if fleet.model?.hosts.length}<input class="filter" type="search" placeholder="Filter by name, system or service" aria-label="Filter machines" data-search bind:value={filter} />{/if}
      <button class="primary" onclick={() => (wizard = true)}><Icon name="plus" size={14} />New machine</button>
    </div>
    <ModelGate>
      {@const m = fleet.model!}

      <div class="grid stagger">
        {#each hosts as h (h.name)}
          <a class="card lift machine" class:err={!!h.error} href={machineLink(h.name)} aria-label="Open {h.name}" use:spotlight>
            <MachineCard host={h} />
          </a>
        {:else}
          <p class="muted">No machine matches “{filter}”.</p>
        {/each}
      </div>

      {#if m.homes.length}
        <section class="section">
          <h2>Home configurations</h2>
          <p class="muted small intro">Standalone home-manager configurations (<span class="mono">homeConfigurations</span>), for machines that are not NixOS or not managed here.</p>
          <div class="homes stagger">
            {#each m.homes as home (home.name)}
              <div class="card home" class:err={!!home.error}>
                <div class="home-top"><Icon name="user" size={15} /><strong>{home.name}</strong></div>
                {#if home.error}
                  <p class="error-text small mono">{home.error.split('\n')[0]}</p>
                {:else}
                  <p class="muted small">{plural(home.packages.length, 'package')} · {plural(home.programs.length, 'program')}</p>
                  <div class="chips">{#each home.programs as p (p)}<span class="chip g-programs">{p}</span>{/each}</div>
                {/if}
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <section class="section">
        <h2>Flake inputs</h2>
        {#if m.inputs.length}
          <table class="data card-table">
            <thead><tr><th>Input</th><th>Source</th><th>Revision</th><th>Updated</th><th><span class="sr-only">Actions</span></th></tr></thead>
            <tbody>
              {#each m.inputs as i (i.name)}
                <tr>
                  <td><strong>{i.name}</strong></td>
                  <td class="mono small">{i.url}</td>
                  <td class="mono small">{i.rev.slice(0, 12)}</td>
                  <td title={new Date(i.lastModified * 1000).toLocaleString()}>{age(i.lastModified)}</td>
                  <td class="act"><button class="small" disabled={updating} onclick={() => startJob({ kind: 'update', inputs: [i.name] })}>Update</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
          <div class="below">
            <button disabled={updating} onclick={() => startJob({ kind: 'update', inputs: [] })}><Icon name="refresh" size={14} />Update all inputs</button>
            <span class="muted small">Updates change <span class="mono">flake.lock</span> in the working copy; review and commit them from Changes.</span>
          </div>
        {:else}
          <div class="card empty-inputs">
            <p class="muted">No <span class="mono">flake.lock</span> yet, so the inputs are not pinned. Lock the flake to record the exact revision of nixpkgs and every other input.</p>
            <button onclick={() => startJob({ kind: 'lock' })}><Icon name="key" size={14} />Lock the flake</button>
          </div>
        {/if}
      </section>
    </ModelGate>
  </div>
</div>

<style>
  .filter {
    width: 280px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: var(--s3);
  }
  .machine {
    height: 148px;
    color: inherit;
    text-decoration: none;
  }
  .machine.err {
    --lift-c: var(--red);
    background: linear-gradient(180deg, color-mix(in srgb, var(--red) 7%, var(--bg-raised)), var(--bg-raised) 70%);
    border-color: color-mix(in srgb, var(--red) 35%, var(--border));
  }
  .intro {
    margin: -6px 0 12px;
  }
  .homes {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: var(--s3);
  }
  .home {
    display: grid;
    gap: 8px;
    padding: 12px 14px;
  }
  .home-top {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--g-programs);
  }
  .home-top strong {
    color: var(--fg-strong);
  }
  .card-table {
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .card-table tbody tr:last-child td {
    border-bottom: 0;
  }
  .act {
    text-align: right;
  }
  .below {
    display: flex;
    align-items: center;
    gap: var(--s3);
    margin-top: var(--s3);
  }
  .empty-inputs {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s4);
    padding: var(--s4);
  }
</style>
