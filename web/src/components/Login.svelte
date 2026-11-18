<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError } from '../lib/api'
  import { app, login } from '../lib/state.svelte'
  import Icon from './Icon.svelte'

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
    <div class="brand"><span class="mark"><Icon name="mark" size={20} /></span><h1 id="login-title">nixos-manager</h1></div>
    <p class="muted small">Your fleet's NixOS configuration, in one place.</p>
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
    width: min(340px, 90vw);
    padding: 28px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .mark {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 12%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
    border-radius: 9px;
  }
  h1 {
    font-size: var(--fs-xl);
  }
  .card > p {
    margin-bottom: 14px;
  }
  .error {
    margin: 0;
    color: var(--red);
  }
  button {
    height: 34px;
    margin-top: 8px;
  }
</style>
