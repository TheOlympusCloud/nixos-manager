<script lang="ts">
  /** An on/off switch: ○── off, ──● on, ─◐─ busy. `onchange` gets the new value; the parent decides whether it sticks. */
  let {
    checked,
    label,
    disabled = false,
    busy = false,
    onchange,
  }: { checked: boolean; label: string; disabled?: boolean; busy?: boolean; onchange: (on: boolean) => void } = $props()
</script>

<!-- app.css draws a role=switch checkbox as ○── / ──●; the box shows the prop until the parent changes it. -->
<input
  type="checkbox"
  class="switch"
  class:busy
  role="switch"
  {checked}
  aria-checked={checked}
  aria-label={label}
  title={label}
  disabled={disabled || busy}
  onchange={(e) => {
    const on = e.currentTarget.checked
    e.currentTarget.checked = checked
    onchange(on)
  }} />

<style>
  .switch:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  /* Busy: the knob is a dot spinning on the clock, half way. */
  .switch.busy:disabled {
    cursor: progress;
  }
  .switch.busy:disabled::before {
    color: var(--accent);
    content: '─◐─';
  }
  :global(:is([data-t8='1'], [data-t8='5'])) .switch.busy:disabled::before {
    content: '─◓─';
  }
  :global(:is([data-t8='2'], [data-t8='6'])) .switch.busy:disabled::before {
    content: '─◑─';
  }
  :global(:is([data-t8='3'], [data-t8='7'])) .switch.busy:disabled::before {
    content: '─◒─';
  }
</style>
