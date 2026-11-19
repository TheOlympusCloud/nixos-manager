<script lang="ts">
  /** An on/off switch. `onchange` gets the new value; the parent decides whether it sticks. */
  let {
    checked,
    label,
    disabled = false,
    busy = false,
    onchange,
  }: { checked: boolean; label: string; disabled?: boolean; busy?: boolean; onchange: (on: boolean) => void } = $props()
</script>

<button
  type="button"
  class="switch"
  class:on={checked}
  class:busy
  role="switch"
  aria-checked={checked}
  aria-label={label}
  title={label}
  disabled={disabled || busy}
  onclick={() => onchange(!checked)}>
  <span class="knob"></span>
</button>

<style>
  .switch {
    position: relative;
    flex: none;
    width: 34px;
    height: 20px;
    padding: 0;
    background: var(--bg-active);
    border: 1px solid var(--line-strong);
    border-radius: 10px;
    transition:
      background 220ms var(--ease),
      border-color 220ms var(--ease),
      box-shadow 220ms var(--ease);
  }
  .switch:hover:not(:disabled) {
    background: #2d3845;
  }
  .switch.on {
    background: var(--accent-strong);
    border-color: var(--accent-strong);
    box-shadow: 0 0 14px -3px color-mix(in srgb, var(--accent-strong) 90%, transparent);
  }
  .switch.on:hover:not(:disabled) {
    background: var(--accent-hover);
  }
  .knob {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 14px;
    height: 14px;
    background: var(--fg-strong);
    border-radius: 50%;
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.4);
    transition:
      transform 260ms var(--ease-spring),
      width var(--t-fast) var(--ease);
  }
  .on .knob {
    transform: translateX(14px);
  }
  /* The knob squashes toward where it is about to go. */
  .switch:active:not(:disabled) .knob {
    width: 18px;
  }
  .switch.on:active:not(:disabled) .knob {
    transform: translateX(10px);
  }
  .busy .knob {
    animation: pulse 0.9s ease-in-out infinite;
  }
  .switch:disabled {
    opacity: 0.5;
  }
  .switch.busy:disabled {
    opacity: 0.8;
  }
</style>
