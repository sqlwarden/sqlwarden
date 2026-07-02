import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Background, ControlButton, Controls, MiniMap, ReactFlow, ReactFlowProvider, useNodesState, useReactFlow, useUpdateNodeInternals,
  type Edge, type Node, type NodeTypes,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { ToggleGroup, ToggleGroupItem } from '#/components/ui/toggle-group'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
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
  estimateNodeSize, planNamespaceSeed, reachableRefs, refKey,
} from './diagramModel'
import { layoutGraph } from './layout'
import { loadDiagram, saveDiagram } from './diagramStore'
import { TableNode, type HoverRelation, type TableNodeData } from './nodes/TableNode'
import { OBJECT_REF_DND_MIME } from './dnd'

const NODE_TYPES: NodeTypes = { table: TableNode }
type FlowNode = Node<TableNodeData, 'table'>

type Box = { x: number; y: number; w: number; h: number }
function boxesOverlap(a: Box, b: Box, pad = 16): boolean {
  return a.x < b.x + b.w + pad && a.x + a.w + pad > b.x && a.y < b.y + b.h + pad && a.y + a.h + pad > b.y
}
// Find an empty slot at startX, scanning vertically (down then up) from startY so
// a newly expanded node lands next to its source without overlapping anything.
function findFreePosition(startX: number, startY: number, w: number, h: number, occupied: Box[]): { x: number; y: number } {
  const step = h + 24
  for (let i = 0; i <= 40; i++) {
    for (const dy of i === 0 ? [0] : [i * step, -i * step]) {
      const box: Box = { x: startX, y: startY + dy, w, h }
      if (!occupied.some((o) => boxesOverlap(box, o))) return { x: box.x, y: box.y }
    }
  }
  return { x: startX, y: startY }
}

export function SchemaDiagramView(props: { orgSlug: string; workspace: Workspace; tab: EditorTab }) {
  return (
    <ReactFlowProvider>
      <DiagramCanvas {...props} />
    </ReactFlowProvider>
  )
}

