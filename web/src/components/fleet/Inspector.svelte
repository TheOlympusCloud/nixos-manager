<script lang="ts">
  import { cubicOut } from 'svelte/easing'
  import { fly } from 'svelte/transition'
  import type { Model } from '../../lib/api'
  import { openPackageSearch } from '../../lib/dialogs.svelte'
  import { enablePath, errorFile, fleet, managedValue, removePackageByName, toggleFeature } from '../../lib/fleet.svelte'
  import { age, arch, plural } from '../../lib/format'
  import { featureKey, isRepoFile, type GNode } from '../../lib/graph'
  import { graphUi, toggleExpanded } from '../../lib/graphui.svelte'
  import { groupLabel } from '../../lib/matrix'
  import { codeLink, link, machineLink } from '../../lib/router.svelte'
  import { startJob } from '../../lib/state.svelte'
  import { copyText } from '../../lib/util'
  import { sourceOf } from '../../lib/values'
  import Icon from '../Icon.svelte'
  import SourceTag from '../SourceTag.svelte'
  import Switch from '../Switch.svelte'

  let { node, model, onclose }: { node: GNode; model: Model; onclose: () => void } = $props()

  const healthy = $derived(model.hosts.filter((h) => !h.error))
  const kindLabel: Record<string, string> = { machine: 'Machine', feature: '', file: 'Repository file', pkgs: 'Packages', package: 'Package', user: 'User', home: 'Home configuration', input: 'Flake input' }

  let busy = $state('')
  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key
    try {
      await fn()
    } finally {
      busy = ''
    }
  }

  const select = (id: string) => (graphUi.selected = id)
  let copied = $state(false)
</script>


