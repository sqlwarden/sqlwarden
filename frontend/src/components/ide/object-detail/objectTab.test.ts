import { describe, it, expect } from 'vitest'
import { objectTabId, newObjectTab } from './objectTab'
import type { Connection, Workspace, ObjectRef } from '#/lib/api/types'

const ref: ObjectRef = { namespace: 'public', kind: 'table', name: 'users' }
const conn = { id: 7, driver: 'postgres', name: 'Prod' } as Connection
const ws = { id: 3 } as Workspace

describe('objectTab', () => {
  it('builds a stable id from connection + ref', () => {
    expect(objectTabId(7, ref)).toBe('object:7:public:table:users')
    expect(objectTabId(7, ref)).toBe(objectTabId(7, ref))
  })

  it('creates an object tab carrying ref + connection + driver', () => {
    const tab = newObjectTab(conn, ws, ref)
    expect(tab.kind).toBe('object')
    expect(tab.id).toBe('object:7:public:table:users')
    expect(tab.workspaceId).toBe(3)
    expect(tab.connectionId).toBe(7)
    expect(tab.driver).toBe('postgres')
    expect(tab.objectRef).toEqual(ref)
    expect(tab.title).toBe('users')
    expect(tab.content).toBe('')
  })
})
