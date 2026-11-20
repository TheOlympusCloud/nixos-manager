<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type SecretFile, type SecretMachine, type SecretsStatus } from '../lib/api'
  import { fleet } from '../lib/fleet.svelte'
  import { go } from '../lib/router.svelte'
  import { cominSecret, reportSecret, secretNameProblem } from '../lib/secrets'
  import { refresh } from '../lib/state.svelte'
  import { confirmBox, toast } from '../lib/ui.svelte'

  /** Settings → Secrets: this server's age key, the machines' keys, and the secrets' names (values only on request). */
  let st = $state<SecretsStatus | null>(null)
  let error = $state('')
  let busy = $state('')
  const show = (e: unknown) => (error = e instanceof Error ? e.message : String(e))

  // A machine's key being added or changed, and the secret being written.
  let keyFor = $state('')
  let keyText = $state('')
  let target = $state('') // '' = every machine with a key
  let name = $state('')
  let value = $state('')
  let comin = $state('')
  /** Values shown on request, by "<host>/<name>"; dropped on Hide and when the panel goes. */
  let shown = $state<Record<string, string>>({})

  const keyed = $derived(new Set(st?.machines.map((m) => m.name) ?? []))
  const inFlake = $derived(new Set(fleet.model?.hosts.map((h) => h.name) ?? []))
  const withoutKey = $derived([...inFlake].filter((h) => !keyed.has(h)).sort())
  const common = $derived(st?.files.find((f) => f.host === '') ?? null)
  const hasReportToken = (h: string) => !!st?.files.find((f) => f.host === h)?.names.includes(reportSecret)
  const nameProblem = $derived(secretNameProblem(name.trim()))

  async function load() {
    try {
      st = await api.secrets()
      error = ''
    } catch (e) {
      show(e)
    }
  }
  onMount(load)

  /** Runs a change, then shows it as a change of the working copy. */
  async function run(what: string, fn: () => Promise<unknown>, done: string): Promise<boolean> {
    busy = what
    error = ''
    try {
      await fn()
      toast(done, 'ok', undefined, { label: 'View changes', run: () => go('#/changes') })
      await Promise.all([load(), refresh()])
      return true
    } catch (e) {
      show(e)
      return false
    } finally {
      busy = ''
    }
  }

  async function saveKey(e: SubmitEvent) {
    e.preventDefault()
    const host = keyFor
    if (await run('key', () => api.setMachineKey(host, keyText), `${host} gets its secrets once this is committed and pushed`)) keyFor = keyText = ''
  }
  async function approve(m: SecretMachine) {
    const ok = await confirmBox({
      title: `Approve ${m.name}'s key`,
      message: `${m.fingerprint}: is this ${m.name}'s SSH host key? Compare it with ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub on the machine. Whoever holds this key can read every secret ${m.name} gets.${keyed.has(m.name) ? ` It replaces the key approved for ${m.name} now.` : ''}`,
      ok: 'Approve',
    })
    if (ok) await run('key', () => api.setMachineKey(m.name, m.key), `Approved ${m.name}'s key`)
  }
  async function removeKey(host: string) {
    const own = st?.files.find((f) => f.host === host)?.names.length ?? 0
    const ok = await confirmBox({
      title: `Remove ${host}'s key`,
      message: `${host} no longer gets secrets${own ? `, and its ${own} own secret${own === 1 ? ' is' : 's are'} deleted` : ''}. The secrets of every machine are encrypted again without it, with a new key, so it cannot open what is added later. Change the values it knew if it is not to be trusted. Settings of ${host} that use a secret (an option set to its path) fail its check until you remove them.`,
      ok: 'Remove',
      danger: true,
    })
    if (ok) await run('remove', () => api.removeMachineKey(host), `Removed ${host}'s key`)
  }

  async function saveSecret(e: SubmitEvent) {
    e.preventDefault()
    const n = name.trim()
    if (await run('secret', () => api.setSecret(target, n, value), `Saved ${n}, encrypted, ${target ? `for ${target}` : 'for every machine with a key'}`)) {
      name = value = ''
      delete shown[`${target}/${n}`]
    }
  }
  async function deleteSecret(f: SecretFile, n: string) {
    const ok = await confirmBox({ title: `Delete ${n}`, message: `Delete ${n} from ${f.path}? Machines lose /run/secrets/${n} with the next deployment.`, ok: 'Delete', danger: true })
    if (ok && (await run('delete', () => api.deleteSecret(f.host, n), `Deleted ${n}`))) delete shown[`${f.host}/${n}`]
  }
  async function reveal(f: SecretFile, n: string) {
    error = ''
    try {
      shown[`${f.host}/${n}`] = await api.revealSecret(f.host, n)
    } catch (e) {
      show(e)
    }
  }
  function change(host: string, n: string) {
    target = host
    name = n
    value = ''
    document.getElementById('secret-value')?.focus()
  }

  async function saveComin(e: SubmitEvent) {
    e.preventDefault()
    if (await run('comin', () => api.setSecret('', cominSecret, comin.trim()), 'Saved comin’s pull token, encrypted for every machine with a key')) comin = ''
  }
  const storeReports = () => run('report', () => api.storeReportTokens(), 'Stored each machine’s report token as its secret')
