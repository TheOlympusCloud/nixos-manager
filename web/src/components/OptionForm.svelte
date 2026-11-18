<script lang="ts">
  import type { OptionInfo } from '../lib/api'
  import { fleet, hostByName, managedValue, setOption, unsetOption } from '../lib/fleet.svelte'
  import { controlFor, docsHTML, fillPath, optionSegments, parseNix, type Scalar } from '../lib/options'
  import { machineLink } from '../lib/router.svelte'
  import Icon from './Icon.svelte'
  import Switch from './Switch.svelte'

  /** The form for one NixOS option on one machine, generated from the option's type. */
  let { host, option }: { host: string; option: OptionInfo } = $props()

  const segs = $derived(optionSegments(option.name))
  const placeholders = $derived(segs.filter((s) => s.kind === 'name'))
  const inList = $derived(segs.some((s) => s.kind === 'list'))
  let names = $state<string[]>([])
  const path = $derived(fillPath(segs, names))
  const pathReady = $derived(placeholders.every((_, i) => names[i]?.trim()))
  const control = $derived(controlFor(option.type))
  const managed = $derived(!!hostByName(host)?.managedFile)
  const editable = $derived(managed && !option.readOnly && !inList && !!fleet.managed[host])
  const current = $derived(pathReady ? managedValue(host, path) : undefined)
  const fallback = $derived(parseNix(option.default))

  // Names already used under the first placeholder, e.g. existing virtual hosts.
  const known = $derived.by(() => {
    const i = segs.findIndex((s) => s.kind === 'name')
    if (i < 0) return []
    const v = managedValue(host, fillPath(segs.slice(0, i), names))
    return v && typeof v === 'object' && !Array.isArray(v) ? Object.keys(v) : []
  })

  // The draft follows the saved value until the user edits it.
  let draft = $state<unknown>(undefined)
  let jsonText = $state('')
  let touched = $state(false)
  $effect(() => {
    const v = current !== undefined ? current : fallback
    if (touched) return
    draft = v
    jsonText = v === undefined ? '' : JSON.stringify(v, null, 2)
  })
  const edit = (v: unknown) => {
    draft = v
    touched = true
  }

  // A number input binds a number (or null when empty), a text input a string.
  let listInput = $state<string | number | null>('')
  const listText = $derived(String(listInput ?? '').trim())
  let busy = $state(false)

  function checkScalar(s: Scalar, v: unknown): string {
    if (s.kind === 'number') {
      if (typeof v !== 'number' || !Number.isInteger(v)) return 'Enter a whole number.'
      if (s.min !== undefined && v < s.min) return `At least ${s.min}.`
      if (s.max !== undefined && v > s.max) return `At most ${s.max}.`
    }
    if (s.kind === 'text' && typeof v === 'string') {
      if (!v) return 'Enter a value, or use Reset to go back to the default.'
      if (s.noSpaces && /\s/.test(v)) return 'No spaces allowed.'
    }
    return ''
  }

  const problem = $derived.by(() => {
    if (!pathReady) return `Name the ${placeholders.map((p) => p.text).join(' and ')} first.`
    const c = control
    if (c.kind === 'json') {
      if (!jsonText.trim()) return 'Enter a JSON value.'
      try {
        const v = JSON.parse(jsonText)
        return v === null ? 'null cannot be stored; use Reset instead.' : ''
      } catch (e) {
        return `Not valid JSON: ${(e as Error).message}`
      }
    }
    if (c.kind === 'list') {
      if (!Array.isArray(draft)) return ''
      for (const item of draft) {
        const p = checkScalar(c.item, item)
        if (p) return `${JSON.stringify(item)}: ${p}`
      }
      return ''
    }
    if (c.kind === 'number' || c.kind === 'text' || c.kind === 'select') return draft === undefined || draft === null ? 'Pick a value.' : checkScalar(c, draft)
    return draft === undefined || draft === null ? 'Pick a value.' : ''
  })

  const value = $derived(control.kind === 'json' ? (problem ? undefined : JSON.parse(jsonText)) : draft)
  const changed = $derived(touched && JSON.stringify(value) !== JSON.stringify(current))

  async function save() {
    busy = true
    if (await setOption(host, path, value)) touched = false
    busy = false
  }
  async function reset() {
    busy = true
    if (await unsetOption(host, path)) touched = false
    busy = false
  }

  function addItem(s: Scalar) {
    const raw = listText
    if (!raw) return
    const item = s.kind === 'number' ? Number(raw) : raw
    edit([...(Array.isArray(draft) ? draft : []), item])
    listInput = ''
  }
  /** Short Nix values on one line: "[\n  22\n]" → "[ 22 ]". */
  const compact = (nix: string) => (nix.length <= 80 ? nix.replace(/\s*\n\s*/g, ' ') : nix)
  const asText = (v: unknown) => (v === undefined || v === null ? '' : String(v))
