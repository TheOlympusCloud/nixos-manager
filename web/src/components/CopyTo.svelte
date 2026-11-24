<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import { frame } from '../lib/ascii'
  import { plural } from '../lib/format'
  import Icon from './Icon.svelte'

  /**
   * "Copy to…": a button that opens a pick of machines, then runs copy with those
   * picked. Its button and panel are siblings, so in a wrapping row the panel
   * takes a line of its own.
   * title: what is copied from where; note: about the copy; targets: a note each.
   */
  let { title, note, targets, copy }: { title: string; note: string; targets: { name: string; note?: string }[]; copy: (to: string[]) => Promise<unknown> } = $props()

  let open = $state(false)
  let busy = $state(false)
  const picked = new SvelteSet<string>()
  function toggle() {
    open = !open
    picked.clear()
    if (open) for (const t of targets) picked.add(t.name)
  }
  async function run() {
    busy = true
    try {
      await copy(targets.map((t) => t.name).filter((n) => picked.has(n)))
    } finally {
      busy = false
      open = false
    }
  }
</script>

{#if targets.length}
  <button class="small" aria-expanded={open} onclick={toggle}><Icon name="copy" /> Copy to…</button>
  {#if open}
    <div class="copy" use:frame={'dash'}>
      <p>{title}</p>
      <div class="chips">
        {#each targets as t (t.name)}
          <label class="target" class:on={picked.has(t.name)}>
            <input type="checkbox" checked={picked.has(t.name)} onchange={() => (picked.has(t.name) ? picked.delete(t.name) : picked.add(t.name))} />
            {t.name}
            {#if t.note}<span class="muted small">{t.note}</span>{/if}
          </label>
        {/each}
      </div>
      <p class="muted small">{note}</p>
      <div class="buttons">
        <button class="primary small" disabled={!picked.size || busy} onclick={run}>{busy ? 'Copying…' : picked.size ? `Copy to ${plural(picked.size, 'machine')}` : 'Pick machines'}</button>
        <button class="small ghost" onclick={() => (open = false)}>Cancel</button>
      </div>
    </div>
  {/if}
{/if}

<style>
  .copy {
    display: grid;
    flex-basis: 100%;
    gap: var(--lh);
    margin: 0;
  }
  .copy p {
    margin: 0;
  }
  .chips {
    gap: 0 2ch;
  }
  .target {
    display: inline-flex;
    gap: 1ch;
    padding-right: 1ch;
    cursor: pointer;
  }
  .target.on {
    color: var(--fg-strong);
  }
  .buttons {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
</style>
