<script lang="ts">
  import { SvelteFlowProvider } from '@xyflow/svelte'
  import FleetGraph from '../components/fleet/FleetGraph.svelte'
  import ModelGate from '../components/ModelGate.svelte'
  import { fleet } from '../lib/fleet.svelte'

  let { active }: { active: boolean } = $props()
</script>

<ModelGate>
  {#if fleet.model}
    <SvelteFlowProvider>
      <FleetGraph model={fleet.model} {active} />
    </SvelteFlowProvider>
  {/if}
</ModelGate>
