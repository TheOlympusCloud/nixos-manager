// Orbit rings and the closure beams that tether each machine to the planet beneath it.
// Machine positions arrive as uniforms, so the beams bend on the GPU every frame.
import {
  AdditiveBlending, BufferGeometry, Float32BufferAttribute, Group, LineSegments, Points, ShaderMaterial, Vector3,
} from 'three'
import { orbitPoint } from './math'
import { PLANET_R } from './planet'
import { GLSL_OUT, GLSL_SPIN, type Shared } from './uniforms'

export const MAX_MACHINES = 6

export interface Orbits {
  group: Group
  uniforms: { uOrbit: { value: number }; uBeams: { value: number }; uMachine: { value: Vector3[] } }
  dispose(): void
}

const BEAM_SEG = 36

const ringVert = /* glsl */ `
uniform float uOrbit;
varying float vA;
void main() {
  vec4 wp = modelMatrix * vec4(position, 1.0);
  vA = uOrbit * (0.75 + 0.25 * smoothstep(-1.0, 1.0, wp.z));
  gl_Position = projectionMatrix * viewMatrix * wp;
}`

const ringFrag = /* glsl */ `
varying float vA;
void main() {
  gl_FragColor = vec4(vec3(0.3, 0.45, 0.75) * 0.28 * vA, vA * 0.28);
  ${GLSL_OUT}
}`

const beamVert = /* glsl */ `
attribute float aT; attribute float aIdx;
uniform vec3 uMachine[${MAX_MACHINES}]; uniform float uBeams; uniform float uTime; uniform float uDpr; uniform float uPoint;
varying float vA; varying float vHead;
${GLSL_SPIN}
void main() {
  vec3 m = uMachine[int(aIdx + 0.5)];
  float R = ${PLANET_R.toFixed(2)};
  vec3 anchor = spinY(normalize(m), -0.3) * R;
  vec3 mid = normalize(anchor + m) * (R + (length(m) - R) * 0.42);
  vec3 p = mix(mix(anchor, mid, aT), mix(mid, m, aT), aT);
  float f = fract(aT - uTime * 0.3 - aIdx * 0.37);
  float pulse = smoothstep(0.0, 0.15, f) * (1.0 - smoothstep(0.15, 0.5, f));
  vHead = smoothstep(0.0, 0.05, f) * (1.0 - smoothstep(0.05, 0.12, f));
  vA = uBeams * smoothstep(0.0, 0.1, aT) * (1.0 - smoothstep(0.88, 1.0, aT)) * (0.16 + 0.8 * pulse);
  vec4 mv = modelViewMatrix * vec4(p, 1.0);
  gl_PointSize = uPoint * uDpr * 3.0 / -mv.z;
  gl_Position = projectionMatrix * mv;
}`

const beamFrag = /* glsl */ `
varying float vA; varying float vHead;
void main() {
  vec3 col = mix(vec3(0.4, 0.62, 1.0), vec3(0.85, 0.92, 1.0), vHead) * (1.0 + vHead);
  gl_FragColor = vec4(col * vA, vA);
  ${GLSL_OUT}
}`

const beamPointFrag = /* glsl */ `
varying float vA; varying float vHead;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.1, d) * vA * vHead;
  gl_FragColor = vec4(vec3(0.85, 0.93, 1.0) * a * 2.0, a);
  ${GLSL_OUT}
}`

export function createOrbits(shared: Shared, machines: number): Orbits {
  const group = new Group()
  group.visible = false
  const uniforms = {
    uOrbit: { value: 0 },
    uBeams: { value: 0 },
    uMachine: { value: Array.from({ length: MAX_MACHINES }, () => new Vector3(0, 2, 0)) },
  }

  // Rings: one LineSegments for all of them.
  const RING_SEG = 160
  const ringPos: number[] = []
  const ringIdx: number[] = []
  const p = { x: 0, y: 0, z: 0 }
  for (let i = 0; i < machines; i++) {
    const base = ringPos.length / 3
    for (let s = 0; s < RING_SEG; s++) {
      orbitPoint(i, (s / RING_SEG) * Math.PI * 2, p)
      ringPos.push(p.x, p.y, p.z)
      ringIdx.push(base + s, base + ((s + 1) % RING_SEG))
    }
  }
  const ringGeo = new BufferGeometry()
  ringGeo.setAttribute('position', new Float32BufferAttribute(ringPos, 3))
  ringGeo.setIndex(ringIdx)
  const ringMat = new ShaderMaterial({
    uniforms: { uOrbit: uniforms.uOrbit }, vertexShader: ringVert, fragmentShader: ringFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const rings = new LineSegments(ringGeo, ringMat)
  rings.frustumCulled = false
  rings.renderOrder = 5
  group.add(rings)

  // Beams: positions come from the shader; the geometry only carries t and the machine index.
  const per = BEAM_SEG + 1
  const n = machines * per
  const at = new Float32Array(n)
  const idx = new Float32Array(n)
  const beamIdx: number[] = []
  for (let i = 0; i < machines; i++) {
    for (let s = 0; s <= BEAM_SEG; s++) {
      const vi = i * per + s
      at[vi] = s / BEAM_SEG
      idx[vi] = i
      if (s < BEAM_SEG) beamIdx.push(vi, vi + 1)
    }
  }
  const beamGeo = new BufferGeometry()
  beamGeo.setAttribute('position', new Float32BufferAttribute(new Float32Array(n * 3), 3))
  beamGeo.setAttribute('aT', new Float32BufferAttribute(at, 1))
  beamGeo.setAttribute('aIdx', new Float32BufferAttribute(idx, 1))
  const beamLineGeo = beamGeo.clone()
  beamLineGeo.setIndex(beamIdx)
  const common = { ...shared, uBeams: uniforms.uBeams, uMachine: uniforms.uMachine }
  const beamMat = new ShaderMaterial({
    uniforms: { ...common, uPoint: { value: 0 } }, vertexShader: beamVert, fragmentShader: beamFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const beamPointMat = new ShaderMaterial({
    uniforms: { ...common, uPoint: { value: 4 } }, vertexShader: beamVert, fragmentShader: beamPointFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const beams = new LineSegments(beamLineGeo, beamMat)
  const packets = new Points(beamGeo, beamPointMat)
  beams.frustumCulled = packets.frustumCulled = false
  beams.renderOrder = packets.renderOrder = 5
  group.add(beams, packets)

  return {
    group, uniforms,
    dispose() {
      ringGeo.dispose(); beamGeo.dispose(); beamLineGeo.dispose()
      ringMat.dispose(); beamMat.dispose(); beamPointMat.dispose()
    },
  }
}
