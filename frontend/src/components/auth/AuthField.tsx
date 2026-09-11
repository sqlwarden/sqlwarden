import type { ReactNode } from 'react'
import { FormField } from '#/components/ui/field'

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
    <FormField label={label} error={error}>
      {children}
    </FormField>
  )
}
