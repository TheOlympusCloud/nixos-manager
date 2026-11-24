<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type Host, type PackagePatch } from '../../lib/api'
  import { copyToMachines, fleet, openOrCreate, setPackageOverride, targetName } from '../../lib/fleet.svelte'
  import { SHARED, cleanOverride, copyTargets, formatArgs, overrideSummary, packageFile, packageFileTemplate, parseArgs } from '../../lib/packages'
  import { link } from '../../lib/router.svelte'
  import { app, refresh } from '../../lib/state.svelte'
  import { confirmBox, report } from '../../lib/ui.svelte'
  import CopyTo from '../CopyTo.svelte'
  import Icon from '../Icon.svelte'

  /**
   * How one package is built on a machine: .override arguments, .overrideAttrs
   * attributes, another version and source, patches, and a Nix file for the rest.
   * The change is an overlay: everything on the machine that uses the package gets it.
   * target: the machine's name (default) or SHARED, to change it on every machine.
   */
  let { host, target: givenTarget, name, attr: given, onclose }: { host: Host; target?: string; name: string; attr?: string; onclose: () => void } = $props()

  const target = untrack(() => givenTarget ?? host.name)
  const everywhere = target === SHARED
  let attr = $state(untrack(() => given ?? name))
  const current = $derived(fleet.managed[target]?.packageOverrides?.[attr])
  /** What modules/managed.json sets, which the machine's own replaces whole. */
  const theirs = $derived(everywhere ? undefined : fleet.managed[SHARED]?.packageOverrides?.[attr])
  const rawPath = $derived(packageFile(host, attr))
  // modules/managed.nix reads no Nix files next to modules/managed.json.
  const rawExists = $derived(!everywhere && app.entries.some((e) => e.path === rawPath))
  const changed = $derived(!!current || rawExists)

  // The form starts from what the file has for the package when the drawer opens.
  const start = untrack(() => fleet.managed[target]?.packageOverrides?.[attr])
  let argsText = $state(formatArgs(start?.override))
  let attrsText = $state(formatArgs(start?.overrideAttrs))
  let version = $state(start?.version ?? '')
  let srcKind = $state<'none' | 'url' | 'github'>(start?.src?.github ? 'github' : start?.src?.url ? 'url' : 'none')
  let url = $state(start?.src?.url ?? '')
  let github = $state(start?.src?.github ?? '')
  let rev = $state(start?.src?.rev ?? '')
  let hash = $state(start?.src?.hash ?? '')
  let patches = $state<PackagePatch[]>((start?.patches ?? []).map((p) => ({ ...p })))

  const args = $derived(parseArgs(argsText))
  const attrs = $derived(parseArgs(attrsText))
  const attrProblem = $derived(/^[A-Za-z0-9_+-][A-Za-z0-9_+'-]*(\.[A-Za-z0-9_+-][A-Za-z0-9_+'-]*)*$/.test(attr) ? '' : 'An attribute such as htop or python3Packages.requests.')
  const problem = $derived(attrProblem || ('problem' in args ? args.problem : '') || ('problem' in attrs ? attrs.problem : ''))
  const override = $derived(
    'args' in args && 'args' in attrs
      ? cleanOverride({
          override: args.args,
          overrideAttrs: attrs.args,
          version,
          src: srcKind === 'url' ? { url, hash } : srcKind === 'github' ? { github, rev, hash } : undefined,
          patches,
        })
      : null,
  )

  let busy = $state(false)
  async function save() {
    if (problem) return
    if (!override && !current) return onclose() // nothing set, nothing to clear
    busy = true
    const ok = await setPackageOverride(target, attr, override)
    busy = false
    if (ok) onclose()
  }
  async function reset() {
    const ok = await confirmBox({
      title: `Put ${attr} back as nixpkgs has it on ${targetName(target)}?`,
      message: rawExists ? `This clears what is set here and deletes ${rawPath}.` : 'This clears what is set here.',
      ok: 'Reset to default',
      danger: true,
    })
    if (!ok) return
    busy = true
    try {
      if (current && !(await setPackageOverride(target, attr, null))) return
      if (rawExists) {
        await api.remove(rawPath).catch((e) => report(e, `Could not delete ${rawPath}`))
        await refresh()
      }
      onclose()
    } finally {
      busy = false
    }
  }
  // A copy takes what is saved, so it waits for the form to be saved.
  const saved = $derived(JSON.stringify(override) === JSON.stringify(current ?? null))
  const copyHosts = $derived(everywhere ? [] : copyTargets(fleet.model?.hosts ?? [], host.name).map((h) => ({ name: h.name })))
  const feature = $derived(host.features.find((f) => (f.group === 'programs' || f.group === 'services') && f.name === name))
