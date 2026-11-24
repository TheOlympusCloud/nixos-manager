<script lang="ts">
  // One fixed text scene behind the story, repainted on the clock: the stars, the planet of packages, its arcs,
  // orbits and commit pulses (scenes.ts `story`). It is empty while the page draws a chapter itself.
  import { onMount } from 'svelte'
  import { scene, still } from '../lib/ascii'
  import { LAYERS, bufs, grid, story } from './scenes'
  import type { Chapter } from './three/types'

  let { chapter, t, visible, machines }: { chapter: Chapter; t: number; visible: boolean; machines: string[] } = $props()

  const pres = $state<Partial<Record<(typeof LAYERS)[number], HTMLPreElement>>>({})
  let paint: (() => void) | undefined

  onMount(() => {
    const p = scene(pres.fg!, (tick) => {
      const { cols, rows } = grid()
      const L = bufs(LAYERS, cols, rows)
      // Motion frozen, the scene stops on a frame with every machine label on screen and no meteor.
      if (visible) story(L, chapter, t, still ? 60 : tick, machines)
      for (const n of LAYERS) if (n !== 'fg') pres[n]!.textContent = L[n].toString()
      return L.fg.toString()
    })
    paint = p
    addEventListener('resize', p)
    return () => {
      p.stop()
      removeEventListener('resize', p)
    }
  })
  // With motion frozen nothing ticks, so moving through the story repaints the scene.
  $effect(() => {
    void [chapter, t, visible]
    if (still) paint?.()
  })
</script>

<div class="scene" aria-hidden="true">
  {#each LAYERS as n (n)}<pre class={n} bind:this={pres[n]}></pre>{/each}
</div>

<style>
  .scene {
    position: fixed;
    inset: 0;
    z-index: 0;
    overflow: hidden;
    pointer-events: none;
  }
  pre {
    position: absolute;
    inset: 0;
    overflow: hidden;
  }
  .stars {
    color: var(--fg-faint);
  }
  .globe {
    color: var(--accent-fill);
  }
  .hot {
    color: var(--accent);
    text-shadow: var(--glow);
  }
  .warm {
    color: var(--yellow);
  }
  .fg {
    color: var(--fg-strong);
  }
  .good {
    color: var(--green);
  }
</style>
