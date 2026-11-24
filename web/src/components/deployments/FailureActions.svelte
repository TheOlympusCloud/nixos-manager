<script lang="ts">
  import type { MachineDeployment } from '../../lib/api'
  import { rollbackOf } from '../../lib/deployments'
  import { endTrial, repo, rollBack } from '../../lib/repo.svelte'
  import Icon from '../Icon.svelte'

  /** What to do about a failed deployment: end the trial that failed, or revert what broke the machine on its main branch. */
  let { m }: { m: MachineDeployment } = $props()
  const trial = $derived(!!m.testing && m.branch === m.testing)
  const r = $derived(rollbackOf(m))
</script>

{#if trial || r}
  <div class="fix">
    {#if trial}
      <button class="small" disabled={!!repo.busy} onclick={() => endTrial(m)} title="Point {m.testing} back at {m.main}, so comin switches {m.host} back to it">
        <Icon name="undo" /> End the trial
      </button>
    {:else if r}
      <button class="small danger" disabled={!!repo.busy} onclick={() => rollBack(m)} title="Stage the revert and run the checks; then commit and push it">
        <Icon name="undo" /> {r.label}
      </button>
    {/if}
  </div>
{/if}

<style>
  .fix {
    display: flex;
    flex-wrap: wrap;
    gap: 0 2ch;
    margin-top: var(--lh);
  }
</style>
