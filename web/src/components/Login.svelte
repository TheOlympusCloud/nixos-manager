<script lang="ts">
  import { onMount } from 'svelte'
  import { Buf, frame, scene } from '../lib/ascii'
  import { ApiError } from '../lib/api'
  import { app, login } from '../lib/state.svelte'
  import { grid, planet, stars } from '../intro/scenes'

  /** When set, the dialog can be closed (Escape or the link) to go back to the intro behind it. */
  let { dismiss }: { dismiss?: () => void } = $props()

  let password = $state('')
  let error = $state('')
  let busy = $state(false)
  let el: HTMLDialogElement
  let starsEl: HTMLPreElement
  let globe: HTMLPreElement
  let hot: HTMLPreElement

  // A modal dialog keeps the workspace (and any unsaved edits) behind it but out of reach.
  onMount(() => el.showModal())

  // Behind the dialog the planet rises over the horizon under the stars.
  onMount(() => {
    const paint = scene(globe, (t) => {
      const { cols, rows } = grid()
      const L = { stars: new Buf(cols, rows), globe: new Buf(cols, rows), hot: new Buf(cols, rows), warm: new Buf(cols, rows) }
      stars(L.stars, t)
      planet(L, cols * 0.5, rows * 1.25, rows * 0.75, t * 0.03, 0, 0.8)
      starsEl.textContent = L.stars.toString()
      hot.textContent = L.hot.toString()
      return L.globe.toString()
    })
    addEventListener('resize', paint)
    return () => {
      paint.stop()
      removeEventListener('resize', paint)
    }
  })

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

  const MARK = '╲  λ  ╱\n ╲ │ ╱ \nλ ──❄── λ\n ╱ │ ╲ \n╱  λ  ╲'
</script>

<!-- Browsers may close a modal on a repeated Escape despite preventDefault, so reopen it. -->
<dialog
  bind:this={el}
  class="screen"
  oncancel={(e) => !dismiss && e.preventDefault()}
  onclose={() => (dismiss ? dismiss() : app.needLogin && el.isConnected && el.showModal())}
  aria-labelledby="login-title"
>
  <pre class="stars" aria-hidden="true" bind:this={starsEl}></pre>
  <pre class="globe" aria-hidden="true" bind:this={globe}></pre>
  <pre class="hot" aria-hidden="true" bind:this={hot}></pre>
  <form class="modal" onsubmit={submit} use:frame={{ kind: 'double', shadow: true, animate: true, title: '❄ nixos-manager' }}>
    <h1 id="login-title" class="sr-only">nixos-manager</h1>
    <pre class="mark" aria-hidden="true">{MARK}</pre>
    <p class="tag">Your fleet's NixOS configuration, in one place.</p>
    <div class="field">
      <label for="login-password" class="prompt">Password</label><b class="prompt" aria-hidden="true">›</b>
      <!-- svelte-ignore a11y_autofocus -->
      <input id="login-password" type="password" autocomplete="current-password" bind:value={password} autofocus required />
    </div>
    {#if error}<p class="error-text" role="alert"><span aria-hidden="true">✖&nbsp;</span>{error}</p>{/if}
    <div class="actions">
      <button class="primary" type="submit" disabled={busy || !password}>{busy ? 'Logging in…' : 'Log in'}</button>
      {#if dismiss}<button class="bare back" type="button" onclick={dismiss}><span aria-hidden="true">‹&nbsp;</span>Back to the story</button>{/if}
    </div>
  </form>
</dialog>

<style>
  /* Login is a screen of its own: the night sky and the planet, and the one double frame. */
  dialog.screen {
    background: var(--bg);
  }
  dialog > pre {
    position: absolute;
    inset: 0;
    overflow: hidden;
    pointer-events: none;
  }
  .stars {
    color: var(--fg-faint);
  }
  .globe {
    color: var(--accent-fill);
  }
  .hot {
    color: var(--accent);
    text-shadow: var(--glow);
  }
  .modal {
    display: grid;
    gap: var(--lh);
    width: 58ch;
  }
  .mark {
    justify-self: center;
    color: var(--accent);
  }
  .tag {
    color: var(--fg-muted);
    text-align: center;
  }
  .field {
    display: flex;
    gap: 1ch;
  }
  .modal .actions {
    margin-top: 0;
  }
  /* The way back sits at the left, the way in at the right. */
  .back {
    order: -1;
    margin-right: auto;
    color: var(--fg-muted);
  }
</style>
