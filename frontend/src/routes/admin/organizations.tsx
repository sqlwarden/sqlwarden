import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import type { AxiosError } from 'axios'
import { useAdminOrgs, useCreateOrg } from '#/lib/queries/useAdmin'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { Alert } from '#/components/ui/alert'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '#/components/ui/sheet'

export const Route = createFileRoute('/admin/organizations')({ component: AdminOrganizations })

function AdminOrganizations() {
  const [page, setPage] = useState(1)
  const { data } = useAdminOrgs(page)
  const createOrg = useCreateOrg()
  const [sheetOpen, setSheetOpen] = useState(false)
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [ownerEmail, setOwnerEmail] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setFormError(null)
    try {
      await createOrg.mutateAsync({ slug, name, ownerEmail })
      setSheetOpen(false)
      setSlug('')
      setName('')
      setOwnerEmail('')
    } catch (err) {
      const msg = (err as AxiosError<{ error: string }>).response?.data?.error ?? 'Failed to create organization'
      setFormError(msg)
    }
  }

  return (
    <div className="p-8 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold text-zinc-100">Organizations</h1>
        <Button onClick={() => setSheetOpen(true)} className="bg-zinc-100 text-zinc-900 hover:bg-zinc-200">
          Create Organization
        </Button>
      </div>

      <div className="border border-zinc-800 rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="border-b border-zinc-800 bg-zinc-900">
            <tr>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Name</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Slug</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {data?.data.map(org => (
              <tr key={org.id} className="border-b border-zinc-800 last:border-0">
                <td className="px-4 py-3 text-zinc-100">{org.name}</td>
                <td className="px-4 py-3 text-zinc-400 font-mono text-xs">{org.slug}</td>
                <td className="px-4 py-3 text-zinc-500 text-xs">{new Date(org.created_at).toLocaleDateString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between mt-4">
        <p className="text-sm text-zinc-500">{data?.total ?? 0} total</p>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Previous</Button>
          <Button variant="outline" size="sm" disabled={!data || page * 50 >= data.total} onClick={() => setPage(p => p + 1)}>Next</Button>
        </div>
      </div>

      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent side="right">
          <SheetHeader>
            <SheetTitle>Create Organization</SheetTitle>
          </SheetHeader>
          <form onSubmit={handleSubmit} className="space-y-4 px-6 pb-6">
            <div className="space-y-1.5">
              <Label>Slug</Label>
              <Input value={slug} onChange={e => setSlug(e.target.value)} placeholder="my-org" required />
            </div>
            <div className="space-y-1.5">
              <Label>Name</Label>
              <Input value={name} onChange={e => setName(e.target.value)} placeholder="My Organization" required />
            </div>
            <div className="space-y-1.5">
              <Label>Owner Email</Label>
              <Input type="email" value={ownerEmail} onChange={e => setOwnerEmail(e.target.value)} placeholder="owner@example.com" required />
            </div>
            {formError && <Alert className="border-red-800 bg-red-900/20 text-red-400">{formError}</Alert>}
            <Button type="submit" disabled={createOrg.isPending} className="w-full">Create</Button>
          </form>
        </SheetContent>
      </Sheet>
    </div>
  )
}
