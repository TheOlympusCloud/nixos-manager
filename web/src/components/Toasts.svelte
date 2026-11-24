<script lang="ts">
  import { bar, clock, frame, on, TICK, typed } from '../lib/ascii'
  import { dismiss, toasts, type Toast } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  const sigil = { ok: ['ok', '✓'], error: ['err', '✖'], info: ['info', '◆'] } as const
  const tone = { ok: 'ok', error: 'err' } as Record<string, string>

  // The fuse only draws what lib/ui.svelte.ts decides (5 s, 8 s with actions; errors and busy stay), in ticks.
  const life = (t: Toast) => (t.kind === 'error' || t.kind === 'busy' ? 0 : (t.actions.length ? 8000 : 5000) / TICK)
  // A toast replaced in place (same key) is a new object, so its fuse is lit again, as its timer is.
  const lit = new WeakMap<Toast, number>()
  let now = $state(clock.t)
  function fuse(t: Toast) {
    if (!lit.has(t)) lit.set(t, clock.t)
    return Math.max(0, 1 - (now - lit.get(t)!) / life(t))
  }
  $effect(() => {
    if (!toasts.some(life)) return
    return on((t) => void (now = t))
  })
</script>

<div class="toasts" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}" data-tone={tone[t.kind]} role={t.kind === 'error' ? 'alert' : 'status'} use:frame={{ shadow: true, animate: true }}>
      <span class="sig">{#if t.kind === 'busy'}<span class="spinner" aria-hidden="true"></span>{:else}<i class={sigil[t.kind][0]} aria-hidden="true">{sigil[t.kind][1]}</i>{/if}</span>
      <div class="body">
        {#if t.title}<strong>{t.title}</strong>{/if}
        <pre aria-hidden="true" use:typed={t.text}></pre>
        <span class="sr-only">{t.text}</span>
        {#if t.actions.length}
          <div class="acts">
            {#each t.actions as a (a.label)}
              <button class="link act" onclick={() => (a.run(), dismiss(t.id))}>{a.label} <Icon name="right" /></button>
            {/each}
          </div>
        {/if}
      </div>
      <button class="icon x" aria-label="Dismiss" onclick={() => dismiss(t.id)}><Icon name="x" /></button>
      {#if life(t)}<span class="bar fuse" aria-hidden="true">{@html bar(fuse(t), 42)}</span>{/if}
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 3ch;
    bottom: calc(var(--lh) * 2);
    z-index: 60;
    display: flex;
    flex-direction: column;
    gap: var(--lh);
    width: 46ch;
    max-width: calc(100vw - 6ch);
  }
  .toast {
    display: grid;
    grid-template-columns: 2ch minmax(0, 1fr) 2ch;
    background: var(--bg-raised);
  }
  .sig {
    color: var(--accent);
  }
  .spinner {
    margin: 0;
  }
  strong {
    display: block;
    color: var(--fg-strong);
  }
  pre {
    max-height: calc(var(--lh) * 20);
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .x {
    justify-self: end;
  }
  .fuse {
    grid-column: 1 / -1;
    overflow: hidden;
  }
</style>