</script>

<div class="pc">
  <p class="muted">
    Changes how <span class="mono">{attr}</span> is built on {everywhere ? `every machine that loads modules/managed.json, unless the machine changes it itself,` : host.name}, for everything there that uses it (home-manager too, when it shares the system's packages: <span class="mono">useGlobalPkgs</span>). Nix then builds it instead of downloading it. Values are JSON, as
    <span class="mono">managed.json</span> keeps them.
  </p>
  {#if theirs}
    <p class="muted small"><Icon name="info" /> Changed on every machine: {overrideSummary(theirs, false)}. What you save here replaces that on {host.name}.</p>
  {/if}

  <label class="field">
    <span class="label">Attribute in nixpkgs</span>
    <input class="mono" bind:value={attr} disabled={!!given || !!current} spellcheck="false" aria-invalid={!!attrProblem} />
    {#if attrProblem}<span class="problem small">{attrProblem}</span>{/if}
  </label>

  <section>
    <h3>Arguments <span class="mono muted">.override</span></h3>
    <textarea class="mono args" bind:value={argsText} spellcheck="false" placeholder={'withVimKeys = true\nsensorsSupport = false'} aria-label="Arguments for .override"></textarea>
    {#if 'problem' in args}<span class="problem small">{args.problem}</span>{/if}
  </section>

  <section>
    <h3>Attributes <span class="mono muted">.overrideAttrs</span></h3>
    <textarea class="mono args" bind:value={attrsText} spellcheck="false" placeholder={'doCheck = false\nconfigureFlags = ["--enable-x"]'} aria-label="Attributes for .overrideAttrs"></textarea>
    {#if 'problem' in attrs}<span class="problem small">{attrs.problem}</span>{/if}
  </section>

  <section>
    <h3>Version and source</h3>
    <div class="row">
      <label class="field short"><span class="label">Version</span><input class="mono" bind:value={version} placeholder="3.4.1" spellcheck="false" /></label>
      <div class="seg" role="radiogroup" aria-label="Source">
        {#each [['none', 'nixpkgs’ own'], ['url', 'URL'], ['github', 'GitHub']] as const as [k, label] (k)}
          <button role="radio" aria-checked={srcKind === k} class:on={srcKind === k} onclick={() => (srcKind = k)}>{label}</button>
        {/each}
      </div>
    </div>
    {#if srcKind === 'url'}
      <label class="field wide"><span class="label">Source URL</span><input class="mono" bind:value={url} placeholder="https://…/htop-3.4.1.tar.xz" spellcheck="false" /></label>
    {:else if srcKind === 'github'}
      <div class="row">
        <label class="field"><span class="label">Repository</span><input class="mono" bind:value={github} placeholder="owner/repo" spellcheck="false" /></label>
        <label class="field short"><span class="label">Tag or commit</span><input class="mono" bind:value={rev} placeholder="v3.4.1" spellcheck="false" /></label>
      </div>
    {/if}
    {#if srcKind !== 'none'}
      <label class="field wide"><span class="label">Hash</span><input class="mono" bind:value={hash} placeholder="empty: the check fails and prints it" spellcheck="false" /></label>
    {/if}
  </section>

  <section>
    <h3>Patches <span class="muted small">after the package's own</span></h3>
    {#each patches as p, i (i)}
      <div class="row patch">
        {#if p.file !== undefined}
          <label class="field grow"><span class="label">{everywhere ? 'File in modules/' : `File in ${host.name}'s folder`}</span><input class="mono" bind:value={p.file} placeholder="patches/fix.patch" spellcheck="false" /></label>
        {:else}
          <label class="field grow"><span class="label">URL</span><input class="mono" bind:value={p.url} placeholder="https://…/fix.patch" spellcheck="false" /></label>
          <label class="field short"><span class="label">Hash</span><input class="mono" bind:value={p.hash} placeholder="empty: printed" spellcheck="false" /></label>
        {/if}
        <button class="icon" aria-label="Remove this patch" onclick={() => patches.splice(i, 1)}><Icon name="x" /></button>
      </div>
    {/each}
    <div class="row">
      <button class="small" onclick={() => patches.push({ url: '', hash: '' })}><Icon name="plus" /> Patch from a URL</button>
      <button class="small" onclick={() => patches.push({ file: '' })}><Icon name="plus" /> Patch file</button>
    </div>
  </section>

  {#if !everywhere}
    <section>
      <h3>Nix</h3>
      <p class="muted small">Anything else, in Nix: <span class="mono">{rawPath}</span> gets the package with the changes above and returns the one to use.</p>
      <button class="small" disabled={!!attrProblem} onclick={() => openOrCreate(rawPath, packageFileTemplate(attr, host.name))}><Icon name="code" /> {rawExists ? 'Edit' : 'Create'} {attr}.nix</button>
    </section>
  {/if}

  <section>
    <h3>Settings</h3>
    <p class="row">
      {#if feature}<a class="btn small" href={link('options', [host.name], { q: `${feature.group}.${feature.name}.` })}><Icon name="sliders" /> Configure {feature.group}.{feature.name}</a>{/if}
      <a class="btn small" href={link('options', [host.name], { q: `${name}.enable` })}><Icon name="search" /> Find modules for {name}</a>
    </p>
  </section>

  {#if changed && copyHosts.length}
    <section>
      <h3>Other machines</h3>
      {#if saved}
        <div class="row">
          <CopyTo
            title="Change {attr} as {host.name} does on:"
            note="Each machine gets this change in its managed.json{rawExists ? `, with ${attr}.nix` : ''}{current?.patches?.some((p) => p.file) ? ' and its patch files' : ''}. A machine that already changes {attr} another way is left as it is."
            targets={copyHosts}
            copy={(to) => copyToMachines(host.name, { packageOverride: attr, to }, `how ${attr} is built`)}
          />
        </div>
      {:else}
        <p class="muted small">Save first: Copy to… copies what is saved.</p>
      {/if}
    </section>
  {/if}

  <div class="buttons">
    <button class="primary" disabled={busy || !!problem} onclick={save}>{busy ? 'Saving…' : 'Save'}</button>
    {#if changed}<button class="danger" disabled={busy} onclick={reset}><Icon name="undo" /> Reset to default</button>{/if}
    <button class="ghost" onclick={onclose}>Cancel</button>
  </div>
</div>

<style>
  .pc {
    display: grid;
    gap: var(--lh);
  }
  section {
    display: grid;
    gap: 0;
  }
  h3 {
    margin: 0;
    font-size: inherit;
  }
  .field {
    display: grid;
    width: 32ch;
  }
  .field.short {
    width: 20ch;
  }
  .field.wide,
  .args {
    width: 100%;
  }
  .grow {
    flex: 1;
  }
  .args {
    height: calc(var(--lh) * 4);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: var(--lh) 2ch;
    margin: 0;
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
    gap: 0 2ch;
  }
</style>
