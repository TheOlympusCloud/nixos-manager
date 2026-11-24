// Small motion helpers shared by the views. Numbers roll up on the one clock with lib/ascii.ts's odometer.

/** The value shown `t` (0..1) of the way from `from` to `to`, easing out; whole numbers only. */
export function countAt(from: number, to: number, t: number): number {
  if (t >= 1) return to
  if (t <= 0) return from
  const eased = 1 - Math.pow(1 - t, 4)
  return Math.round(from + (to - from) * eased)
}