</script>

<section class="card block" aria-labelledby="set-secrets">
  <h2 id="set-secrets">Secrets</h2>
  <p class="muted">
    Passwords, tokens and keys for the machines, kept with sops-nix in the repository's <span class="mono">secrets/</span>, encrypted for this server and the machines
    that get them. Each machine decrypts its own with its SSH host key into <span class="mono">/run/secrets/&lt;name&gt;</span>. A change here is a change of the working copy:
    check, commit and push it as usual.
  </p>

  {#if !st}
    {#if error}<p class="error" role="alert">{error}</p>{:else}<span class="skel" style:width="60%"></span>{/if}
  {:else if st.missing.length}
    <p class="error" role="alert">
      Not found on the server: <span class="mono">{st.missing.join(', ')}</span>. The Nix package and the Docker image bring sops, age and ssh-to-age; a server run otherwise needs them
      on its PATH.
    </p>
  {:else}
    <div class="field">
      <span class="label">This server's key</span>
      <span class="mono small wrap">{st.recipient}</span>
      <span class="muted small">Back up <span class="mono">{st.keyFile}</span>: without it nobody can change these secrets (the machines keep theirs). Never put it in the repository.</span>
    </div>

    {#if st.stale}
      <p class="warn" role="status">
        Some secrets are not encrypted for exactly the machines' keys (a key or a file changed outside this page).
        <button class="small" onclick={() => run('reencrypt', () => api.reencryptSecrets(), 'Encrypted the secrets again for the machines’ keys')} disabled={!!busy}>Re-encrypt</button>
      </p>
    {/if}

    <h3>Machine keys</h3>
    <table>
      <thead><tr><th>Machine</th><th>SSH host key</th><th><span class="sr-only">Actions</span></th></tr></thead>
      <tbody>
        {#each st.machines as m (m.name)}
          <tr>
            <td>{m.name}{#if fleet.model && !inFlake.has(m.name)}<span class="badge err" title="Remove its key so it no longer opens the shared secrets">not in the flake</span>{/if}</td>
            <td>{#if m.error}<span class="error-text">{m.error}</span>{:else}<span class="mono small" title={m.key}>{m.fingerprint}</span>{/if}</td>
            <td class="act">
              <button class="small" onclick={() => ((keyFor = m.name), (keyText = ''))}>Change</button>
              <button class="small ghost" onclick={() => removeKey(m.name)} disabled={!!busy}>Remove</button>
            </td>
          </tr>
        {/each}
        {#each withoutKey as h (h)}
          <tr>
            <td>{h}</td>
            <td class="muted small">No key: gets no secrets</td>
            <td class="act"><button class="small" onclick={() => ((keyFor = h), (keyText = ''))}>Add key</button></td>
          </tr>
        {/each}
        {#if !st.machines.length && !withoutKey.length}<tr><td colspan="3" class="muted">No machines yet.</td></tr>{/if}
      </tbody>
    </table>
    {#if st.proposed.length}
      <div class="sub" role="note">
        <strong>Keys waiting for approval</strong>
        <span class="muted small">
          In <span class="mono">secrets/hosts/</span> but not approved here (they came with a pull or an edit): nothing is encrypted for them until you approve one, and
          Re-encrypt writes back the approved key of a machine that has one.
        </span>
        <ul class="plain">
          {#each st.proposed as m (m.name)}
            <li>
              <span>{m.name}</span>
              {#if m.error}<span class="error-text small">{m.error}</span>{:else}<span class="mono small">{m.fingerprint}</span>{/if}
              {#if keyed.has(m.name)}<span class="badge">differs from its approved key</span>{/if}
              <span class="spacer"></span>
              {#if !m.error}<button class="small" onclick={() => approve(m)} disabled={!!busy}>Approve</button>{/if}
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    {#if keyFor}
      <form class="sub" onsubmit={saveKey}>
        <label class="label" for="machine-key">SSH host key of {keyFor}</label>
        <textarea id="machine-key" class="mono" rows="2" bind:value={keyText} placeholder="ssh-ed25519 AAAA… root@{keyFor}" spellcheck="false" autocomplete="off" required></textarea>
        <span class="muted small">
          On the machine: <span class="mono">cat /etc/ssh/ssh_host_ed25519_key.pub</span>. From here, <span class="mono">ssh-keyscan -t ed25519 {keyFor}</span> works too, but compare its
          fingerprint with the machine's (<span class="mono">ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub</span>): whoever holds this key gets the secrets.
        </span>
        <div class="actions">
          <button class="primary" type="submit" disabled={!keyText.trim() || !!busy}>{busy === 'key' ? 'Saving…' : 'Save key'}</button>
          <button type="button" class="ghost" onclick={() => (keyFor = '')}>Cancel</button>
        </div>
      </form>
    {/if}

    <h3>Secrets</h3>
    {#each st.files as f (f.path)}
      <div class="file">
        <div class="file-head">
          <strong>{f.host ? `${f.host} only` : 'Every machine with a key'}</strong>
          <span class="mono small muted">{f.path}</span>
          {#if f.locked}<span class="badge err" title="Encrypted without this server's key: restore the key it was made with">this server cannot open it</span>{/if}
          {#if f.host && !keyed.has(f.host)}<span class="badge" title="Add its key, or delete the file">no key for {f.host}</span>{/if}
        </div>
        {#if f.error}<p class="error-text small">{f.error}</p>{/if}
        <ul>
          {#each f.names as n (n)}
            {@const k = `${f.host}/${n}`}
            <li>
              <span class="mono">{n}</span>
              {#if shown[k] !== undefined}<textarea class="mono value" readonly rows={Math.min(4, shown[k].split('\n').length)} aria-label="Value of {n}" value={shown[k]}></textarea>{/if}
              <span class="spacer"></span>
              {#if shown[k] !== undefined}
                <button class="small ghost" onclick={() => delete shown[k]}>Hide</button>
              {:else}
                <button class="small ghost" onclick={() => reveal(f, n)} disabled={f.locked}>Show</button>
              {/if}
              <button class="small ghost" onclick={() => change(f.host, n)} disabled={f.locked}>Change</button>
              <button class="small ghost" onclick={() => deleteSecret(f, n)} disabled={f.locked || !!busy}>Delete</button>
            </li>
          {/each}
        </ul>
      </div>
    {:else}
      <p class="muted">No secrets yet.</p>
    {/each}

    <form class="sub" onsubmit={saveSecret}>
      <div class="grid">
        <div class="field">
          <label class="label" for="secret-for">For</label>
          <select id="secret-for" bind:value={target}>
            <option value="">Every machine with a key</option>
            {#each st.machines as m (m.name)}<option value={m.name}>{m.name} only</option>{/each}
          </select>
        </div>
        <div class="field">
          <label class="label" for="secret-name">Name</label>
          <input id="secret-name" class="mono" bind:value={name} placeholder="wifi-password" spellcheck="false" autocomplete="off" aria-describedby="secret-name-hint" />
          <span class="small" class:error-text={!!nameProblem} class:muted={!nameProblem} id="secret-name-hint">{nameProblem || `On the machine: /run/secrets/${name.trim() || '<name>'}`}</span>
        </div>
      </div>
      <label class="label" for="secret-value">Value</label>
      <textarea id="secret-value" class="mono" rows="2" bind:value placeholder="Encrypted before it is written; replaces the value of a secret with this name" spellcheck="false" autocomplete="off"></textarea>
      <div class="actions">
        <button class="primary" type="submit" disabled={!name.trim() || !!nameProblem || !value || !!busy}>{busy === 'secret' ? 'Saving…' : 'Save secret'}</button>
      </div>
    </form>

    <h3>Setup</h3>
    <form class="sub" onsubmit={saveComin}>
      <strong>Private repository</strong>
      <span class="muted small">
        comin on each machine pulls with a read-only Forgejo token, kept as <span class="mono">{cominSecret}</span> for every machine with a key
        ({common?.names.includes(cominSecret) ? 'saved' : 'not set'}). In Forgejo: <strong>Settings → Applications → Generate new token</strong>, as a user who can read the repository,
        with the <strong>repository</strong> permission <strong>Read</strong>. The starter fleet's <span class="mono">modules/common.nix</span> points comin at it; a machine needs it
        before its first pull of a private repository.
      </span>
      <div class="row">
        <input type="password" bind:value={comin} placeholder={common?.names.includes(cominSecret) ? 'saved: paste a new token to replace it' : 'Forgejo access token'} aria-label="comin's pull token" autocomplete="off" />
        <button type="submit" disabled={!comin.trim() || !!busy}>{busy === 'comin' ? 'Saving…' : 'Save token'}</button>
      </div>
    </form>
    <div class="sub">
      <strong>Report tokens</strong>
      <span class="muted small">
        Each machine's own report token (Deployments) as its secret <span class="mono">{reportSecret}</span>, instead of copying it over SSH; the starter fleet's
        <span class="mono">modules/secrets.nix</span> points the report hook at it. Store them again after <strong>New tokens for every machine</strong>.
        {#if st.machines.length}Stored for: {st.machines.filter((m) => hasReportToken(m.name)).map((m) => m.name).join(', ') || 'none yet'}.{/if}
      </span>
      <div class="row">
        <button onclick={storeReports} disabled={!st.machines.length || !!busy}>{busy === 'report' ? 'Storing…' : 'Store every machine’s token'}</button>
      </div>
    </div>
  {/if}
  {#if error && st}<p class="error" role="alert">{error}</p>{/if}
</section>

<style>
  .block {
    display: grid;
    min-width: 0;
    gap: 12px;
    padding: var(--s4) var(--s5) var(--s5);
  }
  h2 {
    font-size: var(--fs-lg);
  }
  h3 {
    margin: 8px 0 0;
    font-size: var(--fs-md);
  }
  p {
    margin: 0;
    line-height: 1.55;
  }
  .wrap {
    overflow-wrap: anywhere;
  }
  .field,
  .sub {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .sub {
    gap: 8px;
    padding: var(--s3);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(160px, 1fr) minmax(200px, 2fr);
    gap: var(--s2) var(--s3);
  }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    padding: 6px 8px 6px 0;
    border-bottom: 1px solid var(--border);
    text-align: left;
    vertical-align: middle;
  }
  th {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    font-weight: 500;
  }
  td .badge {
    margin-left: 8px;
  }
  .act {
    text-align: right;
    white-space: nowrap;
  }
  .file {
    display: grid;
    gap: 4px;
  }
  .file-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 8px;
  }
  .file ul {
    margin: 0;
    padding: 0;
    list-style: none;
    border-top: 1px solid var(--border);
  }
  .file li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 0;
    border-bottom: 1px solid var(--border);
  }
  .plain {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .plain li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    padding: 4px 0;
  }
  .value {
    flex: 1;
    min-width: 0;
    resize: vertical;
  }
  .row,
  .actions {
    display: flex;
    gap: 8px;
  }
  .row input {
    flex: 1;
    min-width: 0;
  }
  .warn {
    color: var(--yellow);
  }
  .error {
    color: var(--red);
  }
</style>
