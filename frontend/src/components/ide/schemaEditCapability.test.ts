import { describe, expect, it } from 'vitest'
import type { SchemaEditSpec } from '#/lib/api/types'
import {
  canCreateTable,
  canDropColumn,
  canDropIndex,
  canDropObject,
  canDropScope,
  canRenameColumn,
  cascadeAvailable,
} from './schemaEditCapability'

const postgresEditor: SchemaEditSpec = {
  operations: [
    'create_table',
    'drop_object',
    'drop_scope',
    'rename_column',
    'drop_column',
    'drop_index',
  ],
  column_types: ['text', 'integer'],
  creatable_table_scope_kinds: ['schema'],
  droppable_object_kinds: ['table', 'view', 'materialized_view'],
  droppable_scope_kinds: ['schema'],
  supports_cascade: true,
}

describe('schemaEditCapability', () => {
  it('allows create_table only for a supported scope kind, session, and permission', () => {
    expect(canCreateTable(postgresEditor, 'session-1', true, 'schema')).toEqual({ allowed: true })
    expect(canCreateTable(postgresEditor, 'session-1', true, 'database').allowed).toBe(false)
  })

  it('reports driver-unsupported when the editor spec is missing entirely', () => {
    const gate = canCreateTable(undefined, 'session-1', true, 'schema')
    expect(gate).toEqual({ allowed: false, reason: 'This driver does not support this change.' })
  })

  it('reports driver-unsupported when the operation is not advertised', () => {
    const editor: SchemaEditSpec = { ...postgresEditor, operations: [] }
    const gate = canCreateTable(editor, 'session-1', true, 'schema')
    expect(gate.allowed).toBe(false)
    if (!gate.allowed) expect(gate.reason).toBe('This driver does not support this change.')
  })

  it('reports not-connected before checking permission', () => {
    const gate = canCreateTable(postgresEditor, undefined, false, 'schema')
    expect(gate).toEqual({
      allowed: false,
      reason: 'Connect to the database to make this change.',
    })
  })

  it('reports missing permission once connected', () => {
    const gate = canCreateTable(postgresEditor, 'session-1', false, 'schema')
    expect(gate).toEqual({
      allowed: false,
      reason: 'You do not have permission to change this schema.',
    })
  })

  it('gates drop_scope on the droppable scope kinds', () => {
    expect(canDropScope(postgresEditor, 'session-1', true, 'schema').allowed).toBe(true)
    expect(canDropScope(postgresEditor, 'session-1', true, 'database').allowed).toBe(false)
  })

  it('gates drop_object on the droppable object kinds', () => {
    expect(canDropObject(postgresEditor, 'session-1', true, 'table').allowed).toBe(true)
    expect(canDropObject(postgresEditor, 'session-1', true, 'function').allowed).toBe(false)
  })

  it('restricts rename/drop column and drop index to table objects only', () => {
    expect(canRenameColumn(postgresEditor, 'session-1', true, 'table').allowed).toBe(true)
    expect(canRenameColumn(postgresEditor, 'session-1', true, 'view').allowed).toBe(false)
    expect(canDropColumn(postgresEditor, 'session-1', true, 'table').allowed).toBe(true)
    expect(canDropColumn(postgresEditor, 'session-1', true, 'view').allowed).toBe(false)
    expect(canDropIndex(postgresEditor, 'session-1', true, 'table').allowed).toBe(true)
    expect(canDropIndex(postgresEditor, 'session-1', true, 'view').allowed).toBe(false)
  })

  it('reflects the driver-advertised cascade support, never assumed', () => {
    expect(cascadeAvailable(postgresEditor, 'scope')).toBe(true)
    expect(cascadeAvailable(postgresEditor, 'object')).toBe(true)
    expect(cascadeAvailable(postgresEditor, 'column')).toBe(true)
    expect(cascadeAvailable({ ...postgresEditor, supports_cascade: false }, 'scope')).toBe(false)
    expect(cascadeAvailable(undefined, 'scope')).toBe(false)
  })

  it('never offers cascade for drop_index, regardless of driver support', () => {
    expect(cascadeAvailable(postgresEditor, 'index')).toBe(false)
  })
})
