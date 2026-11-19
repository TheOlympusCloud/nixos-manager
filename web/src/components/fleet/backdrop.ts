// The space behind the fleet graph: a field of stars at many depths over a slow nebula, drawn with WebGL.
// Panning and zooming the graph moves the near stars more than the far ones, so the canvas has depth.
import {
  AdditiveBlending, BufferGeometry, Float32BufferAttribute, Mesh, OrthographicCamera, PlaneGeometry, Points, Scene,
  ShaderMaterial, Vector2, WebGLRenderer,
} from 'three'

export interface View {
  x: number
  y: number
  zoom: number
}

export interface Backdrop {
  /** Starts or stops the animation (stop it while the graph is hidden). */
  setActive(on: boolean): void
  dispose(): void
}

/** Deterministic pseudo-random numbers in [0, 1). */
export function prng(seed: number): () => number {
  let s = seed >>> 0
  return () => {
    s = (s + 0x6d2b79f5) >>> 0
    let t = Math.imul(s ^ (s >>> 15), 1 | s)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** Star attributes: position (x, y in 0..1 of the tile; z = depth, 0 far to 1 near), size and a random seed. */
export function starField(count: number, seed = 7) {
  const rand = prng(seed)
  const position = new Float32Array(count * 3)
  const size = new Float32Array(count)
  const phase = new Float32Array(count)
  for (let i = 0; i < count; i++) {
    const depth = Math.pow(rand(), 2.2) // most stars are far away
    position.set([rand(), rand(), depth], i * 3)
    const big = rand()
    size[i] = 0.9 + big * big * big * 2.4 + depth * 1.3
    phase[i] = rand()
  }
  return { position, size, phase }
}

/**
 * True when the measured frames are too slow to keep animating (software rendering, a weak GPU):
 * the backdrop then draws only when the view changes.
 */
export function tooSlow(frameMs: number[], budgetMs = 10): boolean {
  if (frameMs.length < 12) return false
  const sorted = [...frameMs].sort((a, b) => a - b)
  return sorted[Math.floor(sorted.length / 2)] > budgetMs
}

const starVert = /* glsl */ `
attribute float aSize; attribute float aPhase;
uniform vec2 uRes; uniform vec2 uPan; uniform vec2 uPointer; uniform float uZoom; uniform float uTime; uniform float uDpr;
varying float vA; varying float vWarm;
void main() {
  float d = position.z;
  float par = mix(0.03, 0.42, d * d);
  float tile = max(uRes.x, uRes.y) * 1.3;
  vec2 px = mod(position.xy * tile + uPan * par + uPointer * par * 46.0, tile) - tile * 0.5;
  float s = mix(1.0, clamp(pow(uZoom, 0.4), 0.8, 1.45), d);
  px *= s;
  float tw = 0.72 + 0.28 * sin(uTime * (0.5 + aPhase * 1.3) + aPhase * 60.0);
  vA = (0.42 + 0.58 * d) * (0.55 + 0.45 * aPhase) * tw;
  vWarm = fract(aPhase * 7.3);
  gl_PointSize = aSize * uDpr * mix(0.9, 1.25, d) * s;
  gl_Position = vec4(px / (uRes * 0.5), 0.0, 1.0);
}`

const starFrag = /* glsl */ `
varying float vA; varying float vWarm;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.04, d) * vA;
  vec3 col = mix(vec3(0.62, 0.80, 1.0), vec3(1.0, 0.94, 0.86), step(0.82, vWarm));
  gl_FragColor = vec4(col * a, a);
}`

const nebulaVert = /* glsl */ `
varying vec2 vUv;
void main() { vUv = uv; gl_Position = vec4(position.xy, 0.0, 1.0); }`

// Two soft clouds in the NixOS blues, drifting; dim enough that the graph stays the subject.
const nebulaFrag = /* glsl */ `
precision highp float;
varying vec2 vUv;
uniform vec2 uRes; uniform vec2 uPan; uniform float uTime;
float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }
float noise(vec2 p) {
  vec2 i = floor(p), f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1, 0)), f.x), mix(hash(i + vec2(0, 1)), hash(i + vec2(1, 1)), f.x), f.y);
}
float fbm(vec2 p) {
  float v = 0.0, a = 0.5;
  for (int i = 0; i < 4; i++) { v += a * noise(p); p = p * 2.03 + 11.7; a *= 0.5; }
  return v;
}
void main() {
  vec2 p = (vUv - 0.5) * vec2(uRes.x / uRes.y, 1.0);
  vec2 q = p * 1.35 - uPan * 0.00022 * vec2(1.0, -1.0) + vec2(uTime * 0.006, uTime * 0.004);
  float n = fbm(q + fbm(q * 1.7 + uTime * 0.01));
  float m = fbm(q * 0.8 + 31.4);
  vec3 blue = vec3(0.16, 0.33, 0.68), cyan = vec3(0.22, 0.56, 0.74), violet = vec3(0.42, 0.30, 0.78);
  vec3 col = vec3(0.027, 0.036, 0.052);
  col += blue * smoothstep(0.30, 0.80, n) * 0.42;
  col += violet * smoothstep(0.42, 0.85, m) * 0.20;
  col += cyan * smoothstep(0.30, 0.70, n * m * 2.0) * 0.12;
  // A horizon glow low on the canvas, like light coming over the edge of a planet.
  float horizon = smoothstep(0.75, -0.25, length(p - vec2(0.0, -1.25)) - 0.62);
  col += vec3(0.20, 0.42, 0.82) * horizon * 0.30;
  float vig = smoothstep(1.25, 0.25, length(p));
  col *= mix(0.6, 1.0, vig);
  // Dither away the banding dark gradients show on 8-bit screens.
  col += (fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(0.06711056, 0.00583715)))) - 0.5) / 255.0;
  gl_FragColor = vec4(max(col, 0.0), 1.0);
}`

/**
 * Draws the backdrop on `canvas`, following `view()` (the graph's pan and zoom). Throws when WebGL is not
 * available; the caller then keeps the plain background.
 */
export function createBackdrop(canvas: HTMLCanvasElement, view: () => View, opts: { still?: boolean } = {}): Backdrop {
  const renderer = new WebGLRenderer({ canvas, antialias: false, alpha: false, powerPreference: 'low-power' })
  renderer.setClearColor(0x080b10, 1)
  const scene = new Scene()
  const camera = new OrthographicCamera(-1, 1, 1, -1, 0, 1)
  const uniforms = {
    uRes: { value: new Vector2(1, 1) },
    uPan: { value: new Vector2() },
    uPointer: { value: new Vector2() },
    uZoom: { value: 1 },
    uTime: { value: 0 },
    uDpr: { value: 1 },
  }

  const nebulaGeo = new PlaneGeometry(2, 2)
  const nebulaMat = new ShaderMaterial({ uniforms, vertexShader: nebulaVert, fragmentShader: nebulaFrag, depthTest: false, depthWrite: false })
  const nebula = new Mesh(nebulaGeo, nebulaMat)
  nebula.frustumCulled = false
  nebula.renderOrder = 0
  scene.add(nebula)

  const field = starField(1500)
  const starGeo = new BufferGeometry()
  starGeo.setAttribute('position', new Float32BufferAttribute(field.position, 3))
  starGeo.setAttribute('aSize', new Float32BufferAttribute(field.size, 1))
  starGeo.setAttribute('aPhase', new Float32BufferAttribute(field.phase, 1))
  const starMat = new ShaderMaterial({
    uniforms, vertexShader: starVert, fragmentShader: starFrag, blending: AdditiveBlending, transparent: true, depthTest: false, depthWrite: false,
  })
  const stars = new Points(starGeo, starMat)
  stars.frustumCulled = false
  stars.renderOrder = 1
  scene.add(stars)

  let width = 0
  let height = 0
  function resize() {
    const w = canvas.clientWidth
    const h = canvas.clientHeight
    if (!w || !h || (w === width && h === height)) return false
    width = w
    height = h
    const dpr = Math.min(devicePixelRatio || 1, 1.5)
    renderer.setPixelRatio(dpr)
    renderer.setSize(w, h, false)
    uniforms.uRes.value.set(w, h)
    uniforms.uDpr.value = dpr
    return true
  }

  // The pointer shifts the stars a little; eased so it never jumps.
  const pointerTarget = new Vector2()
  function onPointer(e: PointerEvent) {
    pointerTarget.set((e.clientX / innerWidth - 0.5) * -2, (e.clientY / innerHeight - 0.5) * 2)
  }

  let still = !!opts.still
  let active = false
  let frame = 0
  let last = 0
  let lastKey = ''
  const samples: number[] = []
  const started = performance.now()

  function draw(now: number) {
    const v = view()
    uniforms.uPan.value.set(v.x, -v.y)
    uniforms.uZoom.value = v.zoom
    if (!still) {
      uniforms.uTime.value = (now - started) / 1000
      uniforms.uPointer.value.lerp(pointerTarget, 0.06)
    }
    renderer.render(scene, camera)
  }

  function loop(now: number) {
    if (!active) return
    frame = requestAnimationFrame(loop)
    const resized = resize()
    if (!width) return
    if (still) {
      // Nothing moves on its own: draw only when the graph was panned, zoomed or resized.
      const v = view()
      const key = `${v.x.toFixed(1)}|${v.y.toFixed(1)}|${v.zoom.toFixed(3)}`
      if (!resized && key === lastKey) return
      lastKey = key
      draw(now)
      return
    }
    if (now - last < 30 && !resized) return // about 30 frames a second is plenty for something this slow
    last = now
    const t0 = performance.now()
    draw(now)
    if (samples.length < 24) {
      // Software WebGL finishes inside render(); a real GPU returns at once. Either way a slow render shows here.
      samples.push(performance.now() - t0)
      if (tooSlow(samples)) still = true
    }
  }

  function onVisibility() {
    if (document.hidden) cancelAnimationFrame(frame)
    else if (active) frame = requestAnimationFrame(loop)
  }
  document.addEventListener('visibilitychange', onVisibility)
  addEventListener('pointermove', onPointer, { passive: true })

  return {
    setActive(on) {
      if (on === active) return
      active = on
      cancelAnimationFrame(frame)
      lastKey = ''
      if (on && !document.hidden) frame = requestAnimationFrame(loop)
    },
    dispose() {
      active = false
      cancelAnimationFrame(frame)
      document.removeEventListener('visibilitychange', onVisibility)
      removeEventListener('pointermove', onPointer)
      nebulaGeo.dispose()
      nebulaMat.dispose()
      starGeo.dispose()
      starMat.dispose()
      renderer.dispose()
    },
  }
}
