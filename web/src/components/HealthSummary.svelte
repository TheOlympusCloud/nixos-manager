<script lang="ts">
  // Compact fleet health for the Fleet toolbar; opens the Health view.
  import { plural } from '../lib/format'
  import { health } from '../lib/health.svelte'
  import { link } from '../lib/router.svelte'

  const count = (s: string) => (health.insights ?? []).filter((i) => i.severity === s).length
  const parts = $derived(
    [
      [count('error'), 'error', 'err'],
      [count('warning'), 'warning', 'warn'],
      [count('info'), 'suggestion', 'info'],
    ].filter(([n]) => n) as [number, string, string][],
  )
</script>

{#if health.insights}
  <a class="hs" href={link('health')} title="Open Health: findings and one-click fixes">
    {#if parts.length}
      {#each parts as [n, word, cls], k (cls)}{#if k}<span class="sep" aria-hidden="true">·</span>{/if}<span class="part {cls}"><span class="dot"></span>{plural(n, word)}</span>{/each}
    {:else}
      <span class="part ok"><span class="dot"></span>Healthy</span>
    {/if}
  </a>
{/if}

<style>
  .hs {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 26px;
    padding: 0 10px;
    color: var(--fg);
    font-size: var(--fs-sm);
    white-space: nowrap;
    text-decoration: none;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: 13px;
    transition: border-color var(--t-fast) var(--ease);
  }
  .hs:hover {
    border-color: var(--line-strong);
    color: var(--fg-strong);
  }
  .part {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .dot {
    width: 7px;
    height: 7px;
    background: var(--c);
  }
  .err {
    --c: var(--red);
  }
  .warn {
    --c: var(--yellow);
  }
  .info {
    --c: var(--accent);
  }
  .ok {
    --c: var(--green);
  }
  .sep {
    color: var(--fg-faint);
  }
</style>
