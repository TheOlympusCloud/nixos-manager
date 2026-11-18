// Dependency arcs between package clusters: faint great-circle lines with packets of light flowing along them.
import {
  AdditiveBlending, BufferGeometry, Float32BufferAttribute, Group, LineSegments, Points, ShaderMaterial, Vector3,
} from 'three'
import { PLANET_R } from './planet'
import { GLSL_OUT, GLSL_SPIN, type Shared } from './uniforms'

export interface Arcs {
  group: Group
  uniforms: { uArcs: { value: number } }
  dispose(): void
}

const SEG = 40

const vert = /* glsl */ `
attribute float aT; attribute float aSeed;
uniform float uSpin; uniform float uTime; uniform float uArcs; uniform float uDpr; uniform float uPoint;
varying float vA; varying float vHead;
${GLSL_SPIN}
void main() {
  vec3 p = spinY(position, uSpin);
  vec4 wp = modelMatrix * vec4(p, 1.0);
  float facing = dot(normalize(mat3(modelMatrix) * p), normalize(cameraPosition - wp.xyz));
  float f = fract(aT - uTime * 0.11 - aSeed);
  float pulse = smoothstep(0.0, 0.12, f) * (1.0 - smoothstep(0.12, 0.4, f));
  vHead = smoothstep(0.0, 0.05, f) * (1.0 - smoothstep(0.05, 0.11, f));
  float ends = sqrt(sin(aT * 3.14159));
  vA = uArcs * ends * smoothstep(-0.05, 0.3, facing) * (0.22 + 0.9 * pulse);
  vec4 mv = viewMatrix * wp;
  gl_PointSize = uPoint * uDpr * 3.0 / -mv.z;
  gl_Position = projectionMatrix * mv;
}`

const lineFrag = /* glsl */ `
varying float vA; varying float vHead;
void main() {
  vec3 col = mix(vec3(0.35, 0.58, 1.0), vec3(0.8, 0.9, 1.0), vHead) * (1.0 + vHead);
  gl_FragColor = vec4(col * vA, vA);
  ${GLSL_OUT}
}`

const pointFrag = /* glsl */ `
varying float vA; varying float vHead;
void main() {
  float d = length(gl_PointCoord - 0.5);
  float a = smoothstep(0.5, 0.1, d) * vA * vHead;
  gl_FragColor = vec4(vec3(0.85, 0.93, 1.0) * a * 2.2, a);
  ${GLSL_OUT}
}`

/** Arcs between each hub and its two nearest neighbours, deduplicated. */
export function arcPairs(hubs: Vector3[]): [number, number][] {
  const pairs = new Set<string>()
  const out: [number, number][] = []
  hubs.forEach((h, i) => {
    const near = hubs
      .map((o, j) => ({ j, d: j === i ? Infinity : h.angleTo(o) }))
      .sort((a, b) => a.d - b.d)
      .slice(0, 2)
    for (const { j } of near) {
      const key = i < j ? `${i}-${j}` : `${j}-${i}`
      if (!pairs.has(key)) {
        pairs.add(key)
        out.push(i < j ? [i, j] : [j, i])
      }
    }
  })
  return out
}

export function createArcs(shared: Shared, hubs: Vector3[]): Arcs {
  const pairs = arcPairs(hubs)
  const per = SEG + 1
  const n = pairs.length * per
  const pos = new Float32Array(n * 3)
  const at = new Float32Array(n)
  const seed = new Float32Array(n)
  const index: number[] = []
  const a = new Vector3()
  const b = new Vector3()
  const p = new Vector3()
  pairs.forEach(([i, j], k) => {
    a.copy(hubs[i])
    b.copy(hubs[j])
    const angle = a.angleTo(b)
    const lift = 0.1 + 0.22 * (angle / Math.PI)
    const sinA = Math.sin(angle)
    for (let s = 0; s <= SEG; s++) {
      const t = s / SEG
      // Great-circle interpolation, then lifted off the surface.
      p.copy(a).multiplyScalar(Math.sin((1 - t) * angle) / sinA).addScaledVector(b, Math.sin(t * angle) / sinA)
      p.multiplyScalar(PLANET_R * (1 + lift * Math.sin(t * Math.PI)))
      const vi = k * per + s
      pos.set([p.x, p.y, p.z], vi * 3)
      at[vi] = t
      seed[vi] = (k * 0.618) % 1
      if (s < SEG) index.push(vi, vi + 1)
    }
  })

  const geo = new BufferGeometry()
  geo.setAttribute('position', new Float32BufferAttribute(pos, 3))
  geo.setAttribute('aT', new Float32BufferAttribute(at, 1))
  geo.setAttribute('aSeed', new Float32BufferAttribute(seed, 1))
  const lineGeo = geo.clone()
  lineGeo.setIndex(index)

  const uniforms = { uArcs: { value: 0 } }
  const common = { ...shared, ...uniforms }
  const lineMat = new ShaderMaterial({
    uniforms: { ...common, uPoint: { value: 0 } }, vertexShader: vert, fragmentShader: lineFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const pointMat = new ShaderMaterial({
    uniforms: { ...common, uPoint: { value: 4.5 } }, vertexShader: vert, fragmentShader: pointFrag,
    blending: AdditiveBlending, transparent: true, depthWrite: false,
  })
  const lines = new LineSegments(lineGeo, lineMat)
  const points = new Points(geo, pointMat)
  lines.frustumCulled = points.frustumCulled = false
  lines.renderOrder = points.renderOrder = 4
  const group = new Group()
  group.add(lines, points)
  group.visible = false
  return {
    group, uniforms,
    dispose() { geo.dispose(); lineGeo.dispose(); lineMat.dispose(); pointMat.dispose() },
  }
}
