import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QueryClientProvider } from '@tanstack/react-query'
import type React from 'react'
import { Table, TableBody } from '#/components/ui/table'
import type { Connection } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { ConnectionRow } from './orgs.$org_slug.workspaces.$workspace_id.connections'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  const ReactModule = await import('react')
  return {
    ...actual,
    Link: ReactModule.forwardRef<
      HTMLAnchorElement,
      {
        to: string
        params?: Record<string, string | number>
        search?: Record<string, string | number | boolean>
        children?: React.ReactNode
      } & React.AnchorHTMLAttributes<HTMLAnchorElement>
    >(({ to, children, ...props }, ref) => (
      <a ref={ref} href={to} {...props}>
        {children}
      </a>
    )),
  }
})

const connection: Connection = {
  id: 1,
  workspace_id: 3,
  environment_id: 5,
  name: 'analytics-pg',
  driver: 'postgres',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

function renderRow() {
  const queryClient = createTestQueryClient()

  return render(
    <QueryClientProvider client={queryClient}>
      <Table>
        <TableBody>
          <ConnectionRow
            connection={connection}
            environmentName="Production"
            orgSlug="acme"
            workspaceId="3"
            canEditConnection={false}
            canDeleteConnection={false}
            isDeleting={false}
            onEdit={() => {}}
            onDelete={() => {}}
          />
        </TableBody>
      </Table>
    </QueryClientProvider>,
  )
}

describe('ConnectionRow', () => {
  it('renders the "Open in Editor" link as a real anchor, not a native-button-claiming element', () => {
    const queryClient = createTestQueryClient()

    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <Table>
          <TableBody>
            <ConnectionRow
              connection={connection}
              environmentName="Production"
              orgSlug="acme"
              workspaceId="3"
              canEditConnection={false}
              canDeleteConnection={false}
              isDeleting={false}
              onEdit={() => {}}
              onDelete={() => {}}
            />
          </TableBody>
        </Table>
      </QueryClientProvider>,
    )

    // With nativeButton={false}, the "Open in Editor" button should render as a real anchor tag
    const allAnchors = container.querySelectorAll('a')
    const link = Array.from(allAnchors).find((a) => a.textContent?.includes('Open in Editor'))
    expect(link).not.toBeNull()
    expect(link?.tagName).toBe('A')
  })
})

describe('ConnectionRow mobile actions column', () => {
  it('keeps the actions cell pinned to the visible edge of the scrollable table', () => {
    const { container } = renderRow()
    const allAnchors = container.querySelectorAll('a')
    const link = Array.from(allAnchors).find((a) => a.textContent?.includes('Open in Editor'))
    const cell = link?.closest('td')
    expect(cell).toHaveClass('sticky', 'right-0')
  })
})
