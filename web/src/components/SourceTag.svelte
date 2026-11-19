<script lang="ts">
  import { codeLink } from '../lib/router.svelte'
  import type { Source } from '../lib/values'

  /** Where a value comes from, in the shared words: Set here, In code · file, Default. */
  let { source, pending = false }: { source: Source; pending?: boolean } = $props()
</script>

{#if pending}
  <span class="src busy" title="Reset here: the next evaluation shows the value that applies now">Reset here</span>
{:else if source.kind === 'here'}
  <span class="src here" title="Set in this app (managed.json). It wins over hand-written Nix.">Set here</span>
{:else if source.kind === 'code'}
  <span class="src code" title={source.files.length > 1 ? `Defined in ${source.files.join(', ')}` : `Defined in ${source.file}`}
    >In code · <a class="mono" href={codeLink(source.file)}>{source.file}</a>{#if source.files.length > 1}<span class="more"> +{source.files.length - 1}</span>{/if}</span>
{:else}
  <span class="src" title="Not set in this repository: NixOS's default, or a flake input's">Default</span>
{/if}

<style>
  .src {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: 4px;
    min-width: 0;
    max-width: 100%;
    height: 20px;
    padding: 0 7px;
    overflow: hidden;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    font-weight: 500;
    white-space: nowrap;
    text-overflow: ellipsis;
    background: var(--bg-hover);
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
  }
  .here {
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
    border-color: color-mix(in srgb, var(--accent) 30%, transparent);
  }
  .busy {
    color: var(--accent);
    background: color-mix(in srgb, var(--accent) 6%, transparent);
    border-style: dashed;
    border-color: color-mix(in srgb, var(--accent) 35%, transparent);
  }
  .code {
    font-weight: 400;
  }
  .code a {
    overflow: hidden;
    color: var(--fg);
    font-size: 10.5px;
    text-decoration: none;
    text-overflow: ellipsis;
  }
  .code a:hover {
    color: var(--accent);
    text-decoration: underline;
  }
  .more {
    color: var(--fg-faint);
  }
</style>
