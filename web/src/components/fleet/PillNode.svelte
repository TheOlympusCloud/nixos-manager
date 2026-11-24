<script lang="ts">
  import { Handle, Position, type NodeProps } from '@xyflow/svelte'
  import { frame } from '../../lib/ascii'
  import { age, plural } from '../../lib/format'
  import type { GNode } from '../../lib/graph'
  import { graphUi } from '../../lib/graphui.svelte'
  import Icon, { type IconName } from '../Icon.svelte'

  /** Features, files, packages, users, homes and flake inputs. */
  let { id, data, sourcePosition, targetPosition }: NodeProps & { data: GNode } = $props()
  const dim = $derived(!!graphUi.lit && !graphUi.lit.has(id))

  const icon: Record<string, IconName> = { file: 'file', package: 'package', user: 'user', home: 'user', input: 'input' }
  const featureIcon: Record<string, IconName> = { services: 'services', programs: 'terminal', virtualisation: 'box', hardware: 'cpu' }

  const sub = $derived.by(() => {
    switch (data.kind) {
      case 'user': {
        const hm = data.hosts.find((h) => h.user.home)?.user.home
        return hm ? `home-manager · ${plural(hm.programs.length, 'program')}` : plural(data.hosts.length, 'machine')
      }
      case 'home':
        return data.home.error ? 'does not evaluate' : `home config · ${plural(data.home.packages.length, 'package')}`
      case 'input':
        return `${data.input.rev.slice(0, 7)} · ${age(data.input.lastModified)}`
      default:
        return ''
    }
  })
  const count = $derived(data.kind === 'feature' || data.kind === 'package' ? data.hosts.length : data.kind === 'file' ? data.hosts.length : 0)
</script>

<div
  class="node pill lift k-{data.kind} {data.kind === 'feature' ? 'g-' + data.group : ''}"
  class:dim
  class:sel={graphUi.selected === id}
  class:tall={!!sub}
  class:err={data.kind === 'home' && !!data.home.error}
  data-tone={data.kind === 'home' && data.home.error ? 'err' : undefined}
  use:frame={{ kind: graphUi.selected === id ? 'heavy' : 'square', lift: true, animate: true }}>
  <Handle type="target" position={targetPosition ?? Position.Left} isConnectable={false} />
  <span class="ico"><Icon name={data.kind === 'feature' ? featureIcon[data.group] ?? 'services' : icon[data.kind]} /></span>
  <span class="text">
    <span class="label" class:mono={data.kind === 'file'}>{data.label}</span>
    {#if sub}<span class="sub">{sub}</span>{/if}
  </span>
  {#if count > 1}<span class="n num" title="Shared by {count} machines">{count}</span>{/if}
  <Handle type="source" position={sourcePosition ?? Position.Right} isConnectable={false} />
</div>
