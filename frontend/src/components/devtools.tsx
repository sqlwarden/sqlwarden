import { TanStackDevtools } from '@tanstack/react-devtools'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'

// Rendered only in development. __root lazy-imports this behind
// import.meta.env.DEV so the devtools are tree-shaken from production builds.
export default function Devtools() {
  return (
    <TanStackDevtools
      config={{ position: 'bottom-right' }}
      plugins={[
        {
          name: 'TanStack Router',
          render: <TanStackRouterDevtoolsPanel />,
        },
      ]}
    />
  )
}
