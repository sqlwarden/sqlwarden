/**
 * Shared active/inactive row chrome for sidebar tree and list rows
 * (DatabasePanel connections, FilesPanel files). Both states span the full
 * sidebar width with square corners so hovering an inactive row doesn't
 * shift position or shape when it becomes active.
 */
export function sidebarActiveRowClass(active: boolean): string {
  return active
    ? 'w-full rounded-none bg-primary/10 text-foreground hover:bg-primary/15'
    : 'w-full rounded-none hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'
}
