import { describe, it, expect } from 'vitest'
import type { StatementSpec } from '#/lib/api/types'
import { statementOperationsFor } from './generateStatementCapability'

const spec: StatementSpec = {
  objects: [
    { kind: 'table', operations: ['delete', 'select', 'insert', 'update'] },
    { kind: 'view', operations: ['select'] },
  ],
}

describe('statementOperationsFor', () => {
  it('returns operations for a matching kind in fixed select/insert/update/delete order', () => {
    expect(statementOperationsFor(spec, 'table')).toEqual(['select', 'insert', 'update', 'delete'])
  })

  it('returns only what the backend advertises for a more limited kind', () => {
    expect(statementOperationsFor(spec, 'view')).toEqual(['select'])
  })

  it('returns an empty list for a kind the backend does not advertise', () => {
    expect(statementOperationsFor(spec, 'sequence')).toEqual([])
  })

  it('returns an empty list when the spec is undefined', () => {
    expect(statementOperationsFor(undefined, 'table')).toEqual([])
  })
})