<aside class="inspector" aria-label="Details" in:fly={{ x: 36, duration: 320, easing: cubicOut }} out:fly={{ x: 24, duration: 160 }}>
  <header>
    <div class="head-text">
      <span class="kind">
        {#if node.kind === 'feature'}<span class="dot" style:--c="var(--g-{node.group})"></span>{groupLabel[node.group]}{:else}{kindLabel[node.kind]}{/if}
      </span>
      <h2 class:mono={node.kind === 'file'}>{node.kind === 'pkgs' ? node.host : node.label}</h2>
    </div>
    <button class="icon" aria-label="Close details" title="Close (Esc)" onclick={onclose}><Icon name="x" /></button>
  </header>

  <div class="body">
    {#if node.kind === 'machine'}
      {@const h = node.host}
      {#if h.error}
        {@const file = errorFile(h.error)}
        <p class="lead err-lead">This machine does not evaluate, so nothing else is known about it.</p>
        <pre class="error-pre">{h.error}</pre>
        {#if file}<a class="btn" href={codeLink(file)}><Icon name="code" size={14} />Open {file}</a>{/if}
      {:else}
        <dl class="facts">
          <dt>Host name</dt><dd>{h.hostName}</dd>
          <dt>System</dt><dd class="mono">{arch(h.system)}</dd>
          <dt>NixOS</dt><dd>{h.release} <span class="muted">(state {h.stateVersion})</span></dd>
          {#if h.kernel}<dt>Kernel</dt><dd>{h.kernel}</dd>{/if}
          {#if h.timeZone}<dt>Time zone</dt><dd>{h.timeZone}</dd>{/if}
          <dt>Editing</dt>
          <dd>
            {#if h.managedFile}Editable here{:else}Code only <a class="hint" href={machineLink(h.name)} title="This machine does not load its managed.json; the machine page shows how to turn editing on">Turn on editing</a>{/if}
          </dd>
        </dl>
        <h3>{plural(h.features.length, 'feature')}</h3>
        <div class="chips">
          {#each h.features as f (f.group + f.name)}
            <button class="chip g-{f.group}" onclick={() => select('f:' + featureKey(f.group, f.name))} title={f.description}>{f.name}</button>
          {/each}
        </div>
        <h3>{plural(h.packages.length, 'package')} · {plural(h.users.length, 'user')}</h3>
        <p class="muted small">Open ports: {h.ports.tcp.length ? h.ports.tcp.map((p) => 'tcp/' + p).join(', ') : 'none'}{h.ports.udp.length ? ', ' + h.ports.udp.map((p) => 'udp/' + p).join(', ') : ''}</p>
      {/if}
      <div class="actions">
        <a class="btn primary" href={machineLink(h.name)}><Icon name="server" size={14} />Open machine page</a>
      </div>
    {:else if node.kind === 'feature'}
      {@const n = node}
      {#if n.description}<p class="lead">{n.description}</p>{/if}
      <h3>On {n.hosts.length} of {healthy.length} machines</h3>
      <ul class="rows">
        {#each healthy as h (h.name)}
          {@const has = n.hosts.find((x) => x.host === h.name)}
          <!-- The managed file is the truth for the switch: the model may still be re-evaluating. -->
          {@const fromManaged = fleet.managed[h.name] ? managedValue(h.name, enablePath(n.group, n.name)) === true : !!has && !!h.managedFile && has.files.includes(h.managedFile)}
          {@const handWritten = !!has && has.files.some((f) => isRepoFile(f) && f !== h.managedFile)}
          {@const setHere = !!has && !!h.managedFile && has.files.includes(h.managedFile)}
          {@const byDefault = !!has && !handWritten && !fromManaged && !setHere}
          <!-- Switched here since the last evaluation: the switch is the truth until the new one lands. -->
          {@const waiting = fleet.pending.includes(h.name) && !!h.managedFile && !handWritten && !byDefault && fromManaged !== !!has}
          <li class:off={!has && !waiting}>
            <div class="row-main">
              <a class="host" href={machineLink(h.name, 'services')}>{h.name}</a>
              {#if waiting}
                <span class="pending small"><span class="dot busy"></span>{fromManaged ? 'Turning on' : 'Turning off'}; re-evaluating…</span>
              {:else if has}
                <span class="src-line"><SourceTag source={sourceOf(has.files, h.managedFile)} /></span>
              {:else}
                <span class="muted small">Not enabled</span>
              {/if}
            </div>
            {#if byDefault}
              <span class="muted small side" title="NixOS or a flake input turns it on">On by default</span>
            {:else if h.managedFile && !handWritten}
              <Switch
                checked={waiting ? fromManaged : fromManaged || setHere}
                busy={busy === h.name || (waiting && fleet.computing)}
                label="{n.name} on {h.name}"
                onchange={(on) => run(h.name, () => toggleFeature(h.name, n.group, n.name, on))} />
            {:else if handWritten}
              <span class="muted small side" title="Enabled in a hand-written file: change it there">Change in code</span>
            {:else}
              <a class="muted small side" href={machineLink(h.name)} title="This machine does not load its managed.json: turn on editing from its page">Code only</a>
            {/if}
          </li>
        {/each}
      </ul>
      {@const first = n.hosts[0]?.host ?? healthy[0]?.name}
      {#if first}
        <div class="actions">
          <a class="btn" href={link('options', [first], { q: `${n.group}.${n.name}.` })}><Icon name="sliders" size={14} />Configure options</a>
        </div>
      {/if}
    {:else if node.kind === 'file'}
      {@const path = node.path}
      <h3>Used by {plural(node.hosts.length, 'machine')}</h3>
      <div class="chips">{#each node.hosts as h (h)}<a class="chip" href={machineLink(h)}>{h}</a>{/each}</div>
      {@const provides = [...new Set(healthy.flatMap((h) => h.features.filter((f) => f.files.includes(path)).map((f) => f.group + '|' + f.name)))]}
      {#if provides.length}
        <h3>Enables</h3>
        <div class="chips">
          {#each provides as p (p)}{@const [g, name] = p.split('|')}<span class="chip g-{g}">{name}</span>{/each}
        </div>
      {/if}
      {@const pkgs = [...new Set(healthy.flatMap((h) => h.packages.filter((p) => p.files.includes(path)).map((p) => p.name)))]}
      {#if pkgs.length}
        <h3>Installs {plural(pkgs.length, 'package')}</h3>
        <div class="chips">{#each pkgs as p (p)}<span class="chip mono">{p}</span>{/each}</div>
      {/if}
      <div class="actions">
        <a class="btn primary" href={codeLink(path)}><Icon name="code" size={14} />Open in Code</a>
        <a class="btn" href={link('history', path.split('/'))}><Icon name="history" size={14} />History</a>
      </div>
    {:else if node.kind === 'pkgs'}
      {@const h = model.hosts.find((x) => x.name === node.host)}
      {#if h}
        <p class="lead">{plural(h.packages.length, 'package')} installed system-wide on {h.name}.</p>
        <ul class="pkglist">
          {#each h.packages as p (p.name)}
            <li><span class="mono">{p.name}</span><span class="muted mono small">{p.version}</span></li>
          {/each}
        </ul>
        <div class="actions">
          <button onclick={() => toggleExpanded(h.name)}>{node.expanded ? 'Collapse in graph' : 'Show each package'}</button>
          <a class="btn" href={machineLink(h.name, 'packages')}>Packages page</a>
          {#if h.managedFile}<button class="primary" onclick={() => openPackageSearch('', [h.name])}><Icon name="plus" size={14} />Add</button>{/if}
        </div>
      {/if}
    {:else if node.kind === 'package'}
      {@const name = node.name}
      <h3>Installed on {plural(node.hosts.length, 'machine')}</h3>
      <ul class="rows">
        {#each node.hosts as ref (ref.host)}
          {@const h = model.hosts.find((x) => x.name === ref.host)}
          {@const managed = !!h?.managedFile && ref.files.includes(h.managedFile)}
          <li>
            <div class="row-main">
              <span class="host-line"><a class="host" href={machineLink(ref.host, 'packages')}>{ref.host}</a><span class="mono small muted">{ref.version}</span></span>
              <span class="src-line"><SourceTag source={sourceOf(ref.files, h?.managedFile ?? null)} /></span>
            </div>
            {#if managed}
              <button class="small" disabled={busy === ref.host} onclick={() => run(ref.host, () => removePackageByName(ref.host, name))}>Remove</button>
            {/if}
          </li>
        {/each}
      </ul>
      {@const missing = healthy.filter((h) => h.managedFile && !node.hosts.some((r) => r.host === h.name))}
      {#if missing.length}
        <div class="actions">
          <button class="primary" onclick={() => openPackageSearch(name, missing.map((h) => h.name))}><Icon name="plus" size={14} />Add to {missing.map((h) => h.name).join(', ')}…</button>
        </div>
      {/if}
    {:else if node.kind === 'user'}
      {#each node.hosts as { host, user } (host)}
        <section class="block">
          <h3><a class="host" href={machineLink(host, 'users')}>{host}</a>{#if user.description}<span class="muted">{' · '}{user.description}</span>{/if}</h3>
          {#if user.groups.length}<div class="chips">{#each user.groups as g (g)}<span class="chip">{g}</span>{/each}</div>{/if}
          {#if user.home}
            <p class="small muted sub">home-manager: {plural(user.home.programs.length, 'program')}, {plural(user.home.packages.length, 'package')}</p>
            <div class="chips">
              {#each user.home.programs as p (p)}<span class="chip g-programs">{p}</span>{/each}
              {#each user.home.packages as p (p)}<span class="chip mono">{p}</span>{/each}
            </div>
          {/if}
        </section>
      {/each}
    {:else if node.kind === 'home'}
      {@const home = node.home}
      {#if home.error}
        <pre class="error-pre">{home.error}</pre>
      {:else}
        <p class="lead">A standalone home-manager configuration (<span class="mono">homeConfigurations</span>).</p>
        <h3>{plural(home.programs.length, 'program')}</h3>
        <div class="chips">{#each home.programs as p (p)}<span class="chip g-programs">{p}</span>{/each}</div>
        <h3>{plural(home.packages.length, 'package')}</h3>
        <div class="chips">{#each home.packages as p (p)}<span class="chip mono">{p}</span>{/each}</div>
      {/if}
    {:else if node.kind === 'input'}
      {@const i = node.input}
      <dl class="facts">
        <dt>Source</dt><dd class="mono">{i.url}</dd>
        <dt>Type</dt><dd>{i.type}</dd>
        <dt>Updated</dt><dd>{new Date(i.lastModified * 1000).toLocaleDateString()} <span class="muted">({age(i.lastModified)})</span></dd>
        <dt>Revision</dt>
        <dd class="rev">
          <span class="mono">{i.rev.slice(0, 12)}</span>
          <button class="icon" aria-label="Copy the full revision" title={copied ? 'Copied' : 'Copy the full revision'} onclick={async () => (copied = await copyText(i.rev))}>
            <Icon name={copied ? 'tick' : 'copy'} size={14} />
          </button>
        </dd>
      </dl>
      <p class="muted small">Every machine is built from this input. Updating it changes <span class="mono">flake.lock</span> in the working copy; review and commit it from Changes.</p>
      <div class="actions">
        <button class="primary" onclick={() => startJob({ kind: 'update', inputs: [i.name] })}><Icon name="refresh" size={14} />Update {i.name}</button>
      </div>
    {/if}
    {#if fleet.computing}<p class="stale muted small"><span class="dot busy"></span>Re-evaluating; this shows the last evaluation.</p>{/if}
  </div>
</aside>

<style>
  .inspector {
    position: absolute;
    top: 12px;
    right: 12px;
    bottom: 12px;
    z-index: 10;
    display: flex;
    flex-direction: column;
    width: 360px;
    max-width: calc(100% - 24px);
    background: color-mix(in srgb, var(--bg-side) 84%, transparent);
    border: 1px solid var(--line);
    border-radius: var(--r-lg);
    box-shadow: var(--edge), var(--shadow-2);
    backdrop-filter: blur(18px) saturate(1.3);
  }
  header {
    display: flex;
    align-items: flex-start;
    gap: var(--s2);
    padding: 14px 10px 12px 16px;
    border-bottom: 1px solid var(--border);
  }
  .head-text {
    flex: 1;
    min-width: 0;
  }
  .kind {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .kind .dot {
    background: var(--c);
  }
  h2 {
    margin-top: 2px;
    overflow-wrap: anywhere;
    font-size: var(--fs-xl);
    letter-spacing: -0.01em;
  }
  h2.mono {
    font-size: var(--fs-lg);
  }
  .body {
    flex: 1;
    min-height: 0;
    padding: 4px 16px 16px;
    overflow: auto;
  }
  h3 {
    margin: 18px 0 8px;
    color: var(--fg);
    font-size: var(--fs-md);
  }
  .lead {
    margin-top: 12px;
    line-height: 1.55;
  }
  .err-lead {
    color: var(--fg);
  }
  .error-pre {
    margin: 10px 0;
    padding: 10px;
    overflow: auto;
    color: color-mix(in srgb, var(--red) 70%, var(--fg-strong));
    font: 11.5px/1.5 var(--mono);
    white-space: pre-wrap;
    background: var(--bg-deep);
    border: 1px solid color-mix(in srgb, var(--red) 25%, var(--border));
    border-radius: var(--r);
  }
  .facts {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 16px;
    margin: 14px 0 0;
  }
  .facts dt {
    color: var(--fg-muted);
  }
  .facts dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }
  .rev {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .rev .icon {
    width: 22px;
    height: 22px;
  }
  .rows {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .rows li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 0;
    border-bottom: 1px solid var(--border);
  }
  .rows li.off .host {
    color: var(--fg-muted);
  }
  .src-line {
    display: flex;
    min-width: 0;
  }
  .pending {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--accent);
  }
  .side {
    flex: none;
    text-decoration: none;
  }
  a.side:hover {
    color: var(--fg-strong);
    text-decoration: underline;
  }
  .hint {
    margin-left: 6px;
    font-size: var(--fs-sm);
  }
  .row-main {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 5px;
    min-width: 0;
  }
  .host-line {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .host {
    color: var(--fg-strong);
    font-weight: 500;
    text-decoration: none;
  }
  .host:hover {
    text-decoration: underline;
  }
  a.chip {
    text-decoration: none;
  }
  a.chip:hover {
    border-color: var(--line-strong);
    color: var(--fg-strong);
  }
  .pkglist {
    max-height: 340px;
    margin: 12px 0 0;
    padding: 0;
    overflow: auto;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .pkglist li {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 5px 10px;
    font-size: var(--fs-sm);
  }
  .pkglist li:nth-child(odd) {
    background: color-mix(in srgb, var(--bg-raised) 60%, transparent);
  }
  .block .sub {
    margin: 10px 0 6px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 20px;
  }
  .stale {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 20px;
  }
</style>
