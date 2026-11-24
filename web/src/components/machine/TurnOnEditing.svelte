<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../../lib/api'
  import { codeLink } from '../../lib/router.svelte'
  import { app } from '../../lib/state.svelte'
  import { copyText } from '../../lib/util'
  import { editingPreview } from '../../lib/values'
  import { frame } from '../../lib/ascii'
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

<section class="card toe" aria-labelledby="toe-title" data-tone="accent" use:frame={{ title: `Turn on editing for ${host}`, animate: true }}>
  <header>
    <h2 id="toe-title" class="sr-only">Turn on editing for {host}</h2>
    <button class="icon" aria-label="Close" onclick={onclose}><Icon name="x" /></button>
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
          <button class="small" onclick={copy}><Icon name={copied ? 'tick' : 'copy'} /> {copied ? 'Copied' : 'Copy'}</button>
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
          <p class="warn small"><Icon name="alert" /> This repository has no <span class="mono">modules/managed.nix</span> yet. Copy it from the starter flake (<span class="mono">templates/fleet</span>) first.</p>
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
    <a class="btn primary" href={codeLink('flake.nix')}><Icon name="code" /> Open flake.nix</a>
  </div>
</section>

<style>
  .toe {
    margin-bottom: var(--lh);
  }
  /* The close button sits at the end of the first line; the text runs beside it. */
  header {
    float: right;
    margin-left: 2ch;
  }
  .lead {
    max-width: 80ch;
  }
  .steps {
    display: grid;
    gap: var(--lh);
    margin-top: var(--lh);
  }
  .steps li {
    display: grid;
    grid-template-columns: 3ch minmax(0, 1fr);
    gap: 0 2ch;
  }
  /* The step number in reverse video. */
  .n {
    align-self: start;
    color: var(--on-accent);
    font-weight: 700;
    text-align: center;
    background: var(--accent);
  }
  .step {
    display: grid;
    gap: var(--lh);
    min-width: 0;
  }
  .line {
    display: flex;
    align-items: flex-start;
    gap: 0 2ch;
    min-width: 0;
  }
  .line code {
    min-width: 0;
    padding: 0 1ch;
    overflow-wrap: anywhere;
    color: var(--fg-strong);
    background: var(--bg-input);
  }
  .preview {
    max-width: 96ch;
    background: var(--bg-input);
  }
  .preview span.ctx,
  .preview span.add {
    display: block;
    padding-right: 1ch;
    white-space: pre-wrap;
  }
  .ctx {
    color: var(--fg-muted);
  }
  .add {
    color: var(--fg-strong);
    background: color-mix(in srgb, var(--green) 14%, transparent);
  }
  .ln {
    display: inline-block;
    width: 6ch;
    padding-right: 2ch;
    color: var(--fg-faint);
    text-align: right;
    user-select: none;
  }
  .add .ln {
    color: var(--green);
  }
  .warn {
    color: var(--yellow);
  }
  .warn :global(.ic) {
    color: inherit;
  }
  .actions {
    display: flex;
    gap: 0 2ch;
    margin-top: var(--lh);
  }
</style>
