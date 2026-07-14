import ELK, { type ElkNode } from 'elkjs/lib/elk.bundled.js'

export type LayoutNode = { id: string; width: number; height: number }
export type LayoutEdge = { id: string; source: string; target: string }

const LAYOUT_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.layered.spacing.nodeNodeBetweenLayers': '80',
  'elk.spacing.nodeNode': '40',
  'elk.edgeRouting': 'ORTHOGONAL',
}

export function toElkGraph(nodes: LayoutNode[], edges: LayoutEdge[]): ElkNode {
  return {
    id: 'root',
    layoutOptions: LAYOUT_OPTIONS,
    children: nodes.map((n) => ({ id: n.id, width: n.width, height: n.height })),
    edges: edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  }
}

const elk = new ELK()

/** Runs elk auto-layout and returns each node id's top-left position. */
export async function layoutGraph(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
): Promise<Map<string, { x: number; y: number }>> {
  const laid = await elk.layout(toElkGraph(nodes, edges))
  const positions = new Map<string, { x: number; y: number }>()
  for (const child of laid.children ?? []) {
    positions.set(child.id, { x: child.x ?? 0, y: child.y ?? 0 })
  }
  return positions
}
