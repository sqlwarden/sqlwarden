import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useRoles, useCreateRole, useDeleteRole } from '#/lib/queries/useRoles'
import { Button } from '#/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'

export const Route = createFileRoute('/$orgSlug/roles')({ component: OrgRoles })

const ACTION_COLORS: Record<string, string> = {
  connect: 'bg-blue-500/10 text-blue-400 border-blue-500/20',
  query:   'bg-green-500/10 text-green-400 border-green-500/20',
  execute: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/20',
  manage:  'bg-red-500/10 text-red-400 border-red-500/20',
}
const ALL_ACTIONS = ['connect', 'query', 'execute', 'manage']

function OrgRoles() {
  const { orgSlug } = Route.useParams()
  const { data: roles = [] } = useRoles(orgSlug)
  const createRole = useCreateRole(orgSlug)
  const deleteRole = useDeleteRole(orgSlug)
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState({ name: '', description: '', actions: [] as string[] })

  return (
    <div className="p-8 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-zinc-100">Roles</h1>
        <Button onClick={() => setOpen(true)} className="bg-zinc-100 text-zinc-900 hover:bg-zinc-200">Create Role</Button>
      </div>
      <p className="text-sm text-zinc-500 mb-4">Custom roles define permission levels for workspace access. These are workspace-scoped — assign them via workspace access grants.</p>

      <div className="border border-zinc-800 rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="border-b border-zinc-800 bg-zinc-900">
            <tr>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Name</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Description</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Actions</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {roles.map(role => (
              <tr key={role.id} className="border-b border-zinc-800 last:border-0">
                <td className="px-4 py-3 font-medium text-zinc-100">{role.name}</td>
                <td className="px-4 py-3 text-zinc-400">{role.description}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    {role.actions.map(a => (
                      <span key={a} className={`text-xs px-2 py-0.5 rounded border ${ACTION_COLORS[a] ?? ''}`}>{a}</span>
                    ))}
                  </div>
                </td>
                <td className="px-4 py-3 text-right">
                  <Button variant="outline" size="sm" className="text-red-400 border-red-400/30 hover:bg-red-400/10"
                    onClick={() => deleteRole.mutate(role.id)}>Delete</Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Create Role</DialogTitle></DialogHeader>
          <form onSubmit={async e => {
            e.preventDefault()
            await createRole.mutateAsync(form)
            setOpen(false); setForm({ name: '', description: '', actions: [] })
          }} className="space-y-4 mt-2">
            <div className="space-y-1.5"><Label>Name</Label><Input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} required /></div>
            <div className="space-y-1.5"><Label>Description</Label><Input value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} /></div>
            <div className="space-y-2">
              <Label>Actions</Label>
              {ALL_ACTIONS.map(a => (
                <div key={a} className="flex items-center gap-2">
                  <input type="checkbox" id={a} checked={form.actions.includes(a)}
                    onChange={e => setForm(f => ({ ...f, actions: e.target.checked ? [...f.actions, a] : f.actions.filter(x => x !== a) }))} />
                  <label htmlFor={a} className="text-sm text-zinc-300 capitalize">{a}</label>
                </div>
              ))}
            </div>
            <Button type="submit" disabled={createRole.isPending} className="w-full">Create Role</Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
