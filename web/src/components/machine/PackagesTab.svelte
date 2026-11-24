<script lang="ts">
  import type { Host, PackageHit } from '../../lib/api'
  import { openPackageSearch } from '../../lib/dialogs.svelte'
  import { editPackages, excludePackages, fleet, healthy, loadManaged, openOrCreate, removePackages, restorePackages, targetName } from '../../lib/fleet.svelte'
  import { isRepoFile } from '../../lib/graph'
  import { SHARED, SHARED_FILE, appliesShared, hostNixFile, hostNixTemplate, overrideSummary, packageFile, systemRows, type PackageRow } from '../../lib/packages'
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
  /** modules/managed.json, set for every machine: managed.nix 3 applies it. */
  const canShare = $derived(full && appliesShared(host))
  $effect(() => {
    if (canShare && !fleet.managed[SHARED]) loadManaged(SHARED)
  })
  const shared = $derived(canShare ? fleet.managed[SHARED] : undefined)
  /** Edit this machine's managed.json, or modules/managed.json for every machine. */
  let everywhere = $state(false)
  const target = $derived(everywhere && shared ? SHARED : host.name)
  const edited = $derived(fleet.managed[target])
  /** Machines a shared edit does not reach: Code only, or an older managed.nix. */
  const unreached = $derived(healthy().filter((h) => !appliesShared(h)).map((h) => h.name))
  const paths = $derived(app.entries.map((e) => e.path))
  const all = $derived(systemRows(host, managed, paths, shared))
  const rows = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return all.filter((p) => !repoOnly || p.removed || p.overridden || p.files.some(isRepoFile)).filter((p) => !q || p.name.toLowerCase().includes(q))
  })
  const hidden = $derived(all.filter((p) => !p.removed && !p.overridden && !p.files.some(isRepoFile)).length)
  const removedCount = $derived(all.filter((p) => p.removed).length)

  /** Evaluated from managed.json (or modules/managed.json), but no longer in it: removed, waiting for the evaluation. */
  const stale = (p: PackageRow, f: string) => (f === host.managedFile && !!managed && !p.attr) || (f === SHARED_FILE && !!shared && !p.sharedAttr)
  const gone = (p: PackageRow) => !p.pending && !p.removed && !!host.managedFile && p.files.some((f) => stale(p, f)) && p.files.every((f) => stale(p, f) || !isRepoFile(f))
  /** The row's attribute in the file being edited. */
  const attrIn = (p: PackageRow) => (target === SHARED ? p.sharedAttr : p.attr)

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }

  async function remove(p: PackageRow) {
    const t = target
    const attr = attrIn(p)
    if (attr) return run(p.name, () => removePackages(t, [attr]))
    const from = p.files.filter(isRepoFile)
    const everywhere = t === SHARED
    const ok = await confirmBox({
      title: `Remove ${p.name} from ${targetName(t)}?`,
      message:
        (from.length ? `${p.name} is added by ${from.join(', ')}, which stays as it is: ` : `${p.name} is one of NixOS's defaults: `) +
        (everywhere
          ? `it is taken out of every machine's packages by name, in ${SHARED_FILE}. A machine whose own managed.json adds it keeps it${p.attr ? `, as ${host.name} does` : ''}. Restore puts it back.`
          : `it is taken out of ${host.name}'s packages by name, in ${host.managedFile}. Restore puts it back.`) +
        (p.core ? ` NixOS needs ${p.name} for a working system: without it ${everywhere ? 'a machine' : host.name} may not boot or let anyone log in.` : ''),
      ok: 'Remove',
      danger: p.core || everywhere,
    })
    if (ok) await run(p.name, () => excludePackages(t, [p.name]))
  }

  async function replace(p: PackageRow, hit: PackageHit) {
    const attr = attrIn(p)
    const edit = attr ? { remove: [attr], add: [hit.attr] } : { exclude: [p.name], add: [hit.attr] }
    if (await editPackages(target, edit, `Replaced ${p.name} with ${hit.attr} on ${targetName(target)}`)) drawer = null
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
  {#if canShare}
    <div class="seg" role="radiogroup" aria-label="Where changes go">
      <button role="radio" aria-checked={!everywhere} class:on={!everywhere} onclick={() => (everywhere = false)} title="Changes go to {host.managedFile}">{host.name}</button>
      <button role="radio" aria-checked={everywhere} class:on={everywhere} disabled={!shared} onclick={() => (everywhere = true)} title="Changes go to {SHARED_FILE}">Every machine</button>
    </div>
  {/if}
  {#if host.managedFile}
    {#if full && !everywhere}<button onclick={() => openOrCreate(hostNixFile(host), hostNixTemplate(host.name))} title="{host.name}'s own NixOS module, {hostNixFile(host)}"><Icon name="code" /> Nix for {host.name}</button>{/if}
    <button class="primary" onclick={() => openPackageSearch('', [target])}><Icon name="plus" /> Add packages</button>
  {:else}
    <span class="muted small">{host.name} is Code only: add packages in its Nix files, or <a href={machineLink(host.name)}>turn on editing</a>.</span>
  {/if}
</div>

{#if target === SHARED}
  <p class="scope small">
    <Icon name="info" /> Changes go to <a class="mono" href={codeLink(SHARED_FILE)}>{SHARED_FILE}</a>, for every machine; a machine's own managed.json wins where both set something.
    {#if unreached.length}Not on {unreached.join(', ')}: Code only, or an older managed.nix.{/if}
  </p>
{/if}

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
            {#if p.removed && (!p.removedShared || managed?.removedPackages?.includes(p.name))}<span class="chip warn" title="Taken out by name in {host.managedFile}, whoever adds it">⌫ Removed here</span>{/if}
            {#if p.removedShared}<span class="chip warn" title="Taken out by name in {SHARED_FILE}, on every machine that does not add it itself">⌫ Removed on every machine</span>{/if}
            {#if p.overridden}
              {@const o = p.sharedOverride ? shared?.packageOverrides?.[p.sharedOverride] : managed?.packageOverrides?.[p.overridden]}
              <button
                class="chip acc link"
                title="Built differently on {p.sharedOverride ? 'every machine' : host.name}: {overrideSummary(o, rawFile)}"
                onclick={() => (drawer = { kind: 'customise', row: p })}>≔ Changed{p.sharedOverride ? ' on every machine' : ''} · {overrideSummary(o, rawFile)}</button>
            {/if}
            {#each p.files as f (f)}
              {#if f === host.managedFile}
                {#if !out}<span class="chip ok" title="Set here, in {f}">✎ Set here</span>{/if}
              {:else if f === SHARED_FILE}
                {#if !out}<span class="chip ok" title="Set for every machine, in {f}">⇉ Every machine</span>{/if}
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
            {@const here = !!edited?.removedPackages?.includes(p.name)}
            <button
              class="small"
              disabled={busy === p.name || !here}
              onclick={() => run(p.name, () => restorePackages(target, [p.name]))}
              title={here ? `Give ${p.name} back to ${targetName(target)}` : p.removedShared && target !== SHARED ? `Removed on every machine: restore it under Every machine, or add it to ${host.name}` : `Not removed in ${edited?.path ?? 'this file'}`}
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
  <Drawer title="Customise {drawer.row.name} on {targetName(target)}" onclose={() => (drawer = null)} width={720}>
    <PackageCustomise {host} {target} name={drawer.row.name} attr={drawer.row.overridden ?? attrIn(drawer.row)} onclose={() => (drawer = null)} />
  </Drawer>
{:else if drawer?.kind === 'replace'}
  {@const row = drawer.row}
  <Drawer title="Replace {row.name} on {targetName(target)}" onclose={() => (drawer = null)} width={640}>
    <p class="muted">
      {attrIn(row) ? `${row.name} comes out of ${edited?.path}` : `${row.name} is taken out of ${targetName(target)} by name (the Nix that adds it stays)`}, and the package you pick goes in.
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
  .scope {
    margin: 0 0 var(--lh);
    color: var(--fg-muted);
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
