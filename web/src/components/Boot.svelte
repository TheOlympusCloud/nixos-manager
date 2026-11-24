<script lang="ts">
  // While the console loads: the snowflake turns on the clock and a ring of dots leaves it every two seconds.
  import { onMount } from 'svelte'
  import { Buf, scene, still } from '../lib/ascii'
  import { init } from '../lib/state.svelte'
  import { ASPECT, grid, snowflake, stars } from '../intro/scenes'

  let starsEl: HTMLPreElement
  let dark: HTMLPreElement
  let lit: HTMLPreElement
  // The row the words sit on, under the mark.
  let y0 = $state(0)

  onMount(() => {
    const paint = scene(lit, (tick) => {
      const t = still ? 69 : tick
      const { cols, rows } = grid()
      const [hot, globe, sky] = [new Buf(cols, rows), new Buf(cols, rows), new Buf(cols, rows)]
      const cx = Math.floor(cols / 2)
      const cy = Math.floor(rows * 0.4)
      y0 = cy + 14
      snowflake({ hot, globe }, cx, cy, 10, t * 0.04)
      const r = 12 + (t % 26) * 0.55
      const ring = '●•∙·· '[Math.min(5, Math.floor((t % 26) / 4.5))]
      for (let a = 0; a < 360; a += 4) {
        const y = cy + Math.sin((a * Math.PI) / 180) * r
        if (y < cy + 13) hot.set(cx + Math.cos((a * Math.PI) / 180) * r * ASPECT, y, ring) // not through the words
      }
      stars(sky, t)
      for (let y = y0 - 1; y < y0 + 4; y++) for (let x = cx - 20; x < cx + 20; x++) sky.set(x, y, ' ')
      starsEl.textContent = sky.toString()
      dark.textContent = globe.toString()
      return hot.toString()
    })
    addEventListener('resize', paint)
    return () => {
      paint.stop()
      removeEventListener('resize', paint)
    }
  })
</script>

<div class="boot" role="status">
  <pre class="stars" aria-hidden="true" bind:this={starsEl}></pre>
  <pre class="dark" aria-hidden="true" bind:this={dark}></pre>
  <pre class="lit" aria-hidden="true" bind:this={lit}></pre>
  <div class="words" style:top="calc(var(--lh) * {y0})">
    <p>Loading nixos-manager…</p>
    <button onclick={init}>Retry</button>
  </div>
</div>

<style>
  .boot {
    position: fixed;
    inset: 0;
    overflow: hidden;
    background: var(--bg);
  }
  pre {
    position: absolute;
    inset: 0;
    overflow: hidden;
    pointer-events: none;
  }
  .stars {
    color: var(--fg-faint);
  }
  .dark {
    color: var(--accent-fill);
  }
  .lit {
    color: var(--accent);
    text-shadow: var(--glow);
  }
  .words {
    position: absolute;
    right: 0;
    left: 0;
    display: grid;
    justify-items: center;
    gap: var(--lh);
    color: var(--fg-strong);
  }
</style>
