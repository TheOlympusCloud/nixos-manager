<script lang="ts">
  import Icon from './Icon.svelte'
  import Switch from './Switch.svelte'
  import ValueTree from './ValueTree.svelte'

  /**
   * A JSON value as a nested form: an object's keys and a list's items each get the
   * control their value's type needs, and keys and items can be added and removed.
   * `onchange` gets the whole new value.
   */
  let { value, label, onchange }: { value: unknown; label: string; onchange: (v: unknown) => void } = $props()

  type Kind = 'text' | 'number' | 'boolean' | 'object' | 'list'
  const kinds: [Kind, string][] = [
    ['text', 'Text'],
    ['number', 'Number'],
    ['boolean', 'Yes/no'],
    ['object', 'Attribute set'],
    ['list', 'List'],
  ]
  const empty = (k: Kind): unknown => ({ text: '', number: 0, boolean: false, object: {}, list: [] })[k]
  const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)

  let newKey = $state('')
  let newKind = $state<Kind>('text')
  const keyTaken = $derived(isObject(value) && Object.hasOwn(value, newKey.trim()))

  function setKey(k: string, v: unknown) {
    onchange({ ...(value as Record<string, unknown>), [k]: v })
  }
  function dropKey(k: string) {
    const next = { ...(value as Record<string, unknown>) }
    delete next[k]
    onchange(next)
  }
  function addKey(e: SubmitEvent) {
    e.preventDefault()
    const k = newKey.trim()
    if (!k || keyTaken) return
    setKey(k, empty(newKind))
    newKey = ''
  }
  const setItem = (i: number, v: unknown) => onchange((value as unknown[]).map((x, j) => (j === i ? v : x)))
  const dropItem = (i: number) => onchange((value as unknown[]).filter((_, j) => j !== i))
  const addItem = () => onchange([...(value as unknown[]), empty(newKind)])
</script>

{#if isObject(value)}
  <div class="tree" role="group" aria-label={label}>
    {#each Object.entries(value) as [k, v] (k)}
      <div class="row" class:nested={isObject(v) || Array.isArray(v)}>
        <span class="key mono">{k}</span>
        <div class="val"><ValueTree value={v} label="{label}.{k}" onchange={(x) => setKey(k, x)} /></div>
        <button class="icon" aria-label="Remove {k}" title="Remove {k}" onclick={() => dropKey(k)}><Icon name="trash" /></button>
      </div>
    {:else}
      <span class="muted small">No attributes</span>
    {/each}
    <form class="add" onsubmit={addKey}>
      <input class="mono short" placeholder="New attribute" aria-label="New attribute of {label}" bind:value={newKey} spellcheck="false" />
      <select class="short" aria-label="Its type" bind:value={newKind}>
        {#each kinds as [k, l] (k)}<option value={k}>{l}</option>{/each}
      </select>
      <button type="submit" class="small" disabled={!newKey.trim() || keyTaken} title={keyTaken ? 'It is there already' : ''}>Add</button>
    </form>
  </div>
{:else if Array.isArray(value)}
  <div class="tree" role="group" aria-label={label}>
    {#each value as v, i (i)}
      <div class="row" class:nested={isObject(v) || Array.isArray(v)}>
        <span class="key mono muted">{i + 1}</span>
        <div class="val"><ValueTree value={v} label="{label}[{i + 1}]" onchange={(x) => setItem(i, x)} /></div>
        <button class="icon" aria-label="Remove item {i + 1}" title="Remove item {i + 1}" onclick={() => dropItem(i)}><Icon name="trash" /></button>
      </div>
    {:else}
      <span class="muted small">Empty list</span>
    {/each}
    <div class="add">
      <select class="short" aria-label="Type of the new item of {label}" bind:value={newKind}>
        {#each kinds as [k, l] (k)}<option value={k}>{l}</option>{/each}
      </select>
      <button class="small" onclick={addItem}>Add an item</button>
    </div>
  </div>
{:else if typeof value === 'boolean'}
  <label class="switch-row"><Switch checked={value} {label} onchange={(on) => onchange(on)} /><span>{value ? 'Yes' : 'No'}</span></label>
{:else if typeof value === 'number'}
  <input class="short num" type="number" aria-label={label} value={value} oninput={(e) => e.currentTarget.value !== '' && onchange(Number(e.currentTarget.value))} />
{:else}
  <input class="mono" aria-label={label} value={value == null ? '' : String(value)} spellcheck="false" oninput={(e) => onchange(e.currentTarget.value)} />
{/if}

<style>
  .tree {
    display: grid;
    gap: calc(var(--lh) / 2);
    border-left: 1px solid var(--line);
    padding-left: 2ch;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(8ch, max-content) 1fr auto;
    gap: 2ch;
    align-items: baseline;
  }
  .row.nested {
    grid-template-columns: 1fr auto;
  }
  .row.nested .key {
    grid-column: 1 / -1;
  }
  .row.nested .val {
    grid-column: 1;
  }
  .key {
    color: var(--fg-strong);
  }
  .add {
    display: flex;
    flex-wrap: wrap;
    gap: 1ch;
    align-items: baseline;
  }
  .switch-row {
    display: inline-flex;
    gap: 1ch;
    align-items: center;
  }
</style>
