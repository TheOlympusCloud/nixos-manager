<script lang="ts">
  import { tick } from 'svelte'
  import type { Host } from '../../lib/api'
  import { essentialOptions, essentials, retentionDays, withRetention, type Essential } from '../../lib/essentials'
  import { fleet, managedValue, setOption, unsetOption, useValues, valueOf } from '../../lib/fleet.svelte'
  import { plural } from '../../lib/format'
  import { fillPath, optionSegments, pathName } from '../../lib/options'
  import { machineLink } from '../../lib/router.svelte'
  import { confirmBox } from '../../lib/ui.svelte'
  import { effective, type Effective } from '../../lib/values'
  import Icon from '../Icon.svelte'
  import SourceTag from '../SourceTag.svelte'
  import Switch from '../Switch.svelte'

  /** The settings most machines need, each with its effective value and source; editable by clicking. */
  let { host }: { host: Host } = $props()

  const keyNames = $derived(host.users.map((u) => pathName(['users', 'users', u.name, 'openssh', 'authorizedKeys', 'keys'])))
  // One request for the whole page (the users' keys too, to warn before locking everyone out).
  const vals = useValues(() => ({ names: [...essentialOptions, ...keyNames].slice(0, 40), hosts: [host.name] }))
  const editable = $derived(!!host.managedFile && !!fleet.managed[host.name])
  const pathOf = (option: string) => fillPath(optionSegments(option), [])
  const effOf = (option: string): Effective | null =>
    effective(valueOf(vals.values, option, host.name), managedValue(host.name, pathOf(option)), host.managedFile, !!fleet.managed[host.name])
  const valueOfOpt = (option: string) => {
    const e = effOf(option)
    return e?.state === 'value' ? e.value : undefined
  }
  const label = (option: string) => essentials.flatMap((g) => g.items).find((i) => i.option === option)?.label ?? option
  const keyCount = $derived(keyNames.reduce((n, k) => n + (Array.isArray(valueOfOpt(k)) ? (valueOfOpt(k) as unknown[]).length : 0), 0))

  let busy = $state('')
  let editing = $state('')
  let text = $state('')
  let inputEl = $state<HTMLInputElement>()

  async function write(item: Essential, value: unknown) {
    busy = item.option
    const ok = await setOption(host.name, pathOf(item.option), value, `${item.label} on ${host.name}: ${shown(item, value)}`)
    busy = ''
    if (ok) editing = ''
  }
  async function reset(item: Essential) {
    busy = item.option
    await unsetOption(host.name, pathOf(item.option), `${item.label} on ${host.name} is back to its hand-written or default value`)
    busy = ''
  }

  /** Turning these off can lock you out or open the machine up: ask first. */
  async function guard(item: Essential, on: boolean): Promise<boolean> {
    if (item.option === 'services.openssh.enable' && !on)
      return confirmBox({ title: `Turn off SSH on ${host.name}?`, message: `If you reach ${host.name} over the network, you cannot log in remotely once this is deployed.`, ok: 'Turn off SSH', danger: true })
    if (item.option === 'services.openssh.settings.PasswordAuthentication' && !on && vals.values && keyCount === 0)
      return confirmBox({
        title: 'Nobody has an SSH key',
        message: `No user on ${host.name} has an authorized SSH key. With password logins off, nobody can log in over SSH. Add a key in Users first.`,
        ok: 'Turn off anyway',
        danger: true,
      })
    if (item.option === 'networking.firewall.enable' && !on)
      return confirmBox({ title: `Turn off the firewall on ${host.name}?`, message: 'Every port that a program listens on becomes reachable from the network.', ok: 'Turn off the firewall', danger: true })
    return true
  }
  async function toggle(item: Essential, on: boolean) {
    if (await guard(item, on)) await write(item, on)
  }

  async function startEdit(item: Essential, e: Effective | null) {
    const v = e?.state === 'value' ? e.value : undefined
    editing = item.option
    text = item.kind === 'retention' ? String(retentionDays(v) ?? 14) : typeof v === 'string' ? v : ''
    await tick()
    inputEl?.focus()
    inputEl?.select()
  }
  const problem = (item: Essential) => {
    if (item.kind === 'retention') {
      const n = Number(text)
      return Number.isInteger(n) && n >= 1 && n <= 3650 ? '' : 'Whole days, from 1 to 3650.'
    }
    return item.check?.(text) ?? ''
  }
  function submit(e: SubmitEvent, item: Essential) {
    e.preventDefault()
    if (problem(item)) return
    write(item, item.kind === 'retention' ? withRetention(valueOfOpt(item.option), Number(text)) : text.trim())
  }

  /** The value in words. */
  function shown(item: Essential, v: unknown): string {
    if (item.kind === 'switch') return v ? 'On' : 'Off'
    if (item.kind === 'tristate') return v === false ? 'Off' : v === true ? 'On' : 'On (NixOS default)'
    if (item.kind === 'retention') {
      const d = retentionDays(v)
      return d !== null ? plural(d, 'day') : typeof v === 'string' && v.trim() ? v : 'No age limit'
    }
    return v === null || v === undefined || v === '' ? 'Not set' : String(v)
  }
  /** Values worth a second look: password logins allowed, root login "yes", the firewall off. */
  const risky = (item: Essential, v: unknown) => item.danger !== undefined && (item.kind === 'tristate' ? v !== false : v === item.danger)
