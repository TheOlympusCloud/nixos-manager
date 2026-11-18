<script lang="ts">
  import type { Host } from '../../lib/api'
  import { homeUsers, plural } from '../../lib/format'
  import { isRepoFile } from '../../lib/graph'
  import { groupLabel, groupOrder } from '../../lib/matrix'
  import { codeLink, machineLink } from '../../lib/router.svelte'
  import Icon from '../Icon.svelte'

  let { host }: { host: Host } = $props()

  const fromRepo = $derived(host.packages.filter((p) => p.files.some(isRepoFile)).length)
  const tiles = $derived([
    { label: 'Packages', value: host.packages.length, note: `${fromRepo} from this repository`, tab: 'packages' },
    { label: 'Services & programs', value: host.features.length, note: plural(host.features.filter((f) => f.group === 'services').length, 'service'), tab: 'services' },
    { label: 'Users', value: host.users.length, note: `${homeUsers(host).length} with home-manager`, tab: 'users' },
    { label: 'Open ports', value: host.ports.tcp.length + host.ports.udp.length, note: host.ports.firewall ? 'firewall on' : 'firewall off', tab: 'network' },
    { label: 'File systems', value: host.fileSystems.length, note: host.fileSystems.map((f) => f.fsType).filter((v, i, a) => a.indexOf(v) === i).join(', '), tab: 'network' },
  ])
</script>

<div class="tiles">
  {#each tiles as t (t.label)}
    <a class="tile" href={machineLink(host.name, t.tab)}>
      <span class="t-label">{t.label}</span>
      <span class="t-value num">{t.value}</span>
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
    <section class="section">
      <h2>Users and access</h2>
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

    <section class="section">
      <h2>Configured by</h2>
      <ul class="files">
        {#each host.files as f (f)}
          <li>
            <a href={codeLink(f)} class="mono"><Icon name="file" size={13} />{f}</a>
            {#if f === host.managedFile}<span class="badge ok">managed</span>{/if}
          </li>
        {/each}
      </ul>
    </section>
  </div>
</div>

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--s3);
  }
  .tile {
    display: grid;
    gap: 2px;
    padding: 12px 14px;
    color: inherit;
    text-decoration: none;
    background: var(--bg-raised);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    transition: border-color var(--t-fast) var(--ease);
  }
  .tile:hover {
    border-color: var(--line-strong);
  }
  .t-label {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .t-value {
    color: var(--fg-strong);
    font-size: var(--fs-2xl);
    font-weight: 600;
    letter-spacing: -0.02em;
  }
  .t-note {
    overflow: hidden;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(260px, 2fr);
    gap: var(--s5);
  }
  .group {
    display: grid;
    grid-template-columns: 110px 1fr;
    gap: 10px;
    padding: 10px 0;
    border-top: 1px solid var(--border);
  }
  .g-label {
    padding-top: 2px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .kv {
    display: grid;
    grid-template-columns: 96px 1fr;
    gap: 10px 12px;
    margin: 0;
  }
  .kv dt {
    padding-top: 2px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .kv dd {
    margin: 0;
  }
  .fs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
  }
  .files {
    margin: 0;
    padding: 0;
    list-style: none;
    border-top: 1px solid var(--border);
  }
  .files li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 0;
    border-bottom: 1px solid var(--border);
  }
  .files a {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--fg);
    font-size: 12.5px;
    text-decoration: none;
  }
  .files a:hover {
    color: var(--accent);
  }
  .files :global(svg) {
    color: var(--fg-muted);
  }
  @media (max-width: 1100px) {
    .cols {
      grid-template-columns: 1fr;
    }
  }
</style>
