<script lang="ts">
  import { dismiss, toasts } from '../lib/ui.svelte'
</script>

<div class="toasts" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}" role={t.kind === 'error' ? 'alert' : 'status'}>
      <div class="body">
        {#if t.title}<strong>{t.title}</strong>{/if}
        <pre>{t.text}</pre>
      </div>
      <button class="icon" aria-label="Dismiss" onclick={() => dismiss(t.id)}>×</button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 12px;
    bottom: 32px;
    z-index: 40;
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(440px, calc(100vw - 24px));
  }
  .toast {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    padding: 8px 8px 8px 12px;
    background: var(--bg-raised);
    border: 1px solid var(--line);
    border-left: 3px solid var(--accent);
    border-radius: 6px;
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.4);
  }
  .toast.error {
    border-left-color: var(--red);
  }
  .toast.ok {
    border-left-color: var(--green);
  }
  .body {
    flex: 1;
    min-width: 0;
  }
  strong {
    display: block;
    margin-bottom: 2px;
  }
  pre {
    margin: 0;
    max-height: 40vh;
    overflow: auto;
    font: inherit;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .toast.error pre {
    font-family: var(--mono);
    font-size: 12px;
  }
</style>
