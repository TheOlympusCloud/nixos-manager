<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import MachineCard from '../components/MachineCard.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import NewMachine from '../components/NewMachine.svelte'
  import { fleet } from '../lib/fleet.svelte'
  import { age, arch, plural } from '../lib/format'
  import { bigText, frame } from '../lib/ascii'
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
      <div>
        <h1 class="big" aria-label="Machines" use:bigText={'Machines'}></h1>
        {#if fleet.model?.hosts.length}<p class="sub num">{plural(fleet.model.hosts.length, 'machine')} in <span class="mono">nixosConfigurations</span></p>{/if}
      </div>
      <span class="spacer"></span>
      {#if fleet.model?.hosts.length}<span class="filter"><Icon name="filter" /><input type="search" placeholder="Filter by name, system or service" aria-label="Filter machines" data-search bind:value={filter} /></span>{/if}
      <button class="primary" onclick={() => (wizard = true)}><Icon name="plus" /> New machine</button>
    </div>
    <ModelGate>
      {@const m = fleet.model!}

      <div class="grid">
        {#each hosts as h (h.name)}
          <a
            class="card lift machine"
            class:err={!!h.error}
            href={machineLink(h.name)}
            aria-label="Open {h.name}"
            data-tone={h.error ? 'err' : undefined}
            use:frame={{ title: h.name, tag: h.system ? arch(h.system) : undefined, lift: true, animate: true }}>
            <MachineCard host={h} />
          </a>
        {:else}
          <p class="muted">No machine matches “{filter}”.</p>
        {/each}
      </div>

      {#if m.homes.length}
        <section class="section">
          <h2>Home configurations</h2>
          <p class="muted intro">Standalone home-manager configurations (<span class="mono">homeConfigurations</span>), for machines that are not NixOS or not managed here.</p>
          <div class="homes">
            {#each m.homes as home (home.name)}
              <div class="card home" class:err={!!home.error} data-tone={home.error ? 'err' : undefined} use:frame={{ title: home.name, animate: true }}>
                <strong class="sr-only">{home.name}</strong>
                {#if home.error}
                  <p class="error-text mono">{home.error.split('\n')[0]}</p>
                {:else}
                  <p><Icon name="user" /> {plural(home.packages.length, 'package')} · {plural(home.programs.length, 'program')}</p>
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
          <table class="data">
            <thead><tr><th>Input</th><th>Source</th><th>Revision</th><th>Updated</th><th><span class="sr-only">Actions</span></th></tr></thead>
            <tbody>
              {#each m.inputs as i (i.name)}
                <tr>
                  <td><strong>{i.name}</strong></td>
                  <td class="mono muted">{i.url}</td>
                  <td class="mono">{i.rev.slice(0, 12)}</td>
                  <td title={new Date(i.lastModified * 1000).toLocaleString()}>{age(i.lastModified)}</td>
                  <td class="act"><button class="small" disabled={updating} onclick={() => startJob({ kind: 'update', inputs: [i.name] })}>Update</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
          <div class="below">
            <button disabled={updating} onclick={() => startJob({ kind: 'update', inputs: [] })}><Icon name="refresh" /> Update all inputs</button>
            <span class="muted">Updates change <span class="mono">flake.lock</span> in the working copy; review and commit them from Changes.</span>
          </div>
        {:else}
          <div class="card empty-inputs" use:frame={'dash'}>
            <p class="muted">No <span class="mono">flake.lock</span> yet, so the inputs are not pinned. Lock the flake to record the exact revision of nixpkgs and every other input.</p>
            <button onclick={() => startJob({ kind: 'lock' })}><Icon name="key" /> Lock the flake</button>
          </div>
        {/if}
      </section>
    </ModelGate>
  </div>
</div>

<style>
  .filter {
    display: flex;
    gap: 1ch;
    width: 44ch;
  }
  .grid,
  .homes {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(38ch, 1fr));
    gap: var(--lh) 2ch;
  }
  .machine {
    height: calc(var(--lh) * 6);
    color: inherit;
  }
  /* The card is the link: its frame shows hover and focus, not a reverse-video block. */
  .machine:focus-visible {
    color: inherit;
    background: none;
  }
  /* The name and the system sit on the frame line. */
  .machine :global(.mc > .top) {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
  .intro {
    margin-bottom: var(--lh);
  }
  .home :global(.ic) {
    color: var(--g-programs);
  }
  .act {
    text-align: right;
  }
  .below {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
    margin-top: var(--lh);
  }
  .empty-inputs {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 4ch;
  }
</style>
