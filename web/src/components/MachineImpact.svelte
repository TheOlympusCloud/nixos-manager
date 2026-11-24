<script lang="ts">
  import { errorFile } from '../lib/fleet.svelte'
  import { firstLine } from '../lib/format'
  import { impactGroups, impactLine, revDistance, shortRev, splitImpact, withSettings, type SettingChange } from '../lib/impact'
  import type { ModelDiff } from '../lib/modeldiff'
  import { codeLink, machineLink } from '../lib/router.svelte'
  import { frame } from '../lib/ascii'
  import Icon from './Icon.svelte'

  /**
   * What a change does to each machine and to the flake inputs.
   * `commit`: the diff is one commit against its parent; `push`: the last commit against what is pushed;
   * otherwise the working copy against the last commit.
   */
  let {
    diff,
    computing = false,
    commit = false,
    push = false,
    quiet = '',
    settings = {},
  }: { diff: ModelDiff | null; computing?: boolean; commit?: boolean; push?: boolean; quiet?: string; settings?: Record<string, SettingChange[]> } = $props()

  const split = $derived(splitImpact(withSettings(diff?.hosts ?? [], settings)))
  const inputs = $derived(diff?.inputs ?? [])
  let open = $state<Record<string, boolean>>({})
  const isOpen = (host: string) => open[host] ?? split.changed.length <= 2
  const dot: Record<string, string> = { added: 'add', changed: 'chg', removed: 'del' }
  // A nixpkgs update moves hundreds of dependencies: long groups show their start until asked.
  const SHOWN = 40
  let all = $state<Record<string, boolean>>({})
  const said = $derived(
    push
      ? { headBroken: 'What is pushed does not evaluate', broken: 'The commit to push does not evaluate', base: 'what is pushed', calm: 'Pushing changes nothing any machine runs.' }
      : commit
        ? { headBroken: 'The commit before this one does not evaluate', broken: 'This commit does not evaluate', base: 'the commit before it', calm: 'This commit changes nothing any machine runs.' }
        : { headBroken: 'The last commit does not evaluate', broken: 'The working copy does not evaluate', base: 'the last commit', calm: 'Nothing any machine runs changes.' },
  )
</script>

