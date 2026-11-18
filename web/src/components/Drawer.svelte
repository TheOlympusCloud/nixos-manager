<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import { fly } from 'svelte/transition'
  import Icon from './Icon.svelte'

  /** A side sheet over the page, for work that keeps the page in view (configure, add). */
  let { title, onclose, children, width = 640 }: { title: string; onclose: () => void; children: Snippet; width?: number } = $props()
  let el: HTMLElement
  onMount(() => el.querySelector<HTMLElement>('[data-search], .close')?.focus())
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && !(e.target as HTMLElement).closest('dialog') && onclose()} />

<div class="drawer" role="dialog" aria-label={title} bind:this={el} style:width="min({width}px, calc(100vw - 80px))" transition:fly={{ x: 40, duration: 200 }}>
  <header>
    <h2>{title}</h2>
    <button class="icon close" aria-label="Close" title="Close (Esc)" onclick={onclose}><Icon name="x" /></button>
  </header>
  <div class="d-body">{@render children()}</div>
</div>

<style>
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
