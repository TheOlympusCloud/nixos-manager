<script lang="ts">
  import { frame } from '../lib/ascii'
  import { dialog } from '../lib/ui.svelte'
  import Dither from './cell/Dither.svelte'

  let el: HTMLDialogElement
  let value = $state('')

  $effect(() => {
    const spec = dialog.spec
    if (spec) {
      value = spec.input?.value ?? ''
      if (!el.open) el.showModal()
    } else if (el.open) el.close()
  })

  function submit(e: SubmitEvent) {
    e.preventDefault()
    if (dialog.spec?.input && !value.trim()) return
    dialog.resolve(dialog.spec?.input ? value : '')
  }
</script>

<!-- Escape fires "cancel"; closing by any other route also resolves as cancelled. The close event comes a
     moment after close(): when a next dialog has opened since (a push after its review), it is not about that one. -->
<dialog bind:this={el} class="screen" onclose={() => !el.open && dialog.spec && dialog.resolve(null)} aria-labelledby="dialog-title">
  <Dither />
  {#if dialog.spec}
    {@const spec = dialog.spec}
    <!-- The only double frame on screen; a danger question has a red one. -->
    <form class="modal" class:wide={spec.wide} data-tone={spec.danger ? 'err' : undefined} onsubmit={submit} use:frame={{ kind: 'double', shadow: true, animate: true, title: spec.title }}>
      <h2 id="dialog-title" class="sr-only">{spec.title}</h2>
      <!-- The text scrolls, not the framed (and shadowed) box. -->
      <div class="content">
        {#if spec.message}<p>{spec.message}</p>{/if}
        {#if spec.pre !== undefined}<pre>{spec.pre}</pre>{/if}
        {#if spec.body}
          {@const Body = spec.body}
          <Body />
        {/if}
        {#if spec.input}
          <div class="field">
            <label for="dialog-input">{spec.input.label}</label>
            <span class="prompt-row"
              ><b class="prompt">λ ›</b>
              <!-- svelte-ignore a11y_autofocus -->
              <input id="dialog-input" bind:value autofocus spellcheck="false" autocomplete="off" /></span>
          </div>
        {/if}
      </div>
      <div class="actions">
        <!-- Destructive questions start on Cancel so a stray Enter does no harm. -->
        {#if spec.cancel !== null}
          <!-- svelte-ignore a11y_autofocus -->
          <button type="button" autofocus={spec.danger && !spec.input} onclick={() => dialog.resolve(null)}>{spec.cancel ?? 'Cancel'}</button>
        {/if}
        <!-- svelte-ignore a11y_autofocus -->
        <button type="submit" class={spec.danger ? 'danger' : 'primary'} autofocus={!spec.input && !spec.danger}>{spec.ok ?? 'OK'}</button>
      </div>
    </form>
  {/if}
</dialog>

<style>
  .modal,
  .content {
    display: grid;
    gap: var(--lh);
  }
  .content {
    max-height: round(down, calc(100vh - var(--lh) * 10), var(--lh));
    overflow: auto;
  }
  .modal.wide {
    width: 90ch;
  }
  /* app.css gives every .modal frame --line-strong, after the tones: a danger question keeps its red. */
  .modal[data-tone='err'] > :global(.fr) {
    color: var(--red);
  }
  .modal .actions {
    margin-top: 0;
  }
  p {
    overflow-wrap: anywhere;
  }
  pre {
    max-height: calc(var(--lh) * 20);
    padding: 0 1ch;
    overflow: auto;
    color: var(--fg);
    white-space: pre-wrap;
    word-break: break-word;
    background: var(--bg-input);
  }
  .field {
    display: grid;
  }
  label {
    color: var(--fg-muted);
  }
  .prompt-row {
    display: flex;
    gap: 1ch;
    white-space: nowrap;
  }
</style>
