<script lang="ts">
  import type { Host } from '../../lib/api'
  import { initial, plural } from '../../lib/format'

  let { host }: { host: Host } = $props()
</script>

<p class="muted small intro">Users and their home-manager configuration, as evaluated. Edit them in Code; clicking does not change users yet.</p>
<div class="users">
  {#each host.users as u (u.name)}
    <section class="card user">
      <div class="u-head">
        <span class="av">{initial(u.name)}</span>
        <div>
          <h3>{u.name}</h3>
          {#if u.description}<p class="muted small">{u.description}</p>{/if}
        </div>
      </div>
      {#if u.groups.length}
        <div class="row"><span class="label">Groups</span><div class="chips">{#each u.groups as g (g)}<span class="chip">{g}</span>{/each}</div></div>
      {/if}
      {#if u.home}
        <div class="row"><span class="label">Programs</span><div class="chips">{#each u.home.programs as p (p)}<span class="chip g-programs">{p}</span>{:else}<span class="muted small">none</span>{/each}</div></div>
        <div class="row"><span class="label">Packages</span><div class="chips">{#each u.home.packages as p (p)}<span class="chip mono">{p}</span>{:else}<span class="muted small">none</span>{/each}</div></div>
        <p class="muted small">home-manager: {plural(u.home.programs.length, 'program')}, {plural(u.home.packages.length, 'package')}</p>
      {:else}
        <p class="muted small">No home-manager configuration.</p>
      {/if}
    </section>
  {:else}
    <p class="muted">No normal users are defined on {host.name}.</p>
  {/each}
</div>

<style>
  .intro {
    margin-bottom: var(--s3);
  }
  .users {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
    gap: var(--s3);
  }
  .user {
    display: grid;
    gap: 12px;
    padding: 14px 16px;
  }
  .u-head {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .av {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    color: var(--fg-strong);
    font-weight: 600;
    background: color-mix(in srgb, var(--g-programs) 35%, var(--bg-active));
    border-radius: 50%;
  }
  h3 {
    font-size: var(--fs-lg);
  }
  .row {
    display: grid;
    grid-template-columns: 80px 1fr;
    gap: 10px;
  }
  .row .label {
    padding-top: 2px;
  }
</style>
