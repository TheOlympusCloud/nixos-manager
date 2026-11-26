<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import DependenciesTab from '../components/machine/DependenciesTab.svelte'
  import EssentialsTab from '../components/machine/EssentialsTab.svelte'
  import HomeTab from '../components/machine/HomeTab.svelte'
  import NetworkTab from '../components/machine/NetworkTab.svelte'
  import OverviewTab from '../components/machine/OverviewTab.svelte'
  import PackagesTab from '../components/machine/PackagesTab.svelte'
  import ServicesTab from '../components/machine/ServicesTab.svelte'
  import ModulesTab from '../components/machine/ModulesTab.svelte'
  import TurnOnEditing from '../components/machine/TurnOnEditing.svelte'
  import DeployBadge from '../components/deployments/DeployBadge.svelte'
  import { deployOf } from '../lib/deployments.svelte'
  import UsersTab from '../components/machine/UsersTab.svelte'
  import { errorFile, evaluateAgain, fleet, hostByName, loadManaged } from '../lib/fleet.svelte'
  import { arch } from '../lib/format'
  import { bigText, frame, ink } from '../lib/ascii'
  import { codeLink, link, machineLink } from '../lib/router.svelte'
  import { copyText } from '../lib/util'

  let { host: name, tab }: { host: string; tab: string } = $props()

  const host = $derived(hostByName(name))
  const deployed = $derived(deployOf(name))
  const pending = $derived(fleet.pending.includes(name) && fleet.computing)
  onMount(() => {
    loadManaged(name)
  })

  const tabs = $derived(
    host && !host.error
      ? [
          { id: 'overview', label: 'Overview' },
          { id: 'essentials', label: 'Essentials' },
          { id: 'packages', label: 'Packages', n: host.packages.length },
          { id: 'dependencies', label: 'Dependencies' },
          { id: 'services', label: 'Services & programs', n: host.features.length },
          { id: 'modules', label: 'Modules', n: host.modules?.filter((m) => m.enable).length },
          { id: 'users', label: 'Users', n: host.users.length },
          { id: 'home', label: 'Home', n: host.users.filter((u) => u.home).length },
          { id: 'network', label: 'Network & storage' },
        ]
      : [],
  )
  let showTurnOn = $state(false)

  // A machine made by the wizard that the evaluation does not list (yet).
  const made = $derived(fleet.created[name])
  let copied = $state(false)
  async function copySnippet(text: string) {
    copied = await copyText(text)
    setTimeout(() => (copied = false), 2000)
  }
</script>

