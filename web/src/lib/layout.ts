// Automatic layered layout of the fleet graph (ELK), computed in a web worker.
import type { LayoutReply, LayoutRequest } from './elk.worker'
import type { GEdge, GNode } from './graph'

export type Direction = 'RIGHT' | 'DOWN'
type Positions = Map<string, { x: number; y: number }>

let worker: Worker | undefined
let nextId = 0
const waiting = new Map<number, { resolve: (p: Positions) => void; reject: (e: Error) => void }>()

/** Start the worker, and with it the layout engine download, before the first layout needs it. */
export function preloadLayout(): Worker {
  if (worker) return worker
  const w = new Worker(new URL('./elk.worker.ts', import.meta.url), { type: 'module' })
  w.onmessage = (e: MessageEvent<LayoutReply>) => {
    const { id, positions, error } = e.data
    const p = waiting.get(id)
    waiting.delete(id)
    if (error !== undefined) p?.reject(new Error(error))
    else p?.resolve(new Map(positions))
  }
  // The script failed to load (offline, an upgrade replaced it): fail what waits, retry on the next layout.
  w.onerror = (e) => {
    e.preventDefault()
    for (const p of waiting.values()) p.reject(new Error('The graph layout engine did not load. Reload the page to try again.'))
    waiting.clear()
    w.terminate()
    if (worker === w) worker = undefined
  }
  return (worker = w)
}

export function layout(nodes: GNode[], edges: GEdge[], dir: Direction, inputs: boolean): Promise<Positions> {
  // Machines start the drawing (after the inputs that feed them); standalone homes end it.
  const constraint = (n: GNode) =>
    (n.kind === 'machine' && !inputs) || n.kind === 'input' ? 'FIRST' : n.kind === 'home' ? 'LAST' : 'NONE'
  const req: LayoutRequest = {
    id: ++nextId,
    graph: {
      id: 'root',
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': dir,
        'elk.separateConnectedComponents': 'false',
        'elk.spacing.nodeNode': '16',
        'elk.layered.spacing.nodeNodeBetweenLayers': dir === 'RIGHT' ? '110' : '70',
        'elk.layered.spacing.edgeNodeBetweenLayers': '24',
        'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
        'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
      },
      children: nodes.map((n) => ({ id: n.id, width: n.width, height: n.height, layoutOptions: { 'elk.layered.layering.layerConstraint': constraint(n) } })),
      edges: edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
    },
  }
  return new Promise((resolve, reject) => {
    waiting.set(req.id, { resolve, reject })
    preloadLayout().postMessage(req)
  })
}
