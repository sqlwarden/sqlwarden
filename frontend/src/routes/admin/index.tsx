import { createFileRoute } from '@tanstack/react-router'
import { useAdminOrgs, useAdminAccounts } from '#/lib/queries/useAdmin'

export const Route = createFileRoute('/admin/')({ component: AdminDashboard })

function AdminDashboard() {
  const { data: orgs } = useAdminOrgs()
  const { data: accounts } = useAdminAccounts()
  return (
    <div className="p-8 max-w-5xl mx-auto">
      <h1 className="text-2xl font-semibold text-zinc-100 mb-6">Admin Dashboard</h1>
      <div className="grid grid-cols-3 gap-4 mb-8">
        <StatCard label="Organizations" value={orgs?.total ?? '—'} />
        <StatCard label="Accounts" value={accounts?.total ?? '—'} />
        <StatCard label="Active Sessions" value="—" note="Coming soon" />
      </div>
      <h2 className="text-lg font-medium text-zinc-200 mb-3">Recent Organizations</h2>
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
            {orgs?.data.slice(0, 5).map(org => (
              <tr key={org.id} className="border-b border-zinc-800 last:border-0">
                <td className="px-4 py-3 text-zinc-100">{org.name}</td>
                <td className="px-4 py-3 text-zinc-400 font-mono text-xs">{org.slug}</td>
                <td className="px-4 py-3 text-zinc-500 text-xs">{new Date(org.created_at).toLocaleDateString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function StatCard({ label, value, note }: { label: string; value: string | number; note?: string }) {
  return (
    <div className="border border-zinc-800 rounded-lg p-4 bg-zinc-900">
      <p className="text-xs text-zinc-500 uppercase tracking-wider mb-1">{label}</p>
      <p className="text-2xl font-semibold text-zinc-100">{value}</p>
      {note && <p className="text-xs text-zinc-600 mt-1">{note}</p>}
    </div>
  )
}
