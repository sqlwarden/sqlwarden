import { Outlet, createRootRoute, useRouterState } from '@tanstack/react-router'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'
import { TanStackDevtools } from '@tanstack/react-devtools'
import { Toaster } from 'sonner'
import { GlobalLoadingBar } from '#/components/GlobalLoadingBar'
import { LayoutWidthProvider } from '#/components/layout-width-provider'
import { ThemeProvider } from '#/components/theme-provider'
import { TooltipProvider } from '#/components/ui/tooltip'
import Header from '#/components/Header'
import '../styles.css'

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const isSetupRoute = pathname === '/setup'
  const isLoginRoute = pathname === '/login'
  const isSettingsRoute = pathname === '/settings' || pathname.startsWith('/settings/')
  const isOrgRoute = pathname.startsWith('/orgs/')

  return (
    <ThemeProvider defaultTheme="system" storageKey="theme">
      <TooltipProvider>
        <GlobalLoadingBar />
        <LayoutWidthProvider>
          <div className="flex min-h-screen flex-col">
            {!isSetupRoute && !isLoginRoute && !isSettingsRoute && !isOrgRoute ? <Header /> : null}
            <div className="flex-1">
              <Outlet />
            </div>
          </div>
        </LayoutWidthProvider>
      </TooltipProvider>
      <Toaster closeButton position="top-right" theme="system" />
      <TanStackDevtools
        config={{ position: 'bottom-right' }}
        plugins={[
          {
            name: 'TanStack Router',
            render: <TanStackRouterDevtoolsPanel />,
          },
        ]}
      />
    </ThemeProvider>
  )
}
