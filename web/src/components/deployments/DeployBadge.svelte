<script lang="ts">
  import type { MachineDeployment } from '../../lib/api'
  import { driftDetail, silenceDetail, stateDetail, stateLabel, stateTone } from '../../lib/deployments'

  /** A machine's deployment state as a small badge, and more when it went silent, drifted or needs a reboot. */
  let { m }: { m: MachineDeployment } = $props()
</script>

<span class="badge dep t-{stateTone[m.state]}" title={stateDetail(m)}><span class="dot {stateTone[m.state]}" aria-hidden="true"></span>{stateLabel(m)}</span>
{#if m.silent}<span class="badge dep t-warn" title={silenceDetail(m)}><span class="dot warn" aria-hidden="true"></span>Silent</span>{/if}
{#if m.drift}<span class="badge dep t-warn" title={driftDetail(m)}><span class="dot warn" aria-hidden="true"></span>{m.drift === 'manual' ? 'Changed by hand' : 'Switch unfinished'}</span>{/if}
{#if m.system?.reboot}<span class="badge dep t-off" title="A deployment changed the kernel, its modules or the initrd: a reboot loads them."><span class="dot off" aria-hidden="true"></span>Reboot pending</span>{/if}

<style>
  /* Glyph and word, no fill: ● Up to date, ▲ 1 behind, ✖ Build failed, ○ Not reporting. */
  .dep {
    padding: 0;
    background: none;
  }
  .dep + .dep {
    margin-left: 1ch;
  }
  .dep .dot {
    margin-right: 1ch;
  }
  .t-off {
    color: var(--fg-muted);
  }
</style>
