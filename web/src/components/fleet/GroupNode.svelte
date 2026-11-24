<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte'
  import { frame } from '../../lib/ascii'
  import type { GNode } from '../../lib/graph'
  import { graphUi, toggleExpanded } from '../../lib/graphui.svelte'
  import Icon from '../Icon.svelte'

  /** A machine's packages, collapsed into one node until expanded. */
  let { id, data, sourcePosition, targetPosition }: NodeProps & { data: GNode & { kind: 'pkgs' } } = $props()
  const dim = $derived(!!graphUi.lit && !graphUi.lit.has(id))
</script>

<div class="node group lift" class:dim class:sel={graphUi.selected === id} use:frame={{ kind: graphUi.selected === id ? 'heavy' : 'square', lift: true, animate: true }}>
  <Handle type="target" position={targetPosition ?? Position.Left} isConnectable={false} />
  <span class="ico"><Icon name="package" /></span>
  <span class="text">
    <span class="label num">{data.label}</span>
    <span class="sub">{data.preview}</span>
  </span>
  <button
    class="icon nodrag"
    aria-label="{data.expanded ? 'Collapse' : 'Expand'} the packages of {data.host}"
    title={data.expanded ? 'Collapse' : 'Show each package'}
    onclick={(e) => {
      e.stopPropagation()
      toggleExpanded(data.host)
    }}>
    <Icon name={data.expanded ? 'minus' : 'plus'} />
  </button>
  <Handle type="source" position={sourcePosition ?? Position.Right} isConnectable={false} />
</div>
