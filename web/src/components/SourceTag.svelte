<script lang="ts">
  import { codeLink } from '../lib/router.svelte'
  import type { Source } from '../lib/values'

  /** Where a value comes from, in the shared words: Set here, In code · file, Default. */
  let { source, pending = false }: { source: Source; pending?: boolean } = $props()
</script>

{#if pending}
  <span class="src busy" title="Reset here: the next evaluation shows the value that applies now"><span class="dot busy" aria-hidden="true"></span> Reset here</span>
{:else if source.kind === 'here'}
  <span class="src here" title="Set in this app (managed.json). It wins over hand-written Nix.">✎ Set here</span>
{:else if source.kind === 'code'}
  <span class="src code" title={source.files.length > 1 ? `Defined in ${source.files.join(', ')}` : `Defined in ${source.file}`}
    >λ In code · <a class="mono" href={codeLink(source.file)}>{source.file}</a>{#if source.files.length > 1}<span class="more"> +{source.files.length - 1}</span>{/if}</span>
{:else}
  <span class="src def" title="Not set in this repository: NixOS's default, or a flake input's">· Default</span>
{/if}

<style>
  .src {
    display: inline-block;
    flex: none;
    min-width: 0;
    max-width: 100%;
    overflow: hidden;
    color: var(--fg-muted);
    white-space: nowrap;
    text-overflow: ellipsis;
    vertical-align: top;
  }
  .here {
    color: var(--green);
  }
  .busy {
    color: var(--accent);
  }
  .code a {
    color: inherit;
  }
  .code a:hover {
    color: var(--on-accent);
  }
  .def,
  .more {
    color: var(--fg-faint);
  }
</style>
