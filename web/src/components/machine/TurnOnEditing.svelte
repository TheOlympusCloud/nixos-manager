<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../../lib/api'
  import { codeLink } from '../../lib/router.svelte'
  import { app } from '../../lib/state.svelte'
  import { copyText } from '../../lib/util'
  import { editingPreview } from '../../lib/values'
  import Icon from '../Icon.svelte'

  /** How to make a Code only machine Editable here: one line in flake.nix, previewed where it goes. */
  let { host, onclose }: { host: string; onclose: () => void } = $props()

  const line = $derived(`(import ./modules/managed.nix ./hosts/${host}/managed.json)`)
  let preview = $state<ReturnType<typeof editingPreview>>(null)
  let loaded = $state(false)
  const hasModule = $derived(app.entries.some((e) => e.path === 'modules/managed.nix'))
  onMount(() => {
    api.file('flake.nix').then(
      (f) => ((preview = editingPreview(f.content, host)), (loaded = true)),
      () => (loaded = true),
    )
  })

  let copied = $state(false)
  async function copy() {
    copied = await copyText(line)
    setTimeout(() => (copied = false), 2000)
  }
</script>

<section class="card toe" aria-labelledby="toe-title">
  <header>
    <h2 id="toe-title">Turn on editing for {host}</h2>
    <button class="icon" aria-label="Close" onclick={onclose}><Icon name="x" size={15} /></button>
  </header>
  <p class="lead">
    Editing here keeps the packages and options you set by clicking in <span class="mono">hosts/{host}/managed.json</span>. Your hand-written Nix stays as it is; a value set here wins over it.
  </p>

  <ol class="steps">
    <li>
      <span class="n">1</span>
      <div class="step">
        <p>Add this line to {host}'s <span class="mono">modules</span> in <span class="mono">flake.nix</span>:</p>
        <div class="line">
          <code>{line}</code>
          <button class="small" onclick={copy}><Icon name={copied ? 'tick' : 'copy'} size={13} />{copied ? 'Copied' : 'Copy'}</button>
        </div>
        {#if preview}
          <div class="preview" role="figure" aria-label="Where it goes in flake.nix">
            {#each preview.before as l, i (i)}<span class="ctx"><span class="ln">{preview.at - preview.before.length + i}</span>{l}</span>{/each}
            <span class="add"><span class="ln">{preview.at}</span>{preview.line}</span>
            {#each preview.after as l, i (i)}<span class="ctx"><span class="ln">{preview.at + 1 + i}</span>{l}</span>{/each}
          </div>
        {:else if loaded}
          <p class="muted small">flake.nix does not import <span class="mono">./hosts/{host}</span> on a line of its own (a helper function may build the machines): add the line to the list of modules that builds {host}.</p>
        {/if}
        {#if !hasModule}
          <p class="warn small"><Icon name="alert" size={13} />This repository has no <span class="mono">modules/managed.nix</span> yet. Copy it from the starter flake (<span class="mono">templates/fleet</span>) first.</p>
        {/if}
      </div>
    </li>
    <li>
      <span class="n">2</span>
      <div class="step">
        <p>Save, then review and commit it in Changes. {host} becomes <strong>Editable here</strong> as soon as it is evaluated again; <span class="mono">managed.json</span> is created with your first change.</p>
      </div>
    </li>
  </ol>
  <div class="actions">
    <a class="btn primary" href={codeLink('flake.nix')}><Icon name="code" size={14} />Open flake.nix</a>
  </div>
</section>

<style>
  .toe {
    display: grid;
    gap: 12px;
    margin-bottom: var(--s4);
    padding: var(--s4) var(--s5);
    background: color-mix(in srgb, var(--accent-strong) 7%, var(--bg-raised));
    border-color: color-mix(in srgb, var(--accent) 22%, var(--border));
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  h2 {
    font-size: var(--fs-lg);
  }
  .lead {
    max-width: 80ch;
    color: var(--fg);
    line-height: 1.55;
  }
  .steps {
    display: grid;
    gap: 14px;
    margin: 4px 0 0;
    padding: 0;
    list-style: none;
  }
  .steps li {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: 12px;
  }
  .n {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    color: var(--accent);
    font-size: var(--fs-xs);
    font-weight: 600;
    border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
    border-radius: 50%;
  }
  .step {
    display: grid;
    gap: 8px;
    min-width: 0;
    line-height: 1.55;
  }
  .line {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .line code {
    min-width: 0;
    padding: 6px 10px;
    overflow-x: auto;
    color: var(--fg-strong);
    white-space: nowrap;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .preview {
    max-width: 760px;
    margin: 0;
    padding: 8px 0;
    overflow-x: auto;
    font: 12px/1.6 var(--mono);
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r);
  }
  .preview span.ctx,
  .preview span.add {
    display: block;
    padding-right: 12px;
    white-space: pre;
  }
  .ctx {
    color: var(--fg-muted);
  }
  .add {
    color: var(--fg-strong);
    background: color-mix(in srgb, var(--green) 14%, transparent);
    box-shadow: inset 2px 0 0 var(--green);
  }
  .ln {
    display: inline-block;
    width: 44px;
    padding-right: 12px;
    color: var(--fg-faint);
    text-align: right;
    user-select: none;
  }
  .warn {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--yellow);
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
