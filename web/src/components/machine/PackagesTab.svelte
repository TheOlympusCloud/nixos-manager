<script lang="ts">
  import type { Host, PackageHit } from '../../lib/api'
  import { openPackageSearch } from '../../lib/dialogs.svelte'
  import { editPackages, excludePackages, fleet, openOrCreate, removePackages, restorePackages } from '../../lib/fleet.svelte'
  import { isRepoFile } from '../../lib/graph'
  import { hostNixFile, hostNixTemplate, overrideSummary, packageFile, systemRows, type PackageRow } from '../../lib/packages'
  import { codeLink, machineLink } from '../../lib/router.svelte'
  import { app } from '../../lib/state.svelte'
  import { confirmBox } from '../../lib/ui.svelte'
  import Drawer from '../Drawer.svelte'
  import Icon from '../Icon.svelte'
  import PackagePicker from '../PackagePicker.svelte'
  import ManagedModuleNotice from './ManagedModuleNotice.svelte'
  import PackageCustomise from './PackageCustomise.svelte'

  let { host }: { host: Host } = $props()

  let filter = $state('')
  let repoOnly = $state(localStorage.getItem('nixmgr.repoOnly') === '1')
  $effect(() => localStorage.setItem('nixmgr.repoOnly', repoOnly ? '1' : '0'))
  let busy = $state('')
  let drawer = $state<null | { kind: 'customise' | 'replace'; row: PackageRow }>(null)

  const managed = $derived(fleet.managed[host.name])
  /** Removing, replacing and changing any package need managed.nix 2. */
  const full = $derived(!!host.managedFile && !!managed && (host.managedVersion ?? 1) >= 2)
  const paths = $derived(app.entries.map((e) => e.path))
  const all = $derived(systemRows(host, managed, paths))
  const rows = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return all.filter((p) => !repoOnly || p.removed || p.overridden || p.files.some(isRepoFile)).filter((p) => !q || p.name.toLowerCase().includes(q))
  })
  const hidden = $derived(all.filter((p) => !p.removed && !p.overridden && !p.files.some(isRepoFile)).length)
  const removedCount = $derived(all.filter((p) => p.removed).length)

  /** Evaluated from managed.json, but no longer in it: removed, waiting for the evaluation. */
  const gone = (p: PackageRow) => !p.pending && !p.removed && !!managed && !!host.managedFile && p.files.includes(host.managedFile) && !p.attr && p.files.every((f) => f === host.managedFile || !isRepoFile(f))

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }

  async function remove(p: PackageRow) {
    if (p.attr) return run(p.name, () => removePackages(host.name, [p.attr!]))
    const from = p.files.filter(isRepoFile)
    const ok = await confirmBox({
      title: `Remove ${p.name} from ${host.name}?`,
      message:
        (from.length ? `${p.name} is added by ${from.join(', ')}, which stays as it is: ` : `${p.name} is one of NixOS's defaults: `) +
        `it is taken out of ${host.name}'s packages by name, in ${host.managedFile}. Restore puts it back.` +
        (p.core ? ` NixOS needs ${p.name} for a working system: without it ${host.name} may not boot or let anyone log in.` : ''),
      ok: 'Remove',
      danger: p.core,
    })
    if (ok) await run(p.name, () => excludePackages(host.name, [p.name]))
  }

  async function replace(p: PackageRow, hit: PackageHit) {
    const edit = p.attr ? { remove: [p.attr], add: [hit.attr] } : { exclude: [p.name], add: [hit.attr] }
    if (await editPackages(host.name, edit, `Replaced ${p.name} with ${hit.attr} on ${host.name}`)) drawer = null
  }
</script>

<ManagedModuleNotice {host} />

