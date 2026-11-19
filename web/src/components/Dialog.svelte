<script lang="ts">
  import { dialog } from '../lib/ui.svelte'

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

<!-- Escape fires "cancel"; closing by any other route also resolves as cancelled. -->
<dialog bind:this={el} onclose={() => dialog.spec && dialog.resolve(null)} aria-labelledby="dialog-title">
  {#if dialog.spec}
    {@const spec = dialog.spec}
    <form onsubmit={submit}>
      <h2 id="dialog-title">{spec.title}</h2>
      {#if spec.message}<p>{spec.message}</p>{/if}
      {#if spec.pre !== undefined}<pre>{spec.pre}</pre>{/if}
      {#if spec.input}
        <label for="dialog-input">{spec.input.label}</label>
        <!-- svelte-ignore a11y_autofocus -->
        <input id="dialog-input" bind:value autofocus spellcheck="false" autocomplete="off" />
      {/if}
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
  dialog {
    width: min(520px, 92vw);
    padding: 0;
    color: var(--fg);
    background: var(--bg-side);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--edge), var(--shadow-3);
  }
  dialog::backdrop {
    background: rgb(0 0 0 / 0.45);
  }
  form {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 18px 20px;
  }
  h2 {
    margin: 0;
    font-size: 15px;
  }
  p {
    margin: 0;
    line-height: 1.5;
    overflow-wrap: anywhere;
  }
  pre {
    margin: 0;
    max-height: 50vh;
    overflow: auto;
    padding: 10px;
    background: var(--bg-deep);
    border-radius: 4px;
    font: 12px/1.5 var(--mono);
    white-space: pre-wrap;
    word-break: break-word;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 6px;
  }
</style>
