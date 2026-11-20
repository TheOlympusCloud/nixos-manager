<script lang="ts">
  import { errorFile } from '../lib/fleet.svelte'
  import { firstLine } from '../lib/format'
  import { impactGroups, impactLine, revDistance, shortRev, splitImpact, withSettings, type SettingChange } from '../lib/impact'
  import type { ModelDiff } from '../lib/modeldiff'
  import { codeLink, machineLink } from '../lib/router.svelte'
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
    <div class="rows" aria-hidden="true">
      {#each [0, 1] as k (k)}<div class="row skel-row"><span class="skel" style:width="14%"></span><span class="skel" style:width="{52 - k * 14}%"></span></div>{/each}
    </div>
    {#if computing}<p class="note"><span class="dot busy"></span>Evaluating both versions…</p>{/if}
  {:else}
    {#if diff.headError}
      <div class="notice" role="alert">
        <Icon name="alert" size={15} />
        <div>
          <p><strong>{said.headBroken}</strong>, so every machine is compared with nothing.</p>
          <p class="err-line mono">{firstLine(diff.headError)}</p>
        </div>
      </div>
    {/if}
    {#if diff.error}
      {@const file = errorFile(diff.error)}
      <div class="notice" role="alert">
        <Icon name="alert" size={15} />
        <div>
          <p><strong>{said.broken}</strong>, so it cannot be compared with {said.base}.</p>
          <p class="err-line mono">{firstLine(diff.error)}</p>
          {#if file && !commit && !push}<a class="fix-link" href={codeLink(file)}><Icon name="code" size={13} />Open {file}</a>{/if}
        </div>
      </div>
    {:else}
      {#if inputs.length}
        <ul class="rows inputs" aria-label="Flake inputs">
          {#each inputs as c (c.name)}
            <li class="row input-row">
              <span class="mark up"><Icon name="input" size={13} /></span>
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
              {#if c.compareUrl}<a class="ext" href={c.compareUrl} target="_blank" rel="noreferrer noopener">Commits<Icon name="external" size={12} /></a>{/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if split.changed.length}
        <ul class="rows" aria-label="Machines">
          {#each split.changed as d (d.host)}
            {@const groups = impactGroups(d)}
            <li class="row machine" class:open={isOpen(d.host)}>
              <!-- type=button: in the push dialog this sits in a form, where a button would push -->
              <button type="button" class="head" aria-expanded={isOpen(d.host)} onclick={() => (open[d.host] = !isOpen(d.host))} disabled={!groups.length}>
                <span class="mark {dot[d.status]}"><Icon name="server" size={13} /></span>
                <span class="name">{d.host}</span>
                <span class="line" title={impactLine(d)}>{impactLine(d)}</span>
                {#if groups.length}<span class="chev"><Icon name="down" size={14} /></span>{/if}
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
                    <p class="hint">Size and dependency versions show once this server has built both systems; a check builds the machines it changes.</p>
                  {/if}
                  {#if !push}<a class="open-machine" href={machineLink(d.host)}>Open {d.host}</a>{/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {:else if !inputs.length}
        <p class="calm"><Icon name="tick" size={15} />{quiet || said.calm}</p>
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
    gap: 10px;
  }
  .rows {
    margin: 0;
    padding: 0;
    list-style: none;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    box-shadow: var(--edge);
  }
  .row + .row {
    border-top: 1px solid var(--border);
  }
  .head,
  .input-row {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 40px;
    padding: 6px 12px;
  }
  .head {
    height: auto;
    color: var(--fg);
    font-weight: 400;
    text-align: left;
    background: none;
    border: 0;
    border-radius: var(--r-lg);
  }
  .head:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: transparent;
  }
  .head:disabled {
    opacity: 1;
  }
  .mark {
    --c: var(--fg-muted);
    display: grid;
    flex: none;
    place-items: center;
    width: 24px;
    height: 24px;
    color: var(--c);
    background: color-mix(in srgb, var(--c) 13%, transparent);
    border-radius: 6px;
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
    min-width: 96px;
    color: var(--fg-strong);
    font-weight: 600;
  }
  .name.mono {
    font-weight: 500;
  }
  .line {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--fg);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arrow {
    margin: 0 6px;
    color: var(--fg-muted);
  }
  .strong {
    color: var(--fg-strong);
  }
  .chev {
    flex: none;
    color: var(--fg-muted);
    transition: transform var(--t-fast) var(--ease);
  }
  .open .chev {
    transform: rotate(180deg);
  }
  .ext {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: 4px;
    font-size: var(--fs-sm);
    text-decoration: none;
  }
  .ext:hover {
    text-decoration: underline;
  }
  .details {
    display: grid;
    grid-template-columns: 88px minmax(0, 1fr);
    gap: 8px 12px;
    padding: 2px 14px 14px 46px;
  }
  .details .chip {
    max-width: 100%;
    overflow-wrap: anywhere;
  }
  @media (max-width: 640px) {
    .details {
      grid-template-columns: minmax(0, 1fr);
      padding-left: 14px;
    }
  }
  .g-label {
    padding-top: 2px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
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
  .open-machine,
  .hint {
    grid-column: 2;
    justify-self: start;
    font-size: var(--fs-sm);
  }
  .hint {
    margin: 0;
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
    gap: 10px;
    padding: 12px 14px;
    color: var(--fg);
    background: color-mix(in srgb, var(--red) 6%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--red) 32%, var(--border));
    border-radius: var(--r-lg);
  }
  .notice > :global(svg) {
    flex: none;
    margin-top: 2px;
    color: var(--red);
  }
  .notice div {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .err-line {
    overflow: hidden;
    color: color-mix(in srgb, var(--red) 65%, var(--fg-strong));
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .fix-link {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    width: fit-content;
    font-size: var(--fs-sm);
  }
  .calm {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--fg);
  }
  .calm :global(svg) {
    color: var(--green);
  }
  .note {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 8px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .skel-row {
    display: flex;
    gap: 16px;
    align-items: center;
    height: 40px;
    padding: 0 12px;
  }
</style>
