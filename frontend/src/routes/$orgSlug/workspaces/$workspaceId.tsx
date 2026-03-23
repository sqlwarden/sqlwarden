import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useWorkspace, useAccessGrants, useGrantAccess, useRevokeAccess } from '#/lib/queries/useWorkspaces'
import { useConnections, useCreateConnection, useDeleteConnection } from '#/lib/queries/useConnections'
import { connectionsApi } from '#/lib/api/connections'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Badge } from '#/components/ui/badge'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '#/components/ui/sheet'

export const Route = createFileRoute('/$orgSlug/workspaces/$workspaceId')({ component: WorkspaceDetail })

const DRIVER_COLORS: Record<string, string> = {
  postgres: 'bg-blue-500/10 text-blue-400',
  mysql:    'bg-orange-500/10 text-orange-400',
  sqlite:   'bg-green-500/10 text-green-400',
}

function WorkspaceDetail() {
  const { orgSlug, workspaceId } = Route.useParams()
  const { data: workspace } = useWorkspace(orgSlug, workspaceId)
  const { data: connections = [] } = useConnections(orgSlug, workspaceId)
  const { data: grants = [] } = useAccessGrants(orgSlug, workspaceId)
  const createConn = useCreateConnection(orgSlug, workspaceId)
  const deleteConn = useDeleteConnection(orgSlug, workspaceId)
  const grantAccess = useGrantAccess(orgSlug, workspaceId)
  const revokeAccess = useRevokeAccess(orgSlug, workspaceId)
  const [connOpen, setConnOpen] = useState(false)
  const [grantOpen, setGrantOpen] = useState(false)
  const [connForm, setConnForm] = useState({ name: '', driver: 'postgres', dsn: '' })
  const [grantForm, setGrantForm] = useState({ subject: '', action: 'query', expiresAt: '' })
  const [testResult, setTestResult] = useState<{ ok: boolean; latency_ms: number; error?: string } | null>(null)

  const handleTestDSN = async () => {
    setTestResult(null)
    try {
      const r = await connectionsApi.testNew(orgSlug, workspaceId, connForm.driver, connForm.dsn)
      setTestResult(r)
    } catch {
      setTestResult({ ok: false, latency_ms: 0, error: 'Test request failed' })
    }
  }

  return (
    <div className="p-8 max-w-5xl mx-auto">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold text-zinc-100">{workspace?.name ?? '...'}</h1>
        {workspace?.description && <p className="text-zinc-400 mt-1 text-sm">{workspace.description}</p>}
      </div>

      {/* Connections */}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-base font-medium text-zinc-200">Connections</h2>
        <Button size="sm" onClick={() => setConnOpen(true)} className="bg-zinc-100 text-zinc-900 hover:bg-zinc-200">Add Connection</Button>
      </div>
      <div className="border border-zinc-800 rounded-lg overflow-hidden mb-8">
        {connections.length === 0
          ? <div className="px-4 py-8 text-center text-zinc-500 text-sm">No connections. Add one above.</div>
          : <table className="w-full text-sm">
              <thead className="border-b border-zinc-800 bg-zinc-900">
                <tr>
                  <th className="text-left px-4 py-3 text-zinc-400 font-medium">Name</th>
                  <th className="text-left px-4 py-3 text-zinc-400 font-medium">Driver</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody>
                {connections.map(conn => (
                  <tr key={conn.id} className="border-b border-zinc-800 last:border-0">
                    <td className="px-4 py-3 text-zinc-100">{conn.name}</td>
                    <td className="px-4 py-3"><span className={`text-xs px-2 py-0.5 rounded font-mono ${DRIVER_COLORS[conn.driver] ?? ''}`}>{conn.driver}</span></td>
                    <td className="px-4 py-3 text-right">
                      <Button variant="outline" size="sm" className="text-red-400 border-red-400/30 hover:bg-red-400/10 ml-2"
                        onClick={() => deleteConn.mutate(conn.id)}>Delete</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
        }
      </div>

      {/* Access Grants */}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-base font-medium text-zinc-200">Access</h2>
        <Button size="sm" variant="outline" onClick={() => setGrantOpen(true)}>Grant Access</Button>
      </div>
      <div className="border border-zinc-800 rounded-lg overflow-hidden">
        {grants.length === 0
          ? <div className="px-4 py-6 text-center text-zinc-500 text-sm">No access grants.</div>
          : <table className="w-full text-sm">
              <thead className="border-b border-zinc-800 bg-zinc-900">
                <tr>
                  <th className="text-left px-4 py-3 text-zinc-400 font-medium">Subject</th>
                  <th className="text-left px-4 py-3 text-zinc-400 font-medium">Action</th>
                  <th className="text-left px-4 py-3 text-zinc-400 font-medium">Expires</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody>
                {grants.map(g => (
                  <tr key={g.id} className="border-b border-zinc-800 last:border-0">
                    <td className="px-4 py-3 text-zinc-100 font-mono text-xs">{g.subject}</td>
                    <td className="px-4 py-3"><Badge variant="secondary">{g.action}</Badge></td>
                    <td className="px-4 py-3 text-zinc-500 text-xs">{g.expires_at ?? 'Never'}</td>
                    <td className="px-4 py-3 text-right">
                      <Button variant="outline" size="sm" className="text-red-400 border-red-400/30 hover:bg-red-400/10" onClick={() => revokeAccess.mutate(g.subject)}>Revoke</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
        }
      </div>

      {/* Add Connection Dialog */}
      <Dialog open={connOpen} onOpenChange={setConnOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Add Connection</DialogTitle></DialogHeader>
          <form onSubmit={async e => {
            e.preventDefault()
            await createConn.mutateAsync(connForm)
            setConnOpen(false); setConnForm({ name: '', driver: 'postgres', dsn: '' }); setTestResult(null)
          }} className="space-y-4 mt-2">
            <div className="space-y-1.5"><Label>Name</Label><Input value={connForm.name} onChange={e => setConnForm(f => ({ ...f, name: e.target.value }))} required /></div>
            <div className="space-y-1.5">
              <Label>Driver</Label>
              <select value={connForm.driver} onChange={e => setConnForm(f => ({ ...f, driver: e.target.value }))}
                className="w-full rounded-md border border-zinc-700 bg-zinc-800 text-zinc-100 px-3 py-2 text-sm">
                <option value="postgres">PostgreSQL</option>
                <option value="mysql">MySQL</option>
                <option value="sqlite">SQLite</option>
              </select>
            </div>
            <div className="space-y-1.5">
              <Label>DSN</Label>
              <Input value={connForm.dsn} onChange={e => setConnForm(f => ({ ...f, dsn: e.target.value }))}
                placeholder="postgres://user:pass@host:5432/db" required />
              <p className="text-xs text-zinc-500">Stored encrypted. Never returned in API responses.</p>
            </div>
            {testResult && (
              <p className={`text-sm ${testResult.ok ? 'text-green-400' : 'text-red-400'}`}>
                {testResult.ok ? `Connected (${testResult.latency_ms}ms)` : `${testResult.error}`}
              </p>
            )}
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={handleTestDSN} className="flex-1">Test</Button>
              <Button type="submit" disabled={createConn.isPending} className="flex-1">Save</Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>

      {/* Grant Access Sheet */}
      <Sheet open={grantOpen} onOpenChange={setGrantOpen}>
        <SheetContent side="right">
          <SheetHeader><SheetTitle>Grant Access</SheetTitle></SheetHeader>
          <form onSubmit={async e => {
            e.preventDefault()
            await grantAccess.mutateAsync({
              subject: grantForm.subject,
              action: grantForm.action,
              expiresAt: grantForm.expiresAt || undefined,
            })
            setGrantForm({ subject: '', action: 'query', expiresAt: '' })
            setGrantOpen(false)
          }} className="space-y-4 mt-6 px-6">
            <div className="space-y-1.5">
              <Label>Subject</Label>
              <Input value={grantForm.subject} onChange={e => setGrantForm(f => ({ ...f, subject: e.target.value }))}
                placeholder='account:ULID or team:slug' required />
              <p className="text-xs text-zinc-500">Format: account:{'{'+'accountId'+'}'} or team:{'{'+'teamSlug'+'}'}</p>
            </div>
            <div className="space-y-1.5">
              <Label>Action</Label>
              <select value={grantForm.action} onChange={e => setGrantForm(f => ({ ...f, action: e.target.value }))}
                className="w-full rounded-md border border-zinc-700 bg-zinc-800 text-zinc-100 px-3 py-2 text-sm">
                <option value="connect">connect</option>
                <option value="query">query</option>
                <option value="execute">execute</option>
                <option value="manage">manage</option>
              </select>
            </div>
            <div className="space-y-1.5">
              <Label>Expires at (optional)</Label>
              <Input type="datetime-local" value={grantForm.expiresAt} onChange={e => setGrantForm(f => ({ ...f, expiresAt: e.target.value }))}
                className="bg-zinc-800 border-zinc-700" />
            </div>
            <Button type="submit" className="w-full">Grant Access</Button>
          </form>
        </SheetContent>
      </Sheet>
    </div>
  )
}
