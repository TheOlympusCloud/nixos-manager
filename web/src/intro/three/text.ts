// Canvas-drawn textures: the only way text reaches the scene under the CSP (no blob URLs, no remote assets).
import { CanvasTexture, LinearFilter, LinearMipmapLinearFilter, SRGBColorSpace } from 'three'
import type { PackageLabel } from './packages'

/** The console's --mono font (app.css), so the scene's labels are set in the same face as the page. */
export const MONO = (typeof document === 'undefined' ? '' : getComputedStyle(document.documentElement).getPropertyValue('--mono').trim()) || 'monospace'

export interface Canvas2D {
  canvas: HTMLCanvasElement
  ctx: CanvasRenderingContext2D
}

export function makeCanvas(width: number, height: number): Canvas2D {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('2d canvas unavailable')
  return { canvas, ctx }
}

export function textureOf(canvas: HTMLCanvasElement, mipmaps = true): CanvasTexture {
  const tex = new CanvasTexture(canvas)
  tex.colorSpace = SRGBColorSpace
  tex.generateMipmaps = mipmaps
  tex.minFilter = mipmaps ? LinearMipmapLinearFilter : LinearFilter
  tex.magFilter = LinearFilter
  tex.anisotropy = 4
  return tex
}

/** Runs redraw once the page's mono font is usable (and immediately with the fallback). */
export function whenFontReady(redraw: () => void): void {
  const fonts = document.fonts
  if (!fonts) return
  fonts.load(`500 24px ${MONO}`).then(redraw, () => {})
}

export const TILE_COLS = 8
export const TILE_ROWS = 8
export const TILE_CELL_W = 256
export const TILE_CELL_H = 128

/** Draws the 8×8 package-card atlas: dark cards with name, version and a store path, hairline borders. */
export function drawTileAtlas(ctx: CanvasRenderingContext2D, labels: PackageLabel[]): void {
  const W = TILE_CELL_W
  const H = TILE_CELL_H
  ctx.clearRect(0, 0, W * TILE_COLS, H * TILE_ROWS)
  ctx.textBaseline = 'alphabetic'
  for (let i = 0; i < TILE_COLS * TILE_ROWS; i++) {
    const l = labels[i % labels.length]
    const x = (i % TILE_COLS) * W
    const y = Math.floor(i / TILE_COLS) * H
    const g = ctx.createLinearGradient(0, y, 0, y + H)
    g.addColorStop(0, '#0d121a')
    g.addColorStop(1, '#080b11')
    ctx.fillStyle = g
    ctx.fillRect(x, y, W, H)
    ctx.strokeStyle = l.conflict ? 'rgba(255,140,70,0.28)' : 'rgba(126,182,255,0.22)'
    ctx.lineWidth = 1.5
    ctx.strokeRect(x + 1, y + 1, W - 2, H - 2)
    // Six-fold motif: a small hexagon in the corner.
    ctx.strokeStyle = 'rgba(126,182,255,0.35)'
    ctx.lineWidth = 1.5
    ctx.beginPath()
    for (let k = 0; k < 6; k++) {
      const a = (k / 6) * Math.PI * 2 + Math.PI / 6
      const hx = x + W - 24 + Math.cos(a) * 8
      const hy = y + 24 + Math.sin(a) * 8
      if (k === 0) ctx.moveTo(hx, hy)
      else ctx.lineTo(hx, hy)
    }
    ctx.closePath()
    ctx.stroke()
    ctx.fillStyle = '#44536a'
    ctx.font = `500 11px ${MONO}`
    ctx.fillText(l.conflict ? 'pinned by hand' : 'nixpkgs', x + 16, y + 28)
    ctx.fillStyle = '#e8eef7'
    ctx.font = `600 24px ${MONO}`
    ctx.fillText(l.name, x + 16, y + 60)
    ctx.fillStyle = l.conflict ? '#ffa264' : '#7eb6ff'
    ctx.font = `500 20px ${MONO}`
    ctx.fillText(l.version, x + 16, y + 88)
    ctx.fillStyle = '#3b485c'
    ctx.font = `400 11px ${MONO}`
    ctx.fillText(`/nix/store/${l.hash.slice(0, 14)}…-${l.name}`, x + 16, y + 114)
  }
}

export interface AtlasRect {
  /** u0, v0, u1, v1 in texture space (v up). */
  uv: [number, number, number, number]
  /** Pixel size at 1× (the atlas is drawn at 2×). */
  w: number
  h: number
}

/** Rows of labels and node glyphs packed into one 1024×512 atlas, drawn at 2× for crisp text. */
export class LabelAtlas {
  readonly canvas: HTMLCanvasElement
  readonly ctx: CanvasRenderingContext2D
  readonly width = 1024
  readonly height = 512
  readonly rects = new Map<string, AtlasRect>()
  private y = 0

