<script lang="ts">
  import type { MachineDeployment } from '../../lib/api'
  import { driftDetail, silenceDetail, stateDetail, stateLabel, stateTone } from '../../lib/deployments'

  /** A machine's deployment state as a small badge, and more when it went silent, drifted or needs a reboot. */
  let { m }: { m: MachineDeployment } = $props()
</script>

<span class="badge dep t-{stateTone[m.state]}" title={stateDetail(m)}><span class="dot" aria-hidden="true"></span>{stateLabel(m)}</span>
{#if m.silent}<span class="badge dep t-warn" title={silenceDetail(m)}><span class="dot" aria-hidden="true"></span>Silent</span>{/if}
{#if m.drift}<span class="badge dep t-warn" title={driftDetail(m)}><span class="dot" aria-hidden="true"></span>{m.drift === 'manual' ? 'Changed by hand' : 'Switch unfinished'}</span>{/if}
{#if m.system?.reboot}<span class="badge dep t-off" title="A deployment changed the kernel, its modules or the initrd: a reboot loads them."><span class="dot" aria-hidden="true"></span>Reboot pending</span>{/if}

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
