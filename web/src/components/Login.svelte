<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError } from '../lib/api'
  import { app, login } from '../lib/state.svelte'

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
<dialog bind:this={el} oncancel={(e) => e.preventDefault()} onclose={() => app.needLogin && el.isConnected && el.showModal()} aria-labelledby="login-title">
  <form class="card" onsubmit={submit}>
    <h1 id="login-title">nixos-manager</h1>
    <label for="login-password">Password</label>
    <!-- svelte-ignore a11y_autofocus -->
    <input id="login-password" type="password" autocomplete="current-password" bind:value={password} autofocus required />
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <button class="primary" type="submit" disabled={busy || !password}>{busy ? 'Logging in…' : 'Log in'}</button>
  </form>
</dialog>

<style>
  dialog {
    padding: 0;
    color: var(--fg);
    background: var(--bg-side);
    border: 1px solid var(--line);
    border-radius: 8px;
  }
  dialog::backdrop {
    background: color-mix(in srgb, var(--bg-deep) 92%, transparent);
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(320px, 90vw);
    padding: 24px;
  }
  h1 {
    margin: 0 0 12px;
    font-size: 18px;
    font-weight: 600;
  }
  .error {
    margin: 0;
    color: var(--red);
  }
  button {
    margin-top: 8px;
    padding: 6px;
  }
</style>
