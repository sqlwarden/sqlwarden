import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useTeams, useTeamMembers, useCreateTeam, useDeleteTeam, useAddTeamMember, useRemoveTeamMember } from '#/lib/queries/useTeams'
import { useOrgMembers } from '#/lib/queries/useOrg'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import type { Team } from '#/lib/types/team'

export const Route = createFileRoute('/$orgSlug/teams')({ component: OrgTeams })

function OrgTeams() {
  const { orgSlug } = Route.useParams()
  const { data: teams = [] } = useTeams(orgSlug)
  const createTeam = useCreateTeam(orgSlug)
  const deleteTeam = useDeleteTeam(orgSlug)
  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [teamName, setTeamName] = useState('')
  const [teamSlug, setTeamSlug] = useState('')

  const { data: teamMembers = [] } = useTeamMembers(orgSlug, selectedTeam?.slug ?? '')
  const { data: orgMembers = [] } = useOrgMembers(orgSlug)
  const addMember = useAddTeamMember(orgSlug, selectedTeam?.slug ?? '')
  const removeMember = useRemoveTeamMember(orgSlug, selectedTeam?.slug ?? '')

  return (
    <div className="p-8 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-zinc-100">Teams</h1>
        <Button onClick={() => setCreateOpen(true)} className="bg-zinc-100 text-zinc-900 hover:bg-zinc-200">New Team</Button>
      </div>
      <div className="grid grid-cols-5 gap-6">
        {/* Left: teams list */}
        <div className="col-span-2 border border-zinc-800 rounded-lg overflow-hidden">
          {teams.length === 0 ? (
            <div className="p-6 text-center text-zinc-500 text-sm">No teams yet.</div>
          ) : (
            <ul>
              {teams.map(team => (
                <li key={team.id}
                  className={`flex items-center justify-between px-4 py-3 cursor-pointer border-b border-zinc-800 last:border-0 ${selectedTeam?.id === team.id ? 'bg-zinc-800' : 'hover:bg-zinc-900'}`}
                  onClick={() => setSelectedTeam(team)}>
                  <span className="text-sm text-zinc-100">{team.name}</span>
                  <Button variant="outline" size="sm" className="text-red-400 border-red-400/30 hover:bg-red-400/10 ml-2"
                    onClick={e => { e.stopPropagation(); deleteTeam.mutate(team.slug) }}>
                    Delete
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
        {/* Right: team members */}
        <div className="col-span-3">
          {!selectedTeam ? (
            <div className="border border-dashed border-zinc-700 rounded-lg p-8 text-center">
              <p className="text-zinc-500 text-sm">Select a team to view members</p>
            </div>
          ) : (
            <div className="border border-zinc-800 rounded-lg overflow-hidden">
              <div className="px-4 py-3 border-b border-zinc-800 bg-zinc-900 flex items-center justify-between">
                <span className="text-sm font-medium text-zinc-100">{selectedTeam.name}</span>
                <select onChange={e => { if (e.target.value) { addMember.mutate(e.target.value); e.target.value = '' } }}
                  className="text-xs rounded border border-zinc-700 bg-zinc-800 text-zinc-100 px-2 py-1">
                  <option value="">+ Add member</option>
                  {orgMembers
                    .filter(m => !teamMembers.some(tm => tm.account_id === m.account_id))
                    .map(m => <option key={m.account_id} value={m.account_id}>{m.account_name}</option>)}
                </select>
              </div>
              <ul>
                {teamMembers.map(m => (
                  <li key={m.account_id} className="flex items-center justify-between px-4 py-3 border-b border-zinc-800 last:border-0">
                    <div>
                      <p className="text-sm text-zinc-100">{m.account_name}</p>
                      <p className="text-xs text-zinc-500">{m.account_email}</p>
                    </div>
                    <Button variant="outline" size="sm" className="text-red-400 border-red-400/30 hover:bg-red-400/10"
                      onClick={() => removeMember.mutate(m.account_id)}>Remove</Button>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Create Team</DialogTitle></DialogHeader>
          <form onSubmit={async e => {
            e.preventDefault()
            await createTeam.mutateAsync({ slug: teamSlug, name: teamName })
            setCreateOpen(false); setTeamName(''); setTeamSlug('')
          }} className="space-y-4 mt-2">
            <div className="space-y-1.5"><Label>Name</Label><Input value={teamName} onChange={e => setTeamName(e.target.value)} required /></div>
            <div className="space-y-1.5"><Label>Slug</Label><Input value={teamSlug} onChange={e => setTeamSlug(e.target.value)} placeholder="my-team" required /></div>
            <Button type="submit" disabled={createTeam.isPending} className="w-full">Create Team</Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