function DiagramCanvas({ orgSlug, workspace, tab }: { orgSlug: string; workspace: Workspace; tab: EditorTab }) {
  const target = tab.diagramTarget
  const connectionId = tab.connectionId
  const driver = tab.driver ?? 'postgres'
  const namespace = target ? (target.kind === 'namespace' ? target.namespace : target.ref.namespace) : ''

  const sessionId = useIde((s) => (connectionId ? s.sessions[connectionId] : undefined))
  const setSession = useIde((s) => s.setSession)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)
  const openTab = useIde((s) => s.openTab)
  const queryClient = useQueryClient()
  const { fitView, screenToFlowPosition, getNodes } = useReactFlow()
  const updateNodeInternals = useUpdateNodeInternals()

  const enabled = Boolean(sessionId && connectionId && target)

  const specQuery = useQuery({
    ...orgConnectionSchemaSpecQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? ''),
    enabled,
  })
  const catalogQuery = useQuery({
    ...orgConnectionCatalogQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? ''),
    enabled,
  })
  const relQuery = useQuery({
    ...orgConnectionRelationshipsQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? '', namespace),
    enabled,
  })

  const spec = specQuery.data?.spec
  const edges = useMemo(() => relQuery.data?.relationships ?? [], [relQuery.data])

  const refByKey = useMemo(() => {
    const map = new Map<string, ObjectRef>()
    const diagramKinds = new Set((spec?.kinds ?? []).filter((k) => k.supports_diagram).map((k) => k.kind))
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

  // Load persisted positions + collapse once the schema data is ready. The
  // working SET is always seeded fresh (below), not restored — so persistence
  // only preserves arrangement, never a stale/partial set of tables.
  useEffect(() => {
    if (hydratedRef.current || !catalogQuery.isSuccess || !relQuery.isSuccess) return
    hydratedRef.current = true
    void loadDiagram(tab.id).then((saved) => {
      savedPositions.current = saved.positions
      if (saved.collapsed.length > 0) setCollapsed(new Set(saved.collapsed))
      setHydrateChecked(true)
    })
  }, [tab.id, catalogQuery.isSuccess, relQuery.isSuccess])

  // Seed the working set from the target — only after the queries have
  // SUCCEEDED (so edges are populated) and persisted positions have loaded.
  // Guarding on isSuccess (not !isLoading) matters: a disabled/idle query
  // reports isLoading:false with no data, which would otherwise seed with empty
  // edges and show only the anchor table.
  useEffect(() => {
    if (seededRef.current || !hydrateChecked || !target) return
    if (!catalogQuery.isSuccess || !relQuery.isSuccess) return
    seededRef.current = true
    const seed = target.kind === 'object'
      ? reachableRefs([target.ref], edges, undefined, depth)
      : planNamespaceSeed([...refByKey.values()], edges).seed
    setPresent(seed)
    // Reuse saved positions when they cover the whole seed (stable reopen);
    // otherwise auto-layout.
    if (!seed.every((r) => savedPositions.current[refKey(r)])) requestLayout()
  }, [target, edges, refByKey, catalogQuery.isSuccess, relQuery.isSuccess, hydrateChecked, requestLayout, depth])

  // Depth control (object diagrams): re-seed from the anchor table to N hops.
  const changeDepth = useCallback((d: number) => {
    setDepth(d)
    if (target?.kind === 'object') {
      setPresent(reachableRefs([target.ref], edges, undefined, d))
      requestLayout()
    }
  }, [target, edges, requestLayout])

  // Fetch column detail for on-canvas nodes (reuses the object-detail cache).
  const detailResults = useQueries({
    queries: present.map((ref) => ({
      ...orgConnectionObjectQueryOptions(orgSlug, workspace.id, connectionId ?? 0, sessionId ?? '', ref),
      enabled,
    })),
  })
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
      if (!byCol) { byCol = new Map(); m.set(tk, byCol) }
      const arr = byCol.get(col) ?? []
      arr.push(e.source)
      byCol.set(col, arr)
    }
    return m
  }, [edges])

  // Per-column expand: add the specific related table(s) next to the source
  // node in free space, keeping every existing node exactly where it is (no
  // relayout).
  const expandFrom = useCallback((refs: ObjectRef[], opts: { fromKey: string; direction: 'in' | 'out' }) => {
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
  }, [getNodes])
  const toggleCollapse = useCallback((key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
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

  // Stable signatures so the reconcile/layout effects fire only on real changes,
  // not on the fresh array identities useQueries returns each render.
  const presentSig = present.map(refKey).join('|')
  const collapsedSig = [...collapsed].sort().join('|')
  const detailSig = present.map((r) => {
    const e = detailByKey.get(refKey(r))
    return `${refKey(r)}:${e?.detail ? 1 : 0}:${e?.loading ? 1 : 0}`
  }).join('|')

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
          outgoingByCol[col] = { target: f.references, hidden: !presentKeys.has(refKey(f.references)) }
        }
        const incomingByCol: Record<string, { hidden: boolean; sources: ObjectRef[] }> = {}
        for (const [col, sources] of incomingByTable.get(key) ?? []) {
          incomingByCol[col] = { hidden: sources.some((s) => !presentKeys.has(refKey(s))), sources }
        }
        const hasHidden =
          Object.values(outgoingByCol).some((o) => o.hidden) || Object.values(incomingByCol).some((i) => i.hidden)

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
            loading: entry?.loading ?? false,
            onToggleCollapse: () => toggleCollapse(key),
            onExpand: expandFrom,
            onOpenDetail: () => {
              if (connectionId) openTab(newObjectTab({ id: connectionId, driver } as never, workspace, ref))
            },
            onRemove: () => removeRef(key),
            onHoverRelation,
          },
        } as FlowNode
      })
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [presentSig, detailSig, collapsedSig])

  // A node's handles change as its columns load or it collapses; tell React Flow
  // to re-scan them so edges re-attach to the per-column handles (avoids #008).
  useEffect(() => {
    for (const r of present) updateNodeInternals(refKey(r))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailSig, collapsedSig])

  // Ring the referenced table when an expanded FK column is hovered.
  useEffect(() => {
    setNodes((prev) => {
      let changed = false
      const next = prev.map((n) => {
        const cls = highlight && n.id === highlight.target ? 'rounded ring-2 ring-primary' : ''
        if ((n.className ?? '') === cls) return n
        changed = true
        return { ...n, className: cls }
      })
      return changed ? next : prev
    })
  }, [highlight, setNodes])

  // Anchor each edge to the specific FK column handles when both nodes are
  // expanded and loaded; otherwise fall back to the node-level handles.
  const flowEdges: Edge[] = useMemo(() => {
    const anchored = (key: string) => detailByKey.get(key)?.detail && !collapsed.has(key)
    return edges
      .filter((e) => presentKeys.has(refKey(e.source)) && presentKeys.has(refKey(e.references)))
      .map((e, i) => {
        const sk = refKey(e.source)
        const tk = refKey(e.references)
        const srcCol = e.columns[0]
        const tgtCol = e.referenced_columns[0]
        const isHi = Boolean(highlight && sk === highlight.source && tk === highlight.target && srcCol === highlight.column)
        return {
          id: `${e.name}-${i}`,
          source: sk,
          target: tk,
          sourceHandle: anchored(sk) && srcCol ? `col:${srcCol}:out` : 'node:out',
          targetHandle: anchored(tk) && tgtCol ? `col:${tgtCol}:in` : 'node:in',
          animated: isHi,
          zIndex: isHi ? 1000 : undefined,
          style: isHi ? { stroke: 'var(--primary)', strokeWidth: 2 } : undefined,
        }
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [edges, presentSig, collapsedSig, detailSig, highlight])

  // Full, consistent elk layout of every node whenever the structure changes
  // (seed / expand / drop / manual re-layout). Applying positions to ALL nodes
  // — not just new ones — keeps the whole graph in one coordinate frame, which
  // is what stops newly-expanded nodes from stacking behind existing ones.
  useEffect(() => {
    if (layoutReq === 0 || present.length === 0) return
    const sizes = present.map((r) => {
      const k = refKey(r)
      const s = estimateNodeSize(detailByKey.get(k)?.detail ?? undefined, collapsed.has(k))
      return { id: k, width: s.width, height: s.height }
    })
    const le = flowEdges.map((e) => ({ id: e.id, source: e.source, target: e.target }))
    let cancelled = false
    void layoutGraph(sizes, le).then((laid) => {
      if (cancelled) return
      setNodes((prev) => prev.map((n) => ({ ...n, position: laid.get(n.id) ?? n.position })))
      requestAnimationFrame(() => fitView({ duration: 200 }))
    })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layoutReq])

  // Persist layout (debounced so per-pixel drags don't hammer IndexedDB).
  const saveTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => {
    if (!seededRef.current) return
    clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      void saveDiagram(tab.id, {
        present: present.map(refKey),
        positions: Object.fromEntries(nodes.map((n) => [n.id, n.position])),
        collapsed: [...collapsed],
      })
    }, 400)
    return () => clearTimeout(saveTimer.current)
  }, [tab.id, nodes, presentSig, collapsedSig])

  // Dropping a table from the tree: place it exactly where it was dropped and
  // leave the rest of the layout untouched — only the new node + its edges
  // appear. (No requestLayout, so nothing else moves.)
  const onDrop = useCallback((e: React.DragEvent) => {
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
    } catch { /* ignore malformed payload */ }
  }, [screenToFlowPosition])

  async function reconnect() {
    if (!connectionId) return
    setConnectionStatus(connectionId, 'connecting')
    try {
      const data = await api.post<{ session_id: string }>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${connectionId}/connect`,
      )
      setSession(connectionId, data.session_id)
    } catch { /* surfaced by the next fetch */ } finally {
      setConnectionStatus(connectionId, null)
    }
  }
  function refresh() {
    void queryClient.invalidateQueries({ queryKey: connectionRelationshipsQueryKey(orgSlug, workspace.id, connectionId ?? 0, namespace) })
    void queryClient.invalidateQueries({ queryKey: connectionObjectsQueryKeyPrefix(orgSlug, workspace.id, connectionId ?? 0) })
  }

  // ── State handling ─────────────────────────────────────────────────────────
  if (!target || !connectionId) return <Center>This tab is missing its diagram target.</Center>
  if (!sessionId) return <Reconnect namespace={namespace} driver={driver} onReconnect={reconnect} />

  const unsupported =
    (isApiError(relQuery.error) && relQuery.error.status === 501) ||
    (spec != null && !spec.kinds.some((k) => k.supports_diagram))
  if (unsupported) return <Center>Diagrams aren&apos;t available for this connection.</Center>

  const forbidden = [specQuery.error, catalogQuery.error, relQuery.error].some((e) => isApiError(e) && e.status === 403)
  if (forbidden) return <Center className="text-destructive">You no longer have access to this connection.</Center>

  if (catalogQuery.isLoading || relQuery.isLoading) {
    return <Center><Icon name="loading-03" size={16} className="animate-spin" /> Loading schema…</Center>
  }
  if (present.length === 0) return <Center>No tables to diagram in this schema.</Center>

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2">
        <span className="truncate text-xs font-medium text-foreground">
          {target.kind === 'namespace' ? namespace : `${namespace}.${target.ref.name}`}
        </span>
        <span className="text-[10px] text-muted-foreground">{driver}</span>
        <div className="flex-1" />
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
              <ToggleGroupItem value="1" aria-label="One hop">1</ToggleGroupItem>
              <ToggleGroupItem value="2" aria-label="Two hops">2</ToggleGroupItem>
              <ToggleGroupItem value="all" aria-label="All connected">All</ToggleGroupItem>
            </ToggleGroup>
          </div>
        )}
        <Button variant="ghost" size="sm" onClick={refresh}>
          <Icon name="refresh" size={13} />
          Refresh
        </Button>
      </div>
      <div className="min-h-0 flex-1" onDrop={onDrop} onDragOver={(e) => e.preventDefault()}>
        <ReactFlow
          nodes={nodes}
          edges={flowEdges}
          nodeTypes={NODE_TYPES}
          onNodesChange={onNodesChange}
          onNodesDelete={(deleted) => removeRefs(new Set(deleted.map((n) => n.id)))}
          fitView
          proOptions={{ hideAttribution: true }}
          minZoom={0.1}
        >
          <Background />
          <Controls>
            <ControlButton onClick={requestLayout} title="Auto-layout">
              <Icon name="sparkles" size={14} />
            </ControlButton>
          </Controls>
          <MiniMap pannable zoomable className="!bg-card" style={{ width: 120, height: 80 }} />
        </ReactFlow>
      </div>
    </div>
  )
}

function Center({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`flex h-full items-center justify-center gap-2 text-xs text-muted-foreground ${className}`}>{children}</div>
}

function Reconnect({ namespace, driver, onReconnect }: { namespace: string; driver: string; onReconnect: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <div className="flex flex-col gap-1">
        <div className="text-sm font-medium text-foreground">{namespace}</div>
        <div className="text-xs text-muted-foreground">{driver} · connection not available</div>
      </div>
      <Button variant="outline" size="sm" onClick={onReconnect}>Reconnect</Button>
    </div>
  )
}
