<script lang="ts">
  import type { Host } from '../../lib/api'
  import { homeUsers, plural } from '../../lib/format'
  import { isRepoFile } from '../../lib/graph'
  import { groupLabel, groupOrder } from '../../lib/matrix'
  import { frame, odometer } from '../../lib/ascii'
  import { codeLink, link, machineLink } from '../../lib/router.svelte'
  import Icon from '../Icon.svelte'
  import DeploymentCard from '../deployments/DeploymentCard.svelte'
  import { app } from '../../lib/state.svelte'
  import InstallCard from './InstallCard.svelte'

  let { host }: { host: Host } = $props()

  const fromRepo = $derived(host.packages.filter((p) => p.files.some(isRepoFile)).length)
  const tiles = $derived([
    { label: 'Packages', value: host.packages.length, note: `${fromRepo} from this repository`, tab: 'packages' },
    { label: 'Services & programs', value: host.features.length, note: plural(host.features.filter((f) => f.group === 'services').length, 'service'), tab: 'services' },
    { label: 'Users', value: host.users.length, note: `${homeUsers(host).length} with home-manager`, tab: 'users' },
    { label: 'Open ports', value: host.ports.tcp.length + host.ports.udp.length, note: host.ports.firewall ? 'firewall on' : 'firewall off', tab: 'network' },
    { label: 'File systems', value: host.fileSystems.length, note: host.fileSystems.map((f) => f.fsType).filter((v, i, a) => a.indexOf(v) === i).join(', '), tab: 'network' },
  ])
  // A machine the wizard gave a disk layout: nixos-anywhere installs it.
  const disko = $derived(app.entries.some((e) => e.path === `hosts/${host.name}/disk-config.nix`))
</script>

<div class="tiles">
  {#each tiles as t (t.label)}
    <a class="tile lift" href={machineLink(host.name, t.tab)} use:frame={{ title: t.label, lift: true, animate: true }}>
      <span class="sr-only">{t.label}</span>
      <pre class="big t-value" use:odometer={t.value}></pre>
      <span class="t-note">{t.note}</span>
    </a>
  {/each}
</div>

<div class="cols">
  <section class="section">
    <h2>Features</h2>
    {#each groupOrder as g (g)}
      {@const fs = host.features.filter((f) => f.group === g)}
      {#if fs.length}
        <div class="group">
          <span class="g-label">{groupLabel[g]}</span>
          <div class="chips">{#each fs as f (f.name)}<span class="chip g-{g}" title={f.description}>{f.name}</span>{/each}</div>
        </div>
      {/if}
    {/each}
    {#if !host.features.length}<p class="muted">No services, programs, virtualisation or hardware support enabled.</p>{/if}
  </section>

  <div>
    {#if disko}<InstallCard host={host.name} />{/if}
    <DeploymentCard host={host.name} />

    <section class="section card" use:frame={{ title: 'Users and access' }}>
      <h2 class="sr-only">Users and access</h2>
      <dl class="kv">
        <dt>Users</dt>
        <dd class="chips">{#each host.users as u (u.name)}<span class="chip" title={u.groups.join(', ')}>{u.name}{u.home ? ' · home-manager' : ''}</span>{:else}<span class="muted">none</span>{/each}</dd>
        <dt>TCP ports</dt>
        <dd class="chips">{#each host.ports.tcp as p (p)}<span class="chip mono num">{p}</span>{:else}<span class="muted">none</span>{/each}</dd>
        <dt>UDP ports</dt>
        <dd class="chips">{#each host.ports.udp as p (p)}<span class="chip mono num">{p}</span>{:else}<span class="muted">none</span>{/each}</dd>
        <dt>File systems</dt>
        <dd class="fs">{#each host.fileSystems as f (f.mountPoint)}<span><span class="mono">{f.mountPoint}</span> <span class="muted">{f.fsType}</span></span>{/each}</dd>
      </dl>
    </section>

    {#if host.secrets}
      <section class="section">
        <h2>Secrets</h2>
        {#if host.secrets.length}
          <ul class="files">
            {#each host.secrets as s (s.name)}
              <li>
                <span class="mono" title={s.name}>{s.path}</span>
                <span class="muted small perm">{s.owner || 'root'} · {s.mode}</span>
                {#if s.file && isRepoFile(s.file)}<a href={codeLink(s.file)} class="mono small from">{s.file}</a>{/if}
              </li>
            {/each}
          </ul>
        {:else}
          <p class="muted">None. A machine gets secrets once its SSH host key is in <a href={link('settings')}>Settings → Secrets</a>.</p>
        {/if}
      </section>
    {/if}

    <section class="section">
      <h2>Configured by</h2>
      <ul class="files">
        {#each host.files as f (f)}
          <li>
            <a href={codeLink(f)}><Icon name="file" /> {f}</a>
            {#if f === host.managedFile}<span class="set" title="Changes made by clicking go here">✎ Set here</span>{/if}
          </li>
        {/each}
      </ul>
    </section>
  </div>
</div>

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(22ch, 1fr));
    gap: var(--lh) 2ch;
  }
  .tile {
    display: grid;
    color: inherit;
  }
  .t-note {
    overflow: hidden;
    color: var(--fg-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(36ch, 2fr);
    gap: 0 3ch;
  }
  .group {
    display: grid;
    grid-template-columns: 18ch 1fr;
    gap: 0 2ch;
  }
  .g-label {
    color: var(--fg-muted);
  }
  .card {
    margin-top: calc(var(--lh) * 2);
  }
  .card + .card,
  :global(.card) + .card {
    margin-top: var(--lh);
  }
  .kv {
    grid-template-columns: 14ch 1fr;
    gap: 0 2ch;
  }
  .fs {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .files li {
    display: flex;
    gap: 2ch;
    padding: 0 1ch;
  }
  .files li:hover {
    background: var(--bg-hover);
  }
  .files a :global(.ic) {
    color: var(--fg-muted);
  }
  .files .perm {
    white-space: nowrap;
  }
  .files .from,
  .files .set {
    margin-left: auto;
  }
  .files .from {
    color: var(--fg-muted);
  }
  .set {
    color: var(--green);
    white-space: nowrap;
  }
  @media (max-width: 1100px) {
    .cols {
      grid-template-columns: 1fr;
    }
  }
</style>
