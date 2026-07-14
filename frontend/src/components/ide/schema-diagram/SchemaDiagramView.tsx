import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Background,
  ControlButton,
  Controls,
  getNodesBounds,
  getViewportForBounds,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useNodesState,
  useReactFlow,
  type Edge,
  type EdgeTypes,
  type Node,
  type NodeTypes,
  type Viewport,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { toPng, toSvg } from 'html-to-image'
import { toast } from 'sonner'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { Button } from '#/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { api } from '#/lib/api/client'
import type { ObjectDetail, ObjectRef, Workspace } from '#/lib/api/types'
import {
  orgConnectionCatalogQueryOptions,
  orgConnectionObjectQueryOptions,
  orgConnectionRelationshipsQueryOptions,
  orgConnectionSchemaSpecQueryOptions,
  connectionObjectsQueryKeyPrefix,
  connectionRelationshipsQueryKey,
} from '#/lib/api/query'
import { useIde, type EditorTab } from '../useIdeStore'
import { newObjectTab } from '../object-detail/objectTab'
import {
  edgeCardinality,
  estimateNodeSize,
  planNamespaceSeed,
  reachableRefs,
  refKey,
  relationshipHandleId,
} from './diagramModel'
import { FkEdge } from './edges/FkEdge'
import { layoutGraph } from './layout'
import { loadDiagram, saveDiagram } from './diagramStore'
import { diagramFileName, downloadDataUrl, exportDimensions, type ExportFormat } from './export'
import { TableNode, type HoverRelation, type TableNodeData } from './nodes/TableNode'
import { OBJECT_REF_DND_MIME } from './dnd'
import { Tip } from './Tip'
import { useEvictGoneSession } from '../sessionErrors'
import { resolveDiagramViewState } from './viewState'

const NODE_TYPES: NodeTypes = { table: TableNode }
const EDGE_TYPES: EdgeTypes = { fk: FkEdge }
type FlowNode = Node<TableNodeData, 'table'>

type Box = { x: number; y: number; w: number; h: number }
function boxesOverlap(a: Box, b: Box, pad = 16): boolean {
  return (
    a.x < b.x + b.w + pad && a.x + a.w + pad > b.x && a.y < b.y + b.h + pad && a.y + a.h + pad > b.y
  )
}
// Find an empty slot at startX, scanning vertically (down then up) from startY so
// a newly expanded node lands next to its source without overlapping anything.
function findFreePosition(
  startX: number,
  startY: number,
  w: number,
  h: number,
  occupied: Box[],
): { x: number; y: number } {
  const step = h + 24
  for (let i = 0; i <= 40; i++) {
    for (const dy of i === 0 ? [0] : [i * step, -i * step]) {
      const box: Box = { x: startX, y: startY + dy, w, h }
      if (!occupied.some((o) => boxesOverlap(box, o))) return { x: box.x, y: box.y }
    }
  }
  return { x: startX, y: startY }
}

export function SchemaDiagramView(props: {
  orgSlug: string
  workspace: Workspace
  tab: EditorTab
}) {
  return (
    <ReactFlowProvider>
      <DiagramCanvas {...props} />
    </ReactFlowProvider>
  )
}

