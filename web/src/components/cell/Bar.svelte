<script lang="ts">
  import { bar, on } from '../../lib/ascii'

  // A meter w cells wide; live runs a ▓▒░ edge through the filled part while work goes on.
  let { v, w = 20, live = false, label }: { v: number; w?: number; live?: boolean; label?: string } = $props()
  let t = $state(-1)
  $effect(() => {
    if (!live) return void (t = -1)
    return on((now) => void (t = now))
  })
</script>

<span class="bar" role="meter" aria-valuemin={0} aria-valuemax={1} aria-valuenow={v} aria-label={label}>{@html bar(v, w, t)}</span>
