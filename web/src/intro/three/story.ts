// The story as numbers: a keyframe at the start of every chapter, interpolated within the chapter.
// Because chapter N ends on chapter N+1's keyframe, transitions are continuous by construction.
import { clamp01, easeIn, easeInOut, easeInOutQuart, easeOut, easeOutExpo, lerp, linear } from './math'
import { CHAPTERS, type Chapter } from './types'

export interface StoryState {
  /** Planet centre, world x (ignored on portrait screens, where the text stacks above the scene). */
  planetX: number
  /** Planet centre, world y. */
  planetY: number
  planetScale: number
  camDist: number
  /** Extra planet rotation driven by scroll (radians). */
  spin: number
  /** Dot-matrix brightness. */
  dots: number
  /** Rim/atmosphere intensity. */
  rim: number
  /** 0 snowflake blue, 1 challenge orange. */
  warm: number
  /** Tiles: 0 locked on the sphere, 1 drifted apart. */
  scatter: number
  /** Tile opacity. */
  tiles: number
  jitter: number
  /** Lock flash as tiles arrive. */
  lock: number
  arcs: number
  orbit: number
  beams: number
  forge: number
  /** Deploy progress 0..1 (pulse timeline). */
  deploy: number
  meteors: number
  fade: number
}

export const STATE_KEYS = [
  'planetX', 'planetY', 'planetScale', 'camDist', 'spin', 'dots', 'rim', 'warm', 'scatter', 'tiles', 'jitter', 'lock',
  'arcs', 'orbit', 'beams', 'forge', 'deploy', 'meteors', 'fade',
] as const satisfies readonly (keyof StoryState)[]

type Ease = (t: number) => number

const base: StoryState = {
  planetX: 0, planetY: 0, planetScale: 1, camDist: 4.2, spin: 0, dots: 1, rim: 1, warm: 0, scatter: 0, tiles: 0, jitter: 0,
  lock: 0, arcs: 0, orbit: 0, beams: 0, forge: 0, deploy: 0, meteors: 0.5, fade: 1,
}

/** State at the START of each chapter; `end` is the state at the end of the outro. */
export const KEYFRAMES: Record<Chapter | 'end', StoryState> = {
  hero: { ...base, planetY: -2.55, planetScale: 2.05, spin: 0, meteors: 0.8 },
  drift: { ...base, planetY: -2.3, planetScale: 2.05, spin: 0.22, meteors: 0.6 },
  assemble: { ...base, spin: 0.55, dots: 0.12, rim: 0.55, warm: 1, scatter: 1, tiles: 1, jitter: 1, meteors: 0.15 },
  planet: { ...base, spin: 0.85, dots: 0.18, rim: 0.75, tiles: 1, lock: 1, meteors: 0.15 },
  // planet/orbit: the text sits in the left column, so the planet moves right. deploy: a step rail
  // occupies the right third, so the planet shrinks into the gap between text and rail.
  orbit: { ...base, planetX: 1.2, camDist: 4.0, spin: 1.25, arcs: 1, meteors: 0.3 },
  deploy: { ...base, planetX: 1.2, camDist: 5.1, spin: 1.6, arcs: 0.55, orbit: 1, beams: 1, meteors: 0.4 },
  outro: { ...base, planetX: -0.11, planetScale: 0.58, camDist: 5.1, spin: 1.95, arcs: 0.45, orbit: 1, beams: 0.6, forge: 1, deploy: 1, meteors: 0.4 },
  end: { ...base, planetY: -2.7, planetScale: 1.8, camDist: 5.4, spin: 2.3, dots: 0.8, rim: 0.9, deploy: 1, meteors: 0.7, fade: 0.9 },
}

/** Per-chapter easing per field; everything else eases in-out. */
const EASING: Partial<Record<Chapter, Partial<Record<keyof StoryState, Ease>>>> = {
  hero: { planetY: linear, spin: linear },
  drift: { tiles: easeOutExpo, scatter: easeOut, dots: easeOut, warm: easeOut, planetY: easeInOut, planetScale: easeInOut },
  assemble: { scatter: easeInOutQuart, warm: easeIn, jitter: easeIn, rim: easeOut },
  planet: { tiles: easeIn, dots: easeIn, arcs: easeIn, camDist: easeInOut, planetX: easeOut },
  orbit: { orbit: easeOut, beams: easeIn, camDist: easeInOut, arcs: easeOut },
  deploy: { deploy: linear, forge: easeOutExpo, planetX: easeOutExpo, planetScale: easeOutExpo },
  // The closing headline sits in the upper half: the planet drops below it early and the fleet fades fast.
  outro: { forge: easeOut, orbit: easeOut, beams: easeOut, arcs: easeOut, planetX: easeOut, planetY: easeOut, planetScale: easeOut, camDist: easeOut },
}

const NEXT: Record<Chapter, Chapter | 'end'> = {
  hero: 'drift', drift: 'assemble', assemble: 'planet', planet: 'orbit', orbit: 'deploy', deploy: 'outro', outro: 'end',
}

export const isChapter = (c: string): c is Chapter => (CHAPTERS as readonly string[]).includes(c)

/** Story position → state, written into out (no allocation on the scroll path). */
export function storyState(chapter: Chapter, t: number, out: StoryState): StoryState {
  const from = KEYFRAMES[chapter]
  const to = KEYFRAMES[NEXT[chapter]]
  const eases = EASING[chapter]
  const tc = clamp01(t)
  for (const k of STATE_KEYS) {
    const e = eases?.[k] ?? easeInOut
    out[k] = lerp(from[k], to[k], e(tc))
  }
  return out
}

export const emptyState = (): StoryState => ({ ...base })