<div class="page">
  <div class="page-inner">
    <ModelGate>
      {#if !host && made}
        <div class="empty-state created" role="status">
          <h2>{made.name} was created</h2>
          {#if fleet.computing}
            <p class="busy-line"><span class="dot busy"></span> Evaluating the fleet with {made.name} in it…</p>
          {:else if !made.registered}
            <p>This flake lists its machines by hand, so add {made.name} to <span class="mono">nixosConfigurations</span> in <span class="mono">flake.nix</span>:</p>
          {:else}
            <p>The flake picks up <span class="mono">hosts/{made.name}/</span> by itself, but the last evaluation does not list it. Check that it evaluates in Changes, or open its files.</p>
          {/if}
          {#if made.snippet && !made.registered}
            <div class="snippet">
              <code>{made.snippet}</code>
              <button class="small" onclick={() => copySnippet(made.snippet!)}><Icon name={copied ? 'tick' : 'copy'} /> {copied ? 'Copied' : 'Copy'}</button>
            </div>
          {/if}
          <ul class="made-files">
            {#each made.files as f (f)}<li><a class="mono" href={codeLink(f)}><Icon name="file" /> {f}</a></li>{/each}
          </ul>
          <div class="row">
            {#if !made.registered}<a class="btn primary" href={codeLink('flake.nix')}><Icon name="code" /> Open flake.nix</a>{/if}
            <a class="btn" href="#/changes">Review in Changes</a>
          </div>
        </div>
      {:else if !host}
        <div class="empty-state">
          <pre class="art" aria-hidden="true">╭───╮
│ ? │
╰───╯</pre>
          <h2>No machine called “{name}”</h2>
          <p>It is not in <span class="mono">nixosConfigurations</span> of the working copy. It may have been renamed or removed.</p>
          <div class="row"><a class="btn" href="#/machines">All machines</a><a class="btn" href={link('machines', [], { new: '1' })}><Icon name="plus" /> New machine</a></div>
        </div>
      {:else}
        <header class="mh">
          <div class="page-head">
            <div class="title">
              <h1 class="big" class:err={!!host.error} aria-label={host.name} use:bigText={host.name}></h1>
              <p class="state-row">
                <!-- The error and busy badges carry their own ✖ and ◐. -->
                {#if !host.error && !pending}<span class="dot ok" aria-hidden="true"></span>{/if}
                {#if host.system}<span class="badge mono">{arch(host.system)}</span>{/if}
                {#if host.error}
                  <span class="badge err">Does not evaluate</span>
                {:else if pending}
                  <span class="badge busy">Re-evaluating</span>
                {:else if host.managedFile}
                  <span class="badge ok here" title="Packages and options can be changed by clicking; they are kept in {host.managedFile}">Editable here</span>
                {:else}
                  <span class="badge code" title="{host.name} does not load a managed.json, so it is changed in its Nix files">Code only</span>
                  {#if !showTurnOn}<button class="link small" onclick={() => (showTurnOn = true)}>Turn on editing</button>{/if}
                {/if}
                {#if deployed && deployed.state !== 'never'}<DeployBadge m={deployed} />{/if}
              </p>
            </div>
            <span class="spacer"></span>
            {#if !host.error}<a class="btn" href={link('options', [host.name])}><Icon name="sliders" /> Options</a>{/if}
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
          <section class="card errcard" role="alert" data-tone="err" use:frame={{ title: 'Evaluation failed' }}>
            <h2 class="sr-only">Evaluation failed</h2>
            <p class="muted">Nix could not evaluate this machine, so its packages and services are unknown. The other machines are not affected.</p>
            <pre>{host.error}</pre>
            <div class="row">
              {#if file}<a class="btn primary" href={codeLink(file)}><Icon name="code" /> Open {file}</a>{/if}
              <a class="btn" href="#/changes">Run checks</a>
              <button class="btn" onclick={evaluateAgain} disabled={fleet.computing}><Icon name="refresh" /> Evaluate again</button>
            </div>
          </section>
        {:else}
          {#if !host.managedFile && showTurnOn}<TurnOnEditing host={host.name} onclose={() => (showTurnOn = false)} />{/if}

          <nav class="tabs" aria-label="Machine sections" use:ink={tab}>
            {#each tabs as t (t.id)}
              <a href={machineLink(host.name, t.id)} aria-current={tab === t.id ? 'page' : undefined}>
                {t.label}{#if t.n !== undefined}<small class="num">{t.n}</small>{/if}
              </a>
            {/each}
          </nav>

          {#key tab}
          <div class="tab-body">
            {#if tab === 'essentials'}
              <EssentialsTab {host} />
            {:else if tab === 'packages'}
              <PackagesTab {host} />
            {:else if tab === 'dependencies'}
              <DependenciesTab {host} />
            {:else if tab === 'services'}
              <ServicesTab {host} />
            {:else if tab === 'modules'}
              <ModulesTab {host} />
            {:else if tab === 'users'}
              <UsersTab {host} />
            {:else if tab === 'home'}
              <HomeTab {host} />
            {:else if tab === 'network'}
              <NetworkTab {host} />
            {:else}
              <OverviewTab {host} />
            {/if}
          </div>
          {/key}
        {/if}
      {/if}
    </ModelGate>
  </div>
</div>

<style>
  .mh {
    margin-bottom: var(--lh);
  }
  .mh .page-head {
    flex-wrap: nowrap;
  }
  .title {
    min-width: 0;
  }
  .state-row {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 2ch;
  }
  .state-row .dot {
    margin-right: -1ch;
  }
  /* The source of the machine's values, in the shared words: ✎ Set here, λ In code. */
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
  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: 0 4ch;
  }
  .facts dt {
    color: var(--fg-muted);
  }
  .facts dd {
    color: var(--fg-strong);
    font-weight: 700;
  }
  .errcard {
    display: grid;
    gap: var(--lh);
    margin-top: var(--lh);
  }
  .errcard pre {
    padding: 0 1ch;
    overflow: auto;
    color: var(--fg);
    white-space: pre-wrap;
    background: var(--bg-input);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
  }
  .empty-state .art {
    width: auto;
    padding: 0;
    overflow: visible;
    color: var(--accent);
    white-space: pre;
    background: none;
  }
  .empty-state h2 {
    text-transform: none;
  }
  .empty-state h2::before {
    content: none;
  }
  .created .busy-line {
    display: flex;
    flex-wrap: wrap;
    gap: 0 1ch;
  }
  .snippet {
    display: flex;
    align-items: flex-start;
    gap: 2ch;
    max-width: 100%;
  }
  .snippet code {
    min-width: 0;
    padding: 0 1ch;
    overflow-wrap: anywhere;
    color: var(--fg-strong);
    background: var(--bg-input);
  }
  .made-files a {
    color: var(--fg);
  }
  .made-files a:hover {
    color: var(--on-accent);
  }
  .tabs {
    position: sticky;
    top: 0;
    z-index: 3;
    background: var(--bg);
  }
  .tabs small {
    margin-left: 1ch;
  }
  .tab-body {
    padding-top: var(--lh);
  }
</style>
