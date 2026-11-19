// Small motion helpers shared by the views: numbers that count up, and surfaces that light up under the pointer.

export const reducedMotion = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches

/** The value shown `t` (0..1) of the way from `from` to `to`, easing out; whole numbers only. */
export function countAt(from: number, to: number, t: number): number {
  if (t >= 1) return to
  if (t <= 0) return from
  const eased = 1 - Math.pow(1 - t, 4)
  return Math.round(from + (to - from) * eased)
}

/**
 * Counts the element's number up to `value` when it appears, and from the old value to the new one
 * when it changes. The final text is always exactly `value`.
 */
export function countUp(el: HTMLElement, value: number) {
  let shown = 0
  let frame = 0
  function run(to: number) {
    cancelAnimationFrame(frame)
    const from = shown
    if (reducedMotion() || from === to || !Number.isFinite(to)) {
      shown = to
      el.textContent = String(to)
      return
    }
    const duration = Math.min(900, 320 + Math.abs(to - from) * 14)
    const start = performance.now()
    const step = (now: number) => {
      const t = (now - start) / duration
      shown = countAt(from, to, t)
      el.textContent = String(shown)
      if (t < 1) frame = requestAnimationFrame(step)
    }
    frame = requestAnimationFrame(step)
  }
  run(value)
  return {
    update: run,
    destroy: () => cancelAnimationFrame(frame),
  }
}

/** Tracks the pointer over the element in --mx / --my (pixels), for the `.lift` spotlight. */
export function spotlight(el: HTMLElement) {
  let frame = 0
  function move(e: PointerEvent) {
    if (frame) return
    frame = requestAnimationFrame(() => {
      frame = 0
      const r = el.getBoundingClientRect()
      el.style.setProperty('--mx', `${e.clientX - r.left}px`)
      el.style.setProperty('--my', `${e.clientY - r.top}px`)
    })
  }
  el.addEventListener('pointermove', move, { passive: true })
  return {
    destroy() {
      cancelAnimationFrame(frame)
      el.removeEventListener('pointermove', move)
    },
  }
}
