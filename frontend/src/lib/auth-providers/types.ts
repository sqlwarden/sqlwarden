import type { ComponentType } from 'react'

export type AuthProviderDefinition = {
  id: string
  label: string
  Icon: ComponentType<{ className?: string }>
  startSignIn: (redirect: string | undefined) => void
}
