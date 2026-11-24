<script lang="ts">
  import { onMount } from 'svelte'
  import { EditorView } from '@codemirror/view'
  import { MergeView } from '@codemirror/merge'
  import { baseExtensions, readOnly } from '../lib/editor'

  /** Read-only file view; with `compare` it shows `content` (left) against `compare` (right). */
  let { content, path, compare }: { content: string; path: string; compare?: string } = $props()
  let host: HTMLElement

  onMount(() => {
    const extensions = [baseExtensions(path), readOnly]
    const view =
      compare === undefined
        ? new EditorView({ parent: host, doc: content, extensions })
        : new MergeView({
            parent: host,
            a: { doc: content, extensions },
            b: { doc: compare, extensions },
            collapseUnchanged: { margin: 3, minSize: 8 },
          })
    return () => view.destroy()
  })
</script>

<div class="code" bind:this={host}></div>

<style>
  .code {
    height: 100%;
    overflow: auto;
  }
  .code :global(.cm-editor) {
    min-height: 100%;
  }
  .code :global(.cm-mergeView .cm-editor) {
    min-height: 0;
  }
  /* The two sides of a compare stand one filled column apart. */
  .code :global(.cm-mergeViewEditors) {
    gap: 0 1ch;
    background: var(--bg-hover);
  }
</style>
