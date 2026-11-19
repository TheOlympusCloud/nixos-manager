<script lang="ts">
  import type { MachineDeployment } from '../../lib/api'
  import { stateDetail, stateLabel, stateTone } from '../../lib/deployments'

  /** A machine's deployment state as a small badge. */
  let { m }: { m: MachineDeployment } = $props()
</script>

<span class="badge dep t-{stateTone[m.state]}" title={stateDetail(m)}><span class="dot" aria-hidden="true"></span>{stateLabel(m)}</span>

<style>
  .dep .dot {
    width: 6px;
    height: 6px;
  }
  .t-ok {
    --c: var(--green);
    color: var(--green);
    background: color-mix(in srgb, var(--green) 10%, transparent);
    border-color: color-mix(in srgb, var(--green) 28%, transparent);
  }
  .t-warn {
    --c: var(--yellow);
    color: var(--yellow);
    background: color-mix(in srgb, var(--yellow) 10%, transparent);
    border-color: color-mix(in srgb, var(--yellow) 30%, transparent);
  }
  .t-err {
    --c: var(--red);
    color: var(--red);
    background: color-mix(in srgb, var(--red) 10%, transparent);
    border-color: color-mix(in srgb, var(--red) 30%, transparent);
  }
  .t-off {
    --c: var(--fg-faint);
  }
</style>
