// Automatic layered layout of the fleet graph (ELK), loaded on first use.
import type ELKType from 'elkjs/lib/elk.bundled.js'
import type { GEdge, GNode } from './graph'

export type Direction = 'RIGHT' | 'DOWN'
let elk: InstanceType<typeof ELKType> | undefined

export async function layout(nodes: GNode[], edges: GEdge[], dir: Direction, inputs: boolean): Promise<Map<string, { x: number; y: number }>> {
  if (!elk) elk = new (await import('elkjs/lib/elk.bundled.js')).default()
  // Machines start the drawing (after the inputs that feed them); standalone homes end it.
  const constraint = (n: GNode) =>
    (n.kind === 'machine' && !inputs) || n.kind === 'input' ? 'FIRST' : n.kind === 'home' ? 'LAST' : 'NONE'
  const res = await elk.layout({
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
  })
  return new Map((res.children ?? []).map((c) => [c.id, { x: c.x ?? 0, y: c.y ?? 0 }]))
}
