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
      {#each parts as [n, word, cls], k (cls)}{#if k}<span class="sep" aria-hidden="true">·</span>{/if}<span class="part {cls}"><span class="dot {cls}"></span>{plural(n, word)}</span>{/each}
    {:else}
      <span class="part ok"><span class="dot ok"></span>Healthy</span>
    {/if}
  </a>
{/if}

<style>
  /* ✖ 1 error · ▲ 2 warnings · ◆ 4 suggestions: a link, each count after its glyph. */
  .hs {
    display: inline-flex;
    gap: 1ch;
    white-space: nowrap;
  }
  .part {
    display: inline-flex;
    gap: 1ch;
  }
  .dot.info {
    --c: var(--blue);
  }
  .dot.info::before {
    content: '◆';
  }
  .hs:hover .dot {
    --c: var(--on-accent);
  }
  .sep {
    color: var(--fg-faint);
  }
  .hs:hover .sep {
    color: inherit;
  }
</style>
