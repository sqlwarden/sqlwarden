import type { ReactNode } from 'react'
import type { AppIcon } from '#/lib/icons'
import type { DbColumn, ObjectDetail, SchemaSpec } from '#/lib/api/types'
import type { SqlDialect } from '../sqlDialect'
import { buildBaseSections } from './baseRenderer'
import { postgresHooks } from './drivers/postgres'
import { mysqlHooks } from './drivers/mysql'
import { sqliteHooks } from './drivers/sqlite'

/** Everything a section/badge/column renderer needs: the object detail plus the
 *  addressing required to lazily run a data-preview query or refetch. */
export interface ObjectViewModel {
  detail: ObjectDetail
  spec?: SchemaSpec
  dialect: SqlDialect
  driver: string
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId: string
}

export interface HeaderBadge {
  id: string
  label: string
  value: string
}

export interface ColumnExtra {
  id: string
  header: string
  cell: (col: DbColumn) => ReactNode
}

export interface SectionDef {
  id: string
  label: string
  icon: AppIcon
  render: (vm: ObjectViewModel) => ReactNode
}

/** Per-driver data hooks. A driver supplies only the bits that differ from the
 *  base renderer; everything is optional, so an un-enriched driver degrades to
 *  the base view. DDL/definitions are produced by the backend SchemaInspector
 *  (a "source" descriptor), not by the frontend. */
export interface DriverHooks {
  headerBadges?(vm: ObjectViewModel): HeaderBadge[]
  columnExtras?(vm: ObjectViewModel): ColumnExtra[]
}

export interface ObjectRenderer {
  sections(vm: ObjectViewModel): SectionDef[]
  headerBadges(vm: ObjectViewModel): HeaderBadge[]
  columnExtras(vm: ObjectViewModel): ColumnExtra[]
}

const HOOKS: Record<string, DriverHooks> = {
  postgres: postgresHooks,
  mysql: mysqlHooks,
  sqlite: sqliteHooks,
}

/** Returns the renderer for a driver, falling back to the base renderer (empty
 *  hooks) for any driver without a dedicated implementation. */
export function getObjectRenderer(driver: string): ObjectRenderer {
  const hooks = HOOKS[driver] ?? {}
  return {
    sections: (vm) => buildBaseSections(vm, hooks),
    headerBadges: (vm) => hooks.headerBadges?.(vm) ?? [],
    columnExtras: (vm) => hooks.columnExtras?.(vm) ?? [],
  }
}
