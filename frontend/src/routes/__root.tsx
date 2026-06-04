import { Outlet, createRootRoute } from '@tanstack/react-router'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'
import { TanStackDevtools } from '@tanstack/react-devtools'
import { Toaster } from 'sonner'
import { GlobalLoadingBar } from '#/components/GlobalLoadingBar'
import { ThemeProvider } from '#/components/theme-provider'
import { TooltipProvider } from '#/components/ui/tooltip'
import { IconPackProvider } from '#/lib/icons'
import '../styles.css'

export const Route = createRootRoute({
  component: RootComponent,
})

function RootComponent() {
  return (
    <ThemeProvider defaultTheme="system" storageKey="theme">
      <IconPackProvider>
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
      </IconPackProvider>
    </ThemeProvider>
  )
}
