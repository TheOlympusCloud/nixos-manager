import { describe, expect, it } from 'vitest'
import { B32, Buf, bar, big, frameHTML, scramble, spark } from './ascii'

describe('big', () => {
  it('prints five pixel rows as three rows of half blocks', () => {
    // I: 111 010 010 010 111, then a blank column; the fifth pixel row is the top half of the third text row.
    expect(big('I')).toBe('▀█▀ \n █  \n▀▀▀ ')
  })
  it('is case-insensitive and one glyph column per pixel plus a gap', () => {
    expect(big('ok')).toBe(big('OK'))
    expect(big('OK').split('\n').map((l) => l.length)).toEqual([8, 8, 8])
  })
  it('doubles every pixel at scale 2, five rows tall', () => {
    expect(big('I', 2)).toBe('██████  \n  ██    \n  ██    \n  ██    \n██████  ')
  })
})

describe('bar', () => {
  it('fills whole cells, one eighth block for the fraction, and tracks the rest', () => {
    expect(bar(0.5, 10)).toBe('█████<s>░░░░░</s>')
    expect(bar(0.55, 10)).toBe('█████▌<s>░░░░</s>')
  })
  it('clamps to empty and full', () => {
    expect(bar(-1, 4)).toBe('<s>░░░░</s>')
    expect(bar(2, 4)).toBe('████<s></s>')
    expect(bar(NaN, 4)).toBe('<s>░░░░</s>')
  })
})

describe('scramble', () => {
  const s = 'nix build .#pi'
  it('at progress 1 is the text itself', () => {
    expect(scramble(s, 1)).toBe(s)
  })
  it('at progress 0 keeps the length and the spaces, and shows only base-32 elsewhere', () => {
    const out = scramble(s, 0)
    expect(out).toHaveLength(s.length)
    for (let i = 0; i < s.length; i++) {
      if (s[i] === ' ') expect(out[i]).toBe(' ')
      else expect(B32).toContain(out[i])
    }
  })
  it('resolves left to right', () => {
    expect(scramble(s, 0.5).slice(0, 7)).toBe(s.slice(0, 7))
  })
})

describe('Buf.path', () => {
  it('joins two crossing lines into ┼', () => {
    const b = new Buf(5, 5)
    b.path([
      [0, 2],
      [4, 2],
    ])
    b.path([
      [2, 0],
      [2, 4],
    ])
    expect(b.toString().split('\n')[2]).toBe('──┼──')
    expect(b.get(2, 2)).toBe(' ') // the joint lives in the line layer, not the text
  })
  it('turns corners and tees, and returns the cells it passes in order', () => {
    const b = new Buf(4, 3)
    const cells = b.path([
      [0, 0],
      [2, 0],
      [2, 2],
    ])
    expect(cells).toEqual([
      [0, 0],
      [1, 0],
      [2, 0],
      [2, 1],
      [2, 2],
    ])
    b.path([
      [2, 1],
      [3, 1],
    ])
    expect(b.toString()).toBe('──╮ \n  ├─\n  │ ')
  })
})

describe('frameHTML', () => {
  it('draws the frame of its weight with the title on the top line', () => {
    expect(frameHTML(10, 3, 'round', { title: 'Hi' })).toBe('╭─<b> Hi </b>───╮\n│        │\n╰────────╯')
    expect(frameHTML(4, 2, 'double')).toBe('╔══╗\n╚══╝')
  })
  it('casts a ░ shadow to the right and below', () => {
    expect(frameHTML(4, 2, 'square', { shadow: true }).split('\n')).toEqual(['┌──┐', '└──┘<s>░</s>', ' <s>░░░░</s>'])
  })
})

describe('spark', () => {
  it('scales values to eighth-height blocks', () => {
    expect(spark([0, 4, 8])).toBe('▁▅█')
  })
})
