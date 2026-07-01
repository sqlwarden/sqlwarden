import { describe, it, expect } from 'vitest'
import { diagramTabId, newDiagramTab } from './diagramTab'
import type { Connection, Workspace, ObjectRef } from '#/lib/api/types'

const conn = { id: 4, driver: 'postgres', name: 'Prod' } as Connection
const ws = { id: 2 } as Workspace
const ref: ObjectRef = { namespace: 'public', kind: 'table', name: 'users' }

describe('diagramTab', () => {
  it('builds distinct stable ids for namespace vs object targets', () => {
    expect(diagramTabId(4, { kind: 'namespace', namespace: 'public' })).toBe('diagram:4:ns:public')
    expect(diagramTabId(4, { kind: 'object', ref })).toBe('diagram:4:obj:public:table:users')
  })
  it('creates a namespace diagram tab', () => {
    const tab = newDiagramTab(conn, ws, { kind: 'namespace', namespace: 'public' })
    expect(tab.kind).toBe('diagram')
    expect(tab.connectionId).toBe(4)
    expect(tab.driver).toBe('postgres')
    expect(tab.title).toBe('public')
    expect(tab.diagramTarget).toEqual({ kind: 'namespace', namespace: 'public' })
  })
  it('creates an object diagram tab titled by object name', () => {
    const tab = newDiagramTab(conn, ws, { kind: 'object', ref })
    expect(tab.title).toBe('users')
    expect(tab.diagramTarget).toEqual({ kind: 'object', ref })
  })
})
