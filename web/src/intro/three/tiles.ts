// Package tiles: one InstancedMesh. Every tile knows its slot on the sphere and its drifted pose;
// the vertex shader blends between them from uScatter, so scrolling costs no CPU work.
import {
  DoubleSide, InstancedBufferAttribute, InstancedMesh, Matrix4, PlaneGeometry, Quaternion, ShaderMaterial, Vector3,
} from 'three'
import { prng, TAU } from './math'
import { packageLabels } from './packages'
import { PLANET_R } from './planet'
import { drawTileAtlas, makeCanvas, textureOf, TILE_CELL_H, TILE_CELL_W, TILE_COLS, TILE_ROWS, whenFontReady } from './text'
import { GLSL_OUT, type Shared } from './uniforms'

export interface Tiles {
  mesh: InstancedMesh
  uniforms: {
    uScatter: { value: number }; uAlpha: { value: number }; uWarm: { value: number }
    uJitter: { value: number }; uLock: { value: number }
  }
  count: number
  dispose(): void
}

export interface TileSlot {
  /** Centre on the unit sphere. */
  dir: [number, number, number]
  /** World width and height. */
  w: number
  h: number
}

/** Lat/long bands of varied heights, each split into cells roughly 2:1, like a faceted mosaic globe. */
export function tileSlots(rand: () => number, radius = PLANET_R): TileSlot[] {
  const slots: TileSlot[] = []
  const heights = [0.19, 0.14, 0.22, 0.16, 0.2, 0.13, 0.21, 0.17, 0.15, 0.2, 0.14, 0.18, 0.16]
  let lat = -1.42
  let row = 0
  while (lat < 1.42) {
    const h = heights[row % heights.length]
    if (lat + h > 1.45) break
    const phi = lat + h / 2
    const cosPhi = Math.cos(phi)
    const ring = TAU * cosPhi
    const n = Math.max(3, Math.round(ring / (h * (1.7 + rand() * 0.6))))
    const w = ring / n
    const offset = rand() * TAU
    for (let i = 0; i < n; i++) {
      if (rand() < 0.06) continue // a few gaps: the sphere is a mosaic, not a shell
      const lam = offset + (i + 0.5) * w
      const split = rand() < 0.22
      const parts = split ? 2 : 1
      for (let p = 0; p < parts; p++) {
        const l2 = lam + (split ? (p - 0.5) * (w / 2) : 0)
        slots.push({
          dir: [cosPhi * Math.cos(l2), Math.sin(phi), cosPhi * Math.sin(l2)],
          w: radius * (w / parts) * cosPhi * 0.9,
          h: radius * h * 0.9,
        })
      }
    }
    lat += h
    row++
  }
  return slots
}

const vert = /* glsl */ `
attribute vec3 aSlotPos; attribute vec4 aSlotQuat; attribute vec3 aDriftPos; attribute vec4 aDriftQuat;
attribute vec2 aSize; attribute float aCell; attribute float aSeed; attribute float aConflict;
uniform float uTime; uniform float uScatter; uniform float uJitter; uniform float uAlpha;
varying vec2 vUv; varying float vCell; varying float vS; varying float vConflict; varying float vAspect; varying float vFacing;
vec3 rotateQ(vec4 q, vec3 v) { return v + 2.0 * cross(q.xyz, cross(q.xyz, v) + q.w * v); }
void main() {
  float stag = aSeed * 0.55;
  float s = smoothstep(stag, stag + 0.45, uScatter);
  vec4 q = normalize(mix(aSlotQuat, aDriftQuat, s));
  // Tumble slowly while adrift.
  float tum = s * (uTime * 0.35 + aSeed * 6.283) * (aSeed - 0.5);
  float ct = cos(tum), st = sin(tum);
  vec3 local = vec3(position.xy * aSize, 0.0);
  local.xy = vec2(local.x * ct - local.y * st, local.x * st + local.y * ct);
  vec3 p = mix(aSlotPos, aDriftPos, s);
  vec3 side = normalize(cross(aSlotPos, vec3(0.0, 1.0, 0.0)) + vec3(0.001, 0.0, 0.002));
  p += side * sin(s * 3.14159) * (aSeed - 0.5) * 1.6;
  p += uJitter * s * 0.045 * vec3(sin(uTime * 2.3 + aSeed * 31.0), cos(uTime * 1.9 + aSeed * 17.0), sin(uTime * 2.7 + aSeed * 7.0));
  // Shrink into a point as the mosaic resolves into the dot planet.
  local *= smoothstep(0.0, 1.0, uAlpha) * 0.85 + 0.15;
  vec3 world = p + rotateQ(q, local);
  vec4 wp = modelMatrix * vec4(world, 1.0);
  vec3 n = normalize(mat3(modelMatrix) * rotateQ(q, vec3(0.0, 0.0, 1.0)));
  vFacing = dot(n, normalize(cameraPosition - wp.xyz));
  vUv = uv; vCell = aCell; vS = s; vConflict = aConflict; vAspect = aSize.x / aSize.y;
  gl_Position = projectionMatrix * viewMatrix * wp;
}`

