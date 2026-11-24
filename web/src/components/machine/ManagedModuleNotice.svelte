<script lang="ts">
  import type { Host } from '../../lib/api'
  import { frame } from '../../lib/ascii'
  import { updateManagedModule } from '../../lib/fleet.svelte'
  import { codeLink } from '../../lib/router.svelte'
  import Icon from '../Icon.svelte'

  /** A machine whose modules/managed.nix predates removed packages, overrides and home-manager. */
  let { host }: { host: Host } = $props()
  const old = $derived(!!host.managedFile && !host.error && (host.managedVersion ?? 1) < 2)
  let busy = $state(false)

  async function update() {
    busy = true
    await updateManagedModule()
    busy = false
  }
</script>

{#if old}
  <section class="card notice" data-tone="warn" use:frame={{ title: 'An older managed.nix' }} role="status">
    <p>
      <Icon name="alert" />
      {host.name} loads a <a class="mono" href={codeLink('modules/managed.nix')}>modules/managed.nix</a> from an older nixos-manager. It applies packages and options set here, but cannot remove or change packages, or manage home-manager users.
    </p>
    <button class="primary" disabled={busy} onclick={update}><Icon name="refresh" /> {busy ? 'Updating…' : 'Update managed.nix'}</button>
  </section>
{/if}

<style>
  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--lh) 2ch;
    margin-bottom: var(--lh);
  }
  p {
    flex: 1;
    min-width: 40ch;
    margin: 0;
  }
</style>
