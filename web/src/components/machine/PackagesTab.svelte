<script lang="ts">
  import type { Host } from '../../lib/api'
  import { openPackageSearch } from '../../lib/dialogs.svelte'
  import { fleet, removePackageByName } from '../../lib/fleet.svelte'
  import { isRepoFile } from '../../lib/graph'
  import { codeLink, go } from '../../lib/router.svelte'
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
    const pending = (managed?.packages ?? []).filter((a) => !known.has(a) && !known.has(a.split('.').pop()!))
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
  <input class="search" type="search" placeholder="Filter packages" aria-label="Filter packages" data-search bind:value={filter} />
  <label class="toggle">
    <input type="checkbox" bind:checked={repoOnly} />
    Only from this repository
    {#if repoOnly && hidden}<span class="muted small">({hidden} NixOS defaults hidden)</span>{/if}
  </label>
  <span class="spacer"></span>
  {#if host.managedFile}
    <button class="primary" onclick={() => openPackageSearch('', [host.name])}><Icon name="plus" size={14} />Add packages</button>
  {:else}
    <span class="muted small" title="Turn on visual editing to add packages by clicking">Visual editing is off: add packages in Code</span>
  {/if}
</div>

<div class="table-wrap">
  <table class="data">
    <thead><tr><th>Package</th><th>Version</th><th>From</th><th><span class="sr-only">Actions</span></th></tr></thead>
    <tbody>
      {#each rows as p (p.name)}
        {@const isManaged = !!host.managedFile && p.files.includes(host.managedFile)}
        <tr class:pending={p.pending}>
          <td class="mono name">{p.name}</td>
          <td class="mono muted ver">{p.pending ? '' : p.version}</td>
          <td>
            <div class="chips">
              {#if p.pending && fleet.computing}<span class="badge busy">Pending evaluation</span>{:else if p.pending}<span class="badge" title="In {host.managedFile}, but the evaluation lists no package by this name">Not in the evaluation</span>{/if}
              {#each p.files as f (f)}
                {#if f === host.managedFile}
                  <span class="chip managed" title="Added by clicking ({f})"
                    >managed<button class="x" aria-label="Remove {p.name} from {host.name}" title="Remove from {host.name}" disabled={busy === p.name} onclick={() => remove(p)}
                      ><Icon name="x" size={11} /></button
                    ></span>
                {:else if isRepoFile(f)}
                  <a class="chip mono" href={codeLink(f)} title="Open {f} in Code">{f}</a>
                {:else}
                  <span class="chip" title="Installed by NixOS or a flake input by default">NixOS default</span>
                {/if}
              {/each}
            </div>
          </td>
          <td class="act">
            {#if !isManaged && p.files.some(isRepoFile)}
              <button class="icon" aria-label="Remove {p.name} (hand-written)" title="Hand-written: remove it in Code" onclick={() => remove(p)}><Icon name="trash" size={15} /></button>
            {/if}
          </td>
        </tr>
      {:else}
        <tr><td colspan="4" class="muted none">{filter ? `No package matches “${filter}”.` : 'No packages.'}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .toolbar {
    margin-bottom: var(--s3);
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    margin-left: 6px;
    cursor: pointer;
  }
  .table-wrap {
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    overflow: clip;
  }
  table.data th {
    top: 41px; /* below the machine tabs */
  }
  .name {
    color: var(--fg-strong);
  }
  .ver {
    width: 160px;
    font-size: 12px;
  }
  .act {
    width: 48px;
    text-align: right;
  }
  .act button {
    opacity: 0;
  }
  tr:hover .act button,
  .act button:focus-visible {
    opacity: 1;
  }
  .chip.managed {
    --c: var(--accent);
  }
  a.chip {
    text-decoration: none;
  }
  a.chip:hover {
    color: var(--fg-strong);
    border-color: var(--line-strong);
  }
  tr.pending td {
    background: color-mix(in srgb, var(--accent) 5%, transparent);
  }
  .none {
    padding: 18px var(--s3);
  }
  tbody tr:last-child td {
    border-bottom: 0;
  }
</style>
