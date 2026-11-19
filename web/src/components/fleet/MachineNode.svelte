<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte'
  import type { GNode } from '../../lib/graph'
  import { graphUi } from '../../lib/graphui.svelte'
  import { go, machineLink } from '../../lib/router.svelte'
  import MachineCard from '../MachineCard.svelte'

  let { id, data, sourcePosition, targetPosition }: NodeProps & { data: GNode & { kind: 'machine' } } = $props()
  const dim = $derived(!!graphUi.lit && !graphUi.lit.has(id))
</script>

<!-- Double-click opens the machine page (from the keyboard: Enter selects, the details panel links to it). -->
<div class="node machine" class:dim class:sel={graphUi.selected === id} class:err={!!data.host.error} role="presentation" ondblclick={() => go(machineLink(data.host.name))}>
  <Handle type="target" position={targetPosition ?? Position.Left} isConnectable={false} />
  <div class="card-slot"><MachineCard host={data.host} /></div>
  {#if data.chips.length}
    <!-- What only this machine runs: shared features are nodes of their own. -->
    <ul class="solo" aria-label="Only on {data.host.name}">
      {#each data.chips as f (f.group + f.name)}
        <li class="solo-chip g-{f.group}" class:lit={graphUi.selected === `f:${f.group}.${f.name}`} title={f.description || f.name}>{f.name}</li>
      {/each}
      {#if data.more}<li class="solo-chip more" title="{data.more} more: open the details panel to see them all">+{data.more}</li>{/if}
    </ul>
  {/if}
  <Handle type="source" position={sourcePosition ?? Position.Right} isConnectable={false} />
</div>
