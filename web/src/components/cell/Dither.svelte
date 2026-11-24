<!-- What a scrim shows instead of a blur: a ░ dither over the dimmed screen. Put it first in a dialog.screen. -->
<div class="dither" aria-hidden="true">{'░ '.repeat(12000)}</div>

<style>
  .dither {
    position: absolute;
    inset: 0;
    z-index: 0;
    overflow: hidden;
    color: var(--border);
    white-space: pre-wrap;
    word-break: break-all;
    opacity: 0.55;
    pointer-events: none;
    user-select: none;
  }
</style>
