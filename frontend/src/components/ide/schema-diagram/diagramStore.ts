import { get, set } from 'idb-keyval'

export type DiagramPersistedState = {
  present: string[] // refKeys currently on the canvas
  positions: Record<string, { x: number; y: number }>
  collapsed: string[] // refKeys collapsed to title-only
  depth?: number | 'all' // object-diagram depth control (Infinity persisted as 'all')
  viewport?: { x: number; y: number; zoom: number } // camera pan + zoom
  orthogonalEdges?: boolean // orthogonal (crow's-foot) vs bezier edge routing
}

const EMPTY: DiagramPersistedState = { present: [], positions: {}, collapsed: [] }

export function serializeDiagram(state: DiagramPersistedState): string {
  return JSON.stringify(state)
}

export function deserializeDiagram(raw: string | null): DiagramPersistedState {
  if (!raw) return { ...EMPTY }
  try {
    const parsed = JSON.parse(raw) as Partial<DiagramPersistedState>
    return {
      present: parsed.present ?? [],
      positions: parsed.positions ?? {},
      collapsed: parsed.collapsed ?? [],
      depth: parsed.depth,
      viewport: parsed.viewport,
      orthogonalEdges: parsed.orthogonalEdges,
    }
  } catch {
    return { ...EMPTY }
  }
}

const DB_PREFIX = 'sqlwarden:diagram:'

/** Read a diagram's persisted layout (IndexedDB via idb-keyval), keyed by the
 *  stable diagram tab id. */
export async function loadDiagram(key: string): Promise<DiagramPersistedState> {
  try {
    const raw = (await get(DB_PREFIX + key)) as string | undefined
    return deserializeDiagram(raw ?? null)
  } catch {
    return { ...EMPTY }
  }
}

export async function saveDiagram(key: string, state: DiagramPersistedState): Promise<void> {
  try {
    await set(DB_PREFIX + key, serializeDiagram(state))
  } catch {
    /* persistence is best-effort */
  }
}
