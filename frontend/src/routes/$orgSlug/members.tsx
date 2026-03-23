import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useOrgMembers, useInviteMember, useRemoveMember } from '#/lib/queries/useOrg'
import { Button } from '#/components/ui/button'
import { Badge } from '#/components/ui/badge'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '#/components/ui/sheet'

export const Route = createFileRoute('/$orgSlug/members')({ component: OrgMembers })

function OrgMembers() {
  const { orgSlug } = Route.useParams()
  const { data: members = [] } = useOrgMembers(orgSlug)
  const inviteMember = useInviteMember(orgSlug)
  const removeMember = useRemoveMember(orgSlug)
  const [sheetOpen, setSheetOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [role, setRole] = useState('member')

  const ROLE_COLORS: Record<string, string> = {
    owner: 'bg-amber-500/10 text-amber-400',
    admin: 'bg-blue-500/10 text-blue-400',
    member: 'bg-zinc-500/10 text-zinc-400',
  }

  return (
    <div className="p-8 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-zinc-100">Members</h1>
        <Button onClick={() => setSheetOpen(true)} className="bg-zinc-100 text-zinc-900 hover:bg-zinc-200">
          Invite Member
        </Button>
      </div>
      <div className="border border-zinc-800 rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="border-b border-zinc-800 bg-zinc-900">
            <tr>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Member</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Role</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Joined</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {members.map(m => (
              <tr key={m.account_id} className="border-b border-zinc-800 last:border-0">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-3">
                    <div className="h-8 w-8 rounded-full bg-zinc-700 flex items-center justify-center flex-shrink-0">
                      <span className="text-xs font-medium text-zinc-200">
                        {m.account_name.split(' ').map((n: string) => n[0]).join('').toUpperCase().slice(0, 2)}
                      </span>
                    </div>
                    <div>
                      <p className="text-zinc-100 font-medium">{m.account_name}</p>
                      <p className="text-zinc-500 text-xs">{m.account_email}</p>
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span className={`text-xs px-2 py-0.5 rounded capitalize ${ROLE_COLORS[m.role] ?? ''}`}>{m.role}</span>
                </td>
                <td className="px-4 py-3 text-zinc-500 text-xs">{new Date(m.created_at).toLocaleDateString()}</td>
                <td className="px-4 py-3 text-right">
                  {m.role !== 'owner' && (
                    <Button variant="outline" size="sm" className="text-red-400 border-red-400/30 hover:bg-red-400/10"
                      onClick={() => removeMember.mutate(m.account_id)}>
                      Remove
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent side="right">
          <SheetHeader><SheetTitle>Invite Member</SheetTitle></SheetHeader>
          <form onSubmit={async e => {
            e.preventDefault()
            await inviteMember.mutateAsync({ email, role })
            setSheetOpen(false); setEmail(''); setRole('member')
          }} className="space-y-4 mt-6 px-6">
            <div className="space-y-1.5"><Label>Email</Label><Input type="email" value={email} onChange={e => setEmail(e.target.value)} required /></div>
            <div className="space-y-1.5">
              <Label>Role</Label>
              <select value={role} onChange={e => setRole(e.target.value)}
                className="w-full rounded-md border border-zinc-700 bg-zinc-800 text-zinc-100 px-3 py-2 text-sm">
                <option value="member">Member</option>
                <option value="admin">Admin</option>
              </select>
            </div>
            <Button type="submit" disabled={inviteMember.isPending} className="w-full">Send Invite</Button>
          </form>
        </SheetContent>
      </Sheet>
    </div>
  )
}
