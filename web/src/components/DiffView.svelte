<script lang="ts" module>
  /** The element id of a file's section, to scroll to it. */
  export const fileAnchor = (path: string) => 'diff-' + path.replace(/[^\w.-]/g, '_')
</script>

<script lang="ts">
  import { parsePatch, patchLines, statusWord, type PatchFile } from '../lib/patch'
  import { codeLink } from '../lib/router.svelte'
  import Icon from './Icon.svelte'

  /** A git diff (`text`), or files parsed already, each with an optional tag ("Staged"). */
  let { text = '', files: given, empty = 'No differences.' }: { text?: string; files?: (PatchFile & { tag?: string })[]; empty?: string } = $props()

  const files = $derived<(PatchFile & { tag?: string })[]>(given ?? parsePatch(text))
  // Long files (flake.lock after an update, generated files) start folded.
  const BIG = 300
  let unfolded = $state<Record<string, boolean>>({})
  let folded = $state<Record<string, boolean>>({})
  const key = (f: PatchFile & { tag?: string }, i: number) => `${i}:${f.path}:${f.tag ?? ''}`
  const isOpen = (f: PatchFile, k: string) => !folded[k] && (patchLines(f) <= BIG || !!unfolded[k])
  const dir = (p: string) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/') + 1) : '')
  const base = (p: string) => p.slice(p.lastIndexOf('/') + 1)
</script>

