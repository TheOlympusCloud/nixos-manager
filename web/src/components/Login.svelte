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
    color: #f2f5f9;
    font-family: 'Geist Variable', var(--sans);
    background: #05070a;
    border: 1px solid rgb(255 255 255 / 0.14);
    border-radius: 0;
    box-shadow:
      0 30px 80px rgb(0 0 0 / 0.7),
      0 0 0 1px rgb(0 0 0 / 0.8);
  }
  dialog::backdrop {
    background: rgb(5 7 10 / 0.72);
    backdrop-filter: blur(6px);
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
    color: #7eb6ff;
    border: 1px solid rgb(126 182 255 / 0.4);
  }
  h1 {
    color: inherit;
    font-weight: 600;
    font-size: 17px;
    letter-spacing: -0.01em;
  }
  .tag {
    margin-bottom: 22px;
    color: #8f9bab;
    font-size: 13px;
  }
  label {
    color: #8f9bab;
    font-size: 11px;
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }
  input {
    height: 44px;
    color: #f2f5f9;
    font-size: 15px;
    background: #0a0e14;
    border: 1px solid rgb(255 255 255 / 0.16);
    border-radius: 0;
  }
  input:focus {
    border-color: #7eb6ff;
    box-shadow: 0 0 0 3px rgb(126 182 255 / 0.2);
  }
  .error {
    margin: 0;
    color: #ff8a8a;
    font-size: 13px;
  }
  button.primary {
    height: 46px;
    margin-top: 10px;
    font-weight: 600;
    font-size: 12px;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    background: #2468e0;
    border-color: #2468e0;
    border-radius: 0;
  }
  button.primary:hover:not(:disabled) {
    background: #2f74ea;
    border-color: #2f74ea;
  }
  .back {
    height: 36px;
    margin-top: 4px;
    color: #8f9bab;
    font-size: 12px;
    background: none;
    border: 0;
  }
  .back:hover:not(:disabled) {
    color: #f2f5f9;
    background: none;
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
