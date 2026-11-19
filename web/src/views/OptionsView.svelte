<script lang="ts">
  import { untrack } from 'svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import OptionBrowser from '../components/OptionBrowser.svelte'
  import { healthy } from '../lib/fleet.svelte'
  import { link, replace, route } from '../lib/router.svelte'

  const hosts = $derived(healthy())
  const host = $derived(hosts.find((h) => h.name === route.parts[0])?.name ?? hosts[0]?.name ?? '')
  // The URL holds the machine, the search and the open option, so they survive reloads and can be linked.
  let query = $state(route.query.q ?? '')
  let selected = $state(route.query.o ?? '')
  // Links into Options while it is open (the command palette, a machine page) change the URL only: follow them.
  $effect(() => {
    const q = route.query.q ?? ''
    const o = route.query.o ?? ''
    untrack(() => {
      if (q !== query.trim()) query = q
      if (o !== selected) selected = o
    })
  })
  $effect(() => {
    if (host) replace(link('options', [host], { q: query.trim(), o: selected }))
  })
</script>

<ModelGate>
  <div class="ov">
    <div class="bar">
      <label class="pick">
        <span class="label">Machine</span>
        <select value={host} onchange={(e) => replace(link('options', [e.currentTarget.value], { q: query.trim(), o: selected }))}>
          {#each hosts as h (h.name)}<option value={h.name}>{h.name}{h.managedFile ? '' : ' · Code only'}</option>{/each}
        </select>
      </label>
      <p class="muted small">Every NixOS option this machine can set, from its own evaluation (modules from its flake inputs included).</p>
    </div>
    {#if host}
      {#key host}<OptionBrowser {host} bind:query bind:selected />{/key}
    {/if}
  </div>
</ModelGate>

<style>
  .ov {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: 10px var(--s4);
    border-bottom: 1px solid var(--border);
  }
  .pick {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .pick select {
    width: 200px;
  }
  .ov > :global(.ob) {
    flex: 1;
    height: auto;
  }
</style>
