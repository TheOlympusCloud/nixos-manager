<script lang="ts">
  import { classifyDiff } from '../lib/diff'

  let { text }: { text: string } = $props()
  const lines = $derived(classifyDiff(text))
</script>

{#if lines.length}
  <div class="diff" role="region" aria-label="Diff">
    {#each lines as l, i (i)}<div class="l {l.kind}">{l.text || ' '}</div>{/each}
  </div>
{:else}
  <p class="empty">No differences.</p>
{/if}

<style>
  .diff {
    min-width: max-content;
    padding: 6px 0 24px;
    font: 12.5px/1.55 var(--mono);
    white-space: pre;
  }
  .l {
    padding: 0 14px;
  }
  .file {
    margin-top: 10px;
    padding-top: 4px;
    padding-bottom: 4px;
    color: var(--fg-strong);
    font-weight: 600;
    background: var(--bg-side);
    border-top: 1px solid var(--line);
  }
  .file:first-child {
    margin-top: 0;
  }
  .meta {
    color: var(--fg-muted);
  }
  .hunk {
    color: var(--cyan);
    background: color-mix(in srgb, var(--cyan) 8%, transparent);
  }
  .add {
    color: #b5dc98;
    background: color-mix(in srgb, var(--green) 14%, transparent);
  }
  .del {
    color: #f0a3aa;
    background: color-mix(in srgb, var(--red) 14%, transparent);
  }
</style>
