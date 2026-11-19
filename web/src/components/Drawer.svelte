<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import { fade, fly } from 'svelte/transition'
  import Icon from './Icon.svelte'

  /** A side sheet over the page, for work that keeps the page in view (configure, add). Modal: focus stays inside. */
  let { title, onclose, children, width = 640 }: { title: string; onclose: () => void; children: Snippet; width?: number } = $props()
  let el: HTMLElement

  onMount(() => {
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
<div class="scrim" aria-hidden="true" onclick={onclose} transition:fade={{ duration: 150 }}></div>
<div class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title" bind:this={el} style:width="min({width}px, calc(100vw - 80px))" transition:fly={{ x: 40, duration: 200 }}>
  <header>
    <h2 id="drawer-title">{title}</h2>
    <button class="icon close" aria-label="Close" title="Close (Esc)" onclick={onclose}><Icon name="x" /></button>
  </header>
  <div class="d-body">{@render children()}</div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 29;
    background: rgb(4 6 9 / 0.38);
  }
  .drawer {
    position: fixed;
    top: 56px;
    right: 8px;
    bottom: 8px;
    z-index: 30;
    display: flex;
    flex-direction: column;
    background: var(--bg-side);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-2);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 10px 10px 18px;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .d-body {
    flex: 1;
    min-height: 0;
  }
</style>
