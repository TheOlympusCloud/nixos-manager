import { describe, expect, it } from 'vitest'
import { emptyState, KEYFRAMES, STATE_KEYS, storyState } from './story'
import { CHAPTERS } from './types'

describe('storyState', () => {
  it('starts every chapter on its keyframe', () => {
    for (const c of CHAPTERS) {
      const s = storyState(c, 0, emptyState())
      for (const k of STATE_KEYS) expect(s[k]).toBeCloseTo(KEYFRAMES[c][k], 9)
    }
  })

  it('is continuous across chapter boundaries', () => {
    for (let i = 0; i < CHAPTERS.length - 1; i++) {
      const end = storyState(CHAPTERS[i], 1, emptyState())
      const next = storyState(CHAPTERS[i + 1], 0, emptyState())
      for (const k of STATE_KEYS) expect(end[k], `${CHAPTERS[i]}→${CHAPTERS[i + 1]} ${k}`).toBeCloseTo(next[k], 9)
    }
    const end = storyState('outro', 1, emptyState())
    for (const k of STATE_KEYS) expect(end[k]).toBeCloseTo(KEYFRAMES.end[k], 9)
  })

  it('is monotonic within a chapter: no field overshoots its keyframes', () => {
    for (const c of CHAPTERS) {
      let prev = storyState(c, 0, emptyState())
      for (let t = 0.05; t <= 1.0001; t += 0.05) {
        const s = storyState(c, t, emptyState())
        for (const k of STATE_KEYS) {
          const dir = Math.sign(storyState(c, 1, emptyState())[k] - KEYFRAMES[c][k])
          expect(dir * (s[k] - prev[k]), `${c} ${k} @${t}`).toBeGreaterThanOrEqual(-1e-9)
        }
        prev = s
      }
    }
  })

  it('clamps t and keeps the story beats', () => {
    expect(storyState('drift', 2, emptyState()).scatter).toBe(1)
    expect(storyState('assemble', 1, emptyState()).scatter).toBe(0)
    expect(storyState('deploy', 1, emptyState()).deploy).toBe(1)
    expect(storyState('deploy', 0.5, emptyState()).deploy).toBeCloseTo(0.5, 9)
    expect(storyState('hero', -1, emptyState()).tiles).toBe(0)
  })
})
