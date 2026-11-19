<script lang="ts">
  import { reducedMotion } from '../../lib/motion'
  import type { Backdrop, View } from './backdrop'

  /** The WebGL space behind the graph. Without WebGL (or if its code fails to load) the plain background stays. */
  let { view, active }: { view: () => View; active: boolean } = $props()

  let canvas = $state<HTMLCanvasElement>()
  let backdrop = $state.raw<Backdrop | null>(null)

  $effect(() => {
    const el = canvas
    if (!el) return
    let gone = false
    let made: Backdrop | null = null
    import('./backdrop').then(
      ({ createBackdrop }) => {
        if (gone) return
        try {
          made = createBackdrop(el, view, { still: reducedMotion() })
          backdrop = made
        } catch {
          // No WebGL here: the CSS background under the canvas is enough.
        }
      },
      () => {},
    )
    return () => {
      gone = true
      made?.dispose()
      backdrop = null
    }
  })
  $effect(() => backdrop?.setActive(active))
</script>

<canvas bind:this={canvas} class:on={!!backdrop} aria-hidden="true"></canvas>

<style>
  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    transition: opacity 900ms var(--ease);
    pointer-events: none;
  }
  canvas.on {
    opacity: 1;
  }
</style>
