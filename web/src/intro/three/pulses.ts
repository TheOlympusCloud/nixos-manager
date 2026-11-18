// Deploy: a commit pulse leaves Forgejo for each machine in turn, along a path bent away from the planet.
import {
  AdditiveBlending, BufferGeometry, Float32BufferAttribute, Group, LineSegments, Points, ShaderMaterial, Vector3,
} from 'three'
import { MAX_MACHINES } from './orbits'
import { GLSL_OUT, type Shared } from './uniforms'

export interface Pulses {
  group: Group
  uniforms: { uForge: { value: Vector3 }; uHead: { value: Float32Array }; uPulse: { value: number } }
  dispose(): void
}

const SEG = 56

const vert = /* glsl */ `
attribute float aT; attribute float aIdx;
uniform vec3 uMachine[${MAX_MACHINES}]; uniform float uHead[${MAX_MACHINES}]; uniform vec3 uForge;
uniform float uPulse; uniform float uDpr; uniform float uPoint;
varying float vA; varying float vHead;
void main() {
  int i = int(aIdx + 0.5);
  vec3 m = uMachine[i];
  float head = uHead[i];
  vec3 mid = mix(uForge, m, 0.5);
  mid += normalize(mid) * 0.9;
  vec3 p = mix(mix(uForge, mid, aT), mix(mid, m, aT), aT);
  float d = head - aT;
  float trail = d > 0.0 ? exp(-d * 7.0) : 0.0;
  vHead = 1.0 - smoothstep(0.0, 0.035, abs(d));
  float live = step(0.0, head) * (1.0 - smoothstep(1.0, 1.3, head));
  float path = 0.12 * smoothstep(0.0, 0.08, head) * (1.0 - smoothstep(0.9, 1.0, head));
  vA = uPulse * live * (path + 0.8 * trail);
  vec4 mv = modelViewMatrix * vec4(p, 1.0);
  gl_PointSize = uPoint * uDpr * 3.0 / -mv.z;
  gl_Position = projectionMatrix * mv;
}`

const lineFrag = /* glsl */ `
varying float vA; varying float vHead;
void main() {
  vec3 col = mix(vec3(0.95, 0.8, 0.55), vec3(1.0, 0.95, 0.85), vHead);
  gl_FragColor = vec4(col * vA, vA);
  ${GLSL_OUT}
}`

const pointFrag = /* glsl */ `
varying float vA; varying float vHead;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.1, d) * vA * (0.25 + vHead);
  gl_FragColor = vec4(vec3(1.0, 0.92, 0.75) * a * 2.4, a);
  ${GLSL_OUT}
}`

export function createPulses(shared: Shared, machineUniform: { value: Vector3[] }, machines: number): Pulses {
  const per = SEG + 1
  const n = machines * per
  const at = new Float32Array(n)
  const idx = new Float32Array(n)
  const lineIdx: number[] = []
  for (let i = 0; i < machines; i++) {
    for (let s = 0; s <= SEG; s++) {
      const vi = i * per + s
      at[vi] = s / SEG
      idx[vi] = i
      if (s < SEG) lineIdx.push(vi, vi + 1)
    }
  }
  const geo = new BufferGeometry()
  geo.setAttribute('position', new Float32BufferAttribute(new Float32Array(n * 3), 3))
  geo.setAttribute('aT', new Float32BufferAttribute(at, 1))
  geo.setAttribute('aIdx', new Float32BufferAttribute(idx, 1))
  const lineGeo = geo.clone()
  lineGeo.setIndex(lineIdx)

  const uniforms = {
    uForge: { value: new Vector3(-1.5, 1.2, 0.3) },
    uHead: { value: new Float32Array(MAX_MACHINES).fill(-1) },
    uPulse: { value: 0 },
  }
  const common = { ...shared, ...uniforms, uMachine: machineUniform }
  const lineMat = new ShaderMaterial({
    uniforms: { ...common, uPoint: { value: 0 } }, vertexShader: vert, fragmentShader: lineFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const pointMat = new ShaderMaterial({
    uniforms: { ...common, uPoint: { value: 5 } }, vertexShader: vert, fragmentShader: pointFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const lines = new LineSegments(lineGeo, lineMat)
  const points = new Points(geo, pointMat)
  lines.frustumCulled = points.frustumCulled = false
  lines.renderOrder = points.renderOrder = 5
  const group = new Group()
  group.add(lines, points)
  group.visible = false
  return {
    group, uniforms,
    dispose() { geo.dispose(); lineGeo.dispose(); lineMat.dispose(); pointMat.dispose() },
  }
}
