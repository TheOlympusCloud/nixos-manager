<script lang="ts">
  // One fixed canvas behind the story. The WebGL scene loads after the hero has painted; if it cannot
  // be created (no WebGL, or the scene throws) a static planet takes its place.
  import { onMount } from 'svelte'
  import type { Chapter, IntroScene } from './three/types'

  let {
    chapter,
    t,
    visible,
    reducedMotion,
    machines,
  }: { chapter: Chapter; t: number; visible: boolean; reducedMotion: boolean; machines: string[] } = $props()

  let canvas = $state.raw<HTMLCanvasElement>()
  let scene = $state.raw<IntroScene | null>(null)
  let fallback = $state(false)

  function resize() {
    scene?.resize(innerWidth, innerHeight, Math.min(devicePixelRatio || 1, 1.75))
  }

  onMount(() => {
    const probe = document.createElement('canvas')
    if (!(probe.getContext('webgl2') || probe.getContext('webgl'))) {
      fallback = true
      return
    }
    let disposed = false
    const start = async () => {
      try {
        const { createIntroScene } = await import('./three/scene')
        if (disposed || !canvas) return
        const s = createIntroScene({ canvas, reducedMotion, machines })
        scene = s
        resize()
      } catch {
        fallback = true
      }
    }
    // Two frames later: the hero text is on screen before three.js is even requested.
    requestAnimationFrame(() => requestAnimationFrame(start))

    const onVisibility = () => scene?.setPaused(document.hidden || !visible)
    const onPointer = (e: PointerEvent) => scene?.setPointer((e.clientX / innerWidth) * 2 - 1, (e.clientY / innerHeight) * 2 - 1)
    const onLeave = () => scene?.setPointer(null, null)
    addEventListener('resize', resize)
    document.addEventListener('visibilitychange', onVisibility)
    if (!reducedMotion) {
      addEventListener('pointermove', onPointer, { passive: true })
      document.documentElement.addEventListener('pointerleave', onLeave)
    }
    return () => {
      disposed = true
      removeEventListener('resize', resize)
      document.removeEventListener('visibilitychange', onVisibility)
      removeEventListener('pointermove', onPointer)
      document.documentElement.removeEventListener('pointerleave', onLeave)
      scene?.dispose()
      scene = null
    }
  })

  $effect(() => {
    scene?.setStory(chapter, t)
  })
  $effect(() => {
    scene?.setPaused(!visible || document.hidden)
  })
</script>

<div class="scene" class:visible aria-hidden="true">
  {#if fallback}
    <div class="planet"></div>
  {:else}
    <canvas bind:this={canvas}></canvas>
  {/if}
</div>

<style>
  .scene {
    position: fixed;
    inset: 0;
    z-index: 0;
    opacity: 0;
    transition: opacity 700ms var(--i-ease);
    pointer-events: none;
  }
  .scene.visible {
    opacity: 1;
  }
  canvas {
    display: block;
    width: 100%;
    height: 100%;
  }
  /* The static stand-in: a dark sphere low in the frame, rim-lit, with a dot-matrix surface. */
  .planet {
    position: absolute;
    left: 50%;
    bottom: -62vmin;
    width: 120vmin;
    height: 120vmin;
    border-radius: 50%;
    background:
      radial-gradient(circle at 50% 20%, rgb(126 182 255 / 0.18), transparent 46%),
      radial-gradient(circle, #0b1220 0%, #070b12 60%, #05070a 100%);
    box-shadow:
      0 -2px 40px rgb(126 182 255 / 0.35),
      0 -1px 2px rgb(200 225 255 / 0.5),
      0 -40px 160px rgb(82 119 195 / 0.25);
    transform: translateX(-50%);
  }
  .planet::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background-image: radial-gradient(rgb(126 182 255 / 0.5) 0.8px, transparent 1.2px);
    background-size: 9px 9px;
    mask-image: radial-gradient(circle at 50% 30%, #000 0%, transparent 62%);
  }
</style>
