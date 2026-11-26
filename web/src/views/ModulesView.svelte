<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import ModulePage from '../components/modules/ModulePage.svelte'
  import { bigText, frame } from '../lib/ascii'
  import { createModule, fleet, healthy, loadModules, updateManagedModule } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { MODULE_FOLDER, machinesWith, moduleNameOk, usesModules } from '../lib/modules'
  import { go, link } from '../lib/router.svelte'

  /** #/modules lists the fleet modules; #/modules/<name> is one of them. */
  let { name }: { name?: string } = $props()

  onMount(() => {
    loadModules()
  })

  const modules = $derived(fleet.modules ?? [])
  const hosts = $derived(healthy())
  // Machines that load managed.json but whose managed.nix cannot apply modules yet.
  const older = $derived(hosts.filter((h) => h.managedFile && !usesModules(h)).map((h) => h.name))

  let creating = $state(false)
  let newName = $state('')
  let newDescription = $state('')
  let busy = $state(false)
  const nameProblem = $derived(
    !newName ? '' : !moduleNameOk(newName) ? 'Letters, digits, _ and -, starting with a letter or _' : modules.some((m) => m.name === newName) ? `There is already a module ${newName}` : '',
  )

  async function create(e: SubmitEvent) {
    e.preventDefault()
    if (!newName || nameProblem) return
    busy = true
    const made = await createModule(newName, newDescription)
    busy = false
    if (made) {
      go(link('modules', [newName]))
      creating = false
      newName = newDescription = ''
    }
  }
</script>

{#if name}
  {#key name}<ModulePage {name} />{/key}
{:else}
  <div class="page">
    <div class="page-inner">
      <ModelGate title="fleet modules">
        <div class="page-head">
          <div>
            <h1 class="big" aria-label="Modules" use:bigText={'Modules'}></h1>
            <p class="sub">Packages and settings bundled as a NixOS module, turned on machine by machine, with options each machine can set its own way. Kept in <span class="mono">{MODULE_FOLDER}/</span>.</p>
          </div>
          <span class="spacer"></span>
          <button class="primary" onclick={() => (creating = !creating)} aria-expanded={creating}><Icon name="plus" /> New module</button>
        </div>

        {#if older.length}
          <section class="card notice" data-tone="warn" use:frame={{ title: 'An older managed.nix' }} role="status">
            <p><Icon name="alert" /> {older.join(', ')} {older.length > 1 ? 'load' : 'loads'} a <span class="mono">modules/managed.nix</span> that cannot apply modules yet.</p>
            <button onclick={() => updateManagedModule()}><Icon name="refresh" /> Update managed.nix</button>
          </section>
        {/if}

        {#if creating}
          <form class="card create" use:frame={{ title: 'New module' }} onsubmit={create}>
            <label>
              <span>Name</span>
              <input class="mono" bind:value={newName} placeholder="media" required aria-invalid={!!nameProblem} aria-describedby="module-name-help" />
            </label>
            <span id="module-name-help" class="small" class:err={!!nameProblem} class:muted={!nameProblem}>{nameProblem || `Machines turn it on with fleet.modules.${newName || '<name>'}.enable`}</span>
            <label>
              <span>Description</span>
              <input bind:value={newDescription} placeholder="Jellyfin, its firewall and the media disk" />
            </label>
            <div class="row">
              <button class="primary" type="submit" disabled={busy || !newName || !!nameProblem}>{busy ? 'Creating…' : 'Create module'}</button>
              <button type="button" class="ghost" onclick={() => (creating = false)}>Cancel</button>
            </div>
          </form>
        {/if}

        {#if fleet.modules === null}
          <p class="muted">Reading the modules…</p>
        {:else if !modules.length}
          <div class="empty card" use:frame={'dash'}>
            <p><strong>No modules yet.</strong></p>
            <p class="muted">A module groups packages, services and option values (a media server, a desktop, a build machine) so a machine gets them all with one switch, and can set the module's options its own way.</p>
          </div>
        {:else}
          <div class="table-wrap">
            <table class="data">
              <thead><tr><th>Module</th><th>Description</th><th>Options</th><th>Packages</th><th>On</th></tr></thead>
              <tbody>
                {#each modules as m (m.name)}
                  {@const on = machinesWith(hosts, m.name)}
                  <tr>
                    <td class="mono"><a href={link('modules', [m.name])}>{m.name}</a></td>
                    {#if m.error}
                      <td colspan="4"><span class="badge err">Cannot be read</span> <span class="small">{m.error}</span></td>
                    {:else}
                      <td>{m.description || ''}</td>
                      <td class="num">{Object.keys(m.options ?? {}).length}</td>
                      <td class="num">{m.packages.length}</td>
                      <td>{#if on.length}{on.join(', ')}{:else}<span class="muted">No machine</span>{/if}</td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          <p class="muted small">{plural(modules.length, 'module')}. A machine's Modules tab turns them on and sets their options.</p>
        {/if}
      </ModelGate>
    </div>
  </div>
{/if}

<style>
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 1ch 2ch;
  }
  .notice,
  .create {
    margin-bottom: var(--lh);
  }
  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--lh) 2ch;
  }
  .notice p {
    flex: 1;
    margin: 0;
  }
  .create {
    display: grid;
    gap: calc(var(--lh) / 2);
    max-width: 72ch;
  }
  .create label {
    display: grid;
    grid-template-columns: 14ch 1fr;
    align-items: baseline;
    gap: 2ch;
  }
  .err {
    color: var(--danger);
  }
  .num {
    text-align: right;
  }
</style>
