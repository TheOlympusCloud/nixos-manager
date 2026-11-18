<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type Job } from '../lib/api'
  import { refreshJobs } from '../lib/state.svelte'
  import { report } from '../lib/ui.svelte'
  import { stripAnsi } from '../lib/util'

  let { jobId }: { jobId: string } = $props()

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

  const running = $derived(jobState === 'queued' || jobState === 'running' || jobState === '')
</script>

<div class="view-bar">
  <div class="title">
    <strong>{job?.kind ?? 'job'} {jobId}</strong>
    <span class="state s-{jobState || 'queued'}" role="status">{jobState || 'connecting'}{running ? '…' : ''}</span>
  </div>
  {#if job?.targets?.length}<div class="muted small">targets: {job.targets.join(', ')}</div>{/if}
  {#if job?.tree}<div class="muted small">tree {job.tree.slice(0, 12)}</div>{/if}
</div>
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
  .small {
    font-size: 12px;
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
