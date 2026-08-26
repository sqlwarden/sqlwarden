/**
 * Shared active/inactive row chrome for sidebar tree and list rows
 * (DatabasePanel connections, FilesPanel files). Active rows span the full
 * sidebar width with square corners; inactive/hover rows keep the inset,
 * rounded chrome used elsewhere in the sidebar.
 */
export function sidebarActiveRowClass(active: boolean): string {
  return active
    ? 'w-full rounded-none bg-primary/10 text-foreground hover:bg-primary/15'
    : 'mx-1 w-[calc(100%-0.5rem)] rounded-md hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'
}
