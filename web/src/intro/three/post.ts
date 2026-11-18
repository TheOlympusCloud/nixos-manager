// Post chain: scene → soft bloom (rims, arcs, nodes) → output transform → vignette and fine grain.
import { Vector2, Vector3, type WebGLRenderer } from 'three'
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js'
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js'
import { ShaderPass } from 'three/addons/postprocessing/ShaderPass.js'
import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js'

export interface Post {
  render(): void
  setSize(width: number, height: number, dpr: number): void
  uniforms: { uTime: { value: number }; uFade: { value: number } }
  dispose(): void
}

const GrainShader = {
  uniforms: {
    tDiffuse: { value: null }, uTime: { value: 0 }, uFade: { value: 1 }, uRes: { value: new Vector2(1, 1) },
    // The intro page background (#05070a), so the canvas never shows a seam against the page.
    uBase: { value: new Vector3(5 / 255, 7 / 255, 10 / 255) },
  },
  vertexShader: /* glsl */ `
varying vec2 vUv;
void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }`,
  fragmentShader: /* glsl */ `
uniform sampler2D tDiffuse; uniform float uTime; uniform float uFade; uniform vec2 uRes; uniform vec3 uBase;
varying vec2 vUv;
float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7)) + uTime) * 43758.5453); }
void main() {
  vec4 c = texture2D(tDiffuse, vUv);
  vec2 q = (vUv - 0.5) * vec2(uRes.x / uRes.y, 1.0) * 0.9;
  c.rgb *= 1.0 - 0.5 * smoothstep(0.45, 1.1, length(q));
  c.rgb *= uFade;
  c.rgb = max(c.rgb, uBase);
  c.rgb += (hash(gl_FragCoord.xy) - 0.5) * 0.035;
  gl_FragColor = c;
}`,
}

export function createPost(renderer: WebGLRenderer, scene: import('three').Scene, camera: import('three').Camera): Post {
  const composer = new EffectComposer(renderer)
  composer.addPass(new RenderPass(scene, camera))
  const bloom = new UnrealBloomPass(new Vector2(1, 1), 0.35, 0.3, 0.9)
  composer.addPass(bloom)
  composer.addPass(new OutputPass())
  // Display-space grain and vignette: after the output transform, so the noise never lifts the blacks.
  const grain = new ShaderPass(GrainShader)
  composer.addPass(grain)
  return {
    render: () => composer.render(),
    setSize(w, h, dpr) {
      composer.setPixelRatio(dpr)
      composer.setSize(w, h)
      grain.uniforms.uRes.value.set(w, h)
    },
    uniforms: { uTime: grain.uniforms.uTime as { value: number }, uFade: grain.uniforms.uFade as { value: number } },
    dispose: () => composer.dispose(),
  }
}
