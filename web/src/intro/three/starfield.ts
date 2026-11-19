// A deep starfield far behind everything, and a handful of meteor streaks that cycle near the planet.
import {
  AdditiveBlending, BufferGeometry, Float32BufferAttribute, InstancedBufferAttribute, InstancedMesh, PlaneGeometry,
  Points, ShaderMaterial,
} from 'three'
import { prng } from './math'
import { GLSL_OUT, type Shared } from './uniforms'

export interface Stars {
  points: Points
  uniforms: { uStars: { value: number } }
  dispose(): void
}

export interface Meteors {
  mesh: InstancedMesh
  uniforms: { uMeteors: { value: number } }
  dispose(): void
}

const starVert = /* glsl */ `
attribute float aSize; attribute float aSeed;
uniform float uTime; uniform float uDpr; uniform float uStars;
varying float vA; varying float vWarm;
void main() {
  float tw = 0.7 + 0.3 * sin(uTime * (0.6 + aSeed * 1.2) + aSeed * 60.0);
  vA = uStars * (0.35 + 0.65 * aSeed) * tw;
  vWarm = fract(aSeed * 7.3);
  vec4 mv = modelViewMatrix * vec4(position, 1.0);
  gl_PointSize = aSize * uDpr;
  gl_Position = projectionMatrix * mv;
}`

const starFrag = /* glsl */ `
varying float vA; varying float vWarm;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.05, d) * vA;
  vec3 col = mix(vec3(0.75, 0.85, 1.0), vec3(1.0, 0.95, 0.88), vWarm);
  gl_FragColor = vec4(col * a, a);
  ${GLSL_OUT}
}`

export function createStars(shared: Shared, count = 2600): Stars {
  const rand = prng(99)
  const pos = new Float32Array(count * 3)
  const size = new Float32Array(count)
  const seed = new Float32Array(count)
  for (let i = 0; i < count; i++) {
    // Uniform on a far sphere, slightly denser away from the bottom where the planet sits.
    const u = rand() * 2 - 1
    const a = rand() * Math.PI * 2
    const r = Math.sqrt(1 - u * u)
    const R = 60
    pos.set([Math.cos(a) * r * R, u * R, Math.sin(a) * r * R], i * 3)
    const big = rand()
    size[i] = 0.8 + big * big * 2.2
    seed[i] = rand()
  }
  const geo = new BufferGeometry()
  geo.setAttribute('position', new Float32BufferAttribute(pos, 3))
  geo.setAttribute('aSize', new Float32BufferAttribute(size, 1))
  geo.setAttribute('aSeed', new Float32BufferAttribute(seed, 1))
  const uniforms = { uStars: { value: 1 } }
  const mat = new ShaderMaterial({
    uniforms: { ...shared, ...uniforms }, vertexShader: starVert, fragmentShader: starFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const points = new Points(geo, mat)
  points.frustumCulled = false
  points.renderOrder = -1
  return { points, uniforms, dispose() { geo.dispose(); mat.dispose() } }
}

const meteorVert = /* glsl */ `
attribute vec3 aOrigin; attribute vec3 aDir; attribute float aLen; attribute float aSeed; attribute float aPeriod;
uniform float uTime; uniform float uMeteors;
varying vec2 vUv; varying float vA;
void main() {
  float phase = fract(uTime / aPeriod + aSeed);
  float on = 0.22;
  float k = clamp(phase / on, 0.0, 1.0);
  float vis = smoothstep(0.0, 0.15, k) * (1.0 - smoothstep(0.75, 1.0, k)) * step(phase, on);
  vA = vis * uMeteors;
  vec3 head = aOrigin + aDir * (k * 2.4);
  vec4 mv = modelViewMatrix * vec4(head, 1.0);
  vec3 d = normalize(mat3(modelViewMatrix) * aDir);
  vec3 perp = normalize(vec3(-d.y, d.x, 0.0));
  // position.x runs -0.5..0.5 along the streak, the head at +0.5.
  mv.xyz += d * (position.x - 0.5) * aLen + perp * position.y * 0.014;
  vUv = uv;
  gl_Position = projectionMatrix * mv;
}`

const meteorFrag = /* glsl */ `
varying vec2 vUv; varying float vA;
void main() {
  float along = pow(vUv.x, 2.2);
  float across = 1.0 - smoothstep(0.1, 0.5, abs(vUv.y - 0.5));
  float a = along * across * vA;
  vec3 col = mix(vec3(0.55, 0.75, 1.0), vec3(1.0), along * along);
  gl_FragColor = vec4(col * a * 1.6, a);
  ${GLSL_OUT}
}`

export function createMeteors(shared: Shared, count = 10): Meteors {
  const rand = prng(2024)
  const origin = new Float32Array(count * 3)
  const dir = new Float32Array(count * 3)
  const len = new Float32Array(count)
  const seed = new Float32Array(count)
  const period = new Float32Array(count)
  for (let i = 0; i < count; i++) {
    origin.set([-1.2 + rand() * 3.2, 0.6 + rand() * 2.2, 0.4 + rand() * 1.4], i * 3)
    const dx = 0.25 + (rand() - 0.5) * 0.3
    const dy = -1
    const dz = (rand() - 0.5) * 0.2
    const l = Math.hypot(dx, dy, dz)
    dir.set([dx / l, dy / l, dz / l], i * 3)
    len[i] = 0.35 + rand() * 0.5
    seed[i] = rand()
    period[i] = 5 + rand() * 7
  }
  const geo = new PlaneGeometry(1, 1)
  geo.setAttribute('aOrigin', new InstancedBufferAttribute(origin, 3))
  geo.setAttribute('aDir', new InstancedBufferAttribute(dir, 3))
  geo.setAttribute('aLen', new InstancedBufferAttribute(len, 1))
  geo.setAttribute('aSeed', new InstancedBufferAttribute(seed, 1))
  geo.setAttribute('aPeriod', new InstancedBufferAttribute(period, 1))
  const uniforms = { uMeteors: { value: 0 } }
  const mat = new ShaderMaterial({
    uniforms: { ...shared, ...uniforms }, vertexShader: meteorVert, fragmentShader: meteorFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const mesh = new InstancedMesh(geo, mat, count)
  mesh.frustumCulled = false
  mesh.renderOrder = 7
  return { mesh, uniforms, dispose() { geo.dispose(); mat.dispose(); mesh.dispose() } }
}
