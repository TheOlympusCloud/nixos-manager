<script lang="ts">
  import type { Host } from '../../lib/api'
  import { frame } from '../../lib/ascii'
  import { updateManagedModule } from '../../lib/fleet.svelte'
  import { codeLink } from '../../lib/router.svelte'
  import Icon from '../Icon.svelte'

  /** A machine whose modules/managed.nix predates fleet modules (6), app configs in homes (5), secrets in homes (4), packages from another nixpkgs (3), modules/managed.json (2), or also removed packages, overrides and home-manager (1). */
  let { host }: { host: Host } = $props()
  const version = $derived(host.managedVersion ?? 1)
  const old = $derived(!!host.managedFile && !host.error && version < 7)
  const lacks = $derived([
    ...(version < 2 ? ['remove or change packages', 'manage home-manager users'] : []),
    ...(version < 3 ? ['apply what is set for every machine'] : []),
    ...(version < 4 ? ['take a package from another nixpkgs'] : []),
    ...(version < 5 ? ['put secrets in homes'] : []),
    ...(version < 6 ? ["manage apps' settings and keyring entries in homes"] : []),
    'turn fleet modules on',
  ])
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
      {host.name} loads a <a class="mono" href={codeLink('modules/managed.nix')}>modules/managed.nix</a> from an older nixos-manager. It applies packages and options set here, but cannot {lacks.length > 1 ? `${lacks.slice(0, -1).join(', ')} or ${lacks.at(-1)}` : lacks[0]}.
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
