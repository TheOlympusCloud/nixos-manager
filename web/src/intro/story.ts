// The scroll story: which chapter the viewport is in and how far through it, plus the
// small pure helpers the chapters scrub with. The scene only renders the chapters in CHAPTERS;
// 'blueprint' and 'console' are drawn by the page with the canvas faded out.
import type { Chapter } from './three/types'

export type StoryChapter = Chapter | 'blueprint' | 'console'

/** Pinned scroll length of each chapter in viewport heights (console and outro add flowing content after their pin). */
export const STORY: { id: StoryChapter; len: number }[] = [
  { id: 'hero', len: 1.5 },
  { id: 'drift', len: 2.6 },
  { id: 'assemble', len: 3 },
  { id: 'planet', len: 2.4 },
  { id: 'orbit', len: 2.6 },
  { id: 'blueprint', len: 5 },
  { id: 'console', len: 1.6 },
  { id: 'deploy', len: 3.2 },
  { id: 'outro', len: 2 },
]

export const isSceneChapter = (id: StoryChapter): id is Chapter => id !== 'blueprint' && id !== 'console'

export type Section = { id: StoryChapter; top: number; height: number; pinned: boolean }
export type Position = { id: StoryChapter; t: number; index: number }

export const clamp01 = (x: number) => (x < 0 ? 0 : x > 1 ? 1 : x)
/** 0 before `a`, 1 after `b`, linear in between. */
export const window01 = (t: number, a: number, b: number) => clamp01((t - a) / (b - a))
export const easeOutExpo = (t: number) => (t >= 1 ? 1 : 1 - Math.pow(2, -10 * t))
export const easeInOut = (t: number) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2)

/**
 * Where the story is for a scroll offset. A pinned chapter progresses while its stage is stuck
 * (its height minus one viewport); an unpinned one progresses over its whole height.
 */
export function locate(y: number, vh: number, sections: Section[]): Position {
  let index = 0
  for (let i = 0; i < sections.length; i++) if (sections[i].top <= y) index = i
  const s = sections[index]
  const range = s.pinned ? s.height - vh : s.height
  return { id: s.id, t: range > 0 ? clamp01((y - s.top) / range) : 0, index }
}

// ---- the assemble counter: 100 000+ packages collapse into 1 graph --------

export const PACKAGES = 100_000

/** Exponential countdown, so the digits collapse like the reference: 100 000 → 1. */
export function countdown(t: number, from = PACKAGES, to = 1): number {
  const k = easeInOut(clamp01(t))
  return Math.round(Math.exp(Math.log(from) * (1 - k) + Math.log(to) * k))
}

/** Thousands grouped with a narrow no-break space: 100000 → "100 000". */
export const formatGrouped = (n: number) => String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ' ')

/** The caption under the counter at this point: the number of packages until it has become one graph. */
export const counterLabel = (t: number) => (t >= 0.98 ? 'graph' : 'packages')

/** Lines of a statement, each with its own reveal window inside [a, b]. */
export function lineWindows(count: number, a = 0.1, b = 0.6, overlap = 0.5): [number, number][] {
  const span = (b - a) / (count - overlap * (count - 1))
  return Array.from({ length: count }, (_, i) => [a + i * span * (1 - overlap), a + i * span * (1 - overlap) + span])
}
