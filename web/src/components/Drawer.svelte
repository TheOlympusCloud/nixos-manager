<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import { cell, frame, sweep } from '../lib/ascii'
  import Dither from './cell/Dither.svelte'
  import Icon from './Icon.svelte'

  /** A side sheet over the view, for work that keeps the page in view (configure, add). Modal: focus stays inside. width is in px, drawn as whole cells. */
  let { title, onclose, children, width = 640 }: { title: string; onclose: () => void; children: Snippet; width?: number } = $props()
  let el: HTMLElement

  onMount(() => {
    sweep(el)
    const before = document.activeElement as HTMLElement | null
    ;(el.querySelector<HTMLElement>('[data-search]') ?? el.querySelector<HTMLElement>('.close'))?.focus()
    return () => before?.isConnected && before.focus() // back to the button that opened it
  })

  const focusables = () =>
    [...el.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')].filter(
      (x) => x.offsetParent !== null,
    )
  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && !(e.target as HTMLElement).closest('dialog')) {
      e.preventDefault()
      onclose()
      return
    }
    if (e.key !== 'Tab' || !el) return
    const all = focusables()
    if (!all.length) return
    const first = all[0]
    const last = all[all.length - 1]
    const inside = el.contains(document.activeElement)
    if (e.shiftKey && (document.activeElement === first || !inside)) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && (document.activeElement === last || !inside)) {
      e.preventDefault()
      first.focus()
    }
  }
</script>

<svelte:window {onkeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions (Escape closes it from the keyboard) -->
<div class="scrim" aria-hidden="true" onclick={onclose}><Dither /></div>
<div
  class="drawer"
  role="dialog"
  aria-modal="true"
  aria-labelledby="drawer-title"
  bind:this={el}
  style:width="min({Math.round(width / cell.cw)}ch, calc(100% - 4ch))"
  use:frame={{ kind: 'heavy', title, tag: 'Esc', animate: true }}>
  <h2 id="drawer-title" class="sr-only">{title}</h2>
  <button class="icon close" aria-label="Close" title="Close (Esc)" onclick={onclose}><Icon name="x" /></button>
  <div class="d-body">{@render children()}</div>
</div>

<style>
  /* Both sit in the view (.content), not over the rail and the top bar. */
  .scrim {
    position: absolute;
    inset: 0;
    z-index: 29;
    background: var(--scrim);
  }
  .drawer {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    z-index: 30;
    display: flex;
    flex-direction: column;
    height: round(down, 100%, var(--lh));
    background: var(--bg-raised);
  }
  /* On the top line, left of the Esc tag: ━ × ━ Esc ━┓ */
  .close {
    position: absolute;
    top: 0;
    right: 8ch;
    z-index: 2;
    width: 3ch;
    background: var(--bg-raised);
  }
  .d-body {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
</style>
