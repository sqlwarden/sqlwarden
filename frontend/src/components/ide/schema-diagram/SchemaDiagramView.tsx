import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Background, Controls, MiniMap, ReactFlow, ReactFlowProvider, useReactFlow,
  type Edge, type Node, type NodeChange, type NodeTypes,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
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
  estimateNodeSize, hiddenNeighbors, planNamespaceSeed, planObjectSeed, refKey,
} from './diagramModel'
import { layoutGraph } from './layout'
import { loadDiagram, saveDiagram } from './diagramStore'
import { TableNode, type TableNodeData } from './nodes/TableNode'
import { OBJECT_REF_DND_MIME } from './dnd'

const NODE_TYPES: NodeTypes = { table: TableNode }

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

  // Candidate table refs in this namespace, plus every edge endpoint, so any
  // neighbor we expand into can be resolved back to an ObjectRef.
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
  const [positions, setPositions] = useState<Record<string, { x: number; y: number }>>({})
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const seededRef = useRef(false)
  const hydratedRef = useRef(false)

  // Hydrate persisted layout once.
  useEffect(() => {
    if (hydratedRef.current || refByKey.size === 0) return
    hydratedRef.current = true
    void loadDiagram(tab.id).then((saved) => {
      if (saved.present.length === 0) return
      const refs = saved.present.map((k) => refByKey.get(k)).filter((r): r is ObjectRef => Boolean(r))
      if (refs.length === 0) return
      seededRef.current = true
      setPresent(refs)
      setPositions(saved.positions)
      setCollapsed(new Set(saved.collapsed))
    })
  }, [tab.id, refByKey])

  // Seed the working set from the target once catalog + relationships are ready.
  useEffect(() => {
    if (seededRef.current || !target || catalogQuery.isLoading || relQuery.isLoading) return
    if (relQuery.isError) return
    seededRef.current = true
    if (target.kind === 'object') {
      setPresent(planObjectSeed(target.ref, edges))
    } else {
      const tableRefs = [...refByKey.values()]
      setPresent(planNamespaceSeed(tableRefs, edges).seed)
    }
  }, [target, edges, refByKey, catalogQuery.isLoading, relQuery.isLoading, relQuery.isError])

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

  const addRef = useCallback((ref: ObjectRef) => {
    setPresent((prev) => (prev.some((r) => refKey(r) === refKey(ref)) ? prev : [...prev, ref]))
  }, [])

  const expandNeighbors = useCallback((ref: ObjectRef) => {
    const neighbors = hiddenNeighbors(ref, edges, presentKeys)
    if (neighbors.length > 0) setPresent((prev) => [...prev, ...neighbors])
  }, [edges, presentKeys])

  const toggleCollapse = useCallback((key: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  // Auto-layout: place any present node that lacks a position (keeps manual
  // positions for existing nodes; only new nodes get elk coordinates).
  useEffect(() => {
    const missing = present.filter((r) => !positions[refKey(r)])
    if (missing.length === 0 || present.length === 0) return
    const nodes = present.map((r) => {
      const key = refKey(r)
      const size = estimateNodeSize(detailByKey.get(key)?.detail ?? undefined, collapsed.has(key))
      return { id: key, width: size.width, height: size.height }
    })
    const rfEdges = edges
      .filter((e) => presentKeys.has(refKey(e.source)) && presentKeys.has(refKey(e.references)))
      .map((e, i) => ({ id: `${e.name}-${i}`, source: refKey(e.source), target: refKey(e.references) }))
    let cancelled = false
    void layoutGraph(nodes, rfEdges).then((laid) => {
      if (cancelled) return
      setPositions((prev) => {
        const next = { ...prev }
        for (const r of missing) {
          const p = laid.get(refKey(r))
          if (p) next[refKey(r)] = p
        }
        return next
      })
    })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [present])

  // Persist layout on change.
  useEffect(() => {
    if (!seededRef.current) return
    void saveDiagram(tab.id, {
      present: present.map(refKey),
      positions,
      collapsed: [...collapsed],
    })
  }, [tab.id, present, positions, collapsed])

  const flowNodes: Node<TableNodeData>[] = useMemo(() => {
    return present.map((ref) => {
      const key = refKey(ref)
      const entry = detailByKey.get(key)
      const rel = entry?.detail?.relational
      const columns = rel?.columns ?? []
      const pk = new Set(rel?.primary_key ?? [])
      const fk = new Set((rel?.foreign_keys ?? []).flatMap((f) => f.columns ?? []))
      const hiddenCount = hiddenNeighbors(ref, edges, presentKeys).length
      return {
        id: key,
        type: 'table',
        position: positions[key] ?? { x: 0, y: 0 },
        data: {
          label: ref.name,
          namespace: ref.namespace,
          columns,
          pk,
          fk,
          hiddenCount,
          collapsed: collapsed.has(key),
          loading: entry?.loading ?? false,
          onToggleCollapse: () => toggleCollapse(key),
          onExpand: () => expandNeighbors(ref),
          onOpenDetail: () => {
            if (connectionId) openTab(newObjectTab({ id: connectionId, driver } as never, workspace, ref))
          },
        },
      }
    })
  }, [present, detailByKey, positions, collapsed, edges, presentKeys, connectionId, driver, workspace, openTab, toggleCollapse, expandNeighbors])

  const flowEdges: Edge[] = useMemo(() => {
    return edges
      .filter((e) => presentKeys.has(refKey(e.source)) && presentKeys.has(refKey(e.references)))
      .map((e, i) => ({ id: `${e.name}-${i}`, source: refKey(e.source), target: refKey(e.references), animated: false }))
  }, [edges, presentKeys])

  const onNodesChange = useCallback((changes: NodeChange[]) => {
    setPositions((prev) => {
      let next = prev
      for (const c of changes) {
        if (c.type === 'position' && c.position) {
          if (next === prev) next = { ...prev }
          next[c.id] = c.position
        }
      }
      return next
    })
  }, [])

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

  async function relayout() {
    const nodes = present.map((r) => {
      const key = refKey(r)
      const size = estimateNodeSize(detailByKey.get(key)?.detail ?? undefined, collapsed.has(key))
      return { id: key, width: size.width, height: size.height }
    })
    const rfEdges = flowEdges.map((e) => ({ id: e.id, source: e.source, target: e.target }))
    const laid = await layoutGraph(nodes, rfEdges)
    setPositions(Object.fromEntries(laid))
    requestAnimationFrame(() => fitView({ duration: 200 }))
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
        <ToolbarButton label="Re-layout" onClick={() => void relayout()} icon="arrow-down-01" />
        <ToolbarButton label="Fit" onClick={() => fitView({ duration: 200 })} icon="maximize" />
        <ToolbarButton label="Refresh" onClick={refresh} icon="refresh" />
      </div>
      <div className="min-h-0 flex-1" onDrop={onDrop} onDragOver={(e) => e.preventDefault()}>
        <ReactFlow
          nodes={flowNodes}
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
