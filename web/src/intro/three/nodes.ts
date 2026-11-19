// Machine and Forgejo nodes with their labels: one instanced billboard mesh over one canvas atlas,
// sized in CSS pixels so labels stay crisp whatever the camera does.
import { AdditiveBlending, InstancedBufferAttribute, InstancedMesh, PlaneGeometry, ShaderMaterial, Vector3 } from 'three'
import { LabelAtlas, textureOf, whenFontReady } from './text'
import { GLSL_OUT, type Shared } from './uniforms'

export interface Nodes {
  mesh: InstancedMesh
  /** Instance slots: machine i owns 2i (glyph) and 2i+1 (label); Forgejo owns the last two. */
  setMachine(i: number, pos: Vector3, r: number, g: number, b: number, alpha: number, labelAlpha?: number): void
  setForge(pos: Vector3, alpha: number, labelAlpha?: number): void
  commit(): void
  dispose(): void
}

const NODE_PX = 64
const FORGE_PX = 56
const LABEL_PX = 12

const vert = /* glsl */ `
attribute vec3 aPos; attribute vec4 aRect; attribute vec2 aSizePx; attribute vec2 aOffsetPx; attribute vec4 aTint;
uniform float uPixelToWorld;
varying vec2 vUv; varying vec4 vTint;
void main() {
  vec4 mv = modelViewMatrix * vec4(aPos, 1.0);
  float k = -mv.z * uPixelToWorld;
  mv.xy += (position.xy * aSizePx + aOffsetPx) * k;
  vUv = mix(aRect.xy, aRect.zw, uv);
  vTint = aTint;
  gl_Position = projectionMatrix * mv;
}`

const frag = /* glsl */ `
uniform sampler2D uAtlas;
varying vec2 vUv; varying vec4 vTint;
void main() {
  vec4 t = texture2D(uAtlas, vUv);
  float a = t.a * vTint.a;
  gl_FragColor = vec4(t.rgb * vTint.rgb * a, a);
  ${GLSL_OUT}
}`

export function createNodes(shared: Shared, machines: string[]): Nodes {
  const atlas = new LabelAtlas()
  const draw = () => {
    atlas.clear()
    atlas.hexNode('node', NODE_PX)
    atlas.repoNode('forge', FORGE_PX)
    machines.forEach((m, i) => atlas.label(`m${i}`, m, LABEL_PX, '#ffffff'))
    atlas.label('forge-label', 'forgejo', LABEL_PX, '#ffffff')
  }
  draw()
  const tex = textureOf(atlas.canvas, false)
  whenFontReady(() => {
    draw()
    fillRects()
    tex.needsUpdate = true
  })

  const count = machines.length * 2 + 2
  const pos = new Float32Array(count * 3)
  const rect = new Float32Array(count * 4)
  const sizePx = new Float32Array(count * 2)
  const offsetPx = new Float32Array(count * 2)
  const tint = new Float32Array(count * 4)
  const rectAttr = new InstancedBufferAttribute(rect, 4)
  const sizeAttr = new InstancedBufferAttribute(sizePx, 2)
  const offsetAttr = new InstancedBufferAttribute(offsetPx, 2)
  const posAttr = new InstancedBufferAttribute(pos, 3)
  const tintAttr = new InstancedBufferAttribute(tint, 4)

  function place(slot: number, key: string, px: number, dy: number) {
    const r = atlas.rects.get(key)
    if (!r) return
    rect.set(r.uv, slot * 4)
    const h = px
    const w = (px * r.w) / r.h
    sizePx.set([w, h], slot * 2)
    offsetPx.set([0, dy], slot * 2)
  }
  function fillRects() {
    machines.forEach((_, i) => {
      place(2 * i, 'node', NODE_PX, 0)
      place(2 * i + 1, `m${i}`, LABEL_PX * 1.3, -NODE_PX * 0.42)
    })
    place(count - 2, 'forge', FORGE_PX, 0)
    place(count - 1, 'forge-label', LABEL_PX * 1.3, -FORGE_PX * 0.45)
    rectAttr.needsUpdate = sizeAttr.needsUpdate = offsetAttr.needsUpdate = true
  }
  fillRects()

  const geo = new PlaneGeometry(1, 1)
  geo.setAttribute('aPos', posAttr)
  geo.setAttribute('aRect', rectAttr)
  geo.setAttribute('aSizePx', sizeAttr)
  geo.setAttribute('aOffsetPx', offsetAttr)
  geo.setAttribute('aTint', tintAttr)
  const mat = new ShaderMaterial({
    uniforms: { uPixelToWorld: shared.uPixelToWorld, uAtlas: { value: tex } },
    vertexShader: vert, fragmentShader: frag,
    blending: AdditiveBlending, transparent: true, depthWrite: false, depthTest: false,
  })
  const mesh = new InstancedMesh(geo, mat, count)
  mesh.frustumCulled = false
  mesh.renderOrder = 6
  mesh.visible = false

  const setSlot = (slot: number, p: Vector3, r: number, g: number, b: number, a: number) => {
    pos.set([p.x, p.y, p.z], slot * 3)
    tint.set([r, g, b, a], slot * 4)
  }
  return {
    mesh,
    setMachine(i, p, r, g, b, a, la = a) {
      setSlot(2 * i, p, r, g, b, a)
      setSlot(2 * i + 1, p, 1.15, 1.2, 1.3, la)
    },
    setForge(p, a, la = a) {
      setSlot(count - 2, p, 1.4, 1.25, 1.0, a)
      setSlot(count - 1, p, 1.15, 1.2, 1.3, la)
    },
    commit() {
      posAttr.needsUpdate = tintAttr.needsUpdate = true
    },
    dispose() { geo.dispose(); mat.dispose(); tex.dispose(); mesh.dispose() },
  }
}
