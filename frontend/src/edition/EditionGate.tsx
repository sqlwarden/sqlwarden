import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { editionCapabilitiesQueryOptions } from '#/lib/api/query'
import { EditionUpsell } from './EditionUpsell'
import { findEditionFeature } from './resolve'

interface EditionGateProps {
  feature: string
  children: ReactNode
}

export function EditionGate({ feature, children }: EditionGateProps) {
  const capabilities = useQuery(editionCapabilitiesQueryOptions())
  if (!capabilities.data) return null
  const capability = findEditionFeature(capabilities.data, feature)
  if (!capability) return null
  if (capability.state === 'available') return <>{children}</>
  return <EditionUpsell feature={capability} />
}
