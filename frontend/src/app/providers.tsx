import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createAppQueryClient } from '#/app/query-client'
import { DesktopStartupGate } from '#/components/desktop/DesktopStartupGate'

const queryClient = createAppQueryClient()

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <QueryClientProvider client={queryClient}>
      <DesktopStartupGate>{children}</DesktopStartupGate>
    </QueryClientProvider>
  )
}
