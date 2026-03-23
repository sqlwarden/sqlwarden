import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { z } from 'zod'
import { useAuth } from '#/contexts/AuthContext'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'

export const Route = createFileRoute('/login')({
  validateSearch: z.object({ next: z.string().optional() }),
  component: LoginPage,
})

function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const { next } = Route.useSearch()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await login(email, password)
      navigate({ to: (next as any) ?? '/' })
    } catch {
      setError('Invalid email or password.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-zinc-950 flex items-center justify-center">
      <div className="w-full max-w-sm px-4">
        <div className="flex items-center justify-center gap-2 mb-8">
          <div className="h-8 w-8 rounded-md bg-zinc-100 flex items-center justify-center">
            <span className="text-sm font-bold text-zinc-900">SW</span>
          </div>
          <span className="text-xl font-semibold text-zinc-100 tracking-tight">SQLWarden</span>
        </div>
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-6">
          <h1 className="text-lg font-semibold text-zinc-100 mb-1">Welcome back</h1>
          <p className="text-sm text-zinc-400 mb-6">Sign in to your account</p>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="email" className="text-zinc-300">Email</Label>
              <Input id="email" type="email" placeholder="you@company.com"
                value={email} onChange={e => setEmail(e.target.value)} required
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password" className="text-zinc-300">Password</Label>
              <Input id="password" type="password" placeholder="••••••••"
                value={password} onChange={e => setPassword(e.target.value)} required
                className="bg-zinc-800 border-zinc-700 text-zinc-100 placeholder:text-zinc-500" />
            </div>
            {error && <p className="text-sm text-red-400">{error}</p>}
            <Button type="submit" disabled={loading}
              className="w-full bg-zinc-100 text-zinc-900 hover:bg-zinc-200">
              {loading ? 'Signing in...' : 'Sign in'}
            </Button>
          </form>
        </div>
      </div>
    </div>
  )
}
