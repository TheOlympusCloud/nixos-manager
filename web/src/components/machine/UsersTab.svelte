<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import type { Host } from '../../lib/api'
  import { fleet, managedValue, setOption, unsetOption, useValues, valueOf } from '../../lib/fleet.svelte'
  import { initial, plural } from '../../lib/format'
  import { pathName } from '../../lib/options'
  import { machineLink } from '../../lib/router.svelte'
  import { confirmBox } from '../../lib/ui.svelte'
  import { commonGroups, editableList, keySummary, parseSSHKeys, userNameProblem } from '../../lib/users'
  import { effective, type Effective } from '../../lib/values'
  import { frame } from '../../lib/ascii'
  import Icon from '../Icon.svelte'
  import SourceTag from '../SourceTag.svelte'

  let { host }: { host: Host } = $props()

  const PW = 'services.openssh.settings.PasswordAuthentication'
  const userPath = (u: string, ...rest: string[]) => ['users', 'users', u, ...rest]
  const keysPath = (u: string) => userPath(u, 'openssh', 'authorizedKeys', 'keys')
  const groupsPath = (u: string) => userPath(u, 'extraGroups')

  const editable = $derived(!!host.managedFile && !!fleet.managed[host.name])
  /** Users created here that the evaluation does not show yet. */
  const pendingUsers = $derived.by(() => {
    const all = managedValue(host.name, ['users', 'users'])
    if (!all || typeof all !== 'object') return []
    return Object.entries(all as Record<string, Record<string, unknown>>)
      .filter(([n, u]) => u?.isNormalUser === true && !host.users.some((x) => x.name === n))
      .map(([n, u]) => ({ name: n, description: typeof u.description === 'string' ? u.description : '', groups: [] as string[], home: null }))
  })
  const users = $derived([...host.users, ...pendingUsers])
  const createdHere = (u: string) => managedValue(host.name, userPath(u, 'isNormalUser')) === true

  const vals = useValues(() => ({ names: [PW, ...host.users.flatMap((u) => [pathName(keysPath(u.name)), pathName(groupsPath(u.name))])].slice(0, 40), hosts: [host.name] }))
  const eff = (path: string[]): Effective | null =>
    effective(valueOf(vals.values, pathName(path), host.name), managedValue(host.name, path), host.managedFile, !!fleet.managed[host.name])
  const fresh = (u: string) => !host.users.some((x) => x.name === u)
  /** The lists to edit, null while unknown (still reading, the read failed, or past the 40-option cap). */
  const knownGroups = (u: string) => editableList(eff(groupsPath(u)), fresh(u))
  const knownKeys = (u: string) => editableList(eff(keysPath(u)), fresh(u))
  /** What to show: the known list, else the last evaluation's groups. */
  const groupsOf = (u: { name: string; groups: string[] }) => knownGroups(u.name) ?? u.groups
  const keysOf = (u: string) => knownKeys(u) ?? []
  const passwordsOff = $derived.by(() => {
    const e = eff(PW.split('.'))
    return e?.state === 'value' ? e.value === false : host.checks?.sshPasswordAuthentication === false
  })
  const wheelUsers = $derived(users.filter((u) => groupsOf(u).includes('wheel')).map((u) => u.name))

  let busy = $state('')
  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }

  // ---- groups ----
  let addingGroup = $state('')
  let groupText = $state('')
  async function addGroup(u: { name: string; groups: string[] }, g: string) {
    g = g.trim()
    const now = knownGroups(u.name)
    if (!g || !now || now.includes(g)) return
    await run(u.name + ':groups', () => setOption(host.name, groupsPath(u.name), [...now, g], `Added ${u.name} to ${g} on ${host.name}`))
    addingGroup = ''
    groupText = ''
  }
  async function removeGroup(u: { name: string; groups: string[] }, g: string) {
    const now = knownGroups(u.name)
    if (!now) return
    if (g === 'wheel' && wheelUsers.length === 1) {
      const ok = await confirmBox({
        title: `Remove ${u.name} from wheel?`,
        message: `${u.name} is the only administrator on ${host.name}. Without wheel, nobody can use sudo there once this is deployed.`,
        ok: 'Remove from wheel',
        danger: true,
      })
      if (!ok) return
    }
    await run(u.name + ':groups', () => setOption(host.name, groupsPath(u.name), now.filter((x) => x !== g), `Removed ${u.name} from ${g} on ${host.name}`))
  }

  // ---- SSH keys ----
  let addingKey = $state('')
  let keyText = $state('')
  const keyProblem = $derived.by(() => {
    if (!keyText.trim()) return ''
    const k = parseSSHKeys(keyText)
    if ('problem' in k) return k.problem
    const data = (x: string) => x.trim().split(/\s+/)[1]
    return addingKey && k.keys.some((n) => keysOf(addingKey).some((x) => data(x) === data(n))) ? 'This key is already there.' : ''
  })
  async function addKey(e: SubmitEvent, u: string) {
    e.preventDefault()
    const k = parseSSHKeys(keyText)
    const now = knownKeys(u)
    if ('problem' in k || keyProblem || !now) return
    const what = k.keys.length > 1 ? `${k.keys.length} keys` : keySummary(k.keys[0]).comment || 'a key'
    await run(u + ':keys', () => setOption(host.name, keysPath(u), [...now, ...k.keys], `Added ${what} to ${u} on ${host.name}`))
    addingKey = ''
    keyText = ''
  }
  async function removeKey(u: string, key: string) {
    const now = knownKeys(u)
    if (!now) return
    const rest = now.filter((x) => x !== key)
    if (!rest.length && passwordsOff) {
      const ok = await confirmBox({
        title: `Remove ${u}'s only key?`,
        message: `Password logins are off on ${host.name}, so ${u} cannot log in over SSH without a key.`,
        ok: 'Remove the key',
        danger: true,
      })
      if (!ok) return
    }
    await run(u + ':keys', () => setOption(host.name, keysPath(u), rest, `Removed ${keySummary(key).comment || 'a key'} from ${u} on ${host.name}`))
  }

  // ---- add and remove users ----
  let adding = $state(false)
  let newName = $state('')
  let newDesc = $state('')
  const newGroups = new SvelteSet<string>(['wheel'])
  let newKeys = $state('')
  const nameProblem = $derived(userNameProblem(newName.trim(), users.map((u) => u.name)))
  const keyLines = $derived(newKeys.split('\n').map((l) => l.trim()).filter(Boolean))
  const parsedNew = $derived(parseSSHKeys(newKeys))
  const keysProblem = $derived(keyLines.length && 'problem' in parsedNew ? parsedNew.problem : '')
  function openAdd() {
    adding = true
    newName = ''
    newDesc = ''
    newKeys = ''
    newGroups.clear()
    newGroups.add('wheel')
  }
  async function addUser(e: SubmitEvent) {
    e.preventDefault()
    const name = newName.trim()
    if (!name || nameProblem || keysProblem) return
    const keys = 'keys' in parsedNew ? parsedNew.keys : []
    const user: Record<string, unknown> = { isNormalUser: true, extraGroups: [...newGroups] }
    if (newDesc.trim()) user.description = newDesc.trim()
    if (keys.length) user.openssh = { authorizedKeys: { keys } }
    let ok = false
    await run('add', async () => (ok = await setOption(host.name, userPath(name), user, `Added user ${name} to ${host.name}`)))
    if (ok) adding = false
  }
  async function removeUser(u: string) {
    const lastAdmin = wheelUsers.length === 1 && wheelUsers[0] === u
    const ok = await confirmBox({
      title: `Remove ${u} from ${host.name}?`,
      message: lastAdmin
        ? `${u} is the only administrator (wheel) on ${host.name}: after this, nobody can use sudo there. The account goes with the next deploy; its home folder stays on the disk.`
        : `The account goes with the next deploy; its home folder stays on the disk.`,
      ok: 'Remove user',
      danger: true,
    })
    if (ok) await run(u + ':remove', () => unsetOption(host.name, userPath(u), `Removed user ${u} from ${host.name}`))
  }
  const toggle = (set: SvelteSet<string>, v: string) => (set.has(v) ? set.delete(v) : set.add(v))
