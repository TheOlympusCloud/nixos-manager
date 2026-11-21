<script lang="ts">
  import { flip } from 'svelte/animate'
  import { backOut } from 'svelte/easing'
  import { fly } from 'svelte/transition'
  import { dismiss, toasts } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  const icon = { ok: 'tick', error: 'alert', info: 'info', busy: 'refresh' } as const
</script>

<div class="toasts" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}" role={t.kind === 'error' ? 'alert' : 'status'} in:fly={{ y: 16, duration: 320, easing: backOut }} out:fly={{ x: 24, duration: 180 }} animate:flip={{ duration: 220 }}>
      <span class="ico">{#if t.kind === 'busy'}<span class="spin" aria-hidden="true"></span>{:else}<Icon name={icon[t.kind]} size={15} />{/if}</span>
      <div class="body">
        {#if t.title}<strong>{t.title}</strong>{/if}
        <pre>{t.text}</pre>
        {#if t.actions.length}
          <div class="acts">
            {#each t.actions as a (a.label)}
              <button class="link act" onclick={() => (a.run(), dismiss(t.id))}>{a.label}</button>
            {/each}
          </div>
        {/if}
      </div>
      <button class="icon" aria-label="Dismiss" onclick={() => dismiss(t.id)}><Icon name="x" size={14} /></button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 16px;
    bottom: 16px;
    z-index: 60;
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(420px, calc(100vw - 32px));
  }
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 8px 10px 12px;
    background: var(--bg-raised);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-2);
  }
  .ico {
    display: grid;
    flex: none;
    place-items: center;
    width: 24px;
    height: 24px;
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 14%, transparent);
    border-radius: 50%;
  }
  .ok .ico {
    color: var(--green);
    background: color-mix(in srgb, var(--green) 14%, transparent);
  }
  .error .ico {
    color: var(--red);
    background: color-mix(in srgb, var(--red) 14%, transparent);
  }
  .error {
    border-color: color-mix(in srgb, var(--red) 35%, var(--line));
  }
  .spin {
    width: 12px;
    height: 12px;
    border: 2px solid color-mix(in srgb, var(--accent) 30%, transparent);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  .body {
    display: grid;
    flex: 1;
    gap: 2px;
    min-width: 0;
    padding-top: 3px;
  }
  strong {
    color: var(--fg-strong);
  }
  pre {
    margin: 0;
    max-height: 40vh;
    overflow: auto;
    font: inherit;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .error pre {
    font-family: var(--mono);
    font-size: 12px;
  }
  .acts {
    display: flex;
    gap: 14px;
    margin-top: 4px;
  }
  .act {
    font-weight: 500;
  }
</style>
