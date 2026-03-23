import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useAdminAccounts } from '#/lib/queries/useAdmin'
import { Button } from '#/components/ui/button'
import { Badge } from '#/components/ui/badge'

export const Route = createFileRoute('/admin/accounts')({ component: AdminAccounts })

function AdminAccounts() {
  const [page, setPage] = useState(1)
  const { data } = useAdminAccounts(page)

  return (
    <div className="p-8 max-w-5xl mx-auto">
      <h1 className="text-2xl font-semibold text-zinc-100 mb-6">Accounts</h1>
      <div className="border border-zinc-800 rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="border-b border-zinc-800 bg-zinc-900">
            <tr>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Name</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Email</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Status</th>
              <th className="text-left px-4 py-3 text-zinc-400 font-medium">Role</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {data?.data.map(account => (
              <tr key={account.id} className="border-b border-zinc-800 last:border-0">
                <td className="px-4 py-3 text-zinc-100">{account.name}</td>
                <td className="px-4 py-3 text-zinc-400 text-xs">{account.email}</td>
                <td className="px-4 py-3">
                  <Badge variant={account.is_active ? 'default' : 'secondary'}>
                    {account.is_active ? 'Active' : 'Inactive'}
                  </Badge>
                </td>
                <td className="px-4 py-3">
                  {account.is_superadmin && <Badge variant="secondary">Superadmin</Badge>}
                </td>
                <td className="px-4 py-3 text-right">
                  <Button variant="outline" size="sm" disabled>Deactivate</Button>
                </td>
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
    </div>
  )
}
