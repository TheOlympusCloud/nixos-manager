<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError } from '../lib/api'
  import { app, login } from '../lib/state.svelte'
  import Icon from './Icon.svelte'

  /** When set, the dialog can be closed (Escape or the link) to go back to the intro behind it. */
  let { dismiss }: { dismiss?: () => void } = $props()

  let password = $state('')
  let error = $state('')
  let busy = $state(false)
  let el: HTMLDialogElement

  // A modal dialog keeps the workspace (and any unsaved edits) behind it but out of reach.
  onMount(() => el.showModal())

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await login(password)
      password = ''
    } catch (err) {
      error = err instanceof ApiError && err.status === 401 ? 'Wrong password.' : err instanceof Error ? err.message : String(err)
    } finally {
      busy = false
    }
  }
</script>

<!-- Browsers may close a modal on a repeated Escape despite preventDefault, so reopen it. -->
<dialog
  bind:this={el}
  oncancel={(e) => !dismiss && e.preventDefault()}
  onclose={() => (dismiss ? dismiss() : app.needLogin && el.isConnected && el.showModal())}
  aria-labelledby="login-title"
>
  <form class="card" onsubmit={submit}>
    <div class="brand"><span class="mark"><Icon name="mark" size={18} /></span><h1 id="login-title">nixos-manager</h1></div>
    <p class="tag">Your fleet's NixOS configuration, in one place.</p>
    <label for="login-password">Password</label>
    <!-- svelte-ignore a11y_autofocus -->
    <input id="login-password" type="password" autocomplete="current-password" bind:value={password} autofocus required />
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <button class="primary" type="submit" disabled={busy || !password}>{busy ? 'Logging in…' : 'Log in'}</button>
    {#if dismiss}<button class="back" type="button" onclick={dismiss}>Back to the story</button>{/if}
  </form>
</dialog>

<style>
  dialog {
    padding: 0;
    color: var(--fg);
    background: var(--bg-raised);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-3);
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(380px, 92vw);
    padding: 32px 32px 28px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .mark {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    color: var(--on-accent);
    background: var(--accent);
    border-radius: var(--r);
  }
  h1 {
    font-weight: 700;
    font-size: 17px;
  }
  .tag {
    margin-bottom: 22px;
    color: var(--fg-muted);
    font-size: 13px;
  }
  label {
    color: var(--fg-strong);
    font-size: var(--fs-sm);
    font-weight: 700;
  }
  input {
    height: 44px;
    font-size: 15px;
  }
  .error {
    margin: 0;
    color: var(--red);
    font-size: 13px;
  }
  button.primary {
    height: 44px;
    margin-top: 10px;
    font-weight: 700;
    font-size: 14px;
  }
  .back {
    height: 36px;
    margin-top: 4px;
    color: var(--fg-muted);
    font-size: 12px;
    background: none;
    border: 0;
    box-shadow: none;
  }
  .back:hover:not(:disabled) {
    color: var(--fg-strong);
    background: none;
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