</script>

<div class="of">
  <div class="name-row">
    <h3 class="mono">{option.name}</h3>
    {#if current !== undefined}<span class="badge ok" title="Set in {fleet.managed[host]?.path}">Set here</span>{/if}
  </div>
  <div class="type mono">{option.type}</div>

  {#if option.description}<div class="docs">{@html docsHTML(option.description)}</div>{/if}

  <dl class="defaults">
    {#if option.default != null}<dt>Default</dt><dd><pre>{compact(option.default)}</pre></dd>{/if}
    {#if option.example != null}<dt>Example</dt><dd><pre>{compact(option.example)}</pre></dd>{/if}
  </dl>

  <div class="editor" class:locked={!editable}>
    {#if option.readOnly}
      <p class="note"><Icon name="info" size={14} />Read-only: NixOS computes this option; it cannot be set.</p>
    {:else if inList}
      <p class="note"><Icon name="info" size={14} />This option sits inside a list of submodules, which a settings path cannot address. Set it in Code.</p>
    {:else if !managed}
      <p class="note"><Icon name="info" size={14} />Visual editing is off on {host}. <a href={machineLink(host)}>Turn it on</a> to set options here.</p>
    {:else if !fleet.managed[host]}
      <span class="skel" style:width="40%"></span>
    {/if}

    {#if placeholders.length && editable}
      <div class="names">
        {#each placeholders as p, i (i)}
          <label class="field">
            <span class="label">{p.text.slice(1, -1)}</span>
            <input
              class="mono"
              list="known-{i}"
              placeholder={p.text}
              spellcheck="false"
              value={names[i] ?? ''}
              oninput={(e) => {
                names[i] = e.currentTarget.value
                touched = false
              }} />
            {#if i === 0 && known.length}<datalist id="known-{i}">{#each known as k (k)}<option value={k}></option>{/each}</datalist>{/if}
          </label>
        {/each}
      </div>
    {/if}

    {#if editable && !option.readOnly && !inList}
      {@const c = control}
      <span class="label">Value</span>
      <div class="control">
        {#if c.kind === 'switch'}
          <label class="switch-row">
            <Switch checked={draft === true} label={option.name} onchange={(on) => edit(on)} />
            <span>{draft === true ? 'Enabled' : 'Disabled'}</span>
          </label>
        {:else if c.kind === 'tristate'}
          {@const shown = touched ? draft : current}
          <div class="seg" role="radiogroup" aria-label={option.name}>
            {#each [['Default', undefined], ['Yes', true], ['No', false]] as const as [label, v] (label)}
              <button role="radio" aria-checked={shown === v} class:on={shown === v} onclick={() => (v !== undefined ? edit(v) : current !== undefined ? reset() : (touched = false))}>
                {label}
              </button>
            {/each}
          </div>
        {:else if c.kind === 'number'}
          <input
            class="short num"
            type="number"
            min={c.min}
            max={c.max}
            step="1"
            aria-label="Value of {option.name}"
            value={asText(draft)}
            oninput={(e) => edit(e.currentTarget.value === '' ? undefined : Number(e.currentTarget.value))} />
          {#if c.min !== undefined || c.max !== undefined}<span class="muted small">{c.min ?? '…'} to {c.max ?? '…'}</span>{/if}
        {:else if c.kind === 'text'}
          <input class="mono" aria-label="Value of {option.name}" value={asText(draft)} spellcheck="false" oninput={(e) => edit(e.currentTarget.value)} />
        {:else if c.kind === 'select'}
          <select class="short" aria-label="Value of {option.name}" value={asText(draft)} onchange={(e) => edit(c.choices.find((x) => String(x) === e.currentTarget.value))}>
            {#if draft === undefined || draft === null}<option value="">Choose…</option>{/if}
            {#each c.choices as ch (ch)}<option value={String(ch)}>{ch}</option>{/each}
          </select>
        {:else if c.kind === 'list'}
          {@const items = Array.isArray(draft) ? draft : []}
          <div class="chips list">
            {#each items as item, i (i)}
              <span class="chip mono">{String(item)}<button class="x" aria-label="Remove {item}" onclick={() => edit(items.filter((_, j) => j !== i))}><Icon name="x" size={11} /></button></span>
            {:else}
              <span class="muted small">Empty list</span>
            {/each}
          </div>
          <form class="add-item" onsubmit={(e) => (e.preventDefault(), addItem(c.item))}>
            {#if c.item.kind === 'select'}
              <select class="short" bind:value={listInput} aria-label="Item to add">
                <option value="">Choose…</option>
                {#each c.item.choices as ch (ch)}<option value={String(ch)}>{ch}</option>{/each}
              </select>
            {:else}
              <input
                class="short mono"
                type={c.item.kind === 'number' ? 'number' : 'text'}
                min={c.item.kind === 'number' ? c.item.min : undefined}
                max={c.item.kind === 'number' ? c.item.max : undefined}
                placeholder="Add an item"
                aria-label="Item to add"
                bind:value={listInput} />
            {/if}
            <button type="submit" disabled={!listText}>Add</button>
          </form>
        {:else}
          <textarea
            class="mono json"
            rows="6"
            spellcheck="false"
            aria-label="Value as JSON"
            bind:value={jsonText}
            oninput={() => (touched = true)}></textarea>
          <span class="muted small">This type has no dedicated control; write the value as JSON (it becomes the same Nix value).</span>
        {/if}
      </div>

      {#if touched && problem}<p class="problem" role="alert">{problem}</p>{/if}

      <div class="buttons">
        <button class="primary" disabled={!changed || !!problem || busy} onclick={save}>{busy ? 'Saving…' : 'Save'}</button>
        {#if current !== undefined}
          <button disabled={busy} onclick={reset} title="Remove it from {fleet.managed[host]?.path}; the default or the hand-written value applies again">Reset</button>
        {/if}
        {#if touched}<button class="ghost" onclick={() => (touched = false)}>Undo edits</button>{/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .of {
    display: grid;
    gap: 10px;
  }
  .name-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
  h3 {
    overflow-wrap: anywhere;
    font-size: var(--fs-lg);
    font-weight: 600;
  }
  .type {
    color: var(--accent);
    font-size: 12px;
  }
  .docs {
    max-width: 72ch;
    color: var(--fg);
    line-height: 1.6;
  }
  .docs :global(p + p),
  .docs :global(ul) {
    margin-top: 8px;
  }
  .docs :global(ul) {
    margin-bottom: 0;
    padding-left: 18px;
  }
  .docs :global(code) {
    padding: 1px 4px;
    color: var(--fg-strong);
    background: var(--bg-hover);
    border-radius: 3px;
  }
  .docs :global(pre) {
    padding: 8px;
    background: var(--bg-deep);
    border-radius: var(--r);
  }
  .defaults {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 14px;
    margin: 4px 0 0;
  }
  .defaults dt {
    padding-top: 4px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .defaults dd {
    margin: 0;
    min-width: 0;
  }
  .defaults pre {
    margin: 0;
    padding: 4px 8px;
    overflow: auto;
    font: 12px/1.5 var(--mono);
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
  }
  .editor {
    display: grid;
    gap: 12px;
    margin-top: 6px;
    padding: 14px;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .note {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--fg-muted);
  }
  .names {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }
  .field {
    display: grid;
    gap: 4px;
    width: 260px;
  }
  .editor > .label {
    margin-bottom: -6px;
  }
  .control {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
  .switch-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .short {
    width: 220px;
  }
  .seg {
    display: inline-flex;
    padding: 2px;
    background: var(--bg-deep);
    border: 1px solid var(--line);
    border-radius: var(--r);
  }
  .seg button {
    height: 26px;
    color: var(--fg-muted);
    background: none;
    border: 0;
  }
  .seg button.on {
    color: var(--fg-strong);
    background: var(--bg-active);
  }
  .list {
    width: 100%;
  }
  .add-item {
    display: flex;
    gap: 6px;
  }
  .json {
    width: 100%;
  }
  .problem {
    color: var(--red);
    font-size: var(--fs-sm);
  }
  .buttons {
    display: flex;
    gap: 8px;
  }
</style>
