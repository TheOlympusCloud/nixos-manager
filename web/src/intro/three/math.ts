// Pure math shared by the scene modules. No three.js here so it stays trivially testable.

export const TAU = Math.PI * 2

export const clamp01 = (x: number): number => (x < 0 ? 0 : x > 1 ? 1 : x)
export const lerp = (a: number, b: number, t: number): number => a + (b - a) * t
export const smoothstep = (a: number, b: number, x: number): number => {
  const t = clamp01((x - a) / (b - a))
  return t * t * (3 - 2 * t)
}

export const linear = (t: number): number => t
export const easeInOut = (t: number): number => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2)
export const easeOut = (t: number): number => 1 - (1 - t) ** 3
export const easeIn = (t: number): number => t * t * t
export const easeOutExpo = (t: number): number => (t >= 1 ? 1 : 1 - 2 ** (-10 * t))
export const easeInOutQuart = (t: number): number => (t < 0.5 ? 8 * t ** 4 : 1 - (-2 * t + 2) ** 4 / 2)

/** mulberry32: a tiny seeded generator, so the scene is identical on every load. */
export function prng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** n points spread evenly over the unit sphere along a golden-angle spiral, as xyz triples. */
export function fibonacciSphere(n: number): Float32Array {
  const out = new Float32Array(n * 3)
  const golden = Math.PI * (3 - Math.sqrt(5))
  for (let i = 0; i < n; i++) {
    const y = 1 - (2 * (i + 0.5)) / n
    const r = Math.sqrt(1 - y * y)
    const a = golden * i
    out[i * 3] = Math.cos(a) * r
    out[i * 3 + 1] = y
    out[i * 3 + 2] = Math.sin(a) * r
  }
  return out
}

function hash3(x: number, y: number, z: number): number {
  let h = Math.imul(x, 374761393) + Math.imul(y, 668265263) + Math.imul(z, 1274126177)
  h = Math.imul(h ^ (h >>> 13), 1103515245)
  h ^= h >>> 16
  return (h >>> 0) / 4294967296
}

/** Value noise in [0,1]: integer-lattice hashes, smoothly interpolated. */
export function noise3(x: number, y: number, z: number): number {
  const ix = Math.floor(x)
  const iy = Math.floor(y)
  const iz = Math.floor(z)
  const fx = x - ix
  const fy = y - iy
  const fz = z - iz
  const ux = fx * fx * (3 - 2 * fx)
  const uy = fy * fy * (3 - 2 * fy)
  const uz = fz * fz * (3 - 2 * fz)
  const x00 = lerp(hash3(ix, iy, iz), hash3(ix + 1, iy, iz), ux)
  const x10 = lerp(hash3(ix, iy + 1, iz), hash3(ix + 1, iy + 1, iz), ux)
  const x01 = lerp(hash3(ix, iy, iz + 1), hash3(ix + 1, iy, iz + 1), ux)
  const x11 = lerp(hash3(ix, iy + 1, iz + 1), hash3(ix + 1, iy + 1, iz + 1), ux)
  return lerp(lerp(x00, x10, uy), lerp(x01, x11, uy), uz)
}

export function fbm3(x: number, y: number, z: number, octaves = 4): number {
  let sum = 0
  let amp = 0.5
  let norm = 0
  let f = 1
  for (let i = 0; i < octaves; i++) {
    sum += amp * noise3(x * f + i * 19.1, y * f + i * 7.3, z * f + i * 3.7)
    norm += amp
    amp *= 0.5
    f *= 2.03
  }
  return sum / norm
}

/** Continent weight 0..1 for a unit-sphere point: package clusters live where the noise peaks. */
export function continent(x: number, y: number, z: number): number {
  const n = fbm3(x * 2.3 + 7.1, y * 2.3 + 3.3, z * 2.3 + 11.7, 4)
  return smoothstep(0.46, 0.6, n)
}

// Orbits: each ring has its own radius, tilt and node so they never overlap on screen.
export const orbitRadius = (i: number): number => 1.3 + 0.2 * i
export const orbitTilt = (i: number): number => 0.28 + 0.22 * i
export const orbitNode = (i: number): number => (i * TAU) / 3 + 0.4
/** Angular speed in rad/s, slower for the outer rings like real orbits. */
export const orbitSpeed = (i: number): number => 0.14 / Math.sqrt(orbitRadius(i) / 1.5)

export interface Vec3Like {
  x: number
  y: number
  z: number
}

/** Position of machine i at orbit angle a, written into out. */
export function orbitPoint(i: number, a: number, out: Vec3Like): Vec3Like {
  const r = orbitRadius(i)
  const tilt = orbitTilt(i)
  const node = orbitNode(i)
  const px = r * Math.cos(a)
  const pz = r * Math.sin(a)
  const ty = -pz * Math.sin(tilt)
  const tz = pz * Math.cos(tilt)
  out.x = px * Math.cos(node) + tz * Math.sin(node)
  out.y = ty
  out.z = -px * Math.sin(node) + tz * Math.cos(node)
  return out
}

/** Deploy timeline: when the commit pulse for machine i (of n) leaves Forgejo, and when it lands. */
export function pulseWindow(i: number, n: number): { start: number; arrive: number } {
  const slot = 0.78 / Math.max(n, 1)
  const start = 0.1 + i * slot
  return { start, arrive: start + Math.min(0.16, slot * 0.7) }
}