  constructor() {
    const { canvas, ctx } = makeCanvas(this.width, this.height)
    this.canvas = canvas
    this.ctx = ctx
  }

  clear(): void {
    this.ctx.clearRect(0, 0, this.width, this.height)
    this.rects.clear()
    this.y = 0
  }

  private alloc(key: string, w: number, h: number): { x: number; y: number } {
    const x = 0
    const y = this.y
    this.y += h + 4
    if (this.y > this.height) throw new Error('label atlas full')
    this.rects.set(key, {
      uv: [x / this.width, 1 - (y + h) / this.height, (x + w) / this.width, 1 - y / this.height],
      w: w / 2,
      h: h / 2,
    })
    return { x, y }
  }

  /** Mono label, 2× drawn; px is the on-screen text size. */
  label(key: string, text: string, px: number, color: string): void {
    const ctx = this.ctx
    ctx.font = `500 ${px * 2}px ${MONO}`
    const w = Math.ceil(ctx.measureText(text).width) + 8
    const h = Math.ceil(px * 2 * 1.3)
    const { x, y } = this.alloc(key, Math.min(w, this.width), h)
    ctx.fillStyle = color
    ctx.textBaseline = 'middle'
    ctx.fillText(text, x + 4, y + h / 2)
  }

  /** Machine node: a soft halo, a hexagon ring and a bright core, 2× drawn in a (size×size) cell. */
  hexNode(key: string, size: number): void {
    const s = size * 2
    const { x, y } = this.alloc(key, s, s)
    const ctx = this.ctx
    const cx = x + s / 2
    const cy = y + s / 2
    const halo = ctx.createRadialGradient(cx, cy, 0, cx, cy, s / 2)
    halo.addColorStop(0, 'rgba(255,255,255,0.55)')
    halo.addColorStop(0.18, 'rgba(255,255,255,0.22)')
    halo.addColorStop(0.5, 'rgba(255,255,255,0.05)')
    halo.addColorStop(1, 'rgba(255,255,255,0)')
    ctx.fillStyle = halo
    ctx.fillRect(x, y, s, s)
    ctx.strokeStyle = 'rgba(255,255,255,0.85)'
    ctx.lineWidth = 2
    ctx.beginPath()
    const r = s * 0.2
    for (let k = 0; k < 6; k++) {
      const a = (k / 6) * Math.PI * 2 + Math.PI / 6
      const px = cx + Math.cos(a) * r
      const py = cy + Math.sin(a) * r
      if (k === 0) ctx.moveTo(px, py)
      else ctx.lineTo(px, py)
    }
    ctx.closePath()
    ctx.stroke()
    ctx.fillStyle = '#ffffff'
    ctx.beginPath()
    ctx.arc(cx, cy, s * 0.055, 0, Math.PI * 2)
    ctx.fill()
  }

  /** Forgejo node: a rounded square with a commit dot and a branch, so it reads as a repository, not a machine. */
  repoNode(key: string, size: number): void {
    const s = size * 2
    const { x, y } = this.alloc(key, s, s)
    const ctx = this.ctx
    const cx = x + s / 2
    const cy = y + s / 2
    const halo = ctx.createRadialGradient(cx, cy, 0, cx, cy, s / 2)
    halo.addColorStop(0, 'rgba(255,255,255,0.4)')
    halo.addColorStop(0.3, 'rgba(255,255,255,0.08)')
    halo.addColorStop(1, 'rgba(255,255,255,0)')
    ctx.fillStyle = halo
    ctx.fillRect(x, y, s, s)
    const r = s * 0.2
    ctx.strokeStyle = 'rgba(255,255,255,0.9)'
    ctx.lineWidth = 2
    ctx.beginPath()
    ctx.roundRect(cx - r, cy - r, r * 2, r * 2, r * 0.25)
    ctx.stroke()
    ctx.beginPath()
    ctx.moveTo(cx - r * 0.35, cy + r * 0.5)
    ctx.lineTo(cx - r * 0.35, cy - r * 0.5)
    ctx.moveTo(cx - r * 0.35, cy + r * 0.1)
    ctx.quadraticCurveTo(cx - r * 0.35, cy - r * 0.45, cx + r * 0.4, cy - r * 0.45)
    ctx.stroke()
    ctx.fillStyle = '#ffffff'
    for (const [dx, dy] of [[-0.35, 0.5], [-0.35, -0.5], [0.4, -0.45]]) {
      ctx.beginPath()
      ctx.arc(cx + dx * r, cy + dy * r, s * 0.035, 0, Math.PI * 2)
      ctx.fill()
    }
  }
}
