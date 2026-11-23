<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type MachineBoot, type MachineDisk, type MachinePlatform, type MachineRole, type MachineSpec } from '../lib/api'
  import { createMachine, fleet } from '../lib/fleet.svelte'
  import { installCommand } from '../lib/install'
  import { architectures, bootLoaders, deviceProblem, diskoInputSnippet, disks, hardwareCheck, hasDiskoInput, machineNameProblem, platforms, previewFiles, roles } from '../lib/machines'
  import { go, machineLink } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { toast } from '../lib/ui.svelte'
  import { copyText } from '../lib/util'
  import Icon from './Icon.svelte'

  /** The new-machine wizard: name, architecture, role, platform and boot loader and hardware or disk layout, then a preview of the files. */
  let { onclose }: { onclose: () => void } = $props()

  let el: HTMLDialogElement
  const steps = ['Name', 'Role', 'Platform', 'Review'] as const
  let step = $state(0)
  let name = $state('')
  let system = $state<MachineSpec['system']>('x86_64-linux')
  let role = $state<MachineRole>('server')
  let platform = $state<MachinePlatform>('hardware')
  let hardware = $state('')
  // nixos-anywhere installs it on a disko layout, instead of NixOS installed by hand.
  let anywhere = $state(false)
  let boot = $state<MachineBoot>('uefi')
  let disk = $state<MachineDisk>('ext4')
  let device = $state('')
  // Its own state: a disk typed for GRUB must never pre-fill the disk nixos-anywhere erases.
  let grubDevice = $state('')
  let creating = $state(false)
  let flakeText = $state<string | null>(null)
  const autoFlake = $derived(flakeText === null ? null : flakeText.includes('readDir ./hosts'))

  const taken = $derived([
    ...(fleet.model?.hosts.map((h) => h.name) ?? []),
    ...Object.keys(fleet.created),
    ...app.entries.filter((e) => e.dir && /^hosts\/[^/]+$/.test(e.path)).map((e) => e.path.slice(6)),
  ])
  const nameProblem = $derived(machineNameProblem(name.trim(), taken))
  // A container has no hardware configuration: whatever was pasted before choosing LXC is not sent.
  const lxc = $derived(platform === 'proxmox-lxc')
  const hw = $derived(lxc || anywhere ? {} : hardwareCheck(hardware))
  const layout = $derived(!lxc && anywhere)
  // Installed by hand on BIOS: GRUB goes on a disk. GRUB's BIOS target is x86 only.
  const grubBIOS = $derived(!lxc && !anywhere && system === 'x86_64-linux' && boot === 'bios')
  const devProblem = $derived(deviceProblem(device.trim()))
  const grubProblem = $derived(deviceProblem(grubDevice.trim()))
  const spec = $derived<MachineSpec>({
    name: name.trim(),
    system,
    role,
    platform,
    ...(layout
      ? { disk, device: device.trim() }
      : lxc
        ? {}
        : { boot: grubBIOS ? 'bios' : 'uefi', ...(grubBIOS ? { device: grubDevice.trim() } : {}), ...(hardware.trim() ? { hardware } : {}) }),
  })
  const noDiskoInput = $derived(layout && flakeText !== null && !hasDiskoInput(flakeText))
  const files = $derived(previewFiles(spec))
  let openName = $state('managed.json')
  const shown = $derived(files.find((f) => f.path.endsWith('/' + openName)) ?? files[files.length - 1])
  const canNext = $derived(
    step === 0 ? !!name.trim() && !nameProblem : step === 2 ? !hw.error && (!layout || (!!device.trim() && !devProblem)) && (!grubBIOS || (!!grubDevice.trim() && !grubProblem)) : true,
  )

  onMount(() => {
    el.showModal()
    // Whether the flake picks up hosts/<name>/ by itself (the starter flake does).
    api.file('flake.nix').then(
      (f) => (flakeText = f.content),
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
      res.inputSnippet
        ? `Add the disko input to flake.nix, then Lock: ${res.name}'s disk layout needs it.`
        : res.registered
          ? `${res.name} is in hosts/${res.name}/; it is being evaluated now.`
          : `Add ${res.name} to flake.nix to finish: the snippet is on its page.`,
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
        <fieldset class="field">
          <legend class="label">Installing NixOS</legend>
          <div class="choices two">
            <label class="choice" class:on={!anywhere}>
              <input type="radio" name="install" value={false} bind:group={anywhere} />
              <span class="c-title"><Icon name="terminal" size={15} />Installed by hand</span>
              <span class="c-note">From the NixOS installer, as usual. Its boot loader and hardware configuration go below.</span>
            </label>
            <label class="choice" class:on={anywhere}>
              <input type="radio" name="install" value={true} bind:group={anywhere} />
              <span class="c-title"><Icon name="layers" size={15} />nixos-anywhere</span>
              <span class="c-note">Erases a disk, lays it out with disko and installs over SSH. It writes the hardware configuration itself.</span>
            </label>
          </div>
        </fieldset>
      {/if}
      {#if layout}
        <fieldset class="field">
          <legend class="label">Disk layout</legend>
          <div class="choices two">
            {#each disks as d (d.disk)}
              <label class="choice" class:on={disk === d.disk}>
                <input type="radio" name="disk" value={d.disk} bind:group={disk} />
                <span class="c-title"><Icon name="box" size={15} />{d.label}</span>
                <span class="c-note">{d.note}</span>
              </label>
            {/each}
          </div>
          <span class="hint">Both have a 1 GiB boot partition and boot with GRUB{system === 'x86_64-linux' ? ', on BIOS and UEFI' : ' on UEFI'}.</span>
        </fieldset>
        <label class="field">
          <span class="label">Disk to erase</span>
          <input class="mono" bind:value={device} placeholder="/dev/nvme0n1" spellcheck="false" autocomplete="off" aria-invalid={!!devProblem} aria-describedby="device-hint" />
          {#if devProblem}
            <span class="problem" id="device-hint">{devProblem}</span>
          {:else}
            <span class="hint" id="device-hint">
              As <span class="mono">lsblk</span> shows it on the machine: <span class="mono">/dev/sda</span>, <span class="mono">/dev/nvme0n1</span>{#if platform === 'proxmox-vm'}, or
                <span class="mono">/dev/vda</span> for a VirtIO Block disk{/if}. A <span class="mono">/dev/disk/by-id/…</span> name stays right when disks are added. Everything on
              it is erased when you run the install.
            </span>
          {/if}
        </label>
      {:else if !lxc}
        {#if system === 'x86_64-linux'}
          <fieldset class="field">
            <legend class="label">Boot loader</legend>
            <div class="choices two">
              {#each bootLoaders as b (b.boot)}
                <label class="choice" class:on={boot === b.boot}>
                  <input type="radio" name="boot" value={b.boot} bind:group={boot} />
                  <span class="c-title"><Icon name="disk" size={15} />{b.label}</span>
                  <span class="c-note">{b.note}</span>
                </label>
              {/each}
            </div>
            <span class="hint">As the installer set it up: <span class="mono">grep boot.loader /etc/nixos/configuration.nix</span> on the machine tells.</span>
          </fieldset>
          {#if grubBIOS}
            <label class="field">
              <span class="label">Disk GRUB is on</span>
              <input class="mono" bind:value={grubDevice} placeholder="/dev/sda" spellcheck="false" autocomplete="off" aria-invalid={!!grubProblem} aria-describedby="grub-hint" />
              {#if grubProblem}
                <span class="problem" id="grub-hint">{grubProblem}</span>
              {:else}
                <span class="hint" id="grub-hint">The installer's <span class="mono">boot.loader.grub.device</span>: a whole disk such as <span class="mono">/dev/sda</span>, not a partition.</span>
              {/if}
            </label>
          {/if}
        {:else}
          <div class="field">
            <span class="label">Boot loader</span>
            <span class="hint">systemd-boot, on UEFI: GRUB's BIOS target is x86_64 only.</span>
          </div>
        {/if}
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
          {#if noDiskoInput}
            <span class="warn"><Icon name="alert" size={13} />flake.nix has no disko input, which disk-config.nix needs: add it to its inputs, then Lock.</span>
          {/if}
        </p>
        {#if noDiskoInput}
          <div class="cmd">
            <pre class="snippet">{diskoInputSnippet}</pre>
            <button type="button" class="small" onclick={() => copyCommand(diskoInputSnippet)}
              ><Icon name={copied === diskoInputSnippet ? 'tick' : 'copy'} size={13} />{copied === diskoInputSnippet ? 'Copied' : 'Copy'}</button
            >
          </div>
        {/if}
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
        {#if layout}
          <div class="install">
            <span class="label">Installing it</span>
            <p class="hint">
              Once you commit and push, {spec.name}'s page walks you through it: boot the NixOS installer, fetch its SSH host key for secrets, then run this from a clone of the
              repository. Nothing is installed until you do.
            </p>
            <code class="wrap">{#each installCommand(spec.name, '', true).split(' ') as arg, i (i)}<span class="nw">{arg}</span>{' '}{/each}</code>
          </div>
        {/if}
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
    box-shadow: var(--shadow-3);
  }
  .nm::backdrop {
    background: var(--scrim);
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
    border-radius: var(--r);
    box-shadow: none;
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
    color: var(--on-accent);
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
  .review {
    display: grid;
    gap: var(--s3);
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
  .install {
    display: grid;
    gap: 6px;
  }
  .install code,
  .snippet {
    padding: 6px 10px;
    color: var(--fg-strong);
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .snippet {
    flex: 1;
    margin: 0;
    font: 12px/1.55 var(--mono);
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
