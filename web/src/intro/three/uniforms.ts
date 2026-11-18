// Uniform objects shared by every material: one `{ value }` per quantity, updated once per frame.
import { Vector3, type IUniform } from 'three'

export interface Shared {
  uTime: IUniform<number>
  /** Planet rotation, radians. */
  uSpin: IUniform<number>
  uDpr: IUniform<number>
  /** World units per CSS pixel at distance 1 (2·tan(fov/2) / viewport height). */
  uPixelToWorld: IUniform<number>
  uLightDir: IUniform<Vector3>
}

export const sharedUniforms = (): Shared => ({
  uTime: { value: 0 },
  uSpin: { value: 0 },
  uDpr: { value: 1 },
  uPixelToWorld: { value: 0.001 },
  uLightDir: { value: new Vector3(-0.45, 0.72, 0.52).normalize() },
})

/** Rotates p about y by uSpin; shared by every shader that lives on the planet. */
export const GLSL_SPIN = /* glsl */ `
vec3 spinY(vec3 p, float a) {
  float c = cos(a), s = sin(a);
  return vec3(p.x * c + p.z * s, p.y, -p.x * s + p.z * c);
}
`

export const GLSL_OUT = /* glsl */ `
  #include <tonemapping_fragment>
  #include <colorspace_fragment>
`
