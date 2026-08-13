import { describe, it, expect } from 'vitest'
import { diagramTabId, newDiagramTab } from './diagramTab'
import type { Connection, Workspace, ObjectRef } from '#/lib/api/types'

const conn = { id: 4, driver: 'postgres', name: 'Prod' } as Connection
const ws = { id: 2 } as Workspace
const scope = [{ kind: 'schema', name: 'public' }]
const ref: ObjectRef = { scope, kind: 'table', name: 'users' }

describe('diagramTab', () => {
  it('builds distinct stable ids for scope vs object targets', () => {
    expect(diagramTabId(4, { kind: 'scope', scope })).toBe('diagram:4:scope:schema=public')
    expect(diagramTabId(4, { kind: 'object', ref })).toBe('diagram:4:obj:schema=public:table:users')
  })
  it('creates a scope diagram tab', () => {
    const tab = newDiagramTab(conn, ws, { kind: 'scope', scope })
    expect(tab.kind).toBe('diagram')
    expect(tab.connectionId).toBe(4)
    expect(tab.driver).toBe('postgres')
    expect(tab.title).toBe('public')
    expect(tab.diagramTarget).toEqual({ kind: 'scope', scope })
  })
  it('creates an object diagram tab titled by object name', () => {
    const tab = newDiagramTab(conn, ws, { kind: 'object', ref })
    expect(tab.title).toBe('users')
    expect(tab.diagramTarget).toEqual({ kind: 'object', ref })
  })
})
