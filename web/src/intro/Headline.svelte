<script lang="ts">
  // A chapter headline in the pixel face, one line per entry ([text, tone]). The words themselves are the heading's
  // text; a window too narrow for the pixel face shows them as plain bold text instead.
  import { bigText } from '../lib/ascii'

  let { lines, tag = 'h2' }: { lines: [string, string?][]; tag?: 'h1' | 'h2' } = $props()
</script>

<svelte:element this={tag} class="headline">
  <span class="plain sr-only">{lines.map(([s]) => s).join(' ')}</span>
  {#each lines as [text, tone] (text)}<pre class="big {tone ?? ''}" aria-hidden="true" use:bigText={text}></pre>{/each}
</svelte:element>

<style>
  .headline {
    text-transform: none;
  }
  .headline::before {
    content: none;
  }
  @media (max-width: 860px) {
    .plain.sr-only {
      position: static;
      width: auto;
      height: auto;
      overflow: visible;
      clip-path: none;
      white-space: normal;
    }
    pre {
      display: none;
    }
  }
</style>
