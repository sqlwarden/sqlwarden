import { createContext, useContext, useLayoutEffect, type ReactNode } from 'react'

export const PRODUCT_NAME = 'SQLWarden'

type PageTitleSegment = string | null | undefined | false

type PageTitleScope = {
  organizationName?: string
  workspaceName?: string
}

const PageTitleScopeContext = createContext<PageTitleScope>({})

export function formatPageTitle(...segments: PageTitleSegment[]) {
  const parts = segments
    .filter((segment): segment is string => typeof segment === 'string')
    .map((segment) => segment.trim())
    .filter(Boolean)

  if (parts.at(-1)?.toLowerCase() !== PRODUCT_NAME.toLowerCase()) {
    parts.push(PRODUCT_NAME)
  }
  return parts.join(' | ')
}

export function usePageTitle(...segments: PageTitleSegment[]) {
  const title = formatPageTitle(...segments)

  useLayoutEffect(() => {
    const previousTitle = document.title
    document.title = title

    return () => {
      // A newer page may already own the title during a route transition.
      if (document.title === title) {
        document.title = previousTitle || PRODUCT_NAME
      }
    }
  }, [title])
}

export function PageTitleScopeProvider({
  organizationName,
  workspaceName,
  children,
}: PageTitleScope & { children: ReactNode }) {
  return (
    <PageTitleScopeContext.Provider value={{ organizationName, workspaceName }}>
      {children}
    </PageTitleScopeContext.Provider>
  )
}

export function usePageTitleScope() {
  return useContext(PageTitleScopeContext)
}

export function useOrganizationPageTitle(primary: PageTitleSegment) {
  const { organizationName } = usePageTitleScope()
  usePageTitle(primary, organizationName)
}

export function useWorkspacePageTitle(primary: PageTitleSegment) {
  const { workspaceName } = usePageTitleScope()
  usePageTitle(primary, workspaceName)
}
