// The contract between the intro page (which owns scrolling, text and layout)
// and the WebGL scene behind it (which only renders a story position).

/** The chapters the scene renders, in scroll order. */
export const CHAPTERS = ['hero', 'drift', 'assemble', 'planet', 'orbit', 'deploy', 'outro'] as const
export type Chapter = (typeof CHAPTERS)[number]

export interface SceneOptions {
  canvas: HTMLCanvasElement
  /** No autonomous motion: render only when the story position changes. */
  reducedMotion: boolean
  /** Machines shown in orbit, by name (e.g. workstation, homeserver, pi). */
  machines: string[]
}

export interface IntroScene {
  /** Where the story is: a chapter and how far through it (0..1). Called every scroll frame; must be cheap. */
  setStory(chapter: Chapter, t: number): void
  /** Canvas size in CSS pixels and the device pixel ratio to render at (already capped by the page). */
  resize(width: number, height: number, dpr: number): void
  /** Stop rendering while hidden (tab in background, canvas faded out). */
  setPaused(paused: boolean): void
  /** Pointer in -1..1 for subtle parallax; null when the pointer leaves. */
  setPointer(x: number | null, y: number | null): void
  /** Last frame's draw counts and a smoothed CPU time per frame (ms), for a dev overlay. Optional. */
  stats?(): { calls: number; triangles: number; points: number; lines: number; frameMs: number }
  dispose(): void
}

export type CreateScene = (opts: SceneOptions) => IntroScene
