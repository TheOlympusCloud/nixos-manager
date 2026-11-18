<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import NetworkTab from '../components/machine/NetworkTab.svelte'
  import OverviewTab from '../components/machine/OverviewTab.svelte'
  import PackagesTab from '../components/machine/PackagesTab.svelte'
  import ServicesTab from '../components/machine/ServicesTab.svelte'
  import UsersTab from '../components/machine/UsersTab.svelte'
  import { errorFile, evaluateAgain, fleet, hostByName, loadManaged } from '../lib/fleet.svelte'
  import { arch } from '../lib/format'
  import { codeLink, machineLink } from '../lib/router.svelte'
  import { copyText } from '../lib/util'

  let { host: name, tab }: { host: string; tab: string } = $props()

  const host = $derived(hostByName(name))
  const pending = $derived(fleet.pending.includes(name) && fleet.computing)
  onMount(() => {
    loadManaged(name)
  })

  const tabs = $derived(
    host && !host.error
      ? [
          { id: 'overview', label: 'Overview' },
          { id: 'packages', label: 'Packages', n: host.packages.length },
          { id: 'services', label: 'Services & programs', n: host.features.length },
          { id: 'users', label: 'Users', n: host.users.length },
          { id: 'network', label: 'Network & storage' },
        ]
      : [],
  )
  const line = $derived(`(import ./modules/managed.nix ./hosts/${name}/managed.json)`)
  let copied = $state(false)
  async function copyLine() {
    copied = await copyText(line)
    setTimeout(() => (copied = false), 2000)
  }
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate>
      {#if !host}
        <div class="empty-state">
          <h2>No machine called “{name}”</h2>
          <p>It is not in <span class="mono">nixosConfigurations</span> of the working copy. It may have been renamed or removed.</p>
          <div class="row"><a class="btn" href="#/machines">All machines</a></div>
        </div>
      {:else}
        <header class="mh">
          <div class="title-row">
            <span class="dot {host.error ? 'err' : pending ? 'busy' : 'ok'}" aria-hidden="true"></span>
            <h1>{host.name}</h1>
            {#if host.system}<span class="badge mono">{arch(host.system)}</span>{/if}
            {#if host.error}
              <span class="badge err">Does not evaluate</span>
            {:else if pending}
              <span class="badge busy">Re-evaluating</span>
            {:else if host.managedFile}
              <span class="badge ok" title={host.managedFile}>Managed</span>
            {:else}
              <span class="badge">Visual editing off</span>
            {/if}
            <span class="spacer"></span>
            {#if !host.error}<a class="btn" href="#/options/{encodeURIComponent(host.name)}"><Icon name="sliders" size={14} />Options</a>{/if}
          </div>
          {#if !host.error}
            <dl class="facts">
              <div><dt>Host name</dt><dd>{host.hostName}</dd></div>
              <div><dt>System</dt><dd class="mono">{host.system}</dd></div>
              <div><dt>NixOS</dt><dd>{host.release}</dd></div>
              <div><dt>State version</dt><dd>{host.stateVersion}</dd></div>
              {#if host.kernel}<div><dt>Kernel</dt><dd>{host.kernel}</dd></div>{/if}
              {#if host.timeZone}<div><dt>Time zone</dt><dd>{host.timeZone}</dd></div>{/if}
            </dl>
          {/if}
        </header>

        {#if host.error}
          {@const file = errorFile(host.error)}
          <section class="card errcard" role="alert">
            <h2>Evaluation failed</h2>
            <p class="muted">Nix could not evaluate this machine, so its packages and services are unknown. The other machines are not affected.</p>
            <pre>{host.error}</pre>
            <div class="row">
              {#if file}<a class="btn primary" href={codeLink(file)}><Icon name="code" size={14} />Open {file}</a>{/if}
              <a class="btn" href="#/changes">Run checks</a>
              <button class="btn" onclick={evaluateAgain} disabled={fleet.computing}><Icon name="refresh" size={14} />Evaluate again</button>
            </div>
          </section>
        {:else}
          {#if !host.managedFile}
            <section class="card turn-on">
              <div class="to-text">
                <h2>Turn on visual editing</h2>
                <p class="muted">
                  Add this line to {host.name}'s <span class="mono">modules</span> in <span class="mono">flake.nix</span>. The app then keeps the packages and options you set by clicking in
                  <span class="mono">hosts/{host.name}/managed.json</span>; your hand-written Nix stays untouched.
                </p>
                <div class="line">
                  <code>{line}</code>
                  <button class="small" onclick={copyLine}><Icon name={copied ? 'tick' : 'copy'} size={13} />{copied ? 'Copied' : 'Copy'}</button>
                </div>
              </div>
              <a class="btn" href={codeLink('flake.nix')}><Icon name="code" size={14} />Open flake.nix in Code</a>
            </section>
          {/if}

          <nav class="tabs" aria-label="Machine sections">
            {#each tabs as t (t.id)}
              <a href={machineLink(host.name, t.id)} aria-current={tab === t.id ? 'page' : undefined}>
                {t.label}{#if t.n !== undefined}<span class="n num">{t.n}</span>{/if}
              </a>
            {/each}
          </nav>

          <div class="tab-body">
            {#if tab === 'packages'}
              <PackagesTab {host} />
            {:else if tab === 'services'}
              <ServicesTab {host} />
            {:else if tab === 'users'}
              <UsersTab {host} />
            {:else if tab === 'network'}
              <NetworkTab {host} />
            {:else}
              <OverviewTab {host} />
            {/if}
          </div>
        {/if}
      {/if}
    </ModelGate>
  </div>
</div>

<style>
  .mh {
    display: grid;
    gap: 12px;
    margin-bottom: var(--s4);
  }
  .title-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .title-row .dot {
    width: 10px;
    height: 10px;
  }
  h1 {
    font-size: var(--fs-2xl);
    letter-spacing: -0.015em;
  }
  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 28px;
    margin: 0;
  }
  .facts div {
    display: grid;
    gap: 1px;
  }
  .facts dt {
    color: var(--fg-muted);
    font-size: var(--fs-xs);
  }
  .facts dd {
    margin: 0;
    color: var(--fg-strong);
  }
  .errcard,
  .turn-on {
    padding: var(--s4) var(--s5);
  }
  .errcard {
    display: grid;
    gap: 10px;
    border-color: color-mix(in srgb, var(--red) 30%, var(--border));
  }
  .errcard h2,
  .turn-on h2 {
    font-size: var(--fs-lg);
  }
  .errcard pre {
    margin: 0;
    padding: 12px;
    overflow: auto;
    color: color-mix(in srgb, var(--red) 70%, var(--fg-strong));
    font: 12px/1.55 var(--mono);
    white-space: pre-wrap;
    background: var(--bg-deep);
    border-radius: var(--r);
  }
  .row {
    display: flex;
    gap: 8px;
  }
  .turn-on {
    display: flex;
    align-items: flex-start;
    gap: var(--s5);
    margin-bottom: var(--s4);
    background: color-mix(in srgb, var(--accent-strong) 8%, var(--bg-raised));
    border-color: color-mix(in srgb, var(--accent) 22%, var(--border));
  }
  .to-text {
    display: grid;
    flex: 1;
    gap: 8px;
    min-width: 0;
  }
  .to-text p {
    max-width: 80ch;
    line-height: 1.55;
  }
  .line {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .line code {
    padding: 6px 10px;
    overflow-x: auto;
    color: var(--fg-strong);
    white-space: nowrap;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .tabs {
    position: sticky;
    top: 0;
    z-index: 3;
    display: flex;
    gap: 4px;
    margin: 0 calc(-1 * var(--s5));
    padding: 0 var(--s5);
    overflow-x: auto;
    background: var(--bg);
    border-bottom: 1px solid var(--line);
  }
  .tabs a {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 40px;
    padding: 0 12px;
    color: var(--fg-muted);
    font-weight: 500;
    white-space: nowrap;
    text-decoration: none;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }
  .tabs a:hover {
    color: var(--fg-strong);
  }
  .tabs a[aria-current='page'] {
    color: var(--fg-strong);
    border-bottom-color: var(--accent);
  }
  .n {
    padding: 0 6px;
    color: var(--fg);
    font-size: var(--fs-xs);
    line-height: 17px;
    background: var(--bg-hover);
    border-radius: 9px;
  }
  .tab-body {
    padding-top: var(--s4);
  }
</style>