{#if files.length}
  <div class="diff">
    {#each files as f, i (key(f, i))}
      {@const k = key(f, i)}
      {@const open = isOpen(f, k)}
      <section class="file" id={i === 0 || files[i - 1].path !== f.path ? fileAnchor(f.path) : undefined} aria-label="{f.path}, {statusWord[f.status]}">
        <header class="fh">
          <button class="fold" aria-expanded={open} aria-label="{open ? 'Fold' : 'Unfold'} {f.path}" onclick={() => (open ? (folded[k] = true) : ((folded[k] = false), (unfolded[k] = true)))}>
            <Icon name={open ? 'down' : 'right'} size={14} />
          </button>
          <span class="st st-{f.status}">{statusWord[f.status]}</span>
          <span class="path" title={f.path}><span class="d">{dir(f.path)}</span><strong>{base(f.path)}</strong></span>
          {#if f.from}<span class="from muted small">from <span class="mono">{f.from}</span></span>{/if}
          {#if f.tag}<span class="tag">{f.tag}</span>{/if}
          <span class="spacer"></span>
          {#if !f.binary}<span class="counts num"><span class="a">+{f.adds}</span><span class="r">−{f.dels}</span></span>{/if}
          {#if f.status !== 'deleted'}<a class="open" href={codeLink(f.path)} title="Open {f.path} in Code"><Icon name="code" size={13} />Open</a>{/if}
        </header>
        {#if f.binary}
          <p class="note">Binary file; not shown.</p>
        {:else if !f.hunks.length}
          <p class="note">{f.status === 'renamed' ? 'Renamed without changes.' : 'Empty file.'}</p>
        {:else if !open}
          <button class="unfold" onclick={() => ((folded[k] = false), (unfolded[k] = true))}>Show {patchLines(f).toLocaleString()} lines</button>
        {:else}
          <div class="body">
            {#each f.hunks as h, hi (hi)}
              <div class="hunk">Line {h.start}{#if h.context}<span class="ctx-name">{h.context}</span>{/if}</div>
              {#each h.lines as l, li (li)}
                <div class="ln {l.kind}">
                  <span class="no">{l.old ?? ''}</span><span class="no">{l.new ?? ''}</span><span class="sign">{l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : ''}</span><span class="code">{l.text || ' '}</span>
                </div>
              {/each}
            {/each}
          </div>
        {/if}
      </section>
    {/each}
  </div>
{:else}
  <p class="empty">{empty}</p>
{/if}

<style>
  .diff {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 12px;
    padding: 12px 12px 24px;
  }
  .file {
    min-width: 0;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    scroll-margin-top: 12px;
  }
  .file:global(.flash) {
    border-color: var(--accent);
    transition: border-color 600ms var(--ease);
  }
  .fh {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 38px;
    padding: 4px 10px 4px 4px;
    background: var(--bg-raised);
    border-bottom: 1px solid var(--border);
    border-radius: var(--r-lg) var(--r-lg) 0 0;
  }
  .fold {
    width: 26px;
    height: 26px;
    padding: 0;
    color: var(--fg-muted);
    background: none;
    border: 0;
  }
  .fold:hover:not(:disabled) {
    color: var(--fg-strong);
    background: var(--bg-hover);
  }
  .st {
    --c: var(--yellow);
    flex: none;
    padding: 0 6px;
    color: var(--c);
    font-size: var(--fs-xs);
    font-weight: 600;
    line-height: 18px;
    background: color-mix(in srgb, var(--c) 12%, transparent);
    border-radius: var(--r-sm);
  }
  .st-added {
    --c: var(--green);
  }
  .st-deleted {
    --c: var(--red);
  }
  .st-renamed {
    --c: var(--blue);
  }
  .path {
    min-width: 0;
    overflow: hidden;
    font-family: var(--mono);
    font-size: 12.5px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .path .d {
    color: var(--fg-muted);
  }
  .path strong {
    color: var(--fg-strong);
    font-weight: 600;
  }
  .from {
    white-space: nowrap;
  }
  .tag {
    flex: none;
    padding: 0 6px;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    line-height: 18px;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
  }
  .counts {
    display: inline-flex;
    gap: 6px;
    font: 12px var(--mono);
  }
  .counts .a {
    color: var(--green);
  }
  .counts .r {
    color: var(--red);
  }
  .open {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: 6px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
    text-decoration: none;
  }
  .open:hover {
    color: var(--accent);
  }
  .note {
    margin: 0;
    padding: 10px 14px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .unfold {
    width: 100%;
    height: 34px;
    color: var(--fg-muted);
    font-weight: 400;
    background: none;
    border: 0;
    border-radius: 0;
  }
  .unfold:hover:not(:disabled) {
    color: var(--fg-strong);
    background: var(--bg-hover);
  }
  .body {
    overflow-x: auto;
    border-radius: 0 0 var(--r-lg) var(--r-lg);
    font: 12.5px/1.6 var(--mono);
  }
  .hunk {
    min-width: max-content;
    padding: 3px 12px 3px 110px;
    color: var(--fg-muted);
    font: 500 var(--fs-xs) / 18px var(--sans);
    background: color-mix(in srgb, var(--cyan) 6%, var(--bg-side));
    border-block: 1px solid var(--border);
  }
  .hunk:first-child {
    border-top: 0;
  }
  .ctx-name {
    margin-left: 10px;
    font-family: var(--mono);
    font-weight: 400;
  }
  .ln {
    display: grid;
    grid-template-columns: 44px 44px 18px 1fr;
    min-width: max-content;
  }
  .no {
    padding-right: 8px;
    color: var(--fg-faint);
    font-size: 11px;
    text-align: right;
    user-select: none;
  }
  .sign {
    color: var(--fg-muted);
    text-align: center;
    user-select: none;
  }
  .code {
    padding-right: 16px;
    white-space: pre;
  }
  .add {
    background: color-mix(in srgb, var(--green) 11%, transparent);
  }
  .add .code {
    color: var(--fg-strong);
  }
  .add .sign {
    color: var(--green);
  }
  .add .no {
    color: color-mix(in srgb, var(--green) 55%, var(--fg-faint));
  }
  .del {
    background: color-mix(in srgb, var(--red) 11%, transparent);
  }
  .del .code {
    color: var(--fg-strong);
  }
  .del .sign {
    color: var(--red);
  }
  .del .no {
    color: color-mix(in srgb, var(--red) 55%, var(--fg-faint));
  }
  .ln.note {
    color: var(--fg-muted);
    font-style: italic;
  }
  .empty {
    margin: 16px;
    color: var(--fg-muted);
  }
</style>
