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
  <div class="top">
    <span class="dot {status}" aria-hidden="true"></span>
    <span class="name" title={host.name}>{host.name}</span>
    {#if host.system}<span class="badge arch mono" title={host.system}>{arch(host.system)}</span>{/if}
  </div>
  {#if host.error}
    <p class="errline" title={host.error}>{firstLine(host.error)}</p>
    <span class="badge err foot-badge"><Icon name="alert" size={12} />Does not evaluate</span>
  {:else}
    <div class="meta">
      NixOS {host.release}{#if host.hostName && host.hostName !== host.name}<span class="sep">·</span>{host.hostName}{/if}
    </div>
    <div class="counts num">
      <span title="Packages"><Icon name="package" size={13} />{host.packages.length}</span>
      <span title="Services, programs and other features"><Icon name="services" size={13} />{host.features.length}</span>
      <span title="Users"><Icon name="users" size={13} />{host.users.length}</span>
    </div>
    <div class="foot">
      <span class="avatars">
        {#each homes as u (u.name)}<span class="av" title="{u.name} (home-manager)">{initial(u.name)}</span>{/each}
      </span>
      <span class="spacer"></span>
      {#if pending}
        <span class="badge busy">Re-evaluating</span>
      {:else if host.managedFile}
        <span class="badge ok" title="Packages and options can be edited here ({host.managedFile})">Managed</span>
      {:else}
        <span class="badge" title="This machine does not load a managed file, so it can only be edited as code">Visual editing off</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .mc {
    display: flex;
    flex-direction: column;
    gap: 6px;
    height: 100%;
    padding: 12px 14px;
    color: var(--fg);
    text-align: left;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg-strong);
    font-size: var(--fs-lg);
    font-weight: 600;
    letter-spacing: -0.01em;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arch {
    font-size: 10.5px;
  }
  .meta {
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .sep {
    margin: 0 5px;
  }
  .counts {
    display: flex;
    gap: 14px;
    color: var(--fg);
    font-size: var(--fs-sm);
  }
  .counts span {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .counts :global(svg) {
    color: var(--fg-muted);
  }
  .foot {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: auto;
  }
  .avatars {
    display: flex;
  }
  .av {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    margin-right: -4px;
    color: var(--fg-strong);
    font-size: 10.5px;
    font-weight: 600;
    background: color-mix(in srgb, var(--g-programs) 40%, var(--bg-active));
    border: 2px solid var(--bg-raised);
    border-radius: 50%;
  }
  .errline {
    display: -webkit-box;
    overflow: hidden;
    color: color-mix(in srgb, var(--red) 80%, var(--fg-strong));
    font: 11.5px/1.45 var(--mono);
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .foot-badge {
    align-self: flex-start;
    margin-top: auto;
  }
</style>
