<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type MachinePlatform, type MachineRole, type MachineSpec } from '../lib/api'
  import { createMachine, fleet } from '../lib/fleet.svelte'
  import { architectures, hardwareCheck, machineNameProblem, platforms, previewFiles, roles } from '../lib/machines'
  import { go, machineLink } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { toast } from '../lib/ui.svelte'
  import { copyText } from '../lib/util'
  import Icon from './Icon.svelte'

  /** The new-machine wizard: name, architecture, role, platform and hardware, then a preview of the files. */
  let { onclose }: { onclose: () => void } = $props()

  let el: HTMLDialogElement
  const steps = ['Name', 'Role', 'Platform', 'Review'] as const
  let step = $state(0)
  let name = $state('')
  let system = $state<MachineSpec['system']>('x86_64-linux')
  let role = $state<MachineRole>('server')
  let platform = $state<MachinePlatform>('hardware')
  let hardware = $state('')
  let creating = $state(false)
  let autoFlake = $state<boolean | null>(null)

  const taken = $derived([
    ...(fleet.model?.hosts.map((h) => h.name) ?? []),
    ...Object.keys(fleet.created),
    ...app.entries.filter((e) => e.dir && /^hosts\/[^/]+$/.test(e.path)).map((e) => e.path.slice(6)),
  ])
  const nameProblem = $derived(machineNameProblem(name.trim(), taken))
  // A container has no hardware configuration: whatever was pasted before choosing LXC is not sent.
  const lxc = $derived(platform === 'proxmox-lxc')
  const hw = $derived(lxc ? {} : hardwareCheck(hardware))
  const spec = $derived<MachineSpec>({ name: name.trim(), system, role, platform, ...(!lxc && hardware.trim() ? { hardware } : {}) })
  const files = $derived(previewFiles(spec))
  let openName = $state('managed.json')
  const shown = $derived(files.find((f) => f.path.endsWith('/' + openName)) ?? files[files.length - 1])
  const canNext = $derived(step === 0 ? !!name.trim() && !nameProblem : step === 2 ? !hw.error : true)

  onMount(() => {
    el.showModal()
    // Whether the flake picks up hosts/<name>/ by itself (the starter flake does).
    api.file('flake.nix').then(
      (f) => (autoFlake = f.content.includes('readDir ./hosts')),
      () => {},
    )
  })

  function next(e?: SubmitEvent) {
    e?.preventDefault()
    if (!canNext) return
    if (step < steps.length - 1) step++
    else create()
  }
  async function create() {
    creating = true
    const res = await createMachine($state.snapshot(spec))
    creating = false
    if (!res) return
    toast(
      res.registered ? `${res.name} is in hosts/${res.name}/; it is being evaluated now.` : `Add ${res.name} to flake.nix to finish: the snippet is on its page.`,
      'ok',
      `Created ${res.name}`,
      { label: 'Review in Changes', run: () => go('#/changes') },
    )
    go(machineLink(res.name))
    el.close()
  }

  const command = 'nixos-generate-config --show-hardware-config'
  const pctCommand = $derived(
    `pct create <id> local:vztmpl/nixos-26.05-proxmox-lxc.tar.xz --hostname ${spec.name} --ostype nixos --unprivileged 1 --features nesting=1 --net0 name=eth0,bridge=vmbr0,ip=dhcp`,
  )
  let copied = $state('')
  async function copyCommand(text: string) {
    if (await copyText(text)) copied = text
    setTimeout(() => (copied = ''), 2000)
  }
</script>

