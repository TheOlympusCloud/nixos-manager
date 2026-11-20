<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type CommitDetail } from '../lib/api'
  import type { SettingChange } from '../lib/impact'
  import { fetchModelDiff, managedFiles, managedSettings, type ModelDiff } from '../lib/modeldiff'
  import { parsePatch } from '../lib/patch'
  import { repo, revertCommit } from '../lib/repo.svelte'
  import { ago, copyText } from '../lib/util'
  import DiffView from './DiffView.svelte'
  import Icon from './Icon.svelte'
  import MachineImpact from './MachineImpact.svelte'

  /** One commit: what it did to the machines, then its files. */
  let { rev }: { rev: string } = $props()

  let commit = $state<CommitDetail | null>(null)
  let error = $state('')
  let diff = $state<ModelDiff | null>(null)
  let computing = $state(false)
  let impactError = $state('')
  let settings = $state<Record<string, SettingChange[]>>({})
  let alive = true

  onMount(() => {
    api.commitDetail(rev).then(
      (c) => {
        commit = c
        const changed = parsePatch(c.diff)
        const paths = managedFiles(changed.map((f) => f.path))
        const absent = { before: changed.filter((f) => f.status === 'added').map((f) => f.path), after: changed.filter((f) => f.status === 'deleted').map((f) => f.path) }
        if (paths.length) managedSettings(paths, c.sha + '^', c.sha, absent).then((s) => alive && (settings = s))
      },
      (e) => (error = e instanceof Error ? e.message : String(e)),
    )
    fetchModelDiff({ rev }, () => (computing = true), () => alive).then(
      (r) => ((diff = r), (computing = false)),
      (e) => ((impactError = e instanceof Error ? e.message : String(e)), (computing = false)),
    )
    return () => (alive = false)
  })

  const subject = $derived(commit?.message.split('\n')[0] ?? '')
  const body = $derived(commit?.message.trim().split('\n').slice(1).join('\n').trim() ?? '')
  const files = $derived(commit ? parsePatch(commit.diff) : [])
  let copied = $state(false)
</script>

<div class="cp">
  <header class="head">
    <div class="title">
      <h2>{subject || `Commit ${rev.slice(0, 7)}`}</h2>
      {#if commit}
        <p class="meta">
          <button class="sha mono" title={copied ? 'Copied' : 'Copy the full hash'} onclick={async () => (copied = await copyText(commit!.sha))}>
            {commit.sha.slice(0, 7)}<Icon name={copied ? 'tick' : 'copy'} size={12} />
          </button>
          <span>{commit.author}</span>
          <time datetime={commit.date} title={new Date(commit.date).toLocaleString()}>{ago(commit.date)}</time>
        </p>
      {/if}
    </div>
    {#if commit}
      <button class="revert" disabled={!!repo.busy} onclick={() => revertCommit(commit!.sha, subject)} title="Stage the opposite of this commit, to review and commit">
        <Icon name="undo" size={14} />{repo.busy === 'revert' ? 'Reverting…' : 'Revert this commit'}
      </button>
    {/if}
  </header>

  <div class="scroll-body">
    {#if error}
      <p class="error-text pad" role="alert">{error}</p>
    {:else}
      {#if body}<pre class="message">{body}</pre>{/if}
      <section class="block" aria-labelledby="cp-machines">
        <h3 id="cp-machines">On the machines</h3>
        {#if impactError}<p class="error-text small">{impactError}</p>{:else}<MachineImpact {diff} {computing} {settings} commit />{/if}
      </section>
      <section class="block" aria-labelledby="cp-files">
        <h3 id="cp-files">Files{#if commit}<span class="n num">{files.length}</span>{/if}</h3>
        {#if !commit}
          <div class="skel-files" aria-hidden="true">{#each [0, 1] as k (k)}<span class="skel" style:width="{66 - k * 18}%"></span>{/each}</div>
        {:else}
          <DiffView {files} empty="This commit changes no file." />
        {/if}
      </section>
    {/if}
  </div>
</div>

<style>
  .cp {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .head {
    display: flex;
    flex: none;
    align-items: flex-start;
    gap: var(--s3);
    padding: var(--s4) var(--s5) var(--s3);
    border-bottom: 1px solid var(--border);
  }
  .title {
    flex: 1;
    min-width: 0;
  }
  h2 {
    font-size: var(--fs-xl);
    letter-spacing: -0.01em;
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 12px;
    margin-top: 6px;
    color: var(--fg-muted);
    font-size: var(--fs-sm);
  }
  .sha {
    height: 22px;
    padding: 0 6px;
    gap: 5px;
    color: var(--fg);
    font-size: 11.5px;
    background: var(--bg-raised);
    border-color: var(--border);
  }
  .revert {
    flex: none;
  }
  .scroll-body {
    flex: 1;
    min-height: 0;
    padding: var(--s4) var(--s5) var(--s6);
    overflow: auto;
  }
  .message {
    max-width: 80ch;
    margin: 0 0 var(--s4);
    color: var(--fg);
    font: 13px/1.6 var(--sans);
    white-space: pre-wrap;
  }
  .block {
    max-width: 1180px;
  }
  .block + .block {
    margin-top: var(--s5);
  }
  h3 {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
    font-size: var(--fs-lg);
  }
  .n {
    padding: 0 7px;
    color: var(--fg-muted);
    font-size: var(--fs-xs);
    font-weight: 500;
    line-height: 18px;
    background: var(--bg-active);
    border-radius: 9px;
  }
  .block :global(.diff) {
    padding: 0;
  }
  .skel-files {
    display: grid;
    gap: 12px;
    padding: 16px;
    background: var(--bg-deep);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
  }
  .pad {
    padding: 16px 0;
  }
</style>
