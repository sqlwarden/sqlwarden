import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Background, Controls, MiniMap, ReactFlow, ReactFlowProvider, useNodesState, useReactFlow,
  type Edge, type Node, type NodeTypes,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
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
import { TableNode, type TableNodeData } from './nodes/TableNode'
import { OBJECT_REF_DND_MIME } from './dnd'

const NODE_TYPES: NodeTypes = { table: TableNode }
type FlowNode = Node<TableNodeData, 'table'>

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
  const { fitView } = useReactFlow()

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
  // Bumped whenever the node set changes structurally (seed / expand / drop /
  // manual re-layout) to trigger a full, consistent elk layout of every node.
  const [layoutReq, setLayoutReq] = useState(0)
  const requestLayout = useCallback(() => setLayoutReq((v) => v + 1), [])

  // Hydrate persisted layout once.
  useEffect(() => {
    if (hydratedRef.current || refByKey.size === 0) return
    hydratedRef.current = true
    void loadDiagram(tab.id).then((saved) => {
      if (saved.present.length === 0) return
      const refs = saved.present.map((k) => refByKey.get(k)).filter((r): r is ObjectRef => Boolean(r))
      if (refs.length === 0) return
      savedPositions.current = saved.positions
      seededRef.current = true
      // Restore saved positions as-is; no relayout (respects manual arrangement).
      setPresent(refs)
      setCollapsed(new Set(saved.collapsed))
    })
  }, [tab.id, refByKey])

  // Seed the working set from the target once catalog + relationships are ready.
  useEffect(() => {
    if (seededRef.current || !target || catalogQuery.isLoading || relQuery.isLoading || relQuery.isError) return
    seededRef.current = true
    if (target.kind === 'object') setPresent(reachableRefs([target.ref], edges, undefined, depth))
    else setPresent(planNamespaceSeed([...refByKey.values()], edges).seed)
    requestLayout()
  }, [target, edges, refByKey, catalogQuery.isLoading, relQuery.isLoading, relQuery.isError, requestLayout, depth])

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

  // Add one or more specific tables to the canvas (per-column expand + drop).
  const addRefs = useCallback((refs: ObjectRef[]) => {
    setPresent((prev) => {
      const have = new Set(prev.map(refKey))
      const add = refs.filter((r) => !have.has(refKey(r)))
      return add.length ? [...prev, ...add] : prev
    })
    requestLayout()
  }, [requestLayout])
  const addRef = useCallback((ref: ObjectRef) => addRefs([ref]), [addRefs])
  const toggleCollapse = useCallback((key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }, [])

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
            onExpand: addRefs,
            onOpenDetail: () => {
              if (connectionId) openTab(newObjectTab({ id: connectionId, driver } as never, workspace, ref))
            },
          },
        } as FlowNode
      })
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [presentSig, detailSig, collapsedSig])

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
        return {
          id: `${e.name}-${i}`,
          source: sk,
          target: tk,
          sourceHandle: anchored(sk) && srcCol ? `col:${srcCol}:out` : 'node:out',
          targetHandle: anchored(tk) && tgtCol ? `col:${tgtCol}:in` : 'node:in',
        }
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [edges, presentSig, collapsedSig, detailSig])

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

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    const raw = e.dataTransfer.getData(OBJECT_REF_DND_MIME)
    if (!raw) return
    try {
      const ref = JSON.parse(raw) as ObjectRef
      if (ref?.namespace && ref?.name) addRef(ref)
    } catch { /* ignore malformed payload */ }
  }, [addRef])

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
          <div className="mr-1 flex items-center gap-1 text-[10px] text-muted-foreground">
            <span>Depth</span>
            <div className="flex overflow-hidden rounded border border-border">
              {([['1', 1], ['2', 2], ['All', Infinity]] as const).map(([label, d]) => (
                <button
                  key={label}
                  type="button"
                  onClick={() => changeDepth(d)}
                  className={cn(
                    'px-1.5 py-0.5 text-[10px]',
                    depth === d ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-muted',
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        )}
        <ToolbarButton label="Re-layout" onClick={requestLayout} icon="arrow-down-01" />
        <ToolbarButton label="Fit" onClick={() => fitView({ duration: 200 })} icon="maximize" />
        <ToolbarButton label="Refresh" onClick={refresh} icon="refresh" />
      </div>
      <div className="min-h-0 flex-1" onDrop={onDrop} onDragOver={(e) => e.preventDefault()}>
        <ReactFlow
          nodes={nodes}
          edges={flowEdges}
          nodeTypes={NODE_TYPES}
          onNodesChange={onNodesChange}
          fitView
          proOptions={{ hideAttribution: true }}
          minZoom={0.1}
        >
          <Background />
          <Controls />
          <MiniMap pannable zoomable className="!bg-card" />
        </ReactFlow>
      </div>
    </div>
  )
}

function ToolbarButton({ label, onClick, icon }: { label: string; onClick: () => void; icon: Parameters<typeof Icon>[0]['name'] }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground"
    >
      <Icon name={icon} size={13} />
      {label}
    </button>
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
      <button type="button" onClick={onReconnect} className="rounded border border-border px-3 py-1.5 text-xs text-foreground hover:bg-accent">
        Reconnect
      </button>
    </div>
  )
}