<dialog bind:this={el} class="nm" onclose={onclose} aria-labelledby="nm-title">
  <header>
    <h2 id="nm-title">New machine</h2>
    <ol class="progress" aria-label="Steps">
      {#each steps as s, i (s)}
        <li class:done={i < step} class:now={i === step} aria-current={i === step ? 'step' : undefined}>
          <button type="button" class="step-btn" disabled={i > step} onclick={() => (step = i)}><span class="num">{i + 1}</span>{s}</button>
        </li>
      {/each}
    </ol>
    <button class="icon" aria-label="Close" onclick={() => el.close()}><Icon name="x" /></button>
  </header>

  <form class="body" onsubmit={next} id="nm-form">
    {#if step === 0}
      <label class="field">
        <span class="label">Name</span>
        <!-- svelte-ignore a11y_autofocus -->
        <input class="mono big" bind:value={name} placeholder="nas" spellcheck="false" autocomplete="off" autofocus aria-invalid={!!nameProblem} aria-describedby="name-hint" />
        {#if nameProblem}
          <span class="problem" id="name-hint">{nameProblem}</span>
        {:else}
          <span class="hint" id="name-hint">It becomes the folder <span class="mono">hosts/{name.trim() || 'name'}/</span> and the machine's host name, which comin uses to find its configuration.</span>
        {/if}
      </label>
      <fieldset class="field">
        <legend class="label">Architecture</legend>
        <div class="choices two">
          {#each architectures as a (a.system)}
            <label class="choice" class:on={system === a.system}>
              <input type="radio" name="system" value={a.system} bind:group={system} />
              <span class="c-title"><Icon name="cpu" size={15} />{a.label}<span class="mono faint">{a.system}</span></span>
              <span class="c-note">{a.note}</span>
            </label>
          {/each}
        </div>
      </fieldset>
    {:else if step === 1}
      <fieldset class="field">
        <legend class="label">What is {name.trim()} for?</legend>
        <div class="choices three">
          {#each roles as r (r.role)}
            <label class="choice role" class:on={role === r.role}>
              <input type="radio" name="role" value={r.role} bind:group={role} />
              <span class="c-title"><Icon name={r.role === 'server' ? 'server' : r.role === 'desktop' ? 'box' : 'terminal'} size={15} />{r.label}</span>
              <span class="c-note">{r.note}</span>
              <ul class="gets">
                {#each r.gets as g (g)}<li><Icon name="tick" size={12} />{g}</li>{/each}
              </ul>
              <span class="pkgs">{#each r.packages as p (p)}<span class="chip mono">{p}</span>{/each}</span>
            </label>
          {/each}
        </div>
        <p class="hint">All of it lands in <span class="mono">managed.json</span>, so you can change any of it by clicking later.</p>
      </fieldset>
    {:else if step === 2}
      <fieldset class="field">
        <legend class="label">Where does {name.trim()} run?</legend>
        <div class="choices three">
          {#each platforms as p (p.platform)}
            <label class="choice" class:on={platform === p.platform}>
              <input type="radio" name="platform" value={p.platform} bind:group={platform} />
              <span class="c-title"><Icon name={p.platform === 'hardware' ? 'cpu' : p.platform === 'proxmox-vm' ? 'layers' : 'box'} size={15} />{p.label}</span>
              <span class="c-note">{p.note}</span>
            </label>
          {/each}
        </div>
      </fieldset>
      {#if lxc}
        <div class="field">
          <span class="label">Creating the container</span>
          <p class="lead">
            There is no hardware configuration: <span class="mono">default.nix</span> imports the Proxmox container module instead. On the Proxmox host, create
            {name.trim()} from the NixOS LXC template, unprivileged and with nesting on (Nix and systemd's service sandboxing need it):
          </p>
          <div class="cmd">
            <!-- Wrap between arguments only, never inside one such as --unprivileged. -->
            <code class="wrap">{#each pctCommand.split(' ') as arg, i (i)}<span class="nw">{arg}</span>{' '}{/each}</code>
            <button type="button" class="small" onclick={() => copyCommand(pctCommand)}
              ><Icon name={copied === pctCommand ? 'tick' : 'copy'} size={13} />{copied === pctCommand ? 'Copied' : 'Copy'}</button
            >
          </div>
          <span class="hint">
            Then switch it once to this configuration with <span class="mono">nixos-rebuild switch --flake &lt;repository&gt;#{name.trim()}</span>; comin keeps it up to date
            from then on. The README's Proxmox section has the template download and the details.
          </span>
        </div>
      {:else}
        <div class="field">
          <span class="label">Hardware configuration <span class="faint">(optional)</span></span>
          <p class="lead">
            {#if platform === 'proxmox-vm'}
              NixOS needs to know the VM's disks. Install NixOS in it from the ISO (and turn on <span class="mono">QEMU Guest Agent</span> in the VM's Options), then run inside it:
            {:else}
              NixOS needs to know {name.trim()}'s disks and kernel modules. On the machine, booted from the NixOS installer or already running NixOS, run:
            {/if}
          </p>
          <div class="cmd">
            <code>{command}</code>
            <button type="button" class="small" onclick={() => copyCommand(command)}><Icon name={copied === command ? 'tick' : 'copy'} size={13} />{copied === command ? 'Copied' : 'Copy'}</button>
          </div>
          <p class="lead">and paste its output here:</p>
          <textarea class="mono" rows="7" bind:value={hardware} spellcheck="false" placeholder={'{ config, lib, pkgs, modulesPath, ... }:\n{\n  imports = [ ... ];\n  fileSystems."/" = { ... };\n}'} aria-label="hardware-configuration.nix" aria-invalid={!!hw.error}
          ></textarea>
          {#if hw.error}
            <span class="problem">{hw.error}</span>
          {:else if hw.warning}
            <span class="warn"><Icon name="alert" size={13} />{hw.warning}</span>
          {:else if !hardware.trim()}
            <span class="hint">Leave it empty to write a placeholder now; replace <span class="mono">hosts/{name.trim()}/hardware-configuration.nix</span> in Code before the first deploy.</span>
          {/if}
        </div>
      {/if}
    {:else}
      <div class="review">
        <p class="lead">
          These files are written to the working copy; nothing is committed until you review them in Changes.
          {#if autoFlake === true}
            <span class="ok-text"><Icon name="tick" size={13} />flake.nix finds <span class="mono">hosts/{spec.name}/</span> by itself.</span>
          {:else if autoFlake === false}
            <span class="warn"><Icon name="info" size={13} />flake.nix lists its machines by hand: you will get the line to add.</span>
          {/if}
          {#if role === 'server'}
            <span class="warn"><Icon name="key" size={13} />The server preset allows SSH keys only and creates no user: add yours with a key in Users before deploying, or nobody can log in over SSH.</span>
          {/if}
        </p>
        <div class="files">
          <ul class="file-list" role="tablist" aria-label="Files">
            {#each files as f (f.path)}
              <li>
                <button type="button" role="tab" aria-selected={shown === f} class:on={shown === f} onclick={() => (openName = f.path.split('/').pop()!)}>
                  <Icon name="file-plus" size={14} /><span class="mono">{f.path.split('/').pop()}</span>
                </button>
              </li>
            {/each}
          </ul>
          <div class="file-view" role="tabpanel">
            <div class="fv-head"><span class="mono">{shown.path}</span><span class="muted small">{shown.note}</span></div>
            <pre>{shown.content}</pre>
          </div>
        </div>
      </div>
    {/if}
  </form>

  <footer>
    {#if step > 0}<button type="button" class="ghost" onclick={() => step--}><Icon name="left" size={14} />Back</button>{/if}
    <span class="spacer"></span>
    <button type="button" onclick={() => el.close()}>Cancel</button>
    <button type="submit" form="nm-form" class="primary" disabled={!canNext || creating}>
      {#if step < steps.length - 1}Next<Icon name="right" size={14} />{:else}{creating ? 'Creating…' : `Create ${spec.name}`}{/if}
    </button>
  </footer>
</dialog>

<style>
  .nm {
    display: flex;
    flex-direction: column;
    width: min(820px, 94vw);
    height: min(620px, 90vh);
    padding: 0;
    color: var(--fg);
    background: var(--bg-side);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--edge), var(--shadow-3);
  }
  .nm::backdrop {
    background: rgb(4 6 9 / 0.6);
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--s4);
    padding: 12px 12px 12px 20px;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    font-size: var(--fs-lg);
    white-space: nowrap;
  }
  .progress {
    display: flex;
    flex: 1;
    justify-content: center;
    gap: 4px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .step-btn {
    gap: 7px;
    height: 26px;
    padding: 0 10px 0 6px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    background: none;
    border-color: transparent;
    border-radius: 13px;
  }
  .step-btn:disabled {
    opacity: 1;
    color: var(--fg-faint);
  }
  .step-btn .num {
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    font-size: 10.5px;
    border: 1px solid currentColor;
    border-radius: 50%;
  }
  .now .step-btn {
    color: var(--fg-strong);
    background: var(--bg-active);
  }
  .now .num {
    color: #fff;
    background: var(--accent-strong);
    border-color: var(--accent-strong);
  }
  .done .step-btn {
    color: var(--fg);
  }
  .done .num {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 50%, transparent);
  }
  .body {
    display: grid;
    flex: 1;
    align-content: start;
    gap: 22px;
    min-height: 0;
    padding: 22px 24px;
    overflow: auto;
  }
  fieldset {
    margin: 0;
    padding: 0;
    border: 0;
  }
  .field {
    display: grid;
    gap: 8px;
  }
  legend {
    margin-bottom: 8px;
    padding: 0;
  }
  .big {
    height: 36px;
    font-size: var(--fs-lg);
  }
  .hint,
  .lead {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.55;
  }
  .lead {
    color: var(--fg);
    font-size: var(--fs-md);
  }
  .faint {
    color: var(--fg-faint);
  }
  .problem {
    color: var(--red);
    font-size: var(--fs-sm);
  }
  .warn,
  .ok-text {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--yellow);
    font-size: var(--fs-sm);
  }
  .ok-text {
    color: var(--green);
  }
  .review .lead {
    display: grid;
    gap: 6px;
  }
  .choices {
    display: grid;
    gap: 10px;
  }
  .two {
    grid-template-columns: 1fr 1fr;
  }
  .three {
    grid-template-columns: repeat(3, 1fr);
  }
  .choice {
    position: relative;
    display: grid;
    align-content: start;
    gap: 6px;
    padding: 12px 14px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    box-shadow: var(--edge);
    cursor: pointer;
    transition:
      border-color var(--t-fast) var(--ease),
      background var(--t-fast) var(--ease);
  }
  .choice:hover {
    border-color: var(--line-strong);
  }
  .choice.on {
    background: color-mix(in srgb, var(--accent-strong) 12%, var(--bg-raised));
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
  }
  .choice:has(input:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  .choice input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .c-title {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--fg-strong);
    font-weight: 600;
  }
  .c-title :global(svg) {
    color: var(--accent);
  }
  .c-title .mono {
    margin-left: auto;
    font-size: 11px;
    font-weight: 400;
  }
  .c-note {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    line-height: 1.45;
  }
  .gets {
    display: grid;
    gap: 3px;
    margin: 4px 0 0;
    padding: 0;
    list-style: none;
    font-size: var(--fs-sm);
  }
  .gets li {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    line-height: 1.4;
  }
  .gets :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--green);
  }
  .pkgs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 4px;
  }
  .cmd {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .cmd code.wrap {
    flex: 1;
    min-width: 0;
  }
  .nw {
    white-space: nowrap;
  }
  .cmd code {
    padding: 6px 10px;
    color: var(--fg-strong);
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  textarea {
    font-size: 12px;
    resize: vertical;
  }
  .files {
    display: grid;
    grid-template-columns: 250px minmax(0, 1fr);
    min-height: 300px;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .file-list {
    margin: 0;
    padding: 6px;
    list-style: none;
    background: var(--bg-deep);
    border-right: 1px solid var(--border);
  }
  .file-list button {
    justify-content: flex-start;
    width: 100%;
    color: var(--fg-muted);
    font-weight: 400;
    background: none;
    border-color: transparent;
  }
  .file-list button.on {
    color: var(--fg-strong);
    background: var(--bg-active);
  }
  .file-list button :global(svg) {
    color: var(--green);
  }
  .file-view {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .fv-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
    padding: 8px 12px;
    color: var(--fg-strong);
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .file-view pre {
    flex: 1;
    max-height: 340px;
    margin: 0;
    padding: 10px 12px;
    overflow: auto;
    color: var(--fg);
    font: 12px/1.55 var(--mono);
  }
  footer {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 20px 14px;
    background: var(--bg-raised);
    border-top: 1px solid var(--border);
    border-radius: 0 0 var(--r-lg) var(--r-lg);
  }
  @media (max-width: 760px) {
    .three,
    .two {
      grid-template-columns: 1fr;
    }
    .progress .step-btn {
      font-size: 0;
    }
  }
</style>
