// The graph layout engine (ELK, ~1.5 MB) in its own same-origin worker file, so laying out
// a big fleet never freezes the page (the CSP rules out blob: workers).
import ELK, { type ElkNode } from 'elkjs/lib/elk.bundled.js'

export type LayoutRequest = { id: number; graph: ElkNode }
export type LayoutReply = { id: number; positions?: [string, { x: number; y: number }][]; error?: string }

const elk = new ELK()
const post = (reply: LayoutReply) => postMessage(reply)

// Assigned after `new ELK()`: the bundle installs its own handler when it finds itself in a worker.
self.onmessage = async (e: MessageEvent<LayoutRequest>) => {
  const { id, graph } = e.data
  try {
    const res = await elk.layout(graph)
    post({ id, positions: (res.children ?? []).map((c) => [c.id, { x: c.x ?? 0, y: c.y ?? 0 }]) })
  } catch (err) {
    post({ id, error: err instanceof Error ? err.message : String(err) })
  }
}
