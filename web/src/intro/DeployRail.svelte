<script lang="ts">
  // How one change travels, as a rail synced to scroll: each step lights up as it is reached and types what the
  // console would log for it.
  import { typed } from '../lib/ascii'
  import { deployFill } from './scenes'

  let { t }: { t: number } = $props()

  const STEPS = [
    ['Edit', 'homeserver: +jellyfin · port tcp/8096'],
    ['Check', 'nix flake check · built 3/3 · passed'],
    ['Commit', '4e1f9a2 · homeserver: enable jellyfin'],
    ['Push', 'main → forgejo · fleet/nixos'],
    ['Pull', 'comin · fetched 4e1f9a2 on 3 machines'],
    ['Switch', 'workstation ✓ · homeserver ✓ · pi ✓'],
  ] as const

  const fill = $derived(deployFill(t))
  const reached = $derived(Math.min(STEPS.length - 1, Math.floor(fill * STEPS.length + 1e-6)))
</script>

<ol class="rail">
  {#each STEPS as [name, detail], i (name)}
    {@const on = fill >= i / STEPS.length}
    <li class:on class:now={on && i === reached} class:done={on && i < reached}>
      <span class="dot" class:ok={on && i < reached} class:busy={on && i === reached} class:off={!on} aria-hidden="true"></span>
      <span class="idx">{String(i + 1).padStart(2, '0')}</span>
      <span class="name">{name}</span>
      {#key on}<span class="detail" class:sr-only={!on} use:typed={detail}></span>{/key}
      {#if i < STEPS.length - 1}<span class="stem" aria-hidden="true">{on && i < reached ? '┃' : on ? '│' : '┆'}</span>{/if}
    </li>
  {/each}
</ol>

<style>
  .rail {
    display: grid;
  }
  li {
    display: grid;
    grid-template-columns: 2ch 3ch 8ch minmax(0, 1fr);
    color: var(--fg-faint);
  }
  li.on {
    color: var(--fg);
  }
  .name {
    font-weight: 700;
  }
  li.on .name {
    color: var(--fg-strong);
  }
  .idx {
    color: var(--fg-faint);
  }
  .detail {
    overflow: hidden;
    color: var(--accent);
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  li.done .detail {
    color: var(--fg-muted);
  }
  .stem {
    grid-column: 1;
    color: var(--line);
  }
  li.done .stem {
    color: var(--green);
  }
  li.now .stem {
    color: var(--accent);
  }
</style>
