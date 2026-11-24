<script lang="ts">
  // The engineering drawing of the fleet's configuration graph, scrubbed by t (0..1) and drafted in box drawing:
  // nodes and edges are drawn, evaluated and ticked; one machine fails with a real Nix error, a patch fixes it, and
  // the sheet is stamped. All timing comes from timeline.ts; scenes.ts `blueprint` draws it on the clock.
  import { onMount } from 'svelte'
  import { scene, still } from '../../lib/ascii'
  import { SHEET, blueprint, bufs, grid } from '../scenes'
  import { ERROR_LINES, STAMPS } from './timeline'

  let { t }: { t: number } = $props()

  const pres = $state<Partial<Record<(typeof SHEET)[number], HTMLPreElement>>>({})
  let paint: (() => void) | undefined

  onMount(() => {
    const p = scene(pres.fg!, (tick) => {
      const { cols, rows } = grid()
      const L = bufs(SHEET, cols, rows)
      blueprint(L, t, still ? 0 : tick)
      for (const n of SHEET) if (n !== 'fg') pres[n]!.textContent = L[n].toString()
      return L.fg.toString()
    })
    paint = p
    addEventListener('resize', p)
    return () => {
      p.stop()
      removeEventListener('resize', p)
    }
  })
  $effect(() => {
    void t
    if (still) paint?.()
  })
</script>

<div class="bp" role="img" aria-labelledby="bp-title bp-desc">
  <p id="bp-title" class="sr-only">The fleet's configuration graph, drawn and checked</p>
  <p id="bp-desc" class="sr-only">
    flake.nix connects to hosts/workstation, hosts/homeserver and hosts/pi; the hosts import shared modules and their managed.json; those pull in packages and
    services. Every node is evaluated and ticked. hosts/pi fails with "{ERROR_LINES[0]}", a fix rewrites its imports, and the sheet is stamped "{STAMPS[0]}" and
    "{STAMPS[1]}".
  </p>
  {#each SHEET as n (n)}<pre class={n} aria-hidden="true" bind:this={pres[n]}></pre>{/each}
</div>

<style>
  .bp {
    position: absolute;
    inset: 0;
    overflow: hidden;
    pointer-events: none;
  }
  pre {
    position: absolute;
    inset: 0;
    overflow: hidden;
  }
  .mut {
    color: var(--fg-muted);
  }
  .fg {
    color: var(--fg-strong);
  }
  .hot {
    color: var(--accent);
    text-shadow: var(--glow);
  }
  .bad {
    color: var(--red);
  }
  .good {
    color: var(--green);
  }
  /* shortcut: the sheet is drawn for 150 columns and wider (the prototype's limit); a narrow window crops it. Draw
     the narrow layout of timeline.ts (NODES[].m) if phones need it. */
</style>