</script>

{#if !host.managedFile}
  <p class="banner"><Icon name="info" size={14} />{host.name} is Code only: these are its current settings. <a href={machineLink(host.name)}>Turn on editing</a> to change them by clicking.</p>
{/if}

{#if vals.error}
  <div class="card failed" role="alert">
    <p><strong>Could not read the settings of {host.name}.</strong></p>
    <pre>{vals.error}</pre>
  </div>
{/if}

{#if !vals.values && !vals.error}
  <p class="computing" role="status"><span class="dot busy"></span>Reading {plural(essentialOptions.length, 'setting')} from {host.name}'s evaluation…</p>
{/if}

<div class="groups">
  {#each essentials as g (g.title)}
    <section class="group">
      <header>
        <h2>{g.title}</h2>
        <p class="muted small">{g.note}</p>
      </header>
      <ul class="card rows">
        {#each g.items as item (item.option)}
          {@const e = effOf(item.option)}
          {@const v = e?.state === 'value' ? e.value : undefined}
          {@const off = !!item.when && valueOfOpt(item.when) === false}
          <li class="row" class:off class:editing={editing === item.option}>
            <div class="what">
              <span class="name">{item.label}</span>
              <span class="help">{off ? `Applies only when ${label(item.when!).toLowerCase()} is on.` : item.help}</span>
              <span class="opt mono">{item.option}</span>
            </div>
            <div class="how">
              {#if !e}
                <span class="skel" style:width="90px"></span>
              {:else if e.state === 'missing'}
                <span class="muted small">Not available on {host.name}</span>
              {:else if e.state === 'error'}
                <span class="error-text small" title={e.error}>Does not evaluate</span>
              {:else}
                <SourceTag source={e.source} pending={e.pending} />
                {#if editable && e.source.kind === 'here' && !e.pending}
                  <button class="icon" aria-label="Reset {item.label}" title="Remove it from {host.managedFile}: the hand-written or default value applies again" disabled={busy === item.option} onclick={() => reset(item)}
                    ><Icon name="undo" size={13} /></button>
                {/if}
                {#if editing === item.option}
                  <form class="edit" onsubmit={(ev) => submit(ev, item)}>
                    {#if item.kind === 'retention'}
                      <input class="days num" type="number" min="1" max="3650" aria-label="{item.label} (days)" bind:value={text} bind:this={inputEl} />
                      <span class="muted small">days</span>
                    {:else}
                      <input class="mono" list="list-{item.option}" aria-label={item.label} spellcheck="false" autocomplete="off" bind:value={text} bind:this={inputEl} aria-invalid={!!problem(item)} />
                      {#if item.suggest}<datalist id="list-{item.option}">{#each item.suggest() as s (s)}<option value={s}></option>{/each}</datalist>{/if}
                    {/if}
                    <button class="primary small" type="submit" disabled={!!problem(item) || busy === item.option}>{busy === item.option ? 'Saving…' : 'Save'}</button>
                    <button class="small ghost" type="button" onclick={() => (editing = '')}>Cancel</button>
                  </form>
                {:else if item.kind === 'switch' || item.kind === 'tristate'}
                  <span class="state" class:risky={risky(item, v)}>{shown(item, v)}</span>
                  {#if editable}<Switch checked={item.kind === 'tristate' ? v !== false : v === true} busy={busy === item.option} label="{item.label} on {host.name}" onchange={(on) => toggle(item, on)} />{/if}
                {:else if item.kind === 'select' && editable}
                  <select
                    class="short"
                    class:risky={risky(item, v)}
                    aria-label={item.label}
                    value={typeof v === 'string' ? v : ''}
                    disabled={busy === item.option}
                    onchange={(ev) => write(item, ev.currentTarget.value)}>
                    {#if typeof v !== 'string' || !item.choices?.includes(v)}<option value="">{shown(item, v)}</option>{/if}
                    {#each item.choices ?? [] as c (c)}<option value={c}>{c}</option>{/each}
                  </select>
                {:else if editable}
                  <button class="value" onclick={() => startEdit(item, e)} title="Change {item.label.toLowerCase()}">{shown(item, v)}<Icon name="edit" size={12} /></button>
                {:else}
                  <span class="state" class:risky={risky(item, v)}>{shown(item, v)}</span>
                {/if}
              {/if}
            </div>
            {#if editing === item.option && problem(item)}<p class="problem" role="alert">{problem(item)}</p>{/if}
          </li>
        {/each}
      </ul>
    </section>
  {/each}
</div>

<style>
  .banner {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: var(--s4);
    padding: 10px 14px;
    color: var(--fg);
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .banner :global(svg) {
    color: var(--accent);
  }
  .computing {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: var(--s3);
    color: var(--fg-muted);
  }
  .failed {
    display: grid;
    gap: 8px;
    margin-bottom: var(--s4);
    padding: 12px 14px;
    border-color: color-mix(in srgb, var(--red) 30%, var(--border));
  }
  .failed pre {
    margin: 0;
    color: var(--fg-muted);
    font: 12px/1.5 var(--mono);
    white-space: pre-wrap;
  }
  .groups {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(520px, 1fr));
    gap: var(--s5) var(--s4);
    align-items: start;
  }
  .group header {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 8px;
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .rows {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 4px 16px;
    align-items: center;
    min-height: 58px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
  }
  .row:last-child {
    border-bottom: 0;
  }
  /* Editing: the field gets the full width under the label. */
  .row.editing {
    grid-template-columns: minmax(0, 1fr);
  }
  .row.editing .how {
    flex-direction: row-reverse;
    justify-content: flex-end;
  }
  .row.editing .problem {
    text-align: left;
  }
  .what {
    display: grid;
    gap: 1px;
    min-width: 0;
  }
  .name {
    color: var(--fg-strong);
    font-weight: 500;
  }
  .help {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.45;
  }
  .opt {
    overflow: hidden;
    color: var(--fg-faint);
    font-size: 10.5px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .row.off .name,
  .row.off .how {
    opacity: 0.55;
  }
  .how {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    min-width: 0;
  }
  .how > :global(.src) {
    max-width: 220px;
  }
  .state {
    min-width: 28px;
    color: var(--fg-strong);
    text-align: right;
  }
  .risky {
    color: var(--yellow);
  }
  select.risky {
    color: var(--yellow);
  }
  .value {
    max-width: 260px;
    overflow: hidden;
    color: var(--fg-strong);
    font-weight: 400;
    text-overflow: ellipsis;
  }
  .value :global(svg) {
    color: var(--fg-muted);
  }
  .short {
    width: 190px;
  }
  .edit {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .edit input {
    width: 210px;
    height: 26px;
  }
  .edit input.days {
    width: 80px;
  }
  .problem {
    grid-column: 1 / -1;
    color: var(--red);
    font-size: var(--fs-sm);
    text-align: right;
  }
  @media (max-width: 1180px) {
    .groups {
      grid-template-columns: 1fr;
    }
  }
</style>
