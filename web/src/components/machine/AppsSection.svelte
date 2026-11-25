<script lang="ts">
  import type { Host, Managed } from '../../lib/api'
  import { CONFIG_FORMATS, KNOWN_APPS, appsIn, attributesText, configPathProblem, configTree, countKeys, keyText, kindOf, parseAttributes, parseKey, parseValue, type AppFile, type ConfigFormat, type ConfigNode, type HomeConfig, type KeyringEntry, type ValueKind } from '../../lib/apps'
  import { frame } from '../../lib/ascii'
  import { removeHomeConfig, removeKeyringEntry, setConfigKey, setHomeConfig, setKeyringEntry } from '../../lib/fleet.svelte'
  import { managesApps } from '../../lib/packages'
  import { confirmBox } from '../../lib/ui.svelte'
  import Icon from '../Icon.svelte'
  import Switch from '../Switch.svelte'
  import SecretPick from './SecretPick.svelte'

  /**
   * The Home tab's Apps: settings files the apps also write (managed.json's home.<user>.configs),
   * keyring entries, and what known apps keep to themselves. Each edit is one managed.json change.
   */
  let { host, user, managed, packages, taken, editable }: { host: Host; user: string; managed: Managed | undefined; packages: string[]; taken: string[]; editable: boolean } = $props()

  const mine = $derived(managed?.home?.[user])
  const configs = $derived(Object.entries(mine?.configs ?? {}).sort(([a], [b]) => a.localeCompare(b)))
  const keyring = $derived(mine?.keyring ?? [])
  const can = $derived(editable && managesApps(host))
  const sops = $derived(!!host.secrets)
  const known = $derived(appsIn(packages, configs.map(([p]) => p)))
  const appOf = (path: string) => KNOWN_APPS.find((a) => a.files.some((f) => f.path === path))
  const fileOf = (path: string) => KNOWN_APPS.flatMap((a) => a.files).find((f) => f.path === path)

  let busy = $state('')
  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }

  // ---- adding a file ----
  let path = $state('')
  let format = $state<ConfigFormat>('json')
  let mode = $state<'merge' | 'seed'>('merge')
  let process = $state('')
  let lock = $state('')
  const pathProblem = $derived(configPathProblem(path, [...taken, ...configs.map(([p]) => p)]))
  const suggestions = $derived(known.flatMap((a) => a.files.map((f) => ({ app: a.name, f }))).filter(({ f }) => !configs.some(([p]) => p === f.path)))
  function suggest(f: AppFile) {
    path = f.path
    format = f.format
    mode = 'merge'
    process = f.process ?? ''
    lock = f.lock ?? ''
  }
  async function addFile(e: SubmitEvent) {
    e.preventDefault()
    const p = path.trim()
    if (!p || pathProblem) return
    if (await setHomeConfig(host.name, user, p, { format, mode, process: process.trim() || undefined, lock: lock.trim() || undefined })) path = process = lock = ''
  }

  // ---- how a file is written ----
  let changing = $state('')
  let how = $state<{ format: ConfigFormat; mode: 'merge' | 'seed'; process: string; lock: string }>({ format: 'json', mode: 'merge', process: '', lock: '' })
  function startChange(p: string, c: HomeConfig) {
    changing = p
    how = { format: c.format, mode: c.mode ?? 'merge', process: c.process ?? '', lock: c.lock ?? '' }
  }
  async function saveHow(p: string) {
    const ok = await setHomeConfig(host.name, user, p, { format: how.format, mode: how.mode, process: how.process.trim() || undefined, lock: how.lock.trim() || undefined })
    if (ok) changing = ''
  }
  async function removeFile(p: string) {
    const ok = await confirmBox({
      title: `Stop writing ~/${p}?`,
      message: `Its ${countKeys(configTree(mine?.configs?.[p]?.settings, mine?.configs?.[p]?.secrets))} keys are taken out of ${host.managedFile}. The file stays on ${host.name} as it is, with the values it has, until the app changes them.`,
      ok: 'Stop writing it',
    })
    if (ok) await run('file:' + p, () => removeHomeConfig(host.name, user, p))
  }

  // ---- keys ----
  type NewKind = ValueKind | 'secret'
  let adding = $state<Record<string, { key: string; kind: NewKind; text: string; secret: string }>>({})
  $effect(() => {
    for (const [p] of configs) if (!adding[p]) adding[p] = { key: '', kind: 'text', text: '', secret: '' }
  })
  function keyProblem(p: string, c: HomeConfig): string {
    const d = adding[p]
    if (!d?.key.trim()) return ''
    const k = parseKey(d.key, c.format)
    if (typeof k === 'string') return k
    if (d.kind === 'secret') return d.secret ? '' : 'Pick a secret.'
    const v = parseValue(d.kind, d.text)
    return 'problem' in v ? v.problem : ''
  }
  async function addKey(e: SubmitEvent, p: string, c: HomeConfig) {
    e.preventDefault()
    const d = adding[p]
    if (!d) return
    const k = parseKey(d.key, c.format)
    if (typeof k === 'string' || keyProblem(p, c)) return
    const v = d.kind === 'secret' ? { secret: d.secret } : (parseValue(d.kind, d.text) as { value: unknown })
    if (await setConfigKey(host.name, user, p, k, v)) adding[p] = { key: '', kind: d.kind, text: '', secret: '' }
  }
  const save = (p: string, n: ConfigNode, to: { value: unknown } | { secret: string } | { unset: true }) => run(p + '\0' + n.path.join('\0'), () => setConfigKey(host.name, user, p, n.path, to))
  function saveText(p: string, n: ConfigNode, kind: ValueKind, text: string) {
    const v = parseValue(kind, text)
    if ('value' in v && JSON.stringify(v.value) !== JSON.stringify(n.value)) save(p, n, v)
  }

  // ---- keyring ----
  let editing = $state<KeyringEntry | null>(null)
  let label = $state('')
  let attrs = $state('')
  let entrySecret = $state('')
  const parsedAttrs = $derived(parseAttributes(attrs))
  const entryProblem = $derived.by(() => {
    if (!label.trim() && !attrs.trim()) return ''
    if (!label.trim()) return 'Give it a label.'
    if (typeof parsedAttrs === 'string') return parsedAttrs
    if (/<[^>]+>/.test(label + attrs)) return 'Replace each <…> with the real value, such as the server ID from Supersonic’s config.toml.'
    if (keyring.some((e) => e !== editing && attributesText(e.attributes) === attributesText(parsedAttrs))) return 'Another entry has the same attributes.'
    return entrySecret ? '' : 'Pick the secret it stores.'
  })
  function editEntry(e: KeyringEntry | null, from?: { label: string; attributes: Record<string, string> }) {
    editing = e
    label = e?.label ?? from?.label ?? ''
    attrs = attributesText(e?.attributes ?? from?.attributes ?? {})
    entrySecret = e?.secret ?? ''
  }
  async function saveEntry(ev: SubmitEvent) {
    ev.preventDefault()
    if (entryProblem || typeof parsedAttrs === 'string') return
    const was = editing?.attributes
    if (await setKeyringEntry(host.name, user, { label: label.trim(), attributes: parsedAttrs, secret: entrySecret }, was)) editEntry(null)
  }
  const hints = $derived(known.filter((a) => a.keyring))
