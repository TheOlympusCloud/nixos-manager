<script lang="ts">
  import type { Host } from '../lib/api'
  import { fleet } from '../lib/fleet.svelte'
  import { arch, firstLine, homeUsers, initial } from '../lib/format'
  import Icon from './Icon.svelte'

  /** Summary of one machine: used as the graph node and on the Machines grid. */
  let { host }: { host: Host } = $props()

  const pending = $derived(fleet.pending.includes(host.name) && fleet.computing)
  const status = $derived(host.error ? 'err' : pending ? 'busy' : 'ok')
  const homes = $derived(homeUsers(host))
</script>

<div class="mc" class:err={!!host.error}>
  <!-- A framed container puts the name and the system on its frame line and hides this row (MachinesView). -->
  <div class="top">
    <span class="name" title={host.name}>{host.name}</span>
    {#if host.system}<span class="arch mono" title={host.system}>{arch(host.system)}</span>{/if}
  </div>
  {#if host.error}
    <p class="errline" title={host.error}>{firstLine(host.error)}</p>
    <span class="badge err foot-badge">Does not evaluate</span>
  {:else}
    <div class="meta">
      <span class="dot {status}" aria-hidden="true"></span>
      <span class="grow">NixOS {host.release}{#if host.hostName && host.hostName !== host.name}<span class="sep">·</span>{host.hostName}{/if}</span>
      <span class="avatars">
        {#each homes as u (u.name)}<span class="av" title="{u.name} (home-manager)">{initial(u.name)}</span>{/each}
      </span>
    </div>
    <div class="counts num">
      <span title="Packages"><Icon name="package" /> {host.packages.length}</span>
      <span title="Services, programs and other features"><Icon name="services" /> {host.features.length}</span>
      <span title="Users"><Icon name="users" /> {host.users.length}</span>
    </div>
    <div class="foot">
      {#if pending}
        <span class="badge busy">Re-evaluating</span>
      {:else if host.managedFile}
        <span class="badge ok here" title="Packages and options can be changed by clicking ({host.managedFile})">Editable here</span>
      {:else}
        <span class="badge code" title="It does not load a managed.json, so it is changed in its Nix files">Code only</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .mc {
    display: flex;
    flex-direction: column;
    height: 100%;
    color: var(--fg);
    text-align: left;
  }
  .top {
    display: flex;
    gap: 2ch;
    min-width: 0;
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg-strong);
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arch {
    color: var(--fg-muted);
  }
  .meta {
    display: flex;
    gap: 1ch;
    min-width: 0;
    white-space: nowrap;
  }
  .sep {
    margin: 0 1ch;
  }
  .avatars {
    display: flex;
    gap: 1ch;
  }
  /* A home-manager user: its initial in reverse video. */
  .av {
    padding: 0 1ch;
    color: var(--on-accent);
    font-weight: 700;
    background: var(--accent);
  }
  .counts {
    display: flex;
    gap: 2ch;
    white-space: nowrap;
  }
  .foot {
    display: flex;
    gap: 2ch;
    margin-top: auto;
  }
  .badge.here::before {
    content: '✎ ';
  }
  .badge.code {
    padding: 0;
    color: var(--fg-muted);
    background: none;
  }
  .badge.code::before {
    content: 'λ ';
  }
  .errline {
    display: -webkit-box;
    overflow: hidden;
    color: var(--red);
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .foot-badge {
    align-self: flex-start;
    margin-top: auto;
  }
</style>
