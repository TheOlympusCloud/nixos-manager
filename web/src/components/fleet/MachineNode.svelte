<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte'
  import { silenceDetail, stateDetail, stateLabel, stateTone } from '../../lib/deployments'
  import { deployOf } from '../../lib/deployments.svelte'
  import type { GNode } from '../../lib/graph'
  import { graphUi } from '../../lib/graphui.svelte'
  import { go, machineLink } from '../../lib/router.svelte'
  import MachineCard from '../MachineCard.svelte'

  let { id, data, sourcePosition, targetPosition }: NodeProps & { data: GNode & { kind: 'machine' } } = $props()
  const dim = $derived(!!graphUi.lit && !graphUi.lit.has(id))
  const dep = $derived(deployOf(data.host.name))
  const depText = $derived(dep?.silent && dep.state !== 'failed' ? 'silent' : dep?.state === 'current' ? 'deployed' : dep ? stateLabel(dep).toLowerCase() : '')
</script>

<!-- Double-click opens the machine page (from the keyboard: Enter selects, the details panel links to it). -->
<div class="node machine" class:dim class:sel={graphUi.selected === id} class:err={!!data.host.error} role="presentation" ondblclick={() => go(machineLink(data.host.name))}>
  <Handle type="target" position={targetPosition ?? Position.Left} isConnectable={false} />
  <div class="card-slot"><MachineCard host={data.host} /></div>
  {#if dep && (dep.silent || ['current', 'behind', 'failed'].includes(dep.state))}
    {@const quiet = dep.silent && dep.state !== 'failed'}
    <span class="dep t-{quiet ? 'off' : stateTone[dep.state]}" title={quiet ? silenceDetail(dep) : stateDetail(dep)}><span class="dot" aria-hidden="true"></span>{depText}</span>
  {/if}
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

<style>
  /* What comin last did here, on the card's top edge. */
  .dep {
    position: absolute;
    top: -9px;
    right: 12px;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 18px;
    padding: 0 7px;
    color: var(--c);
    font-size: 10.5px;
    font-weight: 500;
    line-height: 1;
    white-space: nowrap;
    background: color-mix(in srgb, var(--c) 12%, var(--bg-raised));
    border: 1px solid color-mix(in srgb, var(--c) 40%, var(--line));
    border-radius: 9px;
  }
  .dep .dot {
    width: 6px;
    height: 6px;
  }
  .t-ok {
    --c: var(--green);
  }
  .t-warn {
    --c: var(--yellow);
  }
  .t-err {
    --c: var(--red);
  }
  .t-off {
    --c: var(--fg-muted);
  }
</style>
