import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useWorkspaces, useCreateWorkspace } from '#/lib/queries/useWorkspaces'
import { useOrg } from '#/lib/queries/useOrg'
import { Button } from '#/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'

export const Route = createFileRoute('/$orgSlug/')({ component: OrgOverview })

function OrgOverview() {
  const { orgSlug } = Route.useParams()
  const { data: org } = useOrg(orgSlug)
  const { data: workspaces = [], isLoading } = useWorkspaces(orgSlug)
  const createWs = useCreateWorkspace(orgSlug)
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  return (
    <div className="p-8 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold text-zinc-100">{org?.name ?? orgSlug}</h1>
          <span className="text-xs text-zinc-500 bg-zinc-800 rounded px-2 py-0.5 font-mono">{orgSlug}</span>
        </div>
        <Button onClick={() => setOpen(true)} className="bg-zinc-100 text-zinc-900 hover:bg-zinc-200">
          New Workspace
        </Button>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-3 gap-4">{[1,2,3].map(i => <div key={i} className="h-32 rounded-lg bg-zinc-800 animate-pulse" />)}</div>
      ) : workspaces.length === 0 ? (
        <div className="border border-dashed border-zinc-700 rounded-lg p-12 text-center">
          <p className="text-zinc-400">No workspaces yet.</p>
          <Button onClick={() => setOpen(true)} variant="outline" className="mt-4">Create first workspace</Button>
        </div>
      ) : (
        <div className="grid grid-cols-3 gap-4">
          {workspaces.map(ws => (
            <Link key={ws.id} to="/$orgSlug/workspaces/$workspaceId" params={{ orgSlug, workspaceId: ws.id }}
              className="border border-zinc-800 rounded-lg p-4 hover:border-zinc-600 hover:bg-zinc-900/50 transition-colors block">
              <p className="font-medium text-zinc-100 mb-1">{ws.name}</p>
              {ws.description && <p className="text-sm text-zinc-400 line-clamp-2">{ws.description}</p>}
              <p className="text-xs text-zinc-600 mt-3">{new Date(ws.created_at).toLocaleDateString()}</p>
            </Link>
          ))}
        </div>
      )}

      <div className="mt-8">
        <h2 className="text-sm font-medium text-zinc-400 uppercase tracking-wider mb-3">Recent Activity</h2>
        <div className="border border-dashed border-zinc-800 rounded-lg p-6 text-center">
          <p className="text-sm text-zinc-600">Activity feed — coming soon</p>
        </div>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>New Workspace</DialogTitle></DialogHeader>
          <form onSubmit={async e => {
            e.preventDefault()
            await createWs.mutateAsync({ name, description })
            setOpen(false); setName(''); setDescription('')
          }} className="space-y-4 mt-2">
            <div className="space-y-1.5"><Label>Name</Label><Input value={name} onChange={e => setName(e.target.value)} required /></div>
            <div className="space-y-1.5"><Label>Description</Label><Input value={description} onChange={e => setDescription(e.target.value)} /></div>
            <Button type="submit" disabled={createWs.isPending} className="w-full">Create</Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
