// The planet: a near-black core, a rim-lit atmosphere shell and a dot-matrix surface of packages.
import {
  AdditiveBlending, BackSide, BufferGeometry, Float32BufferAttribute, Group, Mesh, Points, ShaderMaterial,
  SphereGeometry, Vector3,
} from 'three'
import { continent, fibonacciSphere, prng } from './math'
import { GLSL_OUT, GLSL_SPIN, type Shared } from './uniforms'

export const PLANET_R = 1
const SHELL_R = 1.09

export interface Planet {
  group: Group
  uniforms: { uDots: { value: number }; uRim: { value: number }; uWarm: { value: number } }
  /** Unit directions of the densest package clusters, for arcs and beams. */
  hubs: Vector3[]
  dispose(): void
}

const coreVert = /* glsl */ `
varying vec3 vN; varying vec3 vW;
void main() {
  vN = normalize(mat3(modelMatrix) * normal);
  vec4 wp = modelMatrix * vec4(position, 1.0);
  vW = wp.xyz;
  gl_Position = projectionMatrix * viewMatrix * wp;
}`

const coreFrag = /* glsl */ `
uniform float uRim; uniform float uWarm; uniform vec3 uLightDir;
varying vec3 vN; varying vec3 vW;
void main() {
  vec3 v = normalize(cameraPosition - vW);
  float f = 1.0 - clamp(dot(vN, v), 0.0, 1.0);
  float light = smoothstep(-0.3, 0.9, dot(vN, uLightDir));
  vec3 base = vec3(0.003, 0.004, 0.007) + vec3(0.004, 0.007, 0.014) * light;
  vec3 rimCol = mix(vec3(0.35, 0.62, 1.0), vec3(1.0, 0.42, 0.12), uWarm);
  vec3 col = base + rimCol * pow(f, 4.5) * (0.2 + 0.8 * light) * 0.9 * uRim;
  gl_FragColor = vec4(col, 1.0);
  ${GLSL_OUT}
}`

const shellFrag = /* glsl */ `
uniform float uRim; uniform float uWarm; uniform vec3 uLightDir;
varying vec3 vN; varying vec3 vW;
void main() {
  vec3 v = normalize(cameraPosition - vW);
  // Back faces: 0 at the shell's edge, ~-0.4 where the shell meets the planet limb.
  float t = clamp(-dot(vN, v) / 0.4, 0.0, 1.0);
  float glow = pow(t, 3.0);
  float light = 0.25 + 0.75 * smoothstep(-0.6, 0.9, dot(vN, uLightDir));
  vec3 rimCol = mix(vec3(0.35, 0.62, 1.0), vec3(1.0, 0.45, 0.15), uWarm);
  gl_FragColor = vec4(rimCol * glow * light * uRim * 0.5, 1.0);
  ${GLSL_OUT}
}`

const dotsVert = /* glsl */ `
attribute float aCluster; attribute float aSeed;
uniform float uTime; uniform float uSpin; uniform float uDpr; uniform float uDots; uniform float uWarm; uniform vec3 uLightDir;
varying float vA; varying vec3 vCol;
${GLSL_SPIN}
void main() {
  vec3 p = spinY(position, uSpin);
  vec4 wp = modelMatrix * vec4(p, 1.0);
  vec3 n = normalize(mat3(modelMatrix) * p);
  vec3 v = normalize(cameraPosition - wp.xyz);
  float facing = dot(n, v);
  float light = smoothstep(-0.35, 0.8, dot(n, uLightDir));
  float limb = pow(1.0 - clamp(facing, 0.0, 1.0), 2.0);
  float twinkle = 0.8 + 0.2 * sin(uTime * 1.7 + aSeed * 40.0);
  float a = (0.07 + 0.6 * aCluster) * (0.2 + 0.8 * light) + limb * 0.55 * (0.4 + 0.6 * light);
  vA = a * twinkle * uDots * smoothstep(-0.05, 0.3, facing);
  vec3 cool = mix(vec3(0.22, 0.40, 0.72), vec3(0.55, 0.78, 1.0), aCluster);
  vec3 warm = mix(vec3(0.5, 0.26, 0.12), vec3(1.0, 0.55, 0.25), aCluster);
  vCol = mix(cool, warm, uWarm);
  vec4 mv = modelViewMatrix * vec4(p, 1.0);
  gl_PointSize = max((1.3 + 1.5 * aCluster) * uDpr * 4.2 / -mv.z, uDpr);
  gl_Position = projectionMatrix * mv;
}`

const dotsFrag = /* glsl */ `
varying float vA; varying vec3 vCol;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.15, d) * vA;
  gl_FragColor = vec4(vCol * a, a);
  ${GLSL_OUT}
}`

export function createPlanet(shared: Shared, dotCount = 32000): Planet {
  const uniforms = { uDots: { value: 1 }, uRim: { value: 1 }, uWarm: { value: 0 } }
  const all = { ...shared, ...uniforms }
  const group = new Group()

  const coreGeo = new SphereGeometry(PLANET_R * 0.988, 72, 48)
  const coreMat = new ShaderMaterial({ uniforms: all, vertexShader: coreVert, fragmentShader: coreFrag })
  const core = new Mesh(coreGeo, coreMat)
  core.renderOrder = 0
  group.add(core)

  const shellGeo = new SphereGeometry(SHELL_R, 72, 48)
  const shellMat = new ShaderMaterial({
    uniforms: all, vertexShader: coreVert, fragmentShader: shellFrag,
    side: BackSide, blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const shell = new Mesh(shellGeo, shellMat)
  shell.renderOrder = 2
  group.add(shell)

  const pos = fibonacciSphere(dotCount)
  const cluster = new Float32Array(dotCount)
  const seed = new Float32Array(dotCount)
  const rand = prng(42)
  for (let i = 0; i < dotCount; i++) {
    cluster[i] = continent(pos[i * 3], pos[i * 3 + 1], pos[i * 3 + 2])
    seed[i] = rand()
  }
  const dotsGeo = new BufferGeometry()
  dotsGeo.setAttribute('position', new Float32BufferAttribute(pos, 3))
  dotsGeo.setAttribute('aCluster', new Float32BufferAttribute(cluster, 1))
  dotsGeo.setAttribute('aSeed', new Float32BufferAttribute(seed, 1))
  const dotsMat = new ShaderMaterial({
    uniforms: all, vertexShader: dotsVert, fragmentShader: dotsFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const dots = new Points(dotsGeo, dotsMat)
  dots.renderOrder = 3
  dots.frustumCulled = false
  group.add(dots)

  // Hubs: the strongest cluster points, at least 0.55 rad apart.
  const order = Array.from({ length: dotCount }, (_, i) => i).filter((i) => cluster[i] > 0.9)
  order.sort((a, b) => seed[a] - seed[b])
  const hubs: Vector3[] = []
  for (const i of order) {
    const v = new Vector3(pos[i * 3], pos[i * 3 + 1], pos[i * 3 + 2])
    if (hubs.every((h) => h.angleTo(v) > 0.55)) hubs.push(v)
    if (hubs.length >= 14) break
  }

  return {
    group,
    uniforms,
    hubs,
    dispose() {
      coreGeo.dispose(); shellGeo.dispose(); dotsGeo.dispose()
      coreMat.dispose(); shellMat.dispose(); dotsMat.dispose()
    },
  }
}
