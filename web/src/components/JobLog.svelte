<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type Job } from '../lib/api'
  import { jobTitle, summaryLines } from '../lib/checks'
  import { refreshJobs } from '../lib/state.svelte'
  import { report } from '../lib/ui.svelte'
  import { stripAnsi } from '../lib/util'

  let { jobId, compact = false }: { jobId: string; compact?: boolean } = $props()

  let job = $state<Job | null>(null)
  let output = $state('')
  let jobState = $state('')
  let pre = $state<HTMLElement>()
  let stick = true

  onMount(() => {
    api.job(jobId).then((j) => {
      job = j
      jobState ||= j.state
    }, report)

    // Batch chunks per animation frame; long nix builds stream a lot of small writes.
    let buf = ''
    let frame = 0
    const flush = () => {
      frame = 0
      output = buf
    }
    const es = new EventSource(api.jobEventsUrl(jobId))
    es.addEventListener('open', () => {
      buf = '' // each (re)connect replays the whole log
      flush()
    })
    es.addEventListener('output', (e) => {
      buf += stripAnsi(JSON.parse(e.data))
      frame ||= requestAnimationFrame(flush)
    })
    es.addEventListener('done', (e) => {
      es.close()
      jobState = JSON.parse(e.data).state
      api.job(jobId).then((j) => (job = j), report) // a check's summary comes with the end
      refreshJobs()
    })
    es.addEventListener('error', () => {
      if (es.readyState !== EventSource.CLOSED) return // the browser retries by itself
      api.job(jobId).then((j) => {
        job = j
        jobState = j.state
        buf = stripAnsi(j.output ?? buf)
        flush()
      }, report)
    })
    return () => {
      es.close()
      cancelAnimationFrame(frame)
    }
  })

  $effect(() => {
    void output
    if (stick && pre) pre.scrollTop = pre.scrollHeight
  })

  const kinds: Record<string, string> = { check: 'Check', lock: 'Flake lock', update: 'Input update', model: 'Evaluation', options: 'Option index', packages: 'Package index', system: 'Deployed systems' }
  const running = $derived(jobState === 'queued' || jobState === 'running' || jobState === '')
</script>

{#if !compact}
  <div class="view-bar">
    <div class="title">
      <strong>{job?.commit ? `${jobTitle(job)} · ${jobId}` : `${kinds[job?.kind ?? ''] ?? 'Job'} ${jobId}`}</strong>
      <span class="state s-{jobState || 'queued'}" role="status">{jobState || 'connecting'}{running ? '…' : ''}</span>
    </div>
    {#if job?.targets?.length}<div class="muted small">also dry-built: {job.targets.join(', ')}</div>{/if}
    {#if job?.commit}<div class="muted small">commit <span class="mono">{job.commit.slice(0, 12)}</span>, compared with the last checked commit before it</div>{/if}
    {#if job?.tree}<div class="muted small">tree <span class="mono">{job.tree.slice(0, 12)}</span></div>{/if}
  </div>
{/if}
{#if job?.summary}
  <ul class="summary" aria-label="What the check verified">
    {#each summaryLines(job.summary, !!job.commit) as line (line)}<li>{line}</li>{/each}
  </ul>
{/if}
<!-- svelte-ignore a11y_no_noninteractive_tabindex (a scrollable log must be reachable by keyboard) -->
<pre
  class="log"
  bind:this={pre}
  tabindex="0"
  aria-label="Job output"
  onscroll={() => pre && (stick = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 40)}>{output || (running ? 'Waiting for output…' : '(no output)')}</pre>

<style>
  .title {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .state {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .s-passed {
    color: var(--green);
  }
  .s-failed {
    color: var(--red);
  }
  .s-running {
    color: var(--blue);
  }
  .s-queued {
    color: var(--fg-muted);
  }
  .summary {
    margin: 0;
    padding: 8px 14px 8px 30px;
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .log {
    flex: 1;
    min-height: 0;
    margin: 0;
    padding: 10px 14px 24px;
    overflow: auto;
    font: 12px/1.5 var(--mono);
    white-space: pre-wrap;
    word-break: break-word;
    background: var(--bg-deep);
  }
</style>