<div class="toolbar">
  <span class="filter"><Icon name="filter" /><input class="search" type="search" placeholder="Filter packages" aria-label="Filter packages" data-search bind:value={filter} /></span>
  <label class="toggle">
    <input type="checkbox" bind:checked={repoOnly} />
    Only from this repository
    {#if repoOnly && hidden}<span class="muted small">({hidden} defaults hidden)</span>{/if}
  </label>
  {#if removedCount}<span class="muted small">{removedCount} removed</span>{/if}
  <span class="spacer"></span>
  {#if host.managedFile}
    {#if full}<button onclick={() => openOrCreate(hostNixFile(host), hostNixTemplate(host.name))} title="{host.name}'s own NixOS module, {hostNixFile(host)}"><Icon name="code" /> Nix for {host.name}</button>{/if}
    <button class="primary" onclick={() => openPackageSearch('', [host.name])}><Icon name="plus" /> Add packages</button>
  {:else}
    <span class="muted small">{host.name} is Code only: add packages in its Nix files, or <a href={machineLink(host.name)}>turn on editing</a>.</span>
  {/if}
</div>

<table class="data">
  <thead><tr><th>Package</th><th>Version</th><th>From</th><th><span class="sr-only">Actions</span></th></tr></thead>
  <tbody>
    {#each rows as p (p.name)}
      {@const out = gone(p)}
      {@const rawFile = !!p.overridden && paths.includes(packageFile(host, p.overridden))}
      <tr class:pending={p.pending} class:gone={out || p.removed}>
        <td class="mono name">{p.name}</td>
        <td class="mono ver">{p.pending ? '' : p.version}</td>
        <td>
          <div class="from">
            {#if p.pending && fleet.computing}<span class="badge busy">Adding…</span>{:else if p.pending}<span class="badge" title="In {host.managedFile}, but the evaluation lists no package by this name">Not in the evaluation</span>{/if}
            {#if out}<span class="badge busy" title="Removed from {host.managedFile}; it disappears once {host.name} is evaluated again">Removing{fleet.computing ? '…' : ''}</span>{/if}
            {#if p.removed}<span class="chip warn" title="Taken out by name in {host.managedFile}, whoever adds it">⌫ Removed here</span>{/if}
            {#if p.overridden}
              <button class="chip acc link" title="Built differently on {host.name}: {overrideSummary(managed?.packageOverrides?.[p.overridden], rawFile)}" onclick={() => (drawer = { kind: 'customise', row: p })}
                >≔ Changed · {overrideSummary(managed?.packageOverrides?.[p.overridden], rawFile)}</button>
            {/if}
            {#each p.files as f (f)}
              {#if f === host.managedFile}
                {#if !out}<span class="chip ok" title="Set here, in {f}">✎ Set here</span>{/if}
              {:else if isRepoFile(f)}
                <a class="code mono" href={codeLink(f)} title="In code: open {f}">λ {f}</a>
              {:else}
                <span class="def" title="Installed by NixOS or a flake input by default">· Default{p.core ? ' · core' : ''}</span>
              {/if}
            {/each}
          </div>
        </td>
        <td class="act">
          {#if full && p.removed}
            <button class="small" disabled={busy === p.name || !managed?.removedPackages?.includes(p.name)} onclick={() => run(p.name, () => restorePackages(host.name, [p.name]))} title="Give {p.name} back to {host.name}"
              ><Icon name="undo" /> Restore</button>
          {:else if full && !out}
            <span class="acts">
              <button class="icon" aria-label="Customise {p.name}" title="Customise: arguments, version, patches, Nix" onclick={() => (drawer = { kind: 'customise', row: p })}><Icon name="sliders" /></button>
              <button class="icon" aria-label="Replace {p.name}" title="Replace with another package" onclick={() => (drawer = { kind: 'replace', row: p })}><Icon name="refresh" /></button>
              <button class="icon" aria-label="Remove {p.name} from {host.name}" title="Remove from {host.name} (you can undo)" disabled={busy === p.name} onclick={() => remove(p)}><Icon name="trash" /></button>
            </span>
          {:else if p.attr && !out && host.managedFile}
            <button class="icon" aria-label="Remove {p.name} from {host.name}" title="Remove from {host.name} (you can undo)" disabled={busy === p.name} onclick={() => remove(p)}><Icon name="trash" /></button>
          {/if}
        </td>
      </tr>
    {:else}
      <tr><td colspan="4" class="muted none">{filter ? `No package matches “${filter}”.` : 'No packages.'}</td></tr>
    {/each}
  </tbody>
</table>

{#if drawer?.kind === 'customise'}
  <Drawer title="Customise {drawer.row.name} on {host.name}" onclose={() => (drawer = null)} width={720}>
    <PackageCustomise {host} name={drawer.row.name} attr={drawer.row.overridden ?? drawer.row.attr} onclose={() => (drawer = null)} />
  </Drawer>
{:else if drawer?.kind === 'replace'}
  {@const row = drawer.row}
  <Drawer title="Replace {row.name} on {host.name}" onclose={() => (drawer = null)} width={640}>
    <p class="muted">
      {row.attr ? `${row.name} comes out of ${host.managedFile}` : `${row.name} is taken out of ${host.name} by name (the Nix that adds it stays)`}, and the package you pick goes in.
    </p>
    <PackagePicker label="Use this" query={row.name} onpick={(hit) => replace(row, hit)} />
  </Drawer>
{/if}

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
    width: 14ch;
    text-align: right;
  }
  .acts {
    display: inline-flex;
    gap: 1ch;
  }
  .act button {
    opacity: 0.35;
  }
  tr:hover .act button,
  .act button:focus-visible,
  tr.gone .act button {
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
