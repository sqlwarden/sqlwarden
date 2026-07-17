import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createAppQueryClient } from '#/app/query-client'
import { DistributionProviders } from '#/distribution/composition'

const queryClient = createAppQueryClient()

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <DistributionProviders>{children}</DistributionProviders>
    </QueryClientProvider>
  )
}