</script>

{#snippet tree(nodes: ConfigNode[], p: string, c: HomeConfig)}
  <ul class="tree">
    {#each nodes as n (n.key)}
      <li>
        {#if n.children}
          <span class="k mono">{n.key}</span>
          {@render tree(n.children, p, c)}
        {:else}
          {@const id = p + '\0' + n.path.join('\0')}
          <div class="leaf">
            <span class="k mono">{n.key}</span>
            <span class="v">
              {#if n.secret !== undefined}
                {#if can}
                  <SecretPick {host} value={n.secret} label="Secret for {keyText(n.path, c.format)}" onpick={(s) => s !== n.secret && save(p, n, { secret: s })} />
                {:else}
                  <span class="mono">{n.secret}</span>
                {/if}
                <span class="chip" title="Read on {host.name} from /run/secrets/{n.secret}; never in the Nix store">⚿ secret</span>
                {#if sops && !(host.secrets ?? []).some((s) => s.name === n.secret)}<span class="chip warn" title="Its check fails until {host.name} gets it, in Settings → Secrets">✖ Not one of {host.name}'s secrets</span>{/if}
              {:else if kindOf(n.value) === 'boolean'}
                <Switch checked={n.value === true} label={keyText(n.path, c.format)} disabled={!can || busy === id} onchange={(on) => save(p, n, { value: on })} />
              {:else}
                {@const kind = kindOf(n.value)}
                <input
                  class="mono"
                  class:short={kind === 'number'}
                  type={kind === 'number' ? 'number' : 'text'}
                  aria-label={keyText(n.path, c.format)}
                  value={kind === 'list' ? JSON.stringify(n.value) : String(n.value ?? '')}
                  disabled={!can || busy === id}
                  spellcheck="false"
                  onchange={(e) => saveText(p, n, kind, e.currentTarget.value)} />
                {#if kind === 'list'}<span class="muted small">a list, replaced whole</span>{/if}
              {/if}
            </span>
            {#if can}<button class="icon" aria-label="Stop setting {keyText(n.path, c.format)}" title="Stop setting it (the file keeps its value)" disabled={busy === id} onclick={() => save(p, n, { unset: true })}><Icon name="trash" /></button>{/if}
          </div>
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

<section class="block" aria-labelledby="ha-title">
  <div class="head">
    <h3 id="ha-title">Apps <span class="num">{configs.length + keyring.length}</span></h3>
  </div>
  <p class="muted small">
    Settings of apps that also write their own files, where a linked file would break them. At every switch and login, the keys set here are written into {user}'s file and the app's other keys stay;
    a key changed in the app comes back to the value set here. A value is copied to the Nix store, which every user of {host.name} can read: put passwords and tokens in a secret, read on the machine.
  </p>
  {#if editable && !managesApps(host)}
    <p class="muted small">This needs a newer <span class="mono">modules/managed.nix</span>: <b>Update managed.nix</b> above.</p>
  {/if}

  {#each configs as [p, c] (p)}
    {@const nodes = configTree(c.settings, c.secrets)}
    {@const app = appOf(p)}
    <article class="card cfg" use:frame={{ title: '~/' + p }} aria-label="~/{p}">
      <div class="meta">
        {#if app}<b>{app.name}</b>{/if}
        <span class="chip">{c.format}</span>
        <span class="chip" title={c.mode === 'seed' ? 'Written whole only when the file is missing' : 'The keys below are written at every switch and login; the app keeps its others'}>{c.mode === 'seed' ? 'when missing' : 'merged'}</span>
        {#if c.process}<span class="small muted">waits while <span class="mono">{c.process}</span> runs</span>{/if}
        {#if c.lock}<span class="small muted">takes <span class="mono">~/{c.lock}</span></span>{/if}
        <span class="spacer"></span>
        {#if can}
          <button class="link small" onclick={() => (changing === p ? (changing = '') : startChange(p, c))} aria-expanded={changing === p}>How it is written</button>
          <button class="icon" aria-label="Stop writing ~/{p}" title="Stop writing it (the file stays)" disabled={busy === 'file:' + p} onclick={() => removeFile(p)}><Icon name="trash" /></button>
        {/if}
      </div>
      {#if changing === p}
        <form class="how" onsubmit={(e) => (e.preventDefault(), saveHow(p))}>
          <label class="field"><span class="label">Format</span><select bind:value={how.format}>{#each CONFIG_FORMATS as f (f)}<option value={f}>{f}</option>{/each}</select></label>
          <label class="field"><span class="label">Mode</span><select bind:value={how.mode}><option value="merge">Merge the keys</option><option value="seed">Whole file, when missing</option></select></label>
          <label class="field"><span class="label">Wait while running</span><input class="mono" bind:value={how.process} placeholder="text in its command line" spellcheck="false" /></label>
          <label class="field"><span class="label">Its lock, in ~</span><input class="mono" bind:value={how.lock} placeholder=".claude/settings.json.lock" spellcheck="false" /></label>
          <button type="submit" class="small primary">Save</button>
        </form>
      {/if}
      {#if nodes.length}
        {@render tree(nodes, p, c)}
      {:else}
        <p class="muted small">No keys set yet{fileOf(p) ? `: ${fileOf(p)?.note}` : ''}.</p>
      {/if}
      {#if can && adding[p]}
        {@const problem = keyProblem(p, c)}
        <form class="add" onsubmit={(e) => addKey(e, p, c)}>
          <label class="field">
            <span class="label">Key</span>
            <input class="mono" bind:value={adding[p].key} placeholder={c.format === 'ini' ? 'Section.key' : c.format === 'properties' ? 'some.key' : c.format === 'toml' ? 'Servers[ID=…].Hostname' : 'section.key'} spellcheck="false" aria-invalid={!!problem} />
          </label>
          <label class="field short-field">
            <span class="label">Type</span>
            <select bind:value={adding[p].kind}>
              <option value="text">Text</option>
              <option value="number">Number</option>
              <option value="boolean">Yes/no</option>
              <option value="list">List (JSON)</option>
              {#if sops}<option value="secret">Secret</option>{/if}
            </select>
          </label>
          {#if adding[p].kind === 'secret'}
            <SecretPick {host} value={adding[p].secret} label="Secret for the new key" onpick={(s) => (adding[p].secret = s)} />
          {:else if adding[p].kind === 'boolean'}
            <select class="short" aria-label="Value" bind:value={adding[p].text}><option value="false">No</option><option value="true">Yes</option></select>
          {:else}
            <label class="field"><span class="label">Value</span><input class="mono" bind:value={adding[p].text} placeholder={adding[p].kind === 'list' ? '["a", "b"]' : ''} spellcheck="false" /></label>
          {/if}
          <button type="submit" disabled={!adding[p].key.trim() || !!problem}><Icon name="plus" /> Set</button>
        </form>
        {#if problem}<p class="problem small">{problem}</p>{/if}
      {/if}
    </article>
  {/each}

  {#if can}
    <form class="add file" onsubmit={addFile}>
      <label class="field"><span class="label">App settings file, relative to ~</span><input class="mono" bind:value={path} placeholder=".config/app/settings.json" spellcheck="false" aria-invalid={!!pathProblem} /></label>
      <label class="field short-field"><span class="label">Format</span><select bind:value={format}>{#each CONFIG_FORMATS as f (f)}<option value={f}>{f}</option>{/each}</select></label>
      <label class="field short-field"><span class="label">Mode</span><select bind:value={mode}><option value="merge">Merge keys</option><option value="seed">When missing</option></select></label>
      <label class="field short-field"><span class="label">Wait while running</span><input class="mono" bind:value={process} placeholder="supersonic" spellcheck="false" /></label>
      <button type="submit" disabled={!path.trim() || !!pathProblem}><Icon name="plus" /> Add file</button>
    </form>
    {#if pathProblem}<p class="problem small">{pathProblem}</p>{/if}
    {#if suggestions.length}
      <div class="chips suggest">
        <span class="muted small">Known:</span>
        {#each suggestions as s (s.f.path)}<button class="chip" title={s.f.note} onclick={() => suggest(s.f)}>{s.app}: ~/{s.f.path}</button>{/each}
      </div>
    {/if}
  {/if}

  <h4>Keyring</h4>
  <p class="muted small">Passwords apps read from {user}'s keyring (GNOME Keyring), stored once the graphical session starts, in the login collection, only when missing or different. Never use it for a key an app made for itself (Brave Safe Storage): a new one breaks what it encrypts.</p>
  {#if keyring.length}
    <table class="data">
      <thead><tr><th>Label</th><th>Attributes</th><th>Secret</th><th><span class="sr-only">Actions</span></th></tr></thead>
      <tbody>
        {#each keyring as e (attributesText(e.attributes))}
          <tr>
            <td>{e.label}</td>
            <td class="mono small">{#each Object.entries(e.attributes) as [k, v] (k)}<span class="attr">{k}={v}</span>{/each}</td>
            <td class="mono">
              {e.secret}
              {#if sops && !(host.secrets ?? []).some((s) => s.name === e.secret)}<span class="chip warn" title="Its check fails until {host.name} gets it">✖ missing</span>{/if}
            </td>
            <td class="act">
              {#if can}
                <button class="icon" aria-label="Edit “{e.label}”" title="Edit" onclick={() => editEntry(e)}><Icon name="edit" /></button>
                <button class="icon" aria-label="Remove “{e.label}”" title="Remove (the item already in the keyring stays)" onclick={() => removeKeyringEntry(host.name, user, e)}><Icon name="trash" /></button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
  {#if can && sops}
    <form class="entry" onsubmit={saveEntry} aria-label={editing ? `Edit “${editing.label}”` : 'New keyring entry'}>
      <label class="field"><span class="label">Label</span><input bind:value={label} placeholder="Password for '…' on 'app'" /></label>
      <label class="field"><span class="label">Attributes, one key=value a line</span><textarea class="mono" rows="2" bind:value={attrs} placeholder={'service=supersonic\nusername=<server ID>'} spellcheck="false"></textarea></label>
      <div class="field"><span class="label">Secret</span><SecretPick {host} value={entrySecret} label="Secret the entry stores" onpick={(s) => (entrySecret = s)} /></div>
      <div class="buttons">
        <button type="submit" disabled={!label.trim() || !!entryProblem}><Icon name="key" /> {editing ? 'Save' : 'Add entry'}</button>
        {#if editing}<button type="button" class="ghost" onclick={() => editEntry(null)}>Cancel</button>{/if}
        {#each hints as a (a.name)}<button type="button" class="link small" title={a.keyring?.note} onclick={() => editEntry(null, a.keyring)}>{a.name}'s password</button>{/each}
      </div>
    </form>
    {#if entryProblem}<p class="problem small">{entryProblem}</p>{/if}
  {:else if can}
    <p class="muted small">{host.name} has no sops-nix: keyring entries need its secrets (Settings → Secrets).</p>
  {/if}

  {#if known.length}
    <h4>Kept in the app only</h4>
    <p class="muted small">What these apps keep to themselves: logins in their own databases, tokens that rotate, keys they made. Writing them from outside breaks the login or gets refused, so they are not managed here.</p>
    <dl class="inapp">
      {#each known as a (a.name)}
        <dt>{a.name}</dt>
        <dd>
          <ul>
            {#each a.inApp as item (item)}<li>{item}</li>{/each}
          </ul>
          {#if a.note}<p class="muted small">{a.note}</p>{/if}
        </dd>
      {/each}
    </dl>
  {/if}
</section>

<style>
  .block {
    margin-bottom: calc(var(--lh) * 2);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 2ch;
  }
  h3,
  h4 {
    margin: var(--lh) 0 0;
    font-size: inherit;
  }
  .cfg {
    margin: var(--lh) 0;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
    align-items: baseline;
  }
  .spacer {
    flex: 1;
  }
  .tree {
    margin: calc(var(--lh) / 2) 0 0;
    padding-left: 2ch;
    list-style: none;
    border-left: 1px solid var(--line);
  }
  .cfg > .tree {
    border-left: none;
    padding-left: 0;
  }
  .leaf {
    display: grid;
    grid-template-columns: minmax(12ch, max-content) 1fr auto;
    gap: 2ch;
    align-items: baseline;
  }
  .k {
    color: var(--fg-strong);
  }
  .v {
    display: flex;
    flex-wrap: wrap;
    gap: 1ch;
    align-items: baseline;
  }
  .add,
  .how,
  .entry,
  .buttons {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--lh) 2ch;
    margin-top: var(--lh);
  }
  .field {
    display: grid;
    min-width: 24ch;
  }
  .short-field {
    min-width: 12ch;
  }
  .add.file .field:first-child,
  .entry .field:first-child {
    width: 40ch;
  }
  .entry textarea {
    width: 36ch;
  }
  .suggest {
    margin-top: var(--lh);
  }
  .attr {
    margin-right: 1ch;
  }
  .act {
    width: 10ch;
    text-align: right;
  }
  .problem {
    color: var(--red);
  }
  .problem::before {
    content: '✖ ';
  }
  .inapp dt {
    font-weight: 700;
    margin-top: calc(var(--lh) / 2);
  }
  .inapp dd {
    margin: 0 0 0 2ch;
  }
  .inapp ul {
    margin: 0;
    padding-left: 2ch;
  }
</style>
