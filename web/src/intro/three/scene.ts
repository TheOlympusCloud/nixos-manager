// The WebGL scene behind the intro: owns the renderer and the frame loop, and turns a story position
// (chapter, t) into uniforms. Everything that moves is driven from uniforms, so a frame is cheap.
import { Group, NeutralToneMapping, PerspectiveCamera, Scene, Timer, Vector3, WebGLRenderer } from 'three'
import { createArcs } from './arcs'
import { orbitPoint, orbitSpeed, pulseWindow, smoothstep } from './math'
import { createNodes } from './nodes'
import { createOrbits, MAX_MACHINES } from './orbits'
import { createPlanet } from './planet'
import { createPost } from './post'
import { createPulses } from './pulses'
import { createMeteors, createStars } from './starfield'
import { emptyState, isChapter, STATE_KEYS, storyState, type StoryState } from './story'
import { createTiles } from './tiles'
import { CHAPTERS, type CreateScene } from './types'
import { sharedUniforms } from './uniforms'

const BASE_FOV = 36
const BASE_ASPECT = 1.6
const DEFAULT_MACHINES = ['workstation', 'homeserver', 'pi']

export const createIntroScene: CreateScene = ({ canvas, reducedMotion, machines }) => {
  const names = (machines.length ? machines : DEFAULT_MACHINES).slice(0, MAX_MACHINES)
  const renderer = new WebGLRenderer({
    canvas, antialias: false, alpha: false, stencil: false, powerPreference: 'high-performance',
  })
  // Clear to true black: no light is no ink, so the print pass in post.ts leaves bare paper there.
  renderer.setClearColor(0x000000, 1)
  renderer.info.autoReset = false // the post passes render too; counters cover the whole frame
  renderer.toneMapping = NeutralToneMapping
  renderer.toneMappingExposure = 1.15

  const scene = new Scene()
  const camera = new PerspectiveCamera(BASE_FOV, BASE_ASPECT, 0.1, 200)
  const shared = sharedUniforms()
  const world = new Group()
  scene.add(world)

  const planet = createPlanet(shared)
  const tiles = createTiles(shared)
  const arcs = createArcs(shared, planet.hubs)
  const orbits = createOrbits(shared, names.length)
  const nodes = createNodes(shared, names)
  const pulses = createPulses(shared, orbits.uniforms.uMachine, names.length)
  const meteors = createMeteors(shared)
  const stars = createStars(shared)
  world.add(planet.group, tiles.mesh, arcs.group, orbits.group, nodes.mesh, pulses.group, meteors.mesh)
  scene.add(stars.points)
  const post = createPost(renderer, scene, camera)

  // Compile every program up front so the first scroll never stalls on a shader build.
  for (const o of [tiles.mesh, arcs.group, orbits.group, nodes.mesh, pulses.group]) o.visible = true
  const compiled = renderer.compileAsync(scene, camera).catch(() => {})
  for (const o of [tiles.mesh, arcs.group, orbits.group, nodes.mesh, pulses.group]) o.visible = false

  const target = emptyState()
  const cur = emptyState()
  storyState('hero', 0, target)
  Object.assign(cur, target)
  let storyTime = 0
  let viewScale = 1
  let sideShift = 1
  const pointerTarget = { x: 0, y: 0 }
  const pointer = { x: 0, y: 0 }
  const timer = new Timer()
  const mpos = new Vector3()
  const forgePos = pulses.uniforms.uForge.value
  let paused = false
  let lost = false
  let disposed = false
  let pending = 0
  let frameMs = 0

  function apply(s: StoryState, time: number) {
    shared.uTime.value = time
    shared.uSpin.value = time * 0.025 + s.spin
    // Portrait: the text stacks above the scene, so the planet sits lower and stays centred.
    world.position.set(s.planetX * viewScale * sideShift, (s.planetY - 1.2 * (1 - sideShift)) * viewScale, 0)
    // Shrinks below 1 only make room for text beside the scene; on portrait the text stacks above it.
    const scale = s.planetScale < 1 ? s.planetScale ** sideShift : s.planetScale
    world.scale.setScalar(scale * viewScale)
    camera.position.set(pointer.x * 0.22, pointer.y * 0.14, s.camDist)
    camera.lookAt(0, 0, 0)

    planet.uniforms.uDots.value = s.dots
    planet.uniforms.uRim.value = s.rim
    planet.uniforms.uWarm.value = s.warm

    tiles.mesh.visible = s.tiles > 0.002
    tiles.uniforms.uScatter.value = s.scatter
    tiles.uniforms.uAlpha.value = s.tiles
    tiles.uniforms.uWarm.value = s.warm
    tiles.uniforms.uJitter.value = s.jitter
    tiles.uniforms.uLock.value = s.lock

    arcs.group.visible = s.arcs > 0.002
    arcs.uniforms.uArcs.value = s.arcs

    const showOrbit = s.orbit > 0.002 || s.beams > 0.002
    orbits.group.visible = showOrbit
    orbits.uniforms.uOrbit.value = s.orbit
    orbits.uniforms.uBeams.value = s.beams
    nodes.mesh.visible = showOrbit || s.forge > 0.002
    for (let i = 0; i < names.length; i++) {
      orbitPoint(i, i * 2.1 + 0.6 + time * orbitSpeed(i), mpos)
      orbits.uniforms.uMachine.value[i].copy(mpos)
      // Deploy: blue until the commit lands, a green flash as it switches, then a calm green.
      const { start, arrive } = pulseWindow(i, names.length)
      const since = s.deploy - arrive
      const switched = smoothstep(0, 0.02, since)
      const flash = since > 0 ? Math.exp(-since * 16) * 2.2 : 0
      // Above 1 so the nodes print in solid ink.
      const r = (0.6 + (0.35 - 0.6) * switched) * (1.5 + flash)
      const g = (0.78 + (1.0 - 0.78) * switched) * (1.5 + flash)
      const b = (1.0 + (0.6 - 1.0) * switched) * (1.5 + flash)
      // Behind the planet the node still shows, dimmed, so every machine stays readable.
      const behind = mpos.z < 0 && mpos.x * mpos.x + mpos.y * mpos.y < 1.2 ? 0.4 : 1
      // Portrait: the DOM lists the machines over the scene, so in-scene labels would only collide with it.
      nodes.setMachine(i, mpos, r, g, b, s.orbit * behind, s.orbit * behind * sideShift)
      const span = arrive - start
      pulses.uniforms.uHead.value[i] = s.deploy < start ? -1 : Math.min((s.deploy - start) / span, 1.4)
    }
    nodes.setForge(forgePos, s.forge, s.forge * sideShift)
    nodes.commit()
    pulses.group.visible = s.forge > 0.002 && s.deploy > 0
    pulses.uniforms.uPulse.value = s.forge

    meteors.uniforms.uMeteors.value = reducedMotion ? 0 : s.meteors
    stars.points.rotation.set(pointer.y * 0.02, -pointer.x * 0.03 + shared.uSpin.value * 0.08, 0)
    post.uniforms.uTime.value = time
    post.uniforms.uFade.value = s.fade
  }

  function frame() {
    timer.update()
    const dt = Math.min(timer.getDelta(), 0.05)
    const k = 1 - Math.exp(-dt * 9)
    for (const key of STATE_KEYS) cur[key] += (target[key] - cur[key]) * k
    pointer.x += (pointerTarget.x - pointer.x) * k
    pointer.y += (pointerTarget.y - pointer.y) * k
    const t0 = performance.now()
    renderer.info.reset()
    apply(cur, timer.getElapsed())
    post.render()
    frameMs += (performance.now() - t0 - frameMs) * 0.1
  }

  function syncLoop() {
    const run = !paused && !lost && !disposed && !reducedMotion
    renderer.setAnimationLoop(run ? frame : null)
  }

  /** Reduced motion: one render per story change, coalesced to a frame. */
  function requestRender() {
    if (pending || lost || disposed) return
    pending = requestAnimationFrame(() => {
      pending = 0
      Object.assign(cur, target)
      apply(cur, storyTime)
      post.render()
    })
  }

  const onLost = (e: Event) => {
    e.preventDefault()
    lost = true
    syncLoop()
  }
  const onRestored = () => {
    lost = false
    syncLoop()
    if (reducedMotion) requestRender()
  }
  canvas.addEventListener('webglcontextlost', onLost)
  canvas.addEventListener('webglcontextrestored', onRestored)

  if (reducedMotion) compiled.then(requestRender)
  else syncLoop()

  return {
    setStory(chapter, t) {
      if (!isChapter(chapter)) return
      storyState(chapter, t, target)
      storyTime = (CHAPTERS.indexOf(chapter) + t) * 6
      if (reducedMotion) requestRender()
    },
    resize(width, height, dpr) {
      const w = Math.max(1, width)
      const h = Math.max(1, height)
      renderer.setPixelRatio(dpr)
      renderer.setSize(w, h, false)
      post.setSize(w, h, dpr)
      // Keep the planet framed on narrow screens: widen the view a little, then shrink the world.
      const aspect = w / h
      const fit = Math.max(1, BASE_ASPECT / aspect)
      const fov = 2 * Math.atan(Math.tan((BASE_FOV * Math.PI) / 360) * Math.min(fit, 1.45))
      viewScale = Math.min(1, 1.45 / fit)
      sideShift = Math.min(1, Math.max(0, (aspect - 1) / 0.5)) // portrait: the text stacks above, keep the planet centred
      camera.aspect = aspect
      camera.fov = (fov * 180) / Math.PI
      camera.updateProjectionMatrix()
      shared.uDpr.value = dpr
      shared.uPixelToWorld.value = (2 * Math.tan(fov / 2)) / h
      if (reducedMotion) requestRender()
    },
    setPaused(p) {
      paused = p
      syncLoop()
    },
    setPointer(x, y) {
      if (reducedMotion) return
      pointerTarget.x = x ?? 0
      pointerTarget.y = y ?? 0
    },
    stats: () => ({
      calls: renderer.info.render.calls,
      triangles: renderer.info.render.triangles,
      points: renderer.info.render.points,
      lines: renderer.info.render.lines,
      frameMs,
    }),
    dispose() {
      disposed = true
      renderer.setAnimationLoop(null)
      if (pending) cancelAnimationFrame(pending)
      canvas.removeEventListener('webglcontextlost', onLost)
      canvas.removeEventListener('webglcontextrestored', onRestored)
      for (const m of [planet, tiles, arcs, orbits, nodes, pulses, meteors, stars, post]) m.dispose()
      scene.clear()
      renderer.dispose()
      renderer.forceContextLoss()
    },
  }
}
