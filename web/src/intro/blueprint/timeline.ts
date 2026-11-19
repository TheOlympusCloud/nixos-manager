// The blueprint chapter as pure data: the configuration graph of the example fleet and what the
// drawing shows at every point of its scroll-scrubbed timeline (0..1).
import { clamp01, window01 } from '../story'

export type Kind = 'flake' | 'host' | 'module' | 'managed' | 'pkg'
export interface Node {
  id: string
  label: string
  kind: Kind
  /** Layer (column on wide screens, row on narrow ones) and position inside it. */
  col: number
  row: number
  /** Position on the narrow layout, where packages wrap over two rows, and a label that fits it. */
  m: [number, number]
  short?: string
}

export const NODES: Node[] = [
  { id: 'flake', label: 'flake.nix', kind: 'flake', col: 0, row: 0, m: [0, 1.5] },
  { id: 'ws', label: 'hosts/workstation', kind: 'host', col: 1, row: 0, m: [1, 0.5] },
  { id: 'hs', label: 'hosts/homeserver', kind: 'host', col: 1, row: 1, m: [1, 1.5] },
  { id: 'pi', label: 'hosts/pi', kind: 'host', col: 1, row: 2, m: [1, 2.5] },
  { id: 'common', label: 'modules/common.nix', kind: 'module', col: 2, row: 0, m: [2, 0] },
  { id: 'desktop', label: 'modules/desktop.nix', kind: 'module', col: 2, row: 1, m: [2, 1] },
  { id: 'hsm', label: 'homeserver/managed.json', kind: 'managed', col: 2, row: 2, m: [2, 2], short: 'hs/managed.json' },
  { id: 'pim', label: 'pi/managed.json', kind: 'managed', col: 2, row: 3, m: [2, 3], short: 'pi/managed.json' },
  { id: 'openssh', label: 'openssh', kind: 'pkg', col: 3, row: 0, m: [3, 0] },
  { id: 'tailscale', label: 'tailscale', kind: 'pkg', col: 3, row: 1, m: [3, 1] },
  { id: 'firefox', label: 'firefox', kind: 'pkg', col: 3, row: 2, m: [3, 2] },
  { id: 'jellyfin', label: 'jellyfin', kind: 'pkg', col: 3, row: 3, m: [3, 3] },
  { id: 'nextcloud', label: 'nextcloud', kind: 'pkg', col: 3, row: 4, m: [4, 0] },
  { id: 'forgejo', label: 'forgejo', kind: 'pkg', col: 3, row: 5, m: [4, 1] },
  { id: 'hass', label: 'home-assistant', kind: 'pkg', col: 3, row: 6, m: [4, 2] },
]

export const EDGES: [string, string][] = [
  ['flake', 'ws'],
  ['flake', 'hs'],
  ['flake', 'pi'],
  ['ws', 'common'],
  ['ws', 'desktop'],
  ['hs', 'common'],
  ['hs', 'hsm'],
  ['pi', 'common'],
  ['pi', 'pim'],
  ['common', 'openssh'],
  ['common', 'tailscale'],
  ['desktop', 'firefox'],
  ['hsm', 'jellyfin'],
  ['hsm', 'nextcloud'],
  ['hsm', 'forgejo'],
  ['pim', 'hass'],
]

/** The node that fails its evaluation, and what Nix says about it. */
export const FAILING = 'pi'
export const ERROR_LINES = ['error: infinite recursion encountered', '       at hosts/pi/default.nix:12:3:', '           12|   imports = [ ./default.nix ];', '              |   ^']
export const FIX_LINES = ['- imports = [ ./default.nix ];', '+ imports = [ ../../modules/common.nix ];']
export const STAMPS = ['nix flake check · passed', 'dry-build · 3 machines']

/** The acts of the timeline, in order, as windows of t. */
export const STEPS = [
  { id: 'draw', from: 0, to: 0.3, label: 'Draw the graph' },
  { id: 'evaluate', from: 0.3, to: 0.54, label: 'Evaluate every node' },
  { id: 'fail', from: 0.54, to: 0.66, label: 'One machine fails' },
  { id: 'fix', from: 0.66, to: 0.82, label: 'Fix and re-evaluate' },
  { id: 'stamp', from: 0.82, to: 1, label: 'Checked' },
] as const
export type StepId = (typeof STEPS)[number]['id']

export const stepAt = (t: number): StepId => (STEPS.find((s) => t < s.to) ?? STEPS[STEPS.length - 1]).id

export type Status = 'idle' | 'scanning' | 'ok' | 'fail' | 'fixing'
export interface NodeState {
  draw: number
  /** The scan sweep across the node, 0..1 (1 once it has been evaluated). */
  scan: number
  status: Status
}
export interface State {
  step: StepId
  nodes: Record<string, NodeState>
  edges: number[]
  /** How much of the error callout, the fix callout, and the stamps is visible. */
  error: number
  fix: number
  stamp: number
  /** The annotation layer (dimensions, leaders, title block): 0..1. */
  paper: number
}

// Draw order: the layers left to right; an edge is drawn just before the node it leads to.
const order = [...NODES].sort((a, b) => a.col - b.col || a.row - b.row)
const edgeSlot = EDGES.map(([, b]) => order.findIndex((n) => n.id === b) - 0.5)

const stagger = (i: number, n: number, from: number, to: number, overlap = 0.55) => {
  const span = (to - from) / (n - overlap * (n - 1))
  const start = from + i * span * (1 - overlap)
  return [start, start + span] as const
}

const [DRAW, EVAL, FAIL, FIX, STAMP] = STEPS

export function blueprintAt(raw: number): State {
  const t = clamp01(raw)
  const nodes: Record<string, NodeState> = {}
  const n = order.length
  order.forEach((node, i) => {
    const [a, b] = stagger(i, n, DRAW.from, DRAW.to)
    nodes[node.id] = { draw: window01(t, a, b), scan: 0, status: 'idle' }
  })
  const edges = edgeSlot.map((slot) => {
    const [a, b] = stagger(slot, n, DRAW.from, DRAW.to)
    return window01(t, a, b)
  })

  // Evaluation: every node scans in the same order; the failing one scans last and fails.
  const evalOrder = [...order.filter((x) => x.id !== FAILING), order.find((x) => x.id === FAILING)!]
  evalOrder.forEach((node, i) => {
    const [a, b] = stagger(i, n, EVAL.from, EVAL.to, 0.65)
    const s = nodes[node.id]
    s.scan = window01(t, a, b)
    if (s.scan > 0 && s.scan < 1) s.status = 'scanning'
    else if (s.scan >= 1) s.status = 'ok'
  })
  const failing = nodes[FAILING]
  // The fix is written first (the callout), then the node redraws and scans again.
  const fixProgress = window01(t, FIX.from + 0.4 * (FIX.to - FIX.from), FIX.to)
  if (t >= FAIL.from && t < FIX.from) failing.status = 'fail'
  else if (t >= FIX.from) {
    failing.status = fixProgress < 1 ? 'fixing' : 'ok'
    failing.scan = fixProgress
  }

  return {
    step: stepAt(t),
    nodes,
    edges,
    error: window01(t, FAIL.from, FAIL.from + 0.06),
    fix: window01(t, FIX.from, FIX.from + 0.3),
    stamp: window01(t, STAMP.from, STAMP.from + 0.08),
    paper: window01(t, 0, 0.08),
  }
}
