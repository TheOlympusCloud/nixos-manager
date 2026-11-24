<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type MachineDisk, type MachinePlatform, type MachineRole, type MachineSpec } from '../lib/api'
  import { bigText, frame } from '../lib/ascii'
  import { createMachine, fleet } from '../lib/fleet.svelte'
  import { plural } from '../lib/format'
  import { installCommand } from '../lib/install'
  import { architectures, deviceProblem, diskoInputSnippet, disks, hardwareCheck, hasDiskoInput, machineNameProblem, platforms, previewFiles, roles } from '../lib/machines'
  import { go, machineLink } from '../lib/router.svelte'
  import { app } from '../lib/state.svelte'
  import { toast } from '../lib/ui.svelte'
  import { parseSSHKeys, userNameProblem } from '../lib/users'
  import { copyText } from '../lib/util'
  import Dither from './cell/Dither.svelte'
  import Icon from './Icon.svelte'

  /** The new-machine wizard: name, architecture, role, first user, platform and hardware or disk layout, then a preview of the files. */
  let { onclose }: { onclose: () => void } = $props()

  let el: HTMLDialogElement
  const steps = ['Name', 'Role', 'User', 'Platform', 'Review'] as const
  let step = $state(0)
  let name = $state('')
  let system = $state<MachineSpec['system']>('x86_64-linux')
  let role = $state<MachineRole>('server')
  /** The packages it starts with, when changed from the role's; null: the role's own. */
  let packages = $state<string[] | null>(null)
  const chosen = $derived(packages ?? roles.find((r) => r.role === role)?.packages ?? [])
  let newPkg = $state('')
  const pkgProblem = $derived(newPkg.trim() && !/^[A-Za-z0-9_+-][A-Za-z0-9_+'-]*(\.[A-Za-z0-9_+-][A-Za-z0-9_+'-]*)*$/.test(newPkg.trim()) ? 'A nixpkgs attribute, such as htop.' : '')
  function addPkg() {
    const p = newPkg.trim()
    if (!p || pkgProblem) return
    if (!chosen.includes(p)) packages = [...chosen, p]
    newPkg = ''
  }
  // The first user, as Users → Add user makes one; none while the name is empty.
  let userName = $state('')
  let userKeys = $state('')
  let wheel = $state(true)
  let home = $state(false)
  const uName = $derived(userName.trim())
  const parsedKeys = $derived(parseSSHKeys(userKeys))
  const keyCount = $derived('keys' in parsedKeys ? parsedKeys.keys.length : 0)
  const keysProblem = $derived(userKeys.trim() && 'problem' in parsedKeys ? parsedKeys.problem : '')
  const userProblem = $derived(userNameProblem(uName, []) || (!uName && userKeys.trim() ? 'Give the keys a user name, or clear them.' : ''))
  let platform = $state<MachinePlatform>('hardware')
  let hardware = $state('')
  // nixos-anywhere installs it on a disko layout, instead of NixOS installed by hand.
  let anywhere = $state(false)
  let disk = $state<MachineDisk>('ext4')
  let device = $state('')
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
  const devProblem = $derived(deviceProblem(device.trim()))
  const spec = $derived<MachineSpec>({
    name: name.trim(),
    system,
    role,
    platform,
    ...(packages ? { packages } : {}),
    ...(uName ? { user: { name: uName, keys: 'keys' in parsedKeys ? parsedKeys.keys : [], wheel, home } } : {}),
    ...(layout ? { disk, device: device.trim() } : !lxc && hardware.trim() ? { hardware } : {}),
  })
  const noDiskoInput = $derived(layout && flakeText !== null && !hasDiskoInput(flakeText))
  // shortcut: any mention counts; a flake without home-manager's module fails to evaluate the machine, which says so.
  const noHomeManager = $derived(!!uName && home && flakeText !== null && !flakeText.includes('home-manager'))
  const files = $derived(previewFiles(spec))
  let openName = $state('managed.json')
  const shown = $derived(files.find((f) => f.path.endsWith('/' + openName)) ?? files[files.length - 1])
  const canNext = $derived(
    step === 0 ? !!name.trim() && !nameProblem : step === 2 ? !userProblem && !keysProblem : step === 3 ? !hw.error && (!layout || (!!device.trim() && !devProblem)) : true,
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

<dialog bind:this={el} class="nm screen" onclose={onclose} aria-labelledby="nm-title">
  <Dither />
  <div class="modal" use:frame={{ kind: 'double', shadow: true, animate: true, title: 'New machine', tag: `${step + 1} / ${steps.length}` }}>
    <header>
      <h2 id="nm-title" class="sr-only">New machine</h2>
      <!-- ✓ Name ──── Role ──── User ──── Platform ──── Review -->
      <ol class="steps" aria-label="Steps">
        {#each steps as s, i (s)}
          <li class:done={i < step} class:now={i === step} aria-current={i === step ? 'step' : undefined}>
            <button type="button" class="bare step-btn" disabled={i > step} onclick={() => (step = i)}>{s}</button>
          </li>
        {/each}
      </ol>
      <!-- On the top line, left of the step count: ═ × ═ 2 / 5 ═╗ -->
      <button class="icon close" aria-label="Close" onclick={() => el.close()}><Icon name="x" /></button>
    </header>

    <form class="body" onsubmit={next} id="nm-form">
      {#if step === 0}
        <label class="field">
          <span class="label">Name</span>
          <pre class="big acc" aria-hidden="true" use:bigText={name.trim()}></pre>
          <span class="prompt-row"
            ><b class="prompt">λ ›</b>
            <!-- svelte-ignore a11y_autofocus -->
            <input class="mono" bind:value={name} placeholder="nas" spellcheck="false" autocomplete="off" autofocus aria-invalid={!!nameProblem} aria-describedby="name-hint" /></span>
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
              <label class="choice lift" class:on={system === a.system} use:frame={{ kind: system === a.system ? 'heavy' : 'round', lift: true }}>
                <input type="radio" name="system" value={a.system} bind:group={system} />
                <span class="c-title"><Icon name="cpu" /> {a.label}<span class="mono faint">{a.system}</span></span>
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
              <label class="choice role lift" class:on={role === r.role} use:frame={{ kind: role === r.role ? 'heavy' : 'round', lift: true }}>
                <input type="radio" name="role" value={r.role} bind:group={role} onchange={() => (packages = null)} />
                <span class="c-title"><Icon name={r.role === 'server' ? 'server' : r.role === 'desktop' ? 'box' : 'terminal'} /> {r.label}</span>
                <span class="c-note">{r.note}</span>
                <ul class="gets">
                  {#each r.gets as g (g)}<li><Icon name="tick" /> {g}</li>{/each}
                </ul>
                <span class="pkgs">{#each r.packages as p (p)}<span class="chip mono">{p}</span>{/each}</span>
              </label>
            {/each}
          </div>
          <div class="pkg-edit">
            <span class="label">Packages it starts with</span>
            <span class="pkgs">
              {#each chosen as p (p)}<span class="chip mono">{p}<button type="button" class="x icon" aria-label="Leave out {p}" onclick={() => (packages = chosen.filter((x) => x !== p))}><Icon name="x" /></button></span>{/each}
              <input
                class="mono add-pkg"
                placeholder="+ attribute"
                aria-label="Add a package"
                spellcheck="false"
                bind:value={newPkg}
                aria-invalid={!!pkgProblem}
                onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addPkg())} />
              {#if packages}<button type="button" class="link small" onclick={() => (packages = null)}>Back to the preset</button>{/if}
            </span>
            {#if pkgProblem}<span class="error-text small">{pkgProblem}</span>{/if}
          </div>
          <p class="hint">All of it lands in <span class="mono">managed.json</span>, so you can change any of it by clicking later.</p>
        </fieldset>
      {:else if step === 2}
        <label class="field">
          <span class="label">First user <span class="faint">(optional)</span></span>
          <input class="mono" bind:value={userName} placeholder="alice" spellcheck="false" autocomplete="off" aria-invalid={!!userProblem} aria-describedby="user-hint" />
          {#if userProblem}
            <span class="problem" id="user-hint">{userProblem}</span>
          {:else}
            <span class="hint" id="user-hint">
              No role creates a user. Leave it empty when users come from hand-written Nix, or add them in Users later. Its password is set on the machine
              (<span class="mono">passwd</span>, or when the installer ISO offers to).
            </span>
          {/if}
        </label>
        <label class="field">
          <span class="label">Authorized SSH keys <span class="faint">(one per line)</span></span>
          <textarea class="mono keys" rows="4" bind:value={userKeys} placeholder="ssh-ed25519 AAAA… {uName || 'alice'}@laptop" spellcheck="false" aria-invalid={!!keysProblem}></textarea>
          {#if keysProblem}
            <span class="problem">{keysProblem}</span>
          {:else if keyCount}
            <span class="hint">{plural(keyCount, 'key')}</span>
          {:else if uName && role === 'server'}
            <span class="warn"><Icon name="key" /> The server preset allows SSH keys only: without a key, {uName} cannot log in over SSH.</span>
          {:else}
            <span class="hint">The contents of <span class="mono">~/.ssh/id_ed25519.pub</span> on the computer you log in from.</span>
          {/if}
        </label>
        <div class="field">
          <label class="toggle"><input type="checkbox" bind:checked={wheel} /><strong>Administrator</strong><span class="hint">in the wheel group: can use sudo</span></label>
          <label class="toggle"><input type="checkbox" bind:checked={home} /><strong>home-manager</strong><span class="hint">their own packages, settings and dotfiles, in the Home tab</span></label>
          {#if noHomeManager}
            <span class="warn"><Icon name="alert" /> flake.nix does not mention home-manager: add its input and NixOS module, as the starter flake has, or {name.trim()} will not evaluate.</span>
          {/if}
        </div>
      {:else if step === 3}
        <fieldset class="field">
          <legend class="label">Where does {name.trim()} run?</legend>
          <div class="choices three">
            {#each platforms as p (p.platform)}
              <label class="choice lift" class:on={platform === p.platform} use:frame={{ kind: platform === p.platform ? 'heavy' : 'round', lift: true }}>
                <input type="radio" name="platform" value={p.platform} bind:group={platform} />
                <span class="c-title"><Icon name={p.platform === 'hardware' ? 'cpu' : p.platform === 'proxmox-vm' ? 'layers' : 'box'} /> {p.label}</span>
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
                ><Icon name={copied === pctCommand ? 'tick' : 'copy'} /> {copied === pctCommand ? 'Copied' : 'Copy'}</button
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
              <label class="choice lift" class:on={!anywhere} use:frame={{ kind: !anywhere ? 'heavy' : 'round', lift: true }}>
                <input type="radio" name="install" value={false} bind:group={anywhere} />
                <span class="c-title"><Icon name="terminal" /> Installed by hand</span>
                <span class="c-note">From the NixOS installer, as usual. Its hardware configuration goes below.</span>
              </label>
              <label class="choice lift" class:on={anywhere} use:frame={{ kind: anywhere ? 'heavy' : 'round', lift: true }}>
                <input type="radio" name="install" value={true} bind:group={anywhere} />
                <span class="c-title"><Icon name="layers" /> nixos-anywhere</span>
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
                <label class="choice lift" class:on={disk === d.disk} use:frame={{ kind: disk === d.disk ? 'heavy' : 'round', lift: true }}>
                  <input type="radio" name="disk" value={d.disk} bind:group={disk} />
                  <span class="c-title"><Icon name="box" /> {d.label}</span>
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
              <button type="button" class="small" onclick={() => copyCommand(command)}><Icon name={copied === command ? 'tick' : 'copy'} /> {copied === command ? 'Copied' : 'Copy'}</button>
            </div>
            <p class="lead">and paste its output here:</p>
            <textarea class="mono" rows="7" bind:value={hardware} spellcheck="false" placeholder={'{ config, lib, pkgs, modulesPath, ... }:\n{\n  imports = [ ... ];\n  fileSystems."/" = { ... };\n}'} aria-label="hardware-configuration.nix" aria-invalid={!!hw.error}
            ></textarea>
            {#if hw.error}
              <span class="problem">{hw.error}</span>
            {:else if hw.warning}
              <span class="warn"><Icon name="alert" /> {hw.warning}</span>
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
              <span class="ok-text"><Icon name="tick" /> flake.nix finds <span class="mono">hosts/{spec.name}/</span> by itself.</span>
            {:else if autoFlake === false}
              <span class="warn"><Icon name="info" /> flake.nix lists its machines by hand: you will get the line to add.</span>
            {/if}
            {#if role === 'server' && !spec.user}
              <span class="warn"><Icon name="key" /> The server preset allows SSH keys only and creates no user: add yours with a key in Users before deploying, or nobody can log in over SSH.</span>
            {:else if role === 'server' && !spec.user?.keys?.length}
              <span class="warn"><Icon name="key" /> The server preset allows SSH keys only, and {spec.user?.name} has no key: add one in Users before deploying, or {spec.user?.name} cannot log in over SSH.</span>
            {/if}
            {#if noDiskoInput}
              <span class="warn"><Icon name="alert" /> flake.nix has no disko input, which disk-config.nix needs: add it to its inputs, then Lock.</span>
            {/if}
          </p>
          {#if noDiskoInput}
            <div class="cmd">
              <pre class="snippet">{diskoInputSnippet}</pre>
              <button type="button" class="small" onclick={() => copyCommand(diskoInputSnippet)}
                ><Icon name={copied === diskoInputSnippet ? 'tick' : 'copy'} /> {copied === diskoInputSnippet ? 'Copied' : 'Copy'}</button
              >
            </div>
          {/if}
          <div class="files">
            <ul class="file-list" role="tablist" aria-label="Files">
              {#each files as f (f.path)}
                <li>
                  <button type="button" role="tab" aria-selected={shown === f} class="bare" class:on={shown === f} onclick={() => (openName = f.path.split('/').pop()!)}>
                    <Icon name="file-plus" /> <span class="mono">{f.path.split('/').pop()}</span>
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
      {#if step > 0}<button type="button" class="ghost" onclick={() => step--}><Icon name="left" /> Back</button>{/if}
      <span class="spacer"></span>
      <button type="button" onclick={() => el.close()}>Cancel</button>
      <button type="submit" form="nm-form" class="primary" disabled={!canNext || creating}>
        {#if step < steps.length - 1}Next <Icon name="right" />{:else}{creating ? 'Creating…' : `Create ${spec.name}`}{/if}
      </button>
    </footer>
  </div>
</dialog>

<style>
  /* A fixed height: the steps scroll inside, never the framed (and shadowed) box. */
  .modal {
    display: flex;
    flex-direction: column;
    width: 96ch;
    height: min(calc(var(--lh) * 38), round(down, calc(100vh - var(--lh) * 4), var(--lh)));
  }
  header {
    flex: none;
    margin-bottom: var(--lh);
  }
  .close {
    position: absolute;
    top: 0;
    right: 10ch;
    z-index: 2;
    width: 3ch;
    background: var(--bg-raised);
  }
  /* Steps: ✓ done in green, the one you are on in reverse video, the rest faint, joined by ────. */
  .steps {
    display: flex;
    flex-wrap: wrap;
    list-style: none;
    white-space: pre;
  }
  .steps > li + li::before {
    color: var(--line);
    content: ' ──── ';
  }
  .step-btn {
    color: var(--fg-muted);
    font-weight: 400;
  }
  .step-btn:disabled {
    color: var(--fg-faint);
  }
  .now .step-btn {
    padding: 0 1ch;
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .done .step-btn {
    color: var(--green);
  }
  .done .step-btn::before {
    content: '✓ ';
  }
  .done .step-btn:is(:hover, :focus-visible) {
    color: var(--on-accent);
  }
  .body {
    display: grid;
    flex: 1;
    align-content: start;
    gap: var(--lh);
    min-height: 0;
    padding-right: 1ch;
    overflow: auto;
  }
  fieldset {
    border: 0;
  }
  .field {
    display: grid;
  }
  legend {
    padding: 0;
  }
  .prompt-row {
    display: flex;
    gap: 1ch;
    white-space: nowrap;
  }
  .big {
    margin: var(--lh) 0;
  }
  .hint {
    color: var(--fg-muted);
  }
  .faint {
    color: var(--fg-faint);
  }
  .problem {
    color: var(--red);
  }
  .problem::before {
    content: '✖ ';
  }
  .warn {
    display: block;
    color: var(--yellow);
  }
  .ok-text {
    display: block;
    color: var(--green);
  }
  .toggle {
    display: flex;
    gap: 1ch;
    width: max-content;
    max-width: 100%;
    cursor: pointer;
  }
  .review {
    display: grid;
    gap: var(--lh);
  }
  .choices {
    display: grid;
    gap: var(--lh) 2ch;
  }
  .two {
    grid-template-columns: 1fr 1fr;
  }
  .three {
    grid-template-columns: repeat(3, 1fr);
  }
  .choice {
    display: grid;
    align-content: start;
  }
  .choice.on > :global(.fr) {
    color: var(--accent);
  }
  /* The radio is the whole card: hidden, but focusable (focus lifts the frame). */
  .choice input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .c-title {
    display: flex;
    gap: 1ch;
    color: var(--fg-strong);
    font-weight: 700;
  }
  .c-title :global(.ic) {
    color: var(--accent);
  }
  .c-title .mono {
    margin-left: auto;
    font-weight: 400;
  }
  .c-note {
    color: var(--fg-muted);
  }
  .gets {
    margin-top: var(--lh);
    list-style: none;
  }
  .gets li {
    display: flex;
    gap: 1ch;
  }
  .gets :global(.ic) {
    color: var(--green);
  }
  .pkgs {
    display: flex;
    flex-wrap: wrap;
    gap: 0 1ch;
    margin-top: var(--lh);
  }
  .pkg-edit {
    display: grid;
    margin-top: var(--lh);
  }
  .pkg-edit .pkgs {
    align-items: baseline;
    margin-top: 0;
  }
  .add-pkg {
    width: 16ch;
  }
  .cmd {
    display: flex;
    align-items: flex-start;
    gap: 2ch;
  }
  .cmd code.wrap {
    flex: 1;
    min-width: 0;
  }
  .nw {
    white-space: nowrap;
  }
  code,
  .snippet {
    padding: 0 1ch;
    color: var(--fg-strong);
    background: var(--bg-input);
  }
  .snippet {
    flex: 1;
    white-space: pre;
    overflow: auto;
  }
  textarea {
    height: calc(var(--lh) * 7);
  }
  textarea.keys {
    height: calc(var(--lh) * 4);
  }
  .install {
    display: grid;
  }
  /* The files, as editor tabs over the selected one. */
  .file-list {
    display: flex;
    flex-wrap: wrap;
    list-style: none;
  }
  .file-list button {
    padding: 0 2ch 0 1ch;
    color: var(--fg-muted);
    font-weight: 400;
  }
  .file-list button.on {
    color: var(--fg-strong);
    font-weight: 700;
    background: var(--bg-active);
  }
  .file-list button :global(.ic) {
    color: var(--green);
  }
  .file-list button:is(:hover, :focus-visible) :global(.ic) {
    color: inherit;
  }
  .fv-head {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 0 2ch;
    padding: 0 1ch;
    color: var(--fg-strong);
    background: var(--bg-active);
  }
  .file-view pre {
    max-height: calc(var(--lh) * 20);
    padding: 0 1ch;
    overflow: auto;
    color: var(--fg);
    background: var(--bg-input);
  }
  footer {
    display: flex;
    flex: none;
    align-items: baseline;
    gap: 2ch;
    margin-top: var(--lh);
  }
  @media (max-width: 760px) {
    .three,
    .two {
      grid-template-columns: 1fr;
    }
  }
</style>