const frag = /* glsl */ `
uniform sampler2D uAtlas; uniform vec2 uCells; uniform float uTime; uniform float uWarm; uniform float uAlpha; uniform float uLock;
varying vec2 vUv; varying float vCell; varying float vS; varying float vConflict; varying float vAspect; varying float vFacing;
void main() {
  vec2 cell = vec2(mod(vCell, uCells.x), uCells.y - 1.0 - floor(vCell / uCells.x));
  // Keep the text unstretched: wider tiles show more of the card, narrower tiles crop it.
  vec2 uvA = vec2(min(vUv.x * vAspect / 2.0, 0.995), vUv.y);
  vec4 tex = texture2D(uAtlas, (cell + uvA) / uCells);
  vec3 col = tex.rgb;
  if (!gl_FrontFacing) col = col * 0.3 + vec3(0.01, 0.012, 0.02);
  float conflictGlow = vConflict * (0.55 + 0.45 * sin(uTime * 3.0 + vCell * 1.7));
  vec3 warm = col * vec3(1.45, 0.72, 0.42) + vec3(0.22, 0.07, 0.0) * conflictGlow;
  col = mix(col, warm, uWarm * (0.35 + 0.65 * vS));
  // Lock flash in the last stretch before a tile seats.
  float flash = smoothstep(0.0, 0.08, vS) * (1.0 - smoothstep(0.08, 0.28, vS));
  col += vec3(0.45, 0.65, 1.0) * flash * uLock * 0.5;
  // Edge-on tiles thin out instead of flickering.
  float edge = smoothstep(0.0, 0.18, abs(vFacing));
  gl_FragColor = vec4(col, tex.a * uAlpha * edge);
  ${GLSL_OUT}
}`

export function createTiles(shared: Shared): Tiles {
  const rand = prng(7)
  const slots = tileSlots(rand)
  const count = slots.length
  const labels = packageLabels()
  const { canvas, ctx } = makeCanvas(TILE_CELL_W * TILE_COLS, TILE_CELL_H * TILE_ROWS)
  drawTileAtlas(ctx, labels)
  const atlas = textureOf(canvas)
  whenFontReady(() => {
    drawTileAtlas(ctx, labels)
    atlas.needsUpdate = true
  })

  const slotPos = new Float32Array(count * 3)
  const slotQuat = new Float32Array(count * 4)
  const driftPos = new Float32Array(count * 3)
  const driftQuat = new Float32Array(count * 4)
  const size = new Float32Array(count * 2)
  const cellIdx = new Float32Array(count)
  const seed = new Float32Array(count)
  const conflict = new Float32Array(count)
  const m = new Matrix4()
  const q = new Quaternion()
  const qd = new Quaternion()
  const v = new Vector3()
  const up = new Vector3(0, 1, 0)
  const origin = new Vector3()
  const cells = TILE_COLS * TILE_ROWS
  for (let i = 0; i < count; i++) {
    const s = slots[i]
    v.set(s.dir[0], s.dir[1], s.dir[2]).multiplyScalar(PLANET_R)
    slotPos.set([v.x, v.y, v.z], i * 3)
    m.lookAt(v, origin, up) // +z faces outward
    q.setFromRotationMatrix(m)
    slotQuat.set([q.x, q.y, q.z, q.w], i * 4)
    // Drift pose: outward along the slot normal with a sideways push, 1.3..2.8 from the centre, and a random tilt.
    const r = 1.3 + rand() * 1.5
    const d = v.clone().normalize()
    d.x += (rand() - 0.5) * 1.3
    d.y += (rand() - 0.5) * 1.3
    d.z += (rand() - 0.5) * 0.8 + 0.05
    d.normalize().multiplyScalar(r)
    driftPos.set([d.x, d.y, d.z], i * 3)
    qd.setFromAxisAngle(v.set(rand() - 0.5, rand() - 0.5, rand() - 0.5).normalize(), (rand() - 0.5) * 1.0).multiply(q)
    if (qd.dot(q) < 0) qd.set(-qd.x, -qd.y, -qd.z, -qd.w) // same hemisphere: nlerp takes the short way
    driftQuat.set([qd.x, qd.y, qd.z, qd.w], i * 4)
    size.set([s.w, s.h], i * 2)
    const cell = Math.floor(rand() * cells)
    cellIdx[i] = cell
    conflict[i] = labels[cell % labels.length].conflict ? 1 : 0
    seed[i] = rand()
  }

  const geo = new PlaneGeometry(1, 1)
  geo.setAttribute('aSlotPos', new InstancedBufferAttribute(slotPos, 3))
  geo.setAttribute('aSlotQuat', new InstancedBufferAttribute(slotQuat, 4))
  geo.setAttribute('aDriftPos', new InstancedBufferAttribute(driftPos, 3))
  geo.setAttribute('aDriftQuat', new InstancedBufferAttribute(driftQuat, 4))
  geo.setAttribute('aSize', new InstancedBufferAttribute(size, 2))
  geo.setAttribute('aCell', new InstancedBufferAttribute(cellIdx, 1))
  geo.setAttribute('aSeed', new InstancedBufferAttribute(seed, 1))
  geo.setAttribute('aConflict', new InstancedBufferAttribute(conflict, 1))

  const uniforms = {
    uScatter: { value: 0 }, uAlpha: { value: 0 }, uWarm: { value: 0 }, uJitter: { value: 0 }, uLock: { value: 0 },
  }
  const mat = new ShaderMaterial({
    uniforms: { ...shared, ...uniforms, uAtlas: { value: atlas }, uCells: { value: [TILE_COLS, TILE_ROWS] } },
    vertexShader: vert, fragmentShader: frag,
    side: DoubleSide, transparent: true, forceSinglePass: true,
  })
  const mesh = new InstancedMesh(geo, mat, count)
  mesh.frustumCulled = false
  mesh.renderOrder = 1
  mesh.visible = false

  return {
    mesh, uniforms, count,
    dispose() { geo.dispose(); mat.dispose(); atlas.dispose(); mesh.dispose() },
  }
}