<div class="impact" aria-busy={computing}>
  {#if !diff}
    <div class="list" aria-hidden="true">
      {#each [0, 1] as k (k)}<div class="row skel-row"><span class="skel" style:width="12ch"></span><span class="skel" style:width="{48 - k * 12}ch"></span></div>{/each}
    </div>
    {#if computing}<p class="note"><span class="dot busy"></span>Evaluating both versions…</p>{/if}
  {:else}
    {#if diff.headError}
      <div class="notice" role="alert" data-tone="err" use:frame>
        <Icon name="alert" />
        <div>
          <p><strong>{said.headBroken}</strong>, so every machine is compared with nothing.</p>
          <p class="err-line mono">{firstLine(diff.headError)}</p>
        </div>
      </div>
    {/if}
    {#if diff.error}
      {@const file = errorFile(diff.error)}
      <div class="notice" role="alert" data-tone="err" use:frame>
        <Icon name="alert" />
        <div>
          <p><strong>{said.broken}</strong>, so it cannot be compared with {said.base}.</p>
          <p class="err-line mono">{firstLine(diff.error)}</p>
          {#if file && !commit && !push}<a class="fix-link" href={codeLink(file)}><Icon name="code" /> Open {file}</a>{/if}
        </div>
      </div>
    {:else}
      {#if inputs.length}
        <ul class="list inputs" aria-label="Flake inputs">
          {#each inputs as c (c.name)}
            <li class="row input-row">
              <span class="mark up"><Icon name="input" /></span>
              <span class="name mono">{c.name}</span>
              <span class="line">
                {#if c.status === 'updated'}
                  <span class="mono">{shortRev(c.fromRev)}</span><span class="arrow">→</span><span class="mono strong">{shortRev(c.toRev)}</span>
                  {#if revDistance(c)}<span class="muted">· {revDistance(c)}</span>{/if}
                {:else if c.status === 'added'}
                  Added at <span class="mono">{shortRev(c.toRev)}</span>
                {:else}
                  Removed
                {/if}
              </span>
              {#if c.compareUrl}<a class="ext" href={c.compareUrl} target="_blank" rel="noreferrer noopener">Commits <Icon name="external" /></a>{/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if split.changed.length}
        <ul class="list" aria-label="Machines">
          {#each split.changed as d (d.host)}
            {@const groups = impactGroups(d)}
            <li class="row machine" class:open={isOpen(d.host)}>
              <!-- type=button: in the push dialog this sits in a form, where a button would push -->
              <button type="button" class="head bare" aria-expanded={isOpen(d.host)} onclick={() => (open[d.host] = !isOpen(d.host))} disabled={!groups.length}>
                <span class="mark {dot[d.status]}"><Icon name="server" /></span>
                <span class="name">{d.host}</span>
                <span class="line" title={impactLine(d)}>{impactLine(d)}</span>
                {#if groups.length}<span class="chev"><Icon name={isOpen(d.host) ? 'dir-down' : 'dir-right'} /></span>{/if}
              </button>
              {#if isOpen(d.host) && groups.length}
                <div class="details">
                  {#each groups as g (g.label)}
                    {@const key = d.host + '\n' + g.label}
                    <span class="g-label">{g.label}</span>
                    <div class="chips">
                      {#each all[key] ? g.items : g.items.slice(0, SHOWN) as it (it.text)}<span class="chip mono k-{it.kind}" title={it.title}>{it.text}</span>{/each}
                      {#if g.items.length > SHOWN && !all[key]}<button type="button" class="chip" onclick={() => (all[key] = true)}>{g.items.length - SHOWN} more</button>{/if}
                    </div>
                  {/each}
                  {#if d.status === 'changed' && !d.closure}
                    <p class="hint">Size and dependency versions show once this server has built both systems; a check builds the machines it changes, unless the server sets checks.build = false.</p>
                  {/if}
                  {#if !push}<a class="open-machine" href={machineLink(d.host)}>Open {d.host}</a>{/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {:else if !inputs.length}
        <p class="calm"><Icon name="tick" /> {quiet || said.calm}</p>
      {/if}

      {#if split.broken.length || (split.unchanged.length && split.changed.length)}
        <p class="note">
          {#if split.broken.length}
            {#each split.broken as d, k (d.host)}{#if k}, {/if}<a href={machineLink(d.host)}>{d.host}</a>{/each}
            {split.broken.length === 1 ? 'does' : 'do'} not evaluate, so {split.broken.length === 1 ? 'it is' : 'they are'} not compared.
          {/if}
          {#if split.unchanged.length && split.changed.length}Unchanged: {split.unchanged.map((d) => d.host).join(', ')}.{/if}
        </p>
      {/if}
    {/if}
    {#if computing}<p class="note"><span class="dot busy"></span>Evaluating again; this shows the previous comparison.</p>{/if}
  {/if}
</div>

<style>
  .impact {
    display: grid;
  }
  .head,
  .input-row {
    display: flex;
    align-items: baseline;
    gap: 2ch;
    width: 100%;
    padding: 0 1ch;
  }
  .head {
    height: auto;
    color: var(--fg);
    font-weight: 400;
    text-align: left;
  }
  /* A row: a fill under the pointer (reverse video stays for keyboard focus). */
  .head:hover:not(:disabled),
  .input-row:hover {
    color: var(--fg);
    background: var(--bg-hover);
  }
  .head:disabled {
    color: var(--fg);
    cursor: default;
  }
  .mark {
    --c: var(--fg-muted);
    flex: none;
    color: var(--c);
  }
  .mark.add {
    --c: var(--green);
  }
  .mark.chg,
  .mark.up {
    --c: var(--accent);
  }
  .mark.del {
    --c: var(--red);
  }
  .name {
    flex: none;
    min-width: 14ch;
    color: var(--fg-strong);
    font-weight: 700;
  }
  .line {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arrow {
    margin: 0 1ch;
    color: var(--fg-muted);
  }
  .strong {
    color: var(--fg-strong);
  }
  .chev {
    flex: none;
    color: var(--fg-muted);
  }
  .ext {
    flex: none;
  }
  /* Under the machine's name: a label column, then the chips. */
  .details {
    display: grid;
    grid-template-columns: 14ch minmax(0, 1fr);
    gap: 0 2ch;
    padding: 0 1ch var(--lh) 4ch;
  }
  .details .chip {
    max-width: 100%;
  }
  @media (max-width: 640px) {
    .details {
      grid-template-columns: minmax(0, 1fr);
      padding-left: 1ch;
    }
  }
  .g-label {
    color: var(--fg-muted);
  }
  .k-up {
    --c: var(--accent);
  }
  .k-add {
    --c: var(--green);
  }
  .k-del {
    --c: var(--red);
  }
  .chip:is(.k-up, .k-add, .k-del) {
    background: color-mix(in srgb, var(--c) 14%, transparent);
  }
  .open-machine,
  .hint {
    grid-column: 2;
    justify-self: start;
  }
  .hint {
    color: var(--fg-muted);
  }
  @media (max-width: 640px) {
    .open-machine,
    .hint {
      grid-column: 1;
    }
  }
  .notice {
    display: flex;
    gap: 2ch;
  }
  .notice > :global(.ic) {
    color: var(--red);
  }
  .notice div {
    min-width: 0;
  }
  .err-line {
    overflow: hidden;
    color: var(--red);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .calm {
    display: flex;
    gap: 1ch;
  }
  .calm :global(.ic) {
    color: var(--green);
  }
  .note {
    color: var(--fg-muted);
  }
  .note:not(:has(.dot))::before {
    color: var(--accent);
    content: '‣ ';
  }
  .note .dot {
    margin-right: 1ch;
  }
  .skel-row {
    display: flex;
    gap: 2ch;
    padding: 0 1ch;
  }
</style>
