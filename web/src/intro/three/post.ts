// Post chain: scene → output transform → print. The scene is drawn as light on black; the last pass
// prints that light as ink on the page's paper, so every layer follows the theme without its own colours.
import { Vector2, Vector3, type WebGLRenderer } from 'three'
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js'
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js'
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js'
import { ShaderPass } from 'three/addons/postprocessing/ShaderPass.js'
import { hexRgb } from './math'

export interface Post {
  render(): void
  setSize(width: number, height: number, dpr: number): void
  uniforms: { uTime: { value: number }; uFade: { value: number } }
  dispose(): void
}

/** A theme token from app.css, so the canvas never shows a seam against the page. */
function token(name: string, fallback: [number, number, number]): Vector3 {
  const value = typeof document === 'undefined' ? '' : getComputedStyle(document.documentElement).getPropertyValue(name)
  return new Vector3(...hexRgb(value, fallback))
}

const PrintShader = {
  uniforms: {
    tDiffuse: { value: null }, uTime: { value: 0 }, uFade: { value: 1 }, uRes: { value: new Vector2(1, 1) },
    uPaper: { value: token('--bg', [242 / 255, 241 / 255, 236 / 255]) },
    uInk: { value: token('--accent', [45 / 255, 89 / 255, 172 / 255]) },
    uRed: { value: token('--red', [170 / 255, 50 / 255, 42 / 255]) },
  },
  vertexShader: /* glsl */ `
varying vec2 vUv;
void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }`,
  fragmentShader: /* glsl */ `
uniform sampler2D tDiffuse; uniform float uTime; uniform float uFade; uniform vec2 uRes;
uniform vec3 uPaper; uniform vec3 uInk; uniform vec3 uRed;
varying vec2 vUv;
float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7)) + uTime) * 43758.5453); }
void main() {
  vec3 c = texture2D(tDiffuse, vUv).rgb;
  vec2 q = (vUv - 0.5) * vec2(uRes.x / uRes.y, 1.0) * 0.9;
  c *= (1.0 - 0.5 * smoothstep(0.45, 1.1, length(q))) * uFade;
  // Brightness becomes ink density; faint haze stays bare paper, so glows print as crisp marks.
  // Warm light (conflicts, failures) prints in red pencil.
  float density = smoothstep(0.04, 0.75, dot(c, vec3(0.2126, 0.7152, 0.0722)));
  float warm = clamp((c.r - c.b) * 2.5, 0.0, 1.0);
  vec3 col = mix(uPaper, mix(uInk, uRed, warm), density);
  col += (hash(gl_FragCoord.xy) - 0.5) * 0.02; // the paper's tooth
  gl_FragColor = vec4(col, 1.0);
}`,
}

export function createPost(renderer: WebGLRenderer, scene: import('three').Scene, camera: import('three').Camera): Post {
  const composer = new EffectComposer(renderer)
  composer.addPass(new RenderPass(scene, camera))
  composer.addPass(new OutputPass())
  // Display space: after the output transform, so density follows what the eye would have seen.
  const print = new ShaderPass(PrintShader)
  composer.addPass(print)
  return {
    render: () => composer.render(),
    setSize(w, h, dpr) {
      composer.setPixelRatio(dpr)
      composer.setSize(w, h)
      print.uniforms.uRes.value.set(w, h)
    },
    uniforms: { uTime: print.uniforms.uTime as { value: number }, uFade: print.uniforms.uFade as { value: number } },
    dispose: () => {
      for (const pass of composer.passes) pass.dispose()
      composer.dispose()
    },
  }
}
