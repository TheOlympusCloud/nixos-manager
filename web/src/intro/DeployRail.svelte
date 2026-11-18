<script lang="ts">
  // How one change travels, as a rail synced to scroll: the fill rises with t, each step lights up
  // as it is reached and shows what the console would log for it.
  let { t }: { t: number } = $props()

  const STEPS = [
    ['Edit', 'homeserver: +jellyfin · port tcp/8096'],
    ['Check', 'nix flake check · dry-build 3/3 · passed'],
    ['Commit', '4e1f9a2 · homeserver: enable jellyfin'],
    ['Push', 'main → forgejo · fleet/nixos'],
    ['Pull', 'comin · fetched 4e1f9a2 on 3 machines'],
    ['Switch', 'workstation ✓ · homeserver ✓ · pi ✓'],
  ] as const

  // The rail fills over the middle of the chapter so the heading has time to land first.
  const fill = $derived(Math.min(1, Math.max(0, (t - 0.12) / 0.78)))
  const reached = $derived(Math.min(STEPS.length - 1, Math.floor(fill * STEPS.length + 1e-6)))
</script>

<ol class="rail" style:--fill={fill}>
  {#each STEPS as [name, detail], i (name)}
    {@const on = fill >= i / STEPS.length}
    <li class:on class:now={on && i === reached} class:done={on && i < reached}>
      <span class="dot" aria-hidden="true"></span>
      <span class="idx mono">{String(i + 1).padStart(2, '0')}</span>
      <span class="name">{name}</span>
      <span class="detail mono">{detail}</span>
    </li>
  {/each}
</ol>

<style>
  .rail {
    --fill: 0;
    position: relative;
    display: grid;
    gap: clamp(14px, 2.4vh, 26px);
    margin: 0;
    padding: 0 0 0 36px;
    list-style: none;
  }
  .rail::before,
  .rail::after {
    content: '';
    position: absolute;
    top: 10px;
    bottom: 10px;
    left: 6px;
    width: 1px;
  }
  .rail::before {
    background: var(--i-line);
  }
  .rail::after {
    background: var(--i-blue);
    transform: scaleY(var(--fill));
    transform-origin: top;
    box-shadow: 0 0 12px rgb(126 182 255 / 0.6);
  }
  li {
    position: relative;
    display: grid;
    grid-template-columns: 32px 1fr;
    align-items: baseline;
    column-gap: 10px;
    color: var(--i-muted);
    transition: color 300ms var(--i-ease);
  }
  li.on {
    color: var(--i-fg);
  }
  .dot {
    position: absolute;
    top: 8px;
    left: -34px;
    width: 9px;
    height: 9px;
    background: var(--i-bg);
    border: 1px solid var(--i-line);
    border-radius: 50%;
    transition:
      border-color 300ms var(--i-ease),
      background 300ms var(--i-ease),
      box-shadow 300ms var(--i-ease);
  }
  li.on .dot {
    border-color: var(--i-blue);
    background: var(--i-blue);
    box-shadow: 0 0 0 4px rgb(126 182 255 / 0.18);
  }
  li.done .dot {
    background: var(--i-green);
    border-color: var(--i-green);
    box-shadow: none;
  }
  .idx {
    color: var(--i-muted);
    font-size: 0.7rem;
    letter-spacing: 0.1em;
  }
  .name {
    font-weight: 600;
    font-size: clamp(1.1rem, 1.6vw, 1.5rem);
    letter-spacing: -0.02em;
    line-height: 1.1;
  }
  .detail {
    grid-column: 2;
    max-height: 0;
    overflow: hidden;
    color: var(--i-blue);
    font-size: 0.78rem;
    opacity: 0;
    transition:
      max-height 400ms var(--i-ease),
      opacity 300ms var(--i-ease),
      margin 400ms var(--i-ease);
  }
  li.on .detail {
    max-height: 3em;
    margin-top: 6px;
    opacity: 1;
  }
  li.done .detail {
    color: var(--i-muted);
  }
  :global(.intro.rm) li .detail {
    max-height: none;
    opacity: 1;
    margin-top: 6px;
  }
  :global(.intro.rm) li {
    color: var(--i-fg);
  }
</style>
