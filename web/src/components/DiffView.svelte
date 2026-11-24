<script lang="ts" module>
  /** The element id of a file's section, to scroll to it. */
  export const fileAnchor = (path: string) => 'diff-' + path.replace(/[^\w.-]/g, '_')
</script>

<script lang="ts">
  import { parsePatch, patchLines, statusWord, type PatchFile } from '../lib/patch'
  import { codeLink } from '../lib/router.svelte'
  import Icon from './Icon.svelte'
  import { frame } from '../lib/ascii'

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
      <section class="file" use:frame={'square'} id={i === 0 || files[i - 1].path !== f.path ? fileAnchor(f.path) : undefined} aria-label="{f.path}, {statusWord[f.status]}">
        <!-- The header stands on the frame's top line, like a frame title, and sticks while the file scrolls by. -->
        <header class="fh">
          <button class="fold icon" aria-expanded={open} aria-label="{open ? 'Fold' : 'Unfold'} {f.path}" onclick={() => (open ? (folded[k] = true) : ((folded[k] = false), (unfolded[k] = true)))}>
            <Icon name={open ? 'dir-down' : 'dir-right'} />
          </button>
          <span class="path" title={f.path}><span class="d">{dir(f.path)}</span><strong>{base(f.path)}</strong></span>
          {#if f.from}<span class="from muted small">from <span class="mono">{f.from}</span></span>{/if}
          <span class="spacer" aria-hidden="true"></span>
          {#if f.tag}<span class="tag">{f.tag}</span>{/if}
          <span class="st st-{f.status}">{statusWord[f.status]}</span>
          {#if !f.binary}<span class="counts num"><span class="a">+{f.adds}</span><span class="r">−{f.dels}</span></span>{/if}
          {#if f.status !== 'deleted'}<a class="open" href={codeLink(f.path)} title="Open {f.path} in Code"><Icon name="code" /> Open</a>{/if}
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
    gap: var(--lh);
    padding: var(--lh) 1ch;
  }
  .file {
    min-width: 0;
  }
  /* Picked in the file list: the header line flashes (ChangesView adds .flash for two ticks). */
  .file:global(.flash) {
    color: inherit !important;
    background: none !important;
  }
  .file:global(.flash) .fh,
  .file:global(.flash) .fh * {
    color: var(--bg);
    background: var(--fg-strong);
  }
  .fh {
    position: sticky;
    top: 0;
    z-index: 2;
    display: flex;
    align-items: baseline;
    gap: 1ch;
    height: var(--lh);
    margin-top: calc(var(--lh) * -1);
    padding: 0 1ch;
    white-space: nowrap;
    background: var(--bg);
  }
  /* The gap between the name and the counts is the frame line, drawn on. */
  .fh .spacer {
    overflow: hidden;
    color: var(--line);
  }
  .fh .spacer::before {
    content: '──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────';
  }
  .st {
    --c: var(--yellow);
    flex: none;
    color: var(--c);
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
    text-overflow: ellipsis;
  }
  .path .d {
    color: var(--fg-muted);
  }
  .tag {
    flex: none;
    padding: 0 1ch;
    color: var(--fg-muted);
    background: var(--bg-active);
  }
  .counts {
    display: inline-flex;
    flex: none;
    gap: 1ch;
  }
  .counts .a {
    color: var(--green);
  }
  .counts .r {
    color: var(--red);
  }
  .open {
    flex: none;
    margin-left: 1ch;
  }
  /* The code: two line-number columns and a sign in the gutter; added and removed lines are washed whole. */
  .body {
    overflow-x: auto;
  }
  .hunk {
    min-width: max-content;
    color: var(--accent);
    background: var(--bg-hover);
  }
  .hunk::before {
    display: inline-block;
    width: 12ch;
    padding-left: 1ch;
    content: '@@';
  }
  .ctx-name {
    margin-left: 2ch;
    color: var(--fg-muted);
  }
  .ln {
    display: grid;
    grid-template-columns: 5ch 5ch 2ch 1fr;
    min-width: max-content;
  }
  .no {
    padding-right: 1ch;
    color: var(--fg-faint);
    text-align: right;
    user-select: none;
  }
  .sign {
    text-align: center;
    user-select: none;
  }
  .code {
    padding-right: 2ch;
    white-space: pre;
  }
  .add {
    background: color-mix(in srgb, var(--green) 13%, transparent);
  }
  .add .sign {
    color: var(--green);
  }
  .del {
    background: color-mix(in srgb, var(--red) 13%, transparent);
  }
  .del .sign {
    color: var(--red);
  }
  .ln.note {
    color: var(--fg-muted);
  }
</style>