function DiagramCanvas({
  orgSlug,
  workspace,
  tab,
}: {
  orgSlug: string
  workspace: Workspace
  tab: EditorTab
}) {
  const target = tab.diagramTarget
  const connectionId = tab.connectionId
  const driver = tab.driver ?? 'postgres'
  const namespace = target
    ? target.kind === 'namespace'
      ? target.namespace
      : target.ref.namespace
    : ''

  const sessionId = useIde((s) => (connectionId ? s.sessions[connectionId] : undefined))
  const setSession = useIde((s) => s.setSession)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)
  const openTab = useIde((s) => s.openTab)
  const queryClient = useQueryClient()
  const { fitView, screenToFlowPosition, getNodes, getViewport, setViewport } = useReactFlow()
  const savedViewport = useRef<Viewport | null>(null)
  const viewportRestored = useRef(false)
  const containerRef = useRef<HTMLDivElement | null>(null)

  const enabled = Boolean(sessionId && connectionId && target)

  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(
      orgSlug,
      workspace.id,
      connectionId ?? 0,
      sessionId ?? '',
    ),
    enabled,
  })
  const catalogQuery = useQuery({
    ...orgConnectionCatalogQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? ''),
    enabled,
  })
  const relQuery = useQuery({
    ...orgConnectionRelationshipsQueryOptions(
      orgSlug,
      workspace.id,
      connectionId ?? 0,
      sessionId ?? '',
      namespace,
    ),
    enabled,
  })

  // A 410 from any diagram query means the server-side session died — drop it
  // so the canvas flips to its reconnect pane instead of silently stalling.
  useEvictGoneSession(connectionId, [specQuery.error, catalogQuery.error, relQuery.error])

  const spec = specQuery.data?.spec
  const edges = useMemo(() => relQuery.data?.relationships ?? [], [relQuery.data])

  const refByKey = useMemo(() => {
    const map = new Map<string, ObjectRef>()
    const diagramKinds = new Set(
      (spec?.kinds ?? []).filter((k) => k.supports_diagram).map((k) => k.kind),
    )
    for (const ns of catalogQuery.data?.catalog?.namespaces ?? []) {
      if (ns.name !== namespace) continue
      for (const group of ns.groups ?? []) {
        if (!diagramKinds.has(group.kind)) continue
        for (const ref of group.objects) map.set(refKey(ref), ref)
      }
    }
    for (const e of edges) {
      map.set(refKey(e.source), e.source)
      map.set(refKey(e.references), e.references)
    }
    if (target?.kind === 'object') map.set(refKey(target.ref), target.ref)
    return map
  }, [catalogQuery.data, edges, spec, namespace, target])

  const [present, setPresent] = useState<ObjectRef[]>([])
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  // Default seed depth for an object diagram: 1 hop (focused), like DataGrip/
  // DBeaver. The toolbar depth control re-seeds to 1 / 2 / all hops.
  const [depth, setDepth] = useState<number>(1)
  // Edge routing: bezier (curved, default) vs orthogonal (right-angle, shows
  // crow's-foot cardinality). Toggled from the Controls panel; persisted.
  const [orthogonal, setOrthogonal] = useState(false)
  // Keys-only mode: render just PK/FK columns to de-clutter big diagrams.
  // Toggled from the Controls panel; persisted.
  const [keysOnly, setKeysOnly] = useState(false)
  const [readyEdgeResolution, setReadyEdgeResolution] = useState('')
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>([])
  const savedPositions = useRef<Record<string, { x: number; y: number }>>({})
  const seededRef = useRef(false)
  const hydratedRef = useRef(false)
  // Gates the seed so it runs only after we've checked persisted state — this
  // stops an async hydrate from clobbering a fresh seed (and vice-versa).
  const [hydrateChecked, setHydrateChecked] = useState(false)
  // Bumped whenever the node set changes structurally (seed / expand / drop /
  // manual re-layout) to trigger a full, consistent elk layout of every node.
  const [layoutReq, setLayoutReq] = useState(0)
  const requestLayout = useCallback(() => setLayoutReq((v) => v + 1), [])

  // Restore the persisted diagram once the schema data (and thus refByKey) is
  // ready: positions, collapse, depth, AND the working set of tables — so
  // expanded/removed tables are exactly where the user left them on reload. If
  // there's a saved set, mark it seeded so the fresh seed below doesn't run.
  useEffect(() => {
    if (hydratedRef.current || !catalogQuery.isSuccess || !relQuery.isSuccess) return
    hydratedRef.current = true
    void loadDiagram(tab.id).then((saved) => {
      savedPositions.current = saved.positions
      savedViewport.current = saved.viewport ?? null
      if (saved.collapsed.length > 0) setCollapsed(new Set(saved.collapsed))
      if (saved.depth != null) setDepth(saved.depth === 'all' ? Infinity : saved.depth)
      if (saved.orthogonalEdges != null) setOrthogonal(saved.orthogonalEdges)
      if (saved.keysOnly != null) setKeysOnly(saved.keysOnly)
      const refs = saved.present
        .map((k) => refByKey.get(k))
        .filter((r): r is ObjectRef => Boolean(r))
      if (refs.length > 0) {
        seededRef.current = true
        setPresent(refs)
      }
      setHydrateChecked(true)
    })
  }, [tab.id, refByKey, catalogQuery.isSuccess, relQuery.isSuccess])

  // Seed the working set from the target — only after the queries have
  // SUCCEEDED (so edges are populated) and persisted positions have loaded.
  // Guarding on isSuccess (not !isLoading) matters: a disabled/idle query
  // reports isLoading:false with no data, which would otherwise seed with empty
  // edges and show only the anchor table.
  useEffect(() => {
    if (seededRef.current || !hydrateChecked || !target) return
    if (!catalogQuery.isSuccess || !relQuery.isSuccess) return
    seededRef.current = true
    const seed =
      target.kind === 'object'
        ? reachableRefs([target.ref], edges, undefined, depth)
        : planNamespaceSeed([...refByKey.values()], edges).seed
    setPresent(seed)
    // Reuse saved positions when they cover the whole seed (stable reopen);
    // otherwise auto-layout.
    if (!seed.every((r) => savedPositions.current[refKey(r)])) requestLayout()
  }, [
    target,
    edges,
    refByKey,
    catalogQuery.isSuccess,
    relQuery.isSuccess,
    hydrateChecked,
    requestLayout,
    depth,
  ])

  // Depth control (object diagrams): re-seed from the anchor table to N hops.
  const changeDepth = useCallback(
    (d: number) => {
      setDepth(d)
      if (target?.kind === 'object') {
        setPresent(reachableRefs([target.ref], edges, undefined, d))
        requestLayout()
      }
    },
    [target, edges, requestLayout],
  )

  // Fetch column detail for on-canvas nodes (reuses the object-detail cache).
  const detailResults = useQueries({
    queries: present.map((ref) => ({
      ...orgConnectionObjectQueryOptions(
        orgSlug,
        workspace.id,
        connectionId ?? 0,
        sessionId ?? '',
        ref,
      ),
      enabled,
    })),
  })
  useEvictGoneSession(
    connectionId,
    detailResults.map((r) => r.error),
  )
  const detailByKey = useMemo(() => {
    const map = new Map<string, { detail: ObjectDetail | null; loading: boolean }>()
    present.forEach((ref, i) => {
      const r = detailResults[i]
      map.set(refKey(ref), { detail: r?.data ?? null, loading: r?.isLoading ?? false })
    })
    return map
  }, [present, detailResults])

  const presentKeys = useMemo(() => new Set(present.map(refKey)), [present])

  // For each table, which of its columns are referenced by other tables and by
  // whom (incoming FKs). Outgoing FKs come from each node's own detail.
  const incomingByTable = useMemo(() => {
    const m = new Map<string, Map<string, ObjectRef[]>>()
    for (const e of edges) {
      const tk = refKey(e.references)
      const col = e.referenced_columns[0]
      if (!col) continue
      let byCol = m.get(tk)
      if (!byCol) {
        byCol = new Map()
        m.set(tk, byCol)
      }
      const arr = byCol.get(col) ?? []
      arr.push(e.source)
      byCol.set(col, arr)
    }
    return m
  }, [edges])

  // Per-column expand: add the specific related table(s) next to the source
  // node in free space, keeping every existing node exactly where it is (no
  // relayout).
  const expandFrom = useCallback(
    (refs: ObjectRef[], opts: { fromKey: string; direction: 'in' | 'out' }) => {
      const rfNodes = getNodes()
      const have = new Set(rfNodes.map((n) => n.id))
      const toAdd = refs.filter((r) => !have.has(refKey(r)))
      if (toAdd.length === 0) return
      const occupied: Box[] = rfNodes.map((n) => ({
        x: n.position.x,
        y: n.position.y,
        w: n.measured?.width ?? n.width ?? 240,
        h: n.measured?.height ?? 120,
      }))
      const from = rfNodes.find((n) => n.id === opts.fromKey)
      const fromX = from?.position.x ?? 0
      const fromY = from?.position.y ?? 0
      const fromW = from?.measured?.width ?? from?.width ?? 240
      for (const ref of toAdd) {
        const key = refKey(ref)
        const w = 240
        const h = estimateNodeSize(undefined, false).height
        const startX = opts.direction === 'out' ? fromX + fromW + 60 : fromX - w - 60
        const pos = findFreePosition(startX, fromY, w, h, occupied)
        savedPositions.current[key] = pos
        occupied.push({ x: pos.x, y: pos.y, w, h })
      }
      setPresent((prev) => [...prev, ...toAdd])
    },
    [getNodes],
  )
  const toggleCollapse = useCallback((key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])
  // Remove a table from the diagram only (never the database). No relayout, so
  // the rest of the graph stays put; its edges drop out automatically.
  const removeRefs = useCallback((keys: Set<string>) => {
    setPresent((prev) => prev.filter((r) => !keys.has(refKey(r))))
    setCollapsed((prev) => {
      const next = new Set(prev)
      for (const k of keys) next.delete(k)
      return next
    })
    for (const k of keys) delete savedPositions.current[k]
  }, [])
  const removeRef = useCallback((key: string) => removeRefs(new Set([key])), [removeRefs])

  const [highlight, setHighlight] = useState<HoverRelation | null>(null)
  const onHoverRelation = useCallback((rel: HoverRelation | null) => setHighlight(rel), [])

  // Click-to-select a table: ring it + its directly-connected tables/edges and
  // dim the rest (focus mode). Cleared by clicking the empty canvas.
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const activeSelected = selectedKey && presentKeys.has(selectedKey) ? selectedKey : null
  const selectionRelated = useMemo(() => {
    if (!activeSelected) return null
    const set = new Set<string>([activeSelected])
    for (const e of edges) {
      const s = refKey(e.source)
      const t = refKey(e.references)
      if (s === activeSelected) set.add(t)
      else if (t === activeSelected) set.add(s)
    }
    return set
  }, [activeSelected, edges])

  // Find-a-table search over every table in the namespace (on-canvas or not).
  const [search, setSearch] = useState('')
  const searchResults = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return []
    return [...refByKey.values()].filter((r) => r.name.toLowerCase().includes(q)).slice(0, 8)
  }, [search, refByKey])

  // Focus a table (pan/zoom + select). If it's not on the canvas, add it first
  // in free space, then focus it once it has rendered.
  const [pendingFocus, setPendingFocus] = useState<string | null>(null)
  const focusOrAdd = useCallback(
    (ref: ObjectRef) => {
      const key = refKey(ref)
      const rfNodes = getNodes()
      if (!rfNodes.some((n) => n.id === key)) {
        const occupied: Box[] = rfNodes.map((n) => ({
          x: n.position.x,
          y: n.position.y,
          w: n.measured?.width ?? n.width ?? 240,
          h: n.measured?.height ?? 120,
        }))
        const cx = rfNodes.length
          ? rfNodes.reduce((s, n) => s + n.position.x, 0) / rfNodes.length
          : 0
        const cy = rfNodes.length
          ? rfNodes.reduce((s, n) => s + n.position.y, 0) / rfNodes.length
          : 0
        const pos = findFreePosition(
          cx,
          cy,
          240,
          estimateNodeSize(undefined, false).height,
          occupied,
        )
        savedPositions.current[key] = pos
        setPresent((prev) => (prev.some((r) => refKey(r) === key) ? prev : [...prev, ref]))
      }
      setPendingFocus(key)
    },
    [getNodes],
  )
  useEffect(() => {
    if (!pendingFocus || !getNodes().some((n) => n.id === pendingFocus)) return
    const key = pendingFocus
    setPendingFocus(null)
    setSelectedKey(key)
    requestAnimationFrame(() => void fitView({ nodes: [{ id: key }], duration: 400, maxZoom: 1.2 }))
  }, [pendingFocus, nodes, getNodes, fitView])

  // Stable signatures so the reconcile/layout effects fire only on real changes,
  // not on the fresh array identities useQueries returns each render.
  const presentSig = present.map(refKey).join('|')
  const collapsedSig = [...collapsed].sort().join('|')
  const detailSig = present
    .map((r) => {
      const e = detailByKey.get(refKey(r))
      return `${refKey(r)}:${e?.detail ? 1 : 0}:${e?.loading ? 1 : 0}`
    })
    .join('|')

  // Reconcile the React Flow node list from data, preserving each node's live
  // position (drag state) and only updating its data.
  useEffect(() => {
    setNodes((prev) => {
      const prevById = new Map(prev.map((n) => [n.id, n]))
      return present.map((ref) => {
        const key = refKey(ref)
        const existing = prevById.get(key)
        const position = existing?.position ?? savedPositions.current[key] ?? { x: 0, y: 0 }
        const entry = detailByKey.get(key)
        const rel = entry?.detail?.relational
        const columns = rel?.columns ?? []

        const outgoingByCol: Record<string, { target: ObjectRef; hidden: boolean }> = {}
        for (const f of rel?.foreign_keys ?? []) {
          const col = f.columns?.[0]
          if (!col) continue
          outgoingByCol[col] = {
            target: f.references,
            hidden: !presentKeys.has(refKey(f.references)),
          }
        }
        const incomingByCol: Record<string, { hidden: boolean; sources: ObjectRef[] }> = {}
        for (const [col, sources] of incomingByTable.get(key) ?? []) {
          incomingByCol[col] = { hidden: sources.some((s) => !presentKeys.has(refKey(s))), sources }
        }
        const hasHidden =
          Object.values(outgoingByCol).some((o) => o.hidden) ||
          Object.values(incomingByCol).some((i) => i.hidden)

        return {
          ...(existing ?? {}),
          id: key,
          type: 'table' as const,
          position,
          width: existing?.width ?? 240,
          data: {
            label: ref.name,
            namespace: ref.namespace,
            columns,
            pk: new Set(rel?.primary_key ?? []),
            outgoingByCol,
            incomingByCol,
            hasHidden,
            collapsed: collapsed.has(key),
            keysOnly,
            loading: entry?.loading ?? false,
            onToggleCollapse: () => toggleCollapse(key),
            onExpand: expandFrom,
            onOpenDetail: () => {
              if (connectionId)
                openTab(newObjectTab({ id: connectionId, driver } as never, workspace, ref))
            },
            onRemove: () => removeRef(key),
            onHoverRelation,
          },
        } as FlowNode
      })
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [presentSig, detailSig, collapsedSig, keysOnly])

  // Node emphasis: FK-hover rings the referenced table; click-selection rings
  // the selected table + its neighbors and dims everything else.
  useEffect(() => {
    const classFor = (id: string): string => {
      if (highlight && id === highlight.target) return 'rounded ring-2 ring-primary'
      if (activeSelected) {
        if (id === activeSelected) return 'rounded ring-2 ring-primary'
        if (selectionRelated?.has(id)) return 'rounded ring-1 ring-primary/50'
        return 'opacity-40 transition-opacity'
      }
      return ''
    }
    setNodes((prev) => {
      let changed = false
      const next = prev.map((n) => {
        const cls = classFor(n.id)
        if ((n.className ?? '') === cls) return n
        changed = true
        return { ...n, className: cls }
      })
      return changed ? next : prev
    })
  }, [highlight, activeSelected, selectionRelated, setNodes])

  // Resolve the exact handles each relationship needs from current schema and
  // collapse state. These edges are withheld from React Flow until every
  // selected handle exists in its internal registry.
  const resolvedEdges: Edge[] = useMemo(() => {
    return edges
      .filter((e) => presentKeys.has(refKey(e.source)) && presentKeys.has(refKey(e.references)))
      .map((e, i) => {
        const sk = refKey(e.source)
        const tk = refKey(e.references)
        const srcCol = e.columns[0]
        const tgtCol = e.referenced_columns[0]
        const sourceRel = detailByKey.get(sk)?.detail?.relational
        const targetRel = detailByKey.get(tk)?.detail?.relational
        const sourceColumns = new Set((sourceRel?.columns ?? []).map((column) => column.name))
        const targetColumns = new Set((targetRel?.columns ?? []).map((column) => column.name))
        const outgoingColumns = new Set(
          (sourceRel?.foreign_keys ?? []).flatMap(
            (foreignKey) => foreignKey.columns?.slice(0, 1) ?? [],
          ),
        )
        const incomingColumns = new Set(incomingByTable.get(tk)?.keys() ?? [])
        const isHi = Boolean(
          highlight &&
          sk === highlight.source &&
          tk === highlight.target &&
          srcCol === highlight.column,
        )
        const touchesSel = Boolean(
          activeSelected && (sk === activeSelected || tk === activeSelected),
        )
        const dimmed = Boolean(activeSelected && !touchesSel)
        // Cardinality from the child (source) side: crow's-foot "many" unless the
        // FK columns are unique on the child (then "one" — a 1:1 relationship).
        const card = edgeCardinality(e.columns, detailByKey.get(sk)?.detail ?? undefined)
        const base = { stroke: 'var(--muted-foreground)', strokeWidth: 1 }
        return {
          id: `${e.name}-${i}`,
          type: 'fk',
          source: sk,
          target: tk,
          sourceHandle: relationshipHandleId({
            column: srcCol,
            direction: 'out',
            handlesReady: !collapsed.has(sk),
            availableColumns: sourceColumns,
            connectedColumns: outgoingColumns,
          }),
          targetHandle: relationshipHandleId({
            column: tgtCol,
            direction: 'in',
            handlesReady: !collapsed.has(tk),
            availableColumns: targetColumns,
            connectedColumns: incomingColumns,
          }),
          data: {
            sourceMarker: card === 'one_to_one' ? 'one' : 'many',
            targetMarker: 'one',
            orthogonal,
          },
          animated: isHi,
          zIndex: isHi ? 1000 : touchesSel ? 500 : undefined,
          style: isHi
            ? { ...base, stroke: 'var(--primary)', strokeWidth: 2 }
            : touchesSel
              ? { ...base, stroke: 'var(--primary)', strokeWidth: 1.5 }
              : dimmed
                ? { ...base, opacity: 0.12 }
                : base,
        }
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [edges, presentSig, collapsedSig, detailSig, highlight, activeSelected, orthogonal])

  const edgeResolution = resolvedEdges
    .map(
      (edge) =>
        `${edge.id}:${edge.source}:${edge.sourceHandle ?? ''}:${edge.target}:${edge.targetHandle ?? ''}`,
    )
    .join('|')
  const requiredEdgeHandles = resolvedEdges.flatMap((edge) => [
    `${edge.source}|${edge.sourceHandle}`,
    `${edge.target}|${edge.targetHandle}`,
  ])
  const edgeResolutionSig = `${presentSig}::${collapsedSig}::${detailSig}::${keysOnly ? 1 : 0}::${edgeResolution}`
  const detailsSettled = present.every((ref) => !detailByKey.get(refKey(ref))?.loading)

  useEffect(() => {
    if (!detailsSettled) {
      setReadyEdgeResolution('')
      return
    }
    let cancelled = false
    let stableFrames = 0
    let frame = 0

    const verifyHandles = () => {
      const renderedHandles = new Set(
        Array.from(
          containerRef.current?.querySelectorAll<HTMLElement>('[data-nodeid][data-handleid]') ?? [],
        ).map((handle) => `${handle.dataset.nodeid}|${handle.dataset.handleid}`),
      )
      const allRendered = requiredEdgeHandles.every((handle) => renderedHandles.has(handle))

      stableFrames = allRendered ? stableFrames + 1 : 0
      if (stableFrames >= 2) {
        if (!cancelled) setReadyEdgeResolution(edgeResolutionSig)
        return
      }
      frame = window.requestAnimationFrame(verifyHandles)
    }

    frame = window.requestAnimationFrame(verifyHandles)
    return () => {
      cancelled = true
      window.cancelAnimationFrame(frame)
    }
  }, [detailsSettled, edgeResolutionSig, requiredEdgeHandles])

  const graphReady = readyEdgeResolution === edgeResolutionSig
  const flowEdges = graphReady ? resolvedEdges : []

  // Full, consistent elk layout of every node whenever the structure changes
  // (seed / expand / drop / manual re-layout). Applying positions to ALL nodes
  // — not just new ones — keeps the whole graph in one coordinate frame, which
  // is what stops newly-expanded nodes from stacking behind existing ones.
  useEffect(() => {
    if (layoutReq === 0 || present.length === 0) return
    const sizes = present.map((r) => {
      const k = refKey(r)
      const s = estimateNodeSize(
        detailByKey.get(k)?.detail ?? undefined,
        collapsed.has(k),
        keysOnly,
      )
      return { id: k, width: s.width, height: s.height }
    })
    const le = flowEdges.map((e) => ({ id: e.id, source: e.source, target: e.target }))
    let cancelled = false
    void layoutGraph(sizes, le).then((laid) => {
      if (cancelled) return
      setNodes((prev) => prev.map((n) => ({ ...n, position: laid.get(n.id) ?? n.position })))
      requestAnimationFrame(() => fitView({ duration: 200 }))
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layoutReq])

  // Persist layout (debounced so per-pixel drags don't hammer IndexedDB).
  const saveTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const persist = useCallback(() => {
    if (!seededRef.current) return
    void saveDiagram(tab.id, {
      present: present.map(refKey),
      positions: Object.fromEntries(getNodes().map((n) => [n.id, n.position])),
      collapsed: [...collapsed],
      depth: depth === Infinity ? 'all' : depth,
      viewport: getViewport(),
      orthogonalEdges: orthogonal,
      keysOnly,
    })
  }, [tab.id, present, collapsed, depth, orthogonal, keysOnly, getNodes, getViewport])
  const schedulePersist = useCallback(() => {
    clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(persist, 400)
  }, [persist])
  // Persist on structure/position changes and on camera moves (onMoveEnd).
  useEffect(() => {
    schedulePersist()
    return () => clearTimeout(saveTimer.current)
  }, [schedulePersist, nodes])

  // Restore the saved camera once the nodes exist (no re-fit for restored views).
  useEffect(() => {
    if (viewportRestored.current || !savedViewport.current || nodes.length === 0) return
    viewportRestored.current = true
    const vp = savedViewport.current
    requestAnimationFrame(() => setViewport(vp))
  }, [nodes.length, setViewport])

  // Dropping a table from the tree: place it exactly where it was dropped and
  // leave the rest of the layout untouched — only the new node + its edges
  // appear. (No requestLayout, so nothing else moves.)
  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      const raw = e.dataTransfer.getData(OBJECT_REF_DND_MIME)
      if (!raw) return
      try {
        const ref = JSON.parse(raw) as ObjectRef
        if (!ref?.namespace || !ref?.name) return
        const key = refKey(ref)
        const pos = screenToFlowPosition({ x: e.clientX, y: e.clientY })
        // Seed the reconcile position for the new node (reconcile reads this).
        savedPositions.current[key] = pos
        setPresent((prev) => (prev.some((r) => refKey(r) === key) ? prev : [...prev, ref]))
      } catch {
        /* ignore malformed payload */
      }
    },
    [screenToFlowPosition],
  )

  async function reconnect() {
    if (!connectionId) return
    setConnectionStatus(connectionId, 'connecting')
    try {
      const data = await api.post<{ session_id: string }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${connectionId}/connect`,
      )
      setSession(connectionId, data.session_id)
    } catch {
      /* surfaced by the next fetch */
    } finally {
      setConnectionStatus(connectionId, null)
    }
  }
  function refresh() {
    void queryClient.invalidateQueries({
      queryKey: connectionRelationshipsQueryKey(
        orgSlug,
        workspace.id,
        connectionId ?? 0,
        namespace,
      ),
    })
    void queryClient.invalidateQueries({
      queryKey: connectionObjectsQueryKeyPrefix(orgSlug, workspace.id, connectionId ?? 0),
    })
  }

  // Render the whole diagram (every node, not just the visible viewport) to an
  // image data URL. We render the react-flow viewport element at a transform
  // that frames all nodes, sized to their bounds so nothing is cropped
  // regardless of the current pan/zoom. PNG gets an opaque themed background;
  // SVG stays transparent.
  const [exporting, setExporting] = useState(false)
  const renderImage = useCallback(
    async (format: ExportFormat): Promise<string | null> => {
      const rfNodes = getNodes()
      const viewportEl = containerRef.current?.querySelector<HTMLElement>('.react-flow__viewport')
      if (rfNodes.length === 0 || !viewportEl) return null
      const bounds = getNodesBounds(rfNodes)
      const { width, height } = exportDimensions(bounds)
      const t = getViewportForBounds(bounds, width, height, 0.2, 4, 0.1)
      const bg = getComputedStyle(viewportEl).getPropertyValue('--background').trim() || '#ffffff'
      const options = {
        width,
        height,
        backgroundColor: format === 'png' ? bg : undefined,
        style: {
          width: `${width}px`,
          height: `${height}px`,
          transform: `translate(${t.x}px, ${t.y}px) scale(${t.zoom})`,
        },
      }
      return format === 'png' ? toPng(viewportEl, options) : toSvg(viewportEl, options)
    },
    [getNodes],
  )

  const exportImage = useCallback(
    async (format: ExportFormat) => {
      setExporting(true)
      try {
        const dataUrl = await renderImage(format)
        if (dataUrl) {
          downloadDataUrl(
            dataUrl,
            diagramFileName(
              namespace,
              target?.kind === 'object' ? target.ref.name : undefined,
              format,
            ),
          )
        }
      } catch {
        toast.error('Failed to export the diagram.')
      } finally {
        setExporting(false)
      }
    },
    [renderImage, namespace, target],
  )

  // Copy a PNG of the diagram to the system clipboard (paste-as-picture into
  // docs/chat). The Clipboard API only accepts image/png here — SVG isn't on the
  // browser safelist — so copy is always PNG.
  const copyImage = useCallback(async () => {
    setExporting(true)
    try {
      const dataUrl = await renderImage('png')
      if (!dataUrl) return
      const blob = await (await fetch(dataUrl)).blob()
      await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })])
      toast.success('Diagram copied to clipboard.')
    } catch {
      toast.error('Failed to copy the diagram.')
    } finally {
      setExporting(false)
    }
  }, [renderImage])

  const viewState = resolveDiagramViewState({
    hasTarget: Boolean(target),
    hasConnection: Boolean(connectionId),
    hasSession: Boolean(sessionId),
    spec,
    specError: specQuery.error,
    catalogError: catalogQuery.error,
    relationshipsError: relQuery.error,
    catalogLoading: catalogQuery.isLoading,
    relationshipsLoading: relQuery.isLoading,
    presentCount: present.length,
  })
  if (viewState === 'missing-target' || !target || !connectionId)
    return <Center>This tab is missing its diagram target.</Center>
  if (viewState === 'no-session')
    return <Reconnect namespace={namespace} driver={driver} onReconnect={reconnect} />
  if (viewState === 'unsupported')
    return <Center>Diagrams aren&apos;t available for this connection.</Center>
  if (viewState === 'forbidden')
    return (
      <Center className="text-destructive">You no longer have access to this connection.</Center>
    )
  if (viewState === 'loading') {
    return (
      <Center>
        <Icon name="loading-03" size={16} className="animate-spin" /> Loading schema…
      </Center>
    )
  }
  if (viewState === 'empty') return <Center>No tables to diagram in this schema.</Center>

  return (
    <div ref={containerRef} className="flex h-full min-h-0 flex-col">
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2">
        <span className="truncate text-xs font-medium text-foreground">
          {target.kind === 'namespace' ? namespace : `${namespace}.${target.ref.name}`}
        </span>
        <span className="text-[10px] text-muted-foreground">{driver}</span>
        <div className="flex-1" />
        <div className="relative mr-1">
          <div className="flex items-center gap-1 rounded border border-border px-1.5">
            <Icon name="search-01" size={12} className="shrink-0 text-muted-foreground" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && searchResults[0]) {
                  focusOrAdd(searchResults[0])
                  setSearch('')
                }
                if (e.key === 'Escape') setSearch('')
              }}
              placeholder="Find table…"
              className="h-6 w-32 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            />
          </div>
          {searchResults.length > 0 && (
            <div className="absolute right-0 top-7 z-20 w-56 overflow-hidden rounded-md border border-border bg-card p-1 shadow-md">
              {searchResults.map((r) => (
                <button
                  key={refKey(r)}
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    focusOrAdd(r)
                    setSearch('')
                  }}
                  className="flex w-full items-center justify-between gap-2 rounded px-2 py-1 text-left text-xs hover:bg-accent"
                >
                  <span className="min-w-0 truncate text-foreground">{r.name}</span>
                  <span
                    className={cn(
                      'shrink-0 text-[9px]',
                      presentKeys.has(refKey(r)) ? 'text-muted-foreground' : 'text-primary',
                    )}
                  >
                    {presentKeys.has(refKey(r)) ? 'on canvas' : 'add'}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
        {target.kind === 'object' && (
          <div className="mr-1 flex items-center gap-1.5 text-[10px] text-muted-foreground">
            <span>Depth</span>
            <ToggleGroup
              size="sm"
              variant="outline"
              value={[depth === Infinity ? 'all' : String(depth)]}
              onValueChange={(next) => {
                const v = next[0]
                if (v) changeDepth(v === 'all' ? Infinity : Number(v))
              }}
            >
              <Tip label="Directly related tables only">
                <ToggleGroupItem value="1" aria-label="One hop">
                  1
                </ToggleGroupItem>
              </Tip>
              <Tip label="Relations up to two hops away">
                <ToggleGroupItem value="2" aria-label="Two hops">
                  2
                </ToggleGroupItem>
              </Tip>
              <Tip label="Every connected table">
                <ToggleGroupItem value="all" aria-label="All connected">
                  All
                </ToggleGroupItem>
              </Tip>
            </ToggleGroup>
          </div>
        )}
        <DropdownMenu>
          <Tip label="Export diagram">
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  disabled={exporting}
                  aria-label="Export diagram"
                >
                  <Icon
                    name={exporting ? 'loading-03' : 'download-01'}
                    size={14}
                    className={exporting ? 'animate-spin' : undefined}
                  />
                </Button>
              }
            />
          </Tip>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => void exportImage('png')}>
              <Icon name="download-01" size={13} />
              Download PNG
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => void exportImage('svg')}>
              <Icon name="download-01" size={13} />
              Download SVG
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => void copyImage()}>
              <Icon name="copy-01" size={13} />
              Copy image
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Tip label="Refresh schema">
          <Button variant="ghost" size="icon-sm" aria-label="Refresh schema" onClick={refresh}>
            <Icon name="refresh" size={14} />
          </Button>
        </Tip>
      </div>
      <div
        className="relative min-h-0 flex-1"
        onDrop={onDrop}
        onDragOver={(e) => e.preventDefault()}
      >
        {!graphReady ? (
          <div
            className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center"
            role="status"
            aria-live="polite"
          >
            <div className="flex items-center gap-2 rounded-md border border-border bg-card px-3 py-2 text-xs text-muted-foreground shadow-sm">
              <Icon name="loading-03" size={14} className="animate-spin text-primary" />
              <span className="flex flex-col">
                <span className="font-medium text-foreground">Preparing diagram</span>
                <span>Resolving tables and relationships…</span>
              </span>
            </div>
          </div>
        ) : null}
        <ReactFlow
          className={cn(
            'transition-opacity duration-150',
            graphReady ? 'opacity-100' : 'pointer-events-none opacity-0',
          )}
          nodes={nodes}
          edges={flowEdges}
          nodeTypes={NODE_TYPES}
          edgeTypes={EDGE_TYPES}
          onNodesChange={onNodesChange}
          onNodesDelete={(deleted) => removeRefs(new Set(deleted.map((n) => n.id)))}
          onNodeClick={(_, node) => setSelectedKey(node.id)}
          onPaneClick={() => setSelectedKey(null)}
          onMoveEnd={schedulePersist}
          fitView={!savedViewport.current}
          proOptions={{ hideAttribution: true }}
          minZoom={0.1}
        >
          <Background />
          <Controls>
            <ControlButton onClick={requestLayout} title="Auto-layout">
              <Icon name="sparkles" size={14} />
            </ControlButton>
            <ControlButton
              onClick={() => setOrthogonal((v) => !v)}
              title={
                orthogonal
                  ? 'Switch to curved edges (hide cardinality)'
                  : 'Switch to orthogonal edges with crow’s-foot cardinality'
              }
            >
              <Icon name="flow-connection" size={14} />
            </ControlButton>
            <ControlButton
              onClick={() => setKeysOnly((v) => !v)}
              title={keysOnly ? 'Show all columns' : 'Show only key columns (PK/FK)'}
            >
              <Icon name="key-01" size={14} />
            </ControlButton>
          </Controls>
          <MiniMap pannable zoomable className="!bg-card" style={{ width: 120, height: 80 }} />
        </ReactFlow>
      </div>
    </div>
  )
}

function Center({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <div
      className={`flex h-full items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}
    >
      {children}
    </div>
  )
}

function Reconnect({
  namespace,
  driver,
  onReconnect,
}: {
  namespace: string
  driver: string
  onReconnect: () => void
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <div className="flex flex-col gap-1">
        <div className="text-sm font-medium text-foreground">{namespace}</div>
        <div className="text-xs text-muted-foreground">{driver} · connection not available</div>
      </div>
      <Button variant="outline" size="sm" onClick={onReconnect}>
        Reconnect
      </Button>
    </div>
  )
}
