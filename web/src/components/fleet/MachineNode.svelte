<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte'
  import { frame } from '../../lib/ascii'
  import { silenceDetail, stateDetail, stateLabel, stateTone } from '../../lib/deployments'
  import { deployOf } from '../../lib/deployments.svelte'
  import { arch } from '../../lib/format'
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
<div
  class="node machine lift"
  class:dim
  class:sel={graphUi.selected === id}
  class:err={!!data.host.error}
  data-tone={data.host.error ? 'err' : undefined}
  role="presentation"
  ondblclick={() => go(machineLink(data.host.name))}
  use:frame={{ kind: graphUi.selected === id ? 'heavy' : 'round', title: data.host.name, tag: data.host.system ? arch(data.host.system) : undefined, lift: true, animate: true }}>
  <Handle type="target" position={targetPosition ?? Position.Left} isConnectable={false} />
  <div class="card-slot"><MachineCard host={data.host} /></div>
  {#if dep && (dep.silent || ['current', 'behind', 'failed'].includes(dep.state))}
    {@const quiet = dep.silent && dep.state !== 'failed'}
    {@const tone = quiet ? 'off' : stateTone[dep.state]}
    <span class="dep t-{tone}" title={quiet ? silenceDetail(dep) : stateDetail(dep)}><span class="dot {tone}" aria-hidden="true"></span> {depText}</span>
  {/if}
  {#if data.chips.length}
    <div class="rule dash" aria-hidden="true"></div>
    <!-- What only this machine runs: shared features are nodes of their own. -->
    <ul class="solo chips" aria-label="Only on {data.host.name}">
      {#each data.chips as f (f.group + f.name)}
        <li class="chip solo-chip g-{f.group}" class:lit={graphUi.selected === `f:${f.group}.${f.name}`} title={f.description || f.name}>{f.name}</li>
      {/each}
      {#if data.more}<li class="chip solo-chip more" title="{data.more} more: open the details panel to see them all">+{data.more}</li>{/if}
    </ul>
  {/if}
  <Handle type="source" position={sourcePosition ?? Position.Right} isConnectable={false} />
</div>

<style>
  /* The name and the system sit on the frame's top line (MachineCard keeps them for screen readers). */
  .machine :global(.mc > .top) {
    position: absolute;
    width: 1ch;
    height: var(--lh);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
  /* What comin last did here, on the frame's bottom line. */
  .dep {
    position: absolute;
    bottom: 0;
    right: 2ch;
    z-index: 2;
    padding: 0 1ch;
    color: var(--c);
    background: var(--bg);
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
