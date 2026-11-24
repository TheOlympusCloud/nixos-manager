<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import type { OptionInfo } from '../lib/api'
  import { frame } from '../lib/ascii'
  import { fleet, healthy, hostByName, managedValue, setOption, unsetOption, useValues, valueOf } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { controlFor, docsHTML, fillPath, optionSegments, parseNix, pathName, type Scalar } from '../lib/options'
  import { forUsers, secretPath, takesSecretFile } from '../lib/secrets'
  import { go, machineLink } from '../lib/router.svelte'
  import { quietly, toast } from '../lib/ui.svelte'
  import { copyable, effective, sameValue, showValue } from '../lib/values'
  import Icon from './Icon.svelte'
  import SourceTag from './SourceTag.svelte'
  import Switch from './Switch.svelte'

  /** The form for one NixOS option on one machine, generated from the option's type. */
  let { host, option, initialNames = [] }: { host: string; option: OptionInfo; initialNames?: string[] } = $props()

  const segs = $derived(optionSegments(option.name))
  const placeholders = $derived(segs.filter((s) => s.kind === 'name'))
  const inList = $derived(segs.some((s) => s.kind === 'list'))
  // svelte-ignore state_referenced_locally (the form is keyed on the option and its names)
  let names = $state<string[]>([...initialNames])
  const path = $derived(fillPath(segs, names))
  const pathReady = $derived(placeholders.every((_, i) => names[i]?.trim()))
  const control = $derived(controlFor(option.type))
  const managedFile = $derived(hostByName(host)?.managedFile ?? null)
  const managed = $derived(!!managedFile)
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

  // The effective value on every machine; typed names settle before they are asked for.
  let asked = $state('')
  $effect(() => {
    const name = pathReady && !inList ? pathName(path) : ''
    if (!placeholders.length || !name) return void (asked = name)
    const t = setTimeout(() => (asked = name), 450)
    return () => clearTimeout(t)
  })
  const vals = useValues(() => (asked ? { names: [asked] } : null))
  const machines = $derived(healthy())
  const eff = $derived(asked ? effective(valueOf(vals.values, asked, host), current, managedFile, !!fleet.managed[host]) : null)
  const rows = $derived(
    machines.map((h) => ({
      host: h.name,
      editable: !!h.managedFile,
      eff: asked ? effective(valueOf(vals.values, asked, h.name), managedValue(h.name, path), h.managedFile, !!fleet.managed[h.name]) : null,
    })),
  )
  const distinct = $derived(new Set(rows.filter((r) => r.eff?.state === 'value').map((r) => JSON.stringify(r.eff?.state === 'value' ? r.eff.value : null))).size)
  /** What the form starts from: the value set here, else the machine's effective value, else the documented default. */
  const base = $derived(current !== undefined ? current : eff?.state === 'value' && !eff.pending && copyable(eff.value) ? eff.value : fallback)

  // The draft follows the saved value until the user edits it.
  let draft = $state<unknown>(undefined)
  let jsonText = $state('')
  let touched = $state(false)
  $effect(() => {
    const v = base
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

  /**
   * Lists and attribute sets are written whole, so they must start from the value that applies now:
   * wait for it rather than overwrite hand-written entries with an edited default.
   */
  const waiting = $derived(
    !!asked && current === undefined && (control.kind === 'list' || control.kind === 'json') && !vals.error && (!eff || (eff.state === 'value' && eff.pending)),
  )
  const value = $derived(control.kind === 'json' ? (problem ? undefined : JSON.parse(jsonText)) : draft)
  const changed = $derived(touched && JSON.stringify(value) !== JSON.stringify(current))

  // An option that takes a file path is offered the machine's sops-nix secrets, at the path the machine finds them.
  const secrets = $derived(pathReady && !inList && takesSecretFile(path, option.type) ? (hostByName(host)?.secrets ?? []) : [])
  /** A secret picked for a user's password hash: sops-nix must decrypt it before users are made. */
  let forUsersSecret = $state('')
  function pickSecret(name: string) {
    const s = secrets.find((x) => x.name === name)
    if (!s) return
    const users = forUsers(path)
    edit(users ? secretPath(name, true) : s.path)
    forUsersSecret = users ? name : ''
  }

  async function save() {
    busy = true
    const n = forUsersSecret
    const needed = n && value === secretPath(n, true) && secrets.find((s) => s.name === n)?.path !== value
    if (!needed || (await setOption(host, ['sops', 'secrets', n, 'neededForUsers'], true, `Set ${n} to be decrypted before users are made on ${host}`))) {
      if (await setOption(host, path, value)) {
        touched = false
        forUsersSecret = ''
      }
    }
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

  // Copy one machine's value to others (each written whole to its managed.json).
  let copying = $state<{ from: string; value: unknown } | null>(null)
  const targets = new SvelteSet<string>()
  let copyBusy = $state(false)
  const copyTargets = $derived(copying ? rows.filter((r) => r.editable && r.host !== copying!.from && r.eff?.state !== 'missing') : [])
  function startCopy(from: string, value: unknown) {
    copying = { from, value }
    targets.clear()
    for (const r of copyTargets) if (!(r.eff?.state === 'value' && sameValue(r.eff.value, value))) targets.add(r.host)
  }
  async function copy() {
    if (!copying) return
    copyBusy = true
    const done: string[] = []
    // One toast for the batch instead of one per machine.
    await quietly(async () => {
      for (const t of [...targets]) {
        if (!(await setOption(t, path, copying!.value))) break
        done.push(t)
      }
    })
    if (done.length) toast(`Set ${pathName(path)} to ${showValue(copying.value)} on ${done.join(', ')}`, 'ok', undefined, { label: 'View changes', run: () => go('#/changes') })
    copyBusy = false
    copying = null
  }
</script>

<div class="of" use:frame={{ title: option.name }}>
  <div class="name-row">
    <h3 class="mono sr-only">{option.name}</h3>
  </div>
  <div class="type mono">{option.type}</div>

  {#if option.description}<div class="docs">{@html docsHTML(option.description)}</div>{/if}

  <dl class="defaults">
    {#if option.default != null}<dt>Default</dt><dd><pre>{compact(option.default)}</pre></dd>{/if}
    {#if option.example != null}<dt>Example</dt><dd><pre>{compact(option.example)}</pre></dd>{/if}
  </dl>

  <div class="editor" class:locked={!editable}>
    {#if option.readOnly}
      <p class="note"><Icon name="info" /> Read-only: NixOS computes this option; it cannot be set.</p>
    {:else if inList}
      <p class="note"><Icon name="info" /> This option sits inside a list of submodules, which a settings path cannot address. Set it in Code.</p>
    {:else if !managed}
      <p class="note"><Icon name="info" /> {host} is Code only: change options in its Nix files, or <a href={machineLink(host)}>turn on editing</a> to set them here.</p>
    {:else if !fleet.managed[host]}
      <span class="skel" style:width="40ch"></span>
    {/if}

    {#if placeholders.length && !inList}
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

    {#if asked}
      <div class="now" aria-live="polite">
        <span class="label">On {host}</span>
        {#if vals.error}
          <span class="error-text small">{vals.error}</span>
        {:else if !eff}
          <span class="skel" style:width="20ch"></span>{#if vals.job}<span class="muted small">Evaluating {host}…</span>{/if}
        {:else if eff.state === 'missing'}
          <span class="muted">{placeholders.length ? `Nothing is set for ${names.filter(Boolean).map((n) => `“${n}”`).join(', ')} on ${host} yet.` : `${host} has no such option: none of its modules declares it.`}</span>
        {:else if eff.state === 'error'}
          <span class="error-text small">Could not evaluate it: {eff.error.split('\n')[0]}</span>
        {:else}
          <code class="val" title={showValue(eff.value)}>{showValue(eff.value)}</code>
          <SourceTag source={eff.source} pending={eff.pending} />
        {/if}
      </div>
      {#if eff?.state === 'value' && !eff.pending && !option.readOnly && !inList}
        <p class="why">
          {#if eff.source.kind === 'here'}
            Set here, so it is {host}'s final value: a value set here wins over hand-written Nix (only <span class="mono">lib.mkForce</span> beats it).
          {:else if eff.source.kind === 'code' && editable}
            Written by hand. Saving a value here overrides it: values set here win over hand-written Nix.
          {:else if eff.source.kind === 'code'}
            Change it in <span class="mono">{eff.source.file}</span>.
          {:else}
            Nothing in the repository sets it, so NixOS's default applies.
          {/if}
        </p>
      {/if}
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
          <input class="mono" aria-label="Value of {option.name}" value={asText(draft)} spellcheck="false" oninput={(e) => ((forUsersSecret = ''), edit(e.currentTarget.value))} />
          {#if secrets.length}
            <select class="short" aria-label="Use a secret for {option.name}" value="" onchange={(e) => pickSecret(e.currentTarget.value)}>
              <option value="">Use a secret…</option>
              {#each secrets as s (s.name)}<option value={s.name}>{s.name}</option>{/each}
            </select>
          {/if}
        {:else if c.kind === 'select'}
          <select class="short" aria-label="Value of {option.name}" value={asText(draft)} onchange={(e) => edit(c.choices.find((x) => String(x) === e.currentTarget.value))}>
            {#if draft === undefined || draft === null}<option value="">Choose…</option>{/if}
            {#each c.choices as ch (ch)}<option value={String(ch)}>{ch}</option>{/each}
          </select>
        {:else if c.kind === 'list'}
          {@const items = Array.isArray(draft) ? draft : []}
          <div class="chips list">
            {#each items as item, i (i)}
              <span class="chip mono">{String(item)}<button class="x bare" aria-label="Remove {item}" onclick={() => edit(items.filter((_, j) => j !== i))}><Icon name="x" /></button></span>
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
        <button class="primary" disabled={!changed || !!problem || busy || waiting} onclick={save}>{busy ? 'Saving…' : 'Save'}</button>
        {#if waiting}<span class="muted small wait">{eff ? `Reset here: wait for ${host}'s next evaluation` : 'Reading the value that applies now'}; this value is saved whole, starting from it.</span>{/if}
        {#if current !== undefined}
          <button disabled={busy} onclick={reset} title="Remove it from {fleet.managed[host]?.path}; the default or the hand-written value applies again">Reset</button>
        {/if}
        {#if touched}<button class="ghost" onclick={() => (touched = false)}>Undo edits</button>{/if}
      </div>
    {/if}
  </div>

  {#if asked && machines.length > 1}
    <section class="compare" aria-label="Value on every machine">
      <div class="c-head">
        <h4>On every machine</h4>
        {#if vals.values}<span class="muted small">{distinct <= 1 ? 'The same everywhere it is set' : `${plural(distinct, 'different value')}`}</span>{/if}
      </div>
      <table class="data">
        <colgroup><col style:width="24%" /><col style:width="34%" /><col /><col style:width="16ch" /></colgroup>
        <thead><tr><th>Machine</th><th>Value</th><th>From</th><th><span class="sr-only">Actions</span></th></tr></thead>
        <tbody>
          {#each rows as r (r.host)}
            {@const e = r.eff}
            <tr class:me={r.host === host}>
              <td class="m"><a href={machineLink(r.host)}>{r.host}</a>{#if !r.editable}<span class="co" title="Not editable here: it does not load a managed.json">Code only</span>{/if}</td>
              <td class="v">
                {#if vals.error}<span class="muted small">—</span>
                {:else if !e}<span class="skel" style:width="12ch"></span>
                {:else if e.state === 'missing'}<span class="muted small">{placeholders.length ? 'Not set' : 'No such option'}</span>
                {:else if e.state === 'error'}<span class="error-text small">Does not evaluate</span>
                {:else}<code title={showValue(e.value)}>{showValue(e.value)}</code>{/if}
              </td>
              <td>{#if e?.state === 'value'}<SourceTag source={e.source} pending={e.pending} />{/if}</td>
              <td class="act">
                {#if e?.state === 'value' && !e.pending && copyable(e.value) && rows.some((x) => x.editable && x.host !== r.host && x.eff?.state !== 'missing')}
                  <button class="small ghost" aria-expanded={copying?.from === r.host} onclick={() => (copying?.from === r.host ? (copying = null) : startCopy(r.host, e.value))}><Icon name="copy" /> Copy to…</button>
                {/if}
              </td>
            </tr>
            {#if copying?.from === r.host}
              <tr class="copy-row">
                <td colspan="4">
                  <div class="copy" use:frame={'dash'}>
                    <p>Set <code>{showValue(copying.value)}</code> from {r.host} on:</p>
                    <div class="chips">
                      {#each copyTargets as t (t.host)}
                        <label class="target" class:on={targets.has(t.host)}>
                          <input type="checkbox" checked={targets.has(t.host)} onchange={() => (targets.has(t.host) ? targets.delete(t.host) : targets.add(t.host))} />
                          {t.host}
                          {#if t.eff?.state === 'value' && sameValue(t.eff.value, copying.value)}<span class="muted">already</span>{/if}
                        </label>
                      {/each}
                    </div>
                    <p class="muted small">Each machine gets it in its managed.json, where it wins over hand-written Nix.</p>
                    <div class="buttons">
                      <button class="primary small" disabled={!targets.size || copyBusy} onclick={copy}>{copyBusy ? 'Copying…' : targets.size ? `Copy to ${plural(targets.size, 'machine')}` : 'Pick machines'}</button>
                      <button class="small ghost" onclick={() => (copying = null)}>Cancel</button>
                    </div>
                  </div>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </section>
  {/if}
</div>

<style>
  .of {
    display: grid;
    gap: var(--lh);
    min-width: 0;
  }
  .type {
    color: var(--accent);
    overflow-wrap: anywhere;
  }
  .docs {
    max-width: 80ch;
  }
  .docs > :global(* + *) {
    margin-top: var(--lh);
  }
  .docs :global(li)::before {
    color: var(--fg-muted);
    content: '• ';
  }
  .docs :global(code) {
    color: var(--fg-strong);
    background: var(--bg-hover);
  }
  .docs :global(pre) {
    padding: 0 1ch;
    overflow: auto;
    background: var(--bg-input);
  }
  .docs :global(pre code) {
    background: none;
  }
  .defaults {
    display: grid;
    grid-template-columns: 12ch minmax(0, 1fr);
  }
  .defaults dt {
    color: var(--fg-muted);
  }
  .defaults dd {
    min-width: 0;
  }
  /* Values are Nix literals, in the Code view's colour for them. */
  .defaults pre {
    overflow: auto;
    color: var(--yellow);
  }
  .editor {
    display: grid;
    gap: var(--lh);
  }
  .note {
    display: flex;
    gap: 1ch;
  }
  .note :global(.ic) {
    color: var(--accent);
  }
  .names {
    display: flex;
    flex-wrap: wrap;
    gap: var(--lh) 2ch;
  }
  .field {
    display: grid;
    width: 32ch;
  }
  /* "Value" heads the control it names, with no blank row between. */
  .editor > .label + .control {
    margin-top: calc(var(--lh) * -1);
  }
  .control {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--lh) 2ch;
  }
  .switch-row {
    display: flex;
    gap: 1ch;
  }
  .short {
    width: 26ch;
  }
  .seg > button + button::before {
    margin-right: 1ch;
    margin-left: -1ch;
    color: var(--line);
    content: '│';
  }
  .seg > button.on::before {
    color: var(--on-accent);
  }
  .list {
    width: 100%;
  }
  .add-item {
    display: flex;
    gap: 2ch;
  }
  .json {
    width: 100%;
    height: calc(var(--lh) * 6);
  }
  .problem {
    color: var(--red);
  }
  .problem::before {
    content: '✖ ';
  }
  .buttons {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .now {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
    min-width: 0;
  }
  .val {
    max-width: 100%;
    padding: 0 1ch;
    overflow: hidden;
    color: var(--yellow);
    white-space: nowrap;
    text-overflow: ellipsis;
    background: var(--bg-input);
  }
  .why {
    max-width: 80ch;
    color: var(--fg-muted);
  }
  /* Headings of the form read like the page's: ▌ ON EVERY MACHINE. */
  h4 {
    text-transform: uppercase;
  }
  h4::before {
    margin-right: 1ch;
    color: var(--accent);
    content: '▌';
  }
  .c-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
    margin-bottom: var(--lh);
  }
  .compare table {
    table-layout: fixed;
  }
  .compare td {
    overflow: hidden;
  }
  .m {
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .m a {
    font-weight: 700;
  }
  .co {
    margin-left: 1ch;
    color: var(--fg-faint);
  }
  .co::before {
    content: 'λ ';
  }
  tr.me {
    background: var(--bg-active);
  }
  .v code {
    display: block;
    overflow: hidden;
    color: var(--yellow);
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .compare .act {
    text-align: right;
    white-space: nowrap;
  }
  .compare tbody .copy-row:hover {
    background: none;
  }
  .copy {
    display: grid;
    gap: var(--lh);
    margin-bottom: var(--lh);
  }
  .copy code {
    color: var(--yellow);
  }
  .copy .chips {
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
</style>
