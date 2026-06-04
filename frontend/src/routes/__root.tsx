import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Outlet, createRootRoute, useRouter } from '@tanstack/react-router'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'
import { TanStackDevtools } from '@tanstack/react-devtools'
import { Toaster } from 'sonner'
import { GlobalLoadingBar } from '#/components/GlobalLoadingBar'
import { ThemeProvider } from '#/components/theme-provider'
import { TooltipProvider } from '#/components/ui/tooltip'
import { IconPackProvider } from '#/lib/icons'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { AUTH_INVALIDATED_EVENT } from '#/lib/auth/invalidation'
import '../styles.css'

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  const router = useRouter()
  const queryClient = useQueryClient()

  useEffect(() => {
    function handleAuthInvalidated() {
      clearAuthScopedQueryCache(queryClient)
      void router.navigate({ to: '/login', replace: true })
    }

    window.addEventListener(AUTH_INVALIDATED_EVENT, handleAuthInvalidated)
    return () => window.removeEventListener(AUTH_INVALIDATED_EVENT, handleAuthInvalidated)
  }, [queryClient, router])

  return (
    <ThemeProvider defaultTheme="system" storageKey="theme">
      <IconPackProvider>
      <EditorThemeProvider>
      <TooltipProvider>
        <GlobalLoadingBar />
        <div className="flex min-h-screen flex-col">
          <div className="flex-1">
            <Outlet />
          </div>
        </div>
      </TooltipProvider>
      <Toaster closeButton position="top-center" theme="system" />
      <TanStackDevtools
        config={{ position: 'bottom-right' }}
        plugins={[
          {
            name: 'TanStack Router',
            render: <TanStackRouterDevtoolsPanel />,
          },
        ]}
      />
      </EditorThemeProvider>
      </IconPackProvider>
    </ThemeProvider>
  )
}
