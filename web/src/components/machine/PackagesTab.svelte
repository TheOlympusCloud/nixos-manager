<script lang="ts">
  import type { Host } from '../../lib/api'
  import { openPackageSearch } from '../../lib/dialogs.svelte'
  import { fleet, managedAttr, removePackageByName } from '../../lib/fleet.svelte'
  import { isRepoFile } from '../../lib/graph'
  import { codeLink, go, machineLink } from '../../lib/router.svelte'
  import { confirmBox } from '../../lib/ui.svelte'
  import Icon from '../Icon.svelte'

  let { host }: { host: Host } = $props()

  let filter = $state('')
  let repoOnly = $state(localStorage.getItem('nixmgr.repoOnly') === '1')
  $effect(() => localStorage.setItem('nixmgr.repoOnly', repoOnly ? '1' : '0'))
  let busy = $state('')

  const managed = $derived(fleet.managed[host.name])

  // Packages added since the last evaluation show up at once, marked as pending.
  const rows = $derived.by(() => {
    const known = new Set(host.packages.map((p) => p.name))
    // The model names packages (nodejs); managed.json lists attributes (nodejs_22): managedPackages maps them.
    const pname = (a: string) => host.managedPackages?.[a]
    const pending = (managed?.packages ?? []).filter((a) => !known.has(a) && !known.has(a.split('.').pop()!) && !known.has(pname(a) ?? ''))
    const all = [
      ...host.packages.map((p) => ({ ...p, pending: false })),
      ...pending.map((a) => ({ name: a, version: '', files: host.managedFile ? [host.managedFile] : [], pending: true })),
    ]
    const q = filter.trim().toLowerCase()
    return all
      .filter((p) => !repoOnly || p.files.some(isRepoFile))
      .filter((p) => !q || p.name.toLowerCase().includes(q))
      .sort((a, b) => a.name.localeCompare(b.name))
  })
  const hidden = $derived(host.packages.filter((p) => !p.files.some(isRepoFile)).length)

  /** Evaluated from managed.json, but no longer in it: removed, waiting for the evaluation. */
  const removing = (p: { name: string; files: string[]; pending: boolean }) =>
    !p.pending && !!managed && !!host.managedFile && p.files.includes(host.managedFile) && !managedAttr(host.name, p.name)

  async function remove(p: { name: string; files: string[] }) {
    if (host.managedFile && p.files.includes(host.managedFile)) {
      busy = p.name
      await removePackageByName(host.name, p.name)
      busy = ''
      return
    }
    const file = p.files.find((f) => isRepoFile(f) && f !== host.managedFile)
    if (!file) return
    const ok = await confirmBox({
      title: `Remove ${p.name}`,
      message: `${p.name} is installed by ${file}, which is written by hand. nixos-manager never rewrites hand-written Nix; remove it there in Code.`,
      ok: `Open ${file}`,
    })
    if (ok) go(codeLink(file))
  }
</script>

<div class="toolbar">
  <span class="filter"><Icon name="filter" /><input class="search" type="search" placeholder="Filter packages" aria-label="Filter packages" data-search bind:value={filter} /></span>
  <label class="toggle">
    <input type="checkbox" bind:checked={repoOnly} />
    Only from this repository
    {#if repoOnly && hidden}<span class="muted small">({hidden} defaults hidden)</span>{/if}
  </label>
  <span class="spacer"></span>
  {#if host.managedFile}
    <button class="primary" onclick={() => openPackageSearch('', [host.name])}><Icon name="plus" /> Add packages</button>
  {:else}
    <span class="muted small">{host.name} is Code only: add packages in its Nix files, or <a href={machineLink(host.name)}>turn on editing</a>.</span>
  {/if}
</div>

<table class="data">
  <thead><tr><th>Package</th><th>Version</th><th>From</th><th><span class="sr-only">Actions</span></th></tr></thead>
  <tbody>
    {#each rows as p (p.name)}
      {@const isManaged = !!host.managedFile && p.files.includes(host.managedFile)}
      {@const gone = removing(p)}
      <tr class:pending={p.pending} class:gone>
        <td class="mono name">{p.name}</td>
        <td class="mono ver">{p.pending ? '' : p.version}</td>
        <td>
          <div class="from">
            {#if p.pending && fleet.computing}<span class="badge busy">Adding…</span>{:else if p.pending}<span class="badge" title="In {host.managedFile}, but the evaluation lists no package by this name">Not in the evaluation</span>{/if}
            {#if gone}<span class="badge busy" title="Removed from {host.managedFile}; it disappears once {host.name} is evaluated again">Removing{fleet.computing ? '…' : ''}</span>{/if}
            {#each p.files as f (f)}
              {#if f === host.managedFile}
                {#if !gone}
                  <span class="chip ok" title="Set here, in {f}"
                    >✎ Set here<button class="x icon" aria-label="Remove {p.name} from {host.name}" title="Remove from {host.name} (you can undo)" disabled={busy === p.name} onclick={() => remove(p)}
                      ><Icon name="x" /></button
                    ></span>
                {/if}
              {:else if isRepoFile(f)}
                <a class="code mono" href={codeLink(f)} title="In code: open {f}">λ {f}</a>
              {:else}
                <span class="def" title="Installed by NixOS or a flake input by default">· Default</span>
              {/if}
            {/each}
          </div>
        </td>
        <td class="act">
          {#if !isManaged && p.files.some(isRepoFile)}
            <button class="icon" aria-label="Remove {p.name} (hand-written)" title="Hand-written: remove it in Code" onclick={() => remove(p)}><Icon name="trash" /></button>
          {/if}
        </td>
      </tr>
    {:else}
      <tr><td colspan="4" class="muted none">{filter ? `No package matches “${filter}”.` : 'No packages.'}</td></tr>
    {/each}
  </tbody>
</table>

<style>
  .toolbar {
    margin-bottom: var(--lh);
  }
  .filter {
    display: flex;
    gap: 1ch;
  }
  .toggle {
    display: inline-flex;
    gap: 1ch;
    cursor: pointer;
  }
  table.data th {
    top: calc(var(--lh) * 2); /* below the machine tabs */
  }
  .name {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .ver {
    width: 18ch;
  }
  .from {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .code {
    color: var(--fg-muted);
  }
  .code:hover {
    color: var(--on-accent);
  }
  .def {
    color: var(--fg-faint);
  }
  .act {
    width: 4ch;
    text-align: right;
  }
  .act button {
    opacity: 0.35;
  }
  tr:hover .act button,
  .act button:focus-visible {
    opacity: 1;
  }
  tr.pending td {
    background: color-mix(in srgb, var(--accent) 8%, transparent);
  }
  tr.gone .name,
  tr.gone .ver {
    color: var(--fg-faint);
    text-decoration: line-through;
  }
  .none {
    padding: var(--lh) 1ch;
  }
</style>
