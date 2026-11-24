<script lang="ts">
  import { onMount } from 'svelte'
  import { fetchModelDiff, type ModelDiff } from '../lib/modeldiff'
  import MachineImpact from './MachineImpact.svelte'

  /** The push dialog's body: what the commits to push change on each machine, against what is pushed. */
  let diff = $state<ModelDiff | null>(null)
  let computing = $state(false)
  let error = $state('')

  onMount(() => {
    let alive = true
    fetchModelDiff({ against: 'pushed' }, () => (computing = true), () => alive).then(
      (r) => ((diff = r), (computing = false)),
      (e) => ((error = e instanceof Error ? e.message : String(e)), (computing = false)),
    )
    return () => (alive = false)
  })
</script>

<div class="push-impact">
  {#if error}<p class="error-text small">{error}</p>{:else}<MachineImpact {diff} {computing} push />{/if}
</div>

<style>
  .push-impact {
    max-height: calc(var(--lh) * 24);
    overflow: auto;
  }
</style>
