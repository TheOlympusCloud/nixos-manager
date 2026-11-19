<script lang="ts">
  import type { Host } from '../../lib/api'
  import { categories, recipes, type Recipe } from '../../lib/catalog'
  import { fleet, managedValue, setOption, useValues, valueOf } from '../../lib/fleet.svelte'
  import { pathName } from '../../lib/options'
  import { go } from '../../lib/router.svelte'
  import { quietly, toast } from '../../lib/ui.svelte'
  import { effective, showValue } from '../../lib/values'
  import Icon from '../Icon.svelte'

  let { host }: { host: Host } = $props()

  let filter = $state('')
  let busy = $state('')
  const names = recipes.map((r) => pathName(r.settings[0].path))
  const vals = useValues(() => ({ names, hosts: [host.name] }))

  type State = 'on' | 'partial' | 'off' | 'missing' | 'unknown'
  function stateOf(r: Recipe): State {
    const path = r.settings[0].path
    const e = effective(valueOf(vals.values, pathName(path), host.name), managedValue(host.name, path), host.managedFile, !!fleet.managed[host.name])
    if (!e || e.state === 'error') return 'unknown'
    if (e.state === 'missing') return 'missing'
    if (e.value !== true) return 'off'
    // Added here, but a write after the first one failed: offer to finish.
    const addedHere = managedValue(host.name, path) === true
    return addedHere && r.settings.some((s) => managedValue(host.name, s.path) !== s.value) ? 'partial' : 'on'
  }
  /** A port list set here replaces the whole option, so openFirewall in a recipe adds nothing. */
  const portsSetHere = $derived(managedValue(host.name, ['networking', 'firewall', 'allowedTCPPorts']) !== undefined)
  const q = $derived(filter.trim().toLowerCase())
  const shown = $derived(recipes.filter((r) => !q || `${r.name} ${r.summary} ${r.category}`.toLowerCase().includes(q)))

  async function add(r: Recipe) {
    busy = r.id
    const ok = await quietly(async () => {
      for (const s of r.settings) if (!(await setOption(host.name, s.path, s.value))) return false
      return true
    })
    busy = ''
    if (ok)
      toast(`${r.name} is turned on in ${host.name}'s managed settings. Check and commit to deploy it.`, 'ok', `Added ${r.name}`, {
        label: 'View changes',
        run: () => go('#/changes'),
      })
  }
</script>

<div class="catalog">
  <p class="muted intro">
    Common services, ready to go. Adding one sets the options below in {host.name}'s managed settings; review and commit in Changes to deploy it.
  </p>
  <input class="search" type="search" placeholder="Filter: media, backup, containers…" aria-label="Filter the catalog" bind:value={filter} />
  {#if vals.error}<p class="error small">Could not read the current values: {vals.error}</p>{/if}
  {#if portsSetHere}
    <p class="note small">
      <Icon name="alert" size={13} />{host.name}'s open ports are set here as one list, so services cannot open their own ports. After adding one that needs a port, add it in Network &amp; storage.
    </p>
  {/if}
  {#each categories as c (c)}
    {@const rs = shown.filter((r) => r.category === c)}
    {#if rs.length}
      <h3 class="cat">{c}</h3>
      {#each rs as r (r.id)}
        {@const st = stateOf(r)}
        <article class="card recipe" class:on={st === 'on'}>
          <div class="top">
            <h4>{r.name}</h4>
            {#if st === 'on'}
              <span class="badge ok"><Icon name="tick" size={12} />On</span>
            {:else if st === 'missing'}
              <span class="badge" title="This machine's NixOS has no {pathName(r.settings[0].path)} option">Not in this NixOS</span>
            {:else}
              <button class="small" class:primary={st !== 'unknown'} disabled={st === 'unknown' || !!busy} onclick={() => add(r)}>
                {#if busy === r.id}<span class="spin" aria-hidden="true"></span>Adding…{:else if st === 'partial'}<Icon name="refresh" size={13} />Finish adding{:else}<Icon name="plus" size={13} />Add{/if}
              </button>
            {/if}
          </div>
          <p class="summary">{r.summary}</p>
          {#if st === 'on' && r.url}<p class="small muted">Open <code>{r.url.replace('<machine>', host.hostName || host.name)}</code> once deployed.</p>{/if}
          <ul class="sets" aria-label="Options it sets">
            {#each r.settings as s (s.path.join('.'))}<li><code>{pathName(s.path)} = {showValue(s.value)};</code></li>{/each}
          </ul>
        </article>
      {/each}
    {/if}
  {/each}
  {#if !shown.length}<p class="muted">Nothing matches “{filter}”. The Add service or program search covers every NixOS option.</p>{/if}
  {#if vals.loading}<p class="muted small"><span class="spin" aria-hidden="true"></span>Reading what {host.name} runs today…</p>{/if}
</div>

<style>
  .catalog {
    display: grid;
    gap: var(--s2);
    padding: var(--s3) var(--s4) var(--s4);
  }
  .intro {
    line-height: 1.5;
  }
  .cat {
    margin-top: var(--s3);
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .recipe {
    display: grid;
    gap: 6px;
    padding: 12px 14px;
  }
  .recipe.on {
    border-color: color-mix(in srgb, var(--green) 35%, var(--border));
  }
  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  h4 {
    font-size: var(--fs-md);
  }
  .summary {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.5;
  }
  .sets {
    display: grid;
    gap: 2px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .sets code {
    color: var(--fg-faint);
    font-size: var(--fs-xs);
    overflow-wrap: anywhere;
  }
  .note {
    display: flex;
    gap: 6px;
    align-items: flex-start;
    padding: 8px 10px;
    color: var(--yellow);
    background: color-mix(in srgb, var(--yellow) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--yellow) 25%, transparent);
    border-radius: var(--r-sm);
    line-height: 1.45;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
</style>
