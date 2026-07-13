import type { ReactElement, ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { render, renderHook, type RenderHookOptions, type RenderOptions } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { getRouter } from '#/router'

export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: Infinity,
        staleTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  })
}

function providerWrapper(queryClient: QueryClient) {
  return function TestProviders({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

export function renderWithProviders(
  ui: ReactElement,
  options: RenderOptions & { queryClient?: QueryClient } = {},
) {
  const { queryClient = createTestQueryClient(), ...renderOptions } = options
  return {
    queryClient,
    user: userEvent.setup(),
    ...render(ui, { wrapper: providerWrapper(queryClient), ...renderOptions }),
  }
}

export function renderHookWithProviders<Result, Props>(
  hook: (initialProps: Props) => Result,
  options: RenderHookOptions<Props> & { queryClient?: QueryClient } = {},
) {
  const { queryClient = createTestQueryClient(), ...renderOptions } = options
  return {
    queryClient,
    ...renderHook(hook, { wrapper: providerWrapper(queryClient), ...renderOptions }),
  }
}

export function renderRoute(initialEntry: string) {
  const queryClient = createTestQueryClient()
  const history = createMemoryHistory({ initialEntries: [initialEntry] })
  const router = getRouter({ history })

  return {
    history,
    queryClient,
    router,
    user: userEvent.setup(),
    ...render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    ),
  }
}
