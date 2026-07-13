import type { ReactNode } from 'react'
import type { AppIcon } from '#/lib/icons'
import type { DbColumn, ObjectDetail, SchemaSpec } from '#/lib/api/types'
import type { SqlDialect } from '../dialect'

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

export interface ObjectDetailHooks {
  headerBadges?(vm: ObjectViewModel): HeaderBadge[]
  columnExtras?(vm: ObjectViewModel): ColumnExtra[]
}

export interface ObjectRenderer {
  sections(vm: ObjectViewModel): SectionDef[]
  headerBadges(vm: ObjectViewModel): HeaderBadge[]
  columnExtras(vm: ObjectViewModel): ColumnExtra[]
}