</script>

<div class="toolbar">
  {#if editable}
    <p class="muted small intro">Users and their SSH keys. Changes are kept in <span class="mono">{host.managedFile}</span> and win over hand-written Nix; groups and keys are written as whole lists.</p>
    <span class="spacer"></span>
    {#if !adding}<button class="primary" onclick={openAdd}><Icon name="plus" /> Add user</button>{/if}
  {:else if host.managedFile}
    <span class="skel" style:width="60ch"></span>
  {:else}
    <p class="muted small intro">Users of {host.name}, as evaluated. {host.name} is Code only: change users in its Nix files, or <a href={machineLink(host.name)}>turn on editing</a>.</p>
  {/if}
</div>

{#if adding}
  <form class="card add-user" onsubmit={addUser} aria-labelledby="add-title" use:frame={{ kind: 'dash', title: `New user on ${host.name}`, animate: true }}>
    <h3 id="add-title" class="sr-only">New user on {host.name}</h3>
    <div class="fields">
      <label class="field">
        <span class="label">User name</span>
        <!-- svelte-ignore a11y_autofocus -->
        <input class="mono" bind:value={newName} placeholder="alice" spellcheck="false" autocomplete="off" autofocus aria-invalid={!!nameProblem} />
        {#if nameProblem}<span class="problem">{nameProblem}</span>{/if}
      </label>
      <label class="field">
        <span class="label">Full name <span class="faint">(optional)</span></span>
        <input bind:value={newDesc} placeholder="Alice Example" autocomplete="off" />
      </label>
    </div>
    <div class="field">
      <span class="label" id="groups-label">Groups</span>
      <div class="picks" role="group" aria-labelledby="groups-label">
        {#each commonGroups as g (g.name)}
          <label class="pick" class:on={newGroups.has(g.name)} title={g.note}>
            <input type="checkbox" checked={newGroups.has(g.name)} onchange={() => toggle(newGroups, g.name)} />{g.name}
          </label>
        {/each}
      </div>
      <span class="hint">{newGroups.has('wheel') ? 'wheel makes the user an administrator (sudo).' : 'Not in wheel: no sudo.'}</span>
    </div>
    <label class="field">
      <span class="label">Authorized SSH keys <span class="faint">(one per line)</span></span>
      <textarea class="mono" rows="3" bind:value={newKeys} placeholder="ssh-ed25519 AAAA… alice@laptop" spellcheck="false" aria-invalid={!!keysProblem}></textarea>
      {#if keysProblem}<span class="problem">{keysProblem}</span>{:else}<span class="hint">{keyLines.length ? plural(keyLines.length, 'key') : 'No key: the user logs in with a password set on the machine (passwd).'}{passwordsOff && !keyLines.length ? ' Password logins over SSH are off on this machine.' : ''}</span>{/if}
    </label>
    <div class="buttons">
      <button type="submit" class="primary" disabled={!newName.trim() || !!nameProblem || !!keysProblem || busy === 'add'}>{busy === 'add' ? 'Adding…' : 'Add user'}</button>
      <button type="button" class="ghost" onclick={() => (adding = false)}>Cancel</button>
    </div>
  </form>
{/if}

<div class="users">
  {#each users as u (u.name)}
    {@const pending = !host.users.some((x) => x.name === u.name)}
    {@const groups = groupsOf(u)}
    {@const gEff = eff(groupsPath(u.name))}
    {@const kEff = eff(keysPath(u.name))}
    {@const keys = keysOf(u.name)}
    {@const gKnown = knownGroups(u.name)}
    {@const kKnown = knownKeys(u.name)}
    <section
      class="card user"
      class:pending
      use:frame={{ kind: pending ? 'dash' : 'round', title: u.name, tag: pending ? 'Pending evaluation' : createdHere(u.name) ? 'Created here' : undefined }}>
      <div class="u-head">
        <span class="av">{initial(u.name)}</span>
        <div class="u-title">
          <h3 class="sr-only">{u.name}</h3>
          {#if u.description}<p class="muted small">{u.description}</p>{/if}
        </div>
        <span class="spacer"></span>
        <!-- Shown on the frame line. -->
        {#if pending}<span class="badge busy sr-only">Pending evaluation</span>{:else if createdHere(u.name)}<span class="badge sr-only">Created here</span>{/if}
      </div>

      <div class="row">
        <span class="label">Groups</span>
        <div class="cell">
          <div class="chips">
            {#each groups as g (g)}
              <span class="chip" class:admin={g === 'wheel'} title={commonGroups.find((c) => c.name === g)?.note}
                >{g}{#if editable && gKnown}<button class="x icon" aria-label="Remove {u.name} from {g}" disabled={busy === u.name + ':groups'} onclick={() => removeGroup(u, g)}><Icon name="x" /></button>{/if}</span>
            {:else}
              <span class="muted small">none</span>
            {/each}
            {#if editable && gKnown && addingGroup !== u.name}
              <button class="chip add-btn" onclick={() => ((addingGroup = u.name), (groupText = ''))} aria-label="Add a group to {u.name}"><Icon name="plus" /> Group</button>
            {/if}
          </div>
          {#if addingGroup === u.name}
            <form class="inline" onsubmit={(e) => (e.preventDefault(), addGroup(u, groupText))}>
              <!-- svelte-ignore a11y_autofocus -->
              <input class="mono" list="groups-{u.name}" bind:value={groupText} placeholder="networkmanager" aria-label="Group to add" autofocus spellcheck="false" />
              <datalist id="groups-{u.name}">{#each commonGroups.filter((c) => !groups.includes(c.name)) as c (c.name)}<option value={c.name}>{c.note}</option>{/each}</datalist>
              <button class="small primary" type="submit" disabled={!/^[a-z_][a-z0-9_-]*$/.test(groupText.trim()) || groups.includes(groupText.trim()) || busy === u.name + ':groups'}>Add</button>
              <button class="small ghost" type="button" onclick={() => (addingGroup = '')}>Cancel</button>
            </form>
          {/if}
          {#if gEff?.state === 'value'}<div class="src-line"><span class="spacer"></span><SourceTag source={gEff.source} pending={gEff.pending} /></div>{/if}
        </div>
      </div>

      <div class="row">
        <span class="label">SSH keys</span>
        <div class="cell">
          {#if !vals.values && !vals.error && !pending}
            <span class="skel" style:width="30ch"></span>
          {:else}
            <ul class="keys">
              {#each keys as k (k)}
                {@const s = keySummary(k)}
                <li>
                  <Icon name="key" />
                  <span class="k-type">{s.type}</span>
                  <span class="k-comment" title={k}>{s.comment || 'no comment'}</span>
                  <span class="k-tail mono" title={k}>…{s.tail}</span>
                  {#if editable && kKnown}<button class="icon" aria-label="Remove key {s.comment || s.tail} from {u.name}" title="Remove this key" disabled={busy === u.name + ':keys'} onclick={() => removeKey(u.name, k)}><Icon name="x" /></button>{/if}
                </li>
              {:else}
                <li class="none muted small">
                  {#if !kKnown}{vals.error ? `Could not read the keys (${vals.error}), so they cannot be changed here.` : 'The keys are not known yet, so they cannot be changed here.'}
                  {:else}{passwordsOff ? 'No key, and password logins are off: this user cannot log in over SSH.' : 'No authorized keys.'}{/if}
                </li>
              {/each}
            </ul>
            {#if editable && addingKey === u.name}
              <form class="key-form" onsubmit={(e) => addKey(e, u.name)}>
                <!-- svelte-ignore a11y_autofocus -->
                <textarea class="mono" rows="2" bind:value={keyText} placeholder="ssh-ed25519 AAAA… {u.name}@laptop" aria-label="Public key for {u.name}" spellcheck="false" autofocus aria-invalid={!!keyProblem}></textarea>
                {#if keyProblem}<span class="problem">{keyProblem}</span>{/if}
                <div class="buttons">
                  <button class="small primary" type="submit" disabled={!keyText.trim() || !!keyProblem || busy === u.name + ':keys'}>Add key</button>
                  <button class="small ghost" type="button" onclick={() => (addingKey = '')}>Cancel</button>
                </div>
              </form>
            {/if}
            {#if (editable && kKnown && addingKey !== u.name) || kEff?.state === 'value'}
              <div class="src-line">
                {#if editable && kKnown && addingKey !== u.name}<button class="small link" onclick={() => ((addingKey = u.name), (keyText = ''))}><Icon name="plus" /> Add key</button>{/if}
                <span class="spacer"></span>
                {#if kEff?.state === 'value'}<SourceTag source={kEff.source} pending={kEff.pending} />{/if}
              </div>
            {/if}
          {/if}
        </div>
      </div>

      {#if u.home}
        <div class="row"><span class="label">Programs</span><div class="chips">{#each u.home.programs as p (p)}<span class="chip g-programs">{p}</span>{:else}<span class="muted small">none</span>{/each}</div></div>
        <div class="row"><span class="label">Packages</span><div class="chips">{#each u.home.packages as p (p)}<span class="chip mono">{p}</span>{:else}<span class="muted small">none</span>{/each}</div></div>
        <p class="muted small summary">home-manager: {plural(u.home.programs.length, 'program')}, {plural(u.home.packages.length, 'package')}</p>
      {:else if !pending}
        <p class="muted small summary">No home-manager configuration.</p>
      {/if}

      {#if editable && createdHere(u.name)}
        <div class="u-foot">
          <button class="small danger" disabled={busy === u.name + ':remove'} onclick={() => removeUser(u.name)}><Icon name="trash" /> Remove user</button>
        </div>
      {/if}
    </section>
  {:else}
    <p class="muted">No normal users are defined on {host.name}.</p>
  {/each}
</div>

<style>
  .toolbar {
    margin-bottom: var(--lh);
  }
  .intro {
    max-width: 90ch;
  }
  .add-user {
    display: grid;
    gap: var(--lh);
    max-width: 90ch;
    margin-bottom: var(--lh);
  }
  .fields {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--lh) 2ch;
  }
  .field {
    display: grid;
  }
  .faint {
    color: var(--fg-faint);
  }
  .hint {
    color: var(--fg-muted);
  }
  .problem {
    color: var(--red);
  }
  .picks {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .pick {
    display: inline-flex;
    gap: 1ch;
    padding-right: 1ch;
    cursor: pointer;
  }
  .pick.on {
    color: var(--fg-strong);
  }
  .buttons {
    display: flex;
    gap: 0 2ch;
  }
  .users {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(56ch, 1fr));
    gap: var(--lh) 2ch;
    align-items: start;
  }
  .user {
    display: grid;
  }
  .u-head {
    display: flex;
    gap: 0 2ch;
    margin-bottom: var(--lh);
  }
  .u-title {
    min-width: 0;
  }
  /* The initial in reverse video. */
  .av {
    flex: none;
    padding: 0 1ch;
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .row {
    display: grid;
    grid-template-columns: 10ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  .cell {
    display: grid;
    min-width: 0;
  }
  .chip.admin {
    --c: var(--yellow);
  }
  .add-btn {
    color: var(--accent);
    background: none;
  }
  .inline {
    display: flex;
    gap: 0 2ch;
  }
  .inline input {
    width: 24ch;
  }
  .src-line {
    display: flex;
    gap: 0 2ch;
    min-width: 0;
  }
  .keys li {
    display: grid;
    grid-template-columns: 2ch auto minmax(0, 1fr) auto 2ch;
    gap: 0 1ch;
  }
  .keys li:hover {
    background: var(--bg-hover);
  }
  .keys li :global(.ic) {
    color: var(--accent);
  }
  .keys li.none {
    display: block;
  }
  .keys li.none:hover {
    background: none;
  }
  .k-type {
    color: var(--fg-strong);
  }
  .k-comment {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .k-tail {
    color: var(--fg-faint);
  }
  .keys li:not(:has(button)) {
    grid-template-columns: 2ch auto minmax(0, 1fr) auto;
  }
  .key-form {
    display: grid;
  }
  .summary {
    margin-top: var(--lh);
  }
  .u-foot {
    display: flex;
    margin-top: var(--lh);
  }
  @media (max-width: 700px) {
    .fields {
      grid-template-columns: 1fr;
    }
  }
</style>
