import { createFileRoute } from '@tanstack/react-router'
import { useState, useEffect } from 'react'
import { useOrg, useUpdateOrg } from '#/lib/queries/useOrg'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Badge } from '#/components/ui/badge'

export const Route = createFileRoute('/$orgSlug/settings')({ component: OrgSettings })

function OrgSettings() {
  const { orgSlug } = Route.useParams()
  const { data: org } = useOrg(orgSlug)
  const updateOrg = useUpdateOrg(orgSlug)
  const [name, setName] = useState('')

  useEffect(() => {
    if (org) setName(org.name)
  }, [org])

  return (
    <div className="p-8 max-w-2xl mx-auto">
      <h1 className="text-2xl font-semibold text-zinc-100 mb-6">Settings</h1>

      {/* General */}
      <section className="border border-zinc-800 rounded-lg p-6 mb-6">
        <h2 className="text-base font-medium text-zinc-100 mb-4">General</h2>
        <form onSubmit={async e => {
          e.preventDefault()
          await updateOrg.mutateAsync(name)
        }} className="space-y-4">
          <div className="space-y-1.5">
            <Label>Display Name</Label>
            <Input value={name} onChange={e => setName(e.target.value)} required />
          </div>
          <div className="space-y-1.5">
            <Label>Slug</Label>
            <Input value={orgSlug} readOnly className="opacity-60 cursor-not-allowed" />
            <p className="text-xs text-zinc-500">Slug cannot be changed after creation.</p>
          </div>
          <Button type="submit" disabled={updateOrg.isPending}>Save changes</Button>
        </form>
      </section>

      {/* SSO */}
      <section className="border border-zinc-800 rounded-lg p-6 mb-6">
        <div className="flex items-center justify-between mb-2">
          <h2 className="text-base font-medium text-zinc-100">Single Sign-On</h2>
          <Badge variant="secondary">Coming soon</Badge>
        </div>
        <p className="text-sm text-zinc-500">Configure SAML or OIDC for this organization.</p>
      </section>

      {/* Danger zone */}
      <section className="border border-red-900/40 rounded-lg p-6">
        <h2 className="text-base font-medium text-red-400 mb-4">Danger Zone</h2>
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-100">Leave organization</p>
              <p className="text-xs text-zinc-500">You will lose access to this organization.</p>
            </div>
            <Button variant="outline" disabled className="border-red-800 text-red-400">Leave</Button>
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-100">Delete organization</p>
              <p className="text-xs text-zinc-500">This action is permanent and cannot be undone.</p>
            </div>
            <Button variant="outline" disabled className="border-red-800 text-red-400">Delete</Button>
          </div>
        </div>
      </section>
    </div>
  )
}
