import type { ReactNode } from 'react'

export function AuthField({
  children,
  error,
  label,
}: {
  children: ReactNode
  error?: string
  label: string
}) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
