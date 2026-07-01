import { describe, it, expect } from 'vitest'
import { diagramSupported, diagramSupportedForKind } from './capability'
import type { SchemaSpec } from '#/lib/api/types'

const spec: SchemaSpec = {
  dialect: 'postgres',
  kinds: [
    { kind: 'table', label: 'Table', plural_label: 'Tables', order: 1, relational: true, supports_diagram: true, listing: 'enumerated' },
    { kind: 'function', label: 'Function', plural_label: 'Functions', order: 2, relational: false, supports_diagram: false, listing: 'enumerated' },
  ],
}

describe('diagram capability', () => {
  it('diagramSupported is true when any kind supports a diagram', () => {
    expect(diagramSupported(spec)).toBe(true)
    expect(diagramSupported(undefined)).toBe(false)
    expect(diagramSupported({ dialect: 'redis', kinds: [] })).toBe(false)
  })
  it('diagramSupportedForKind checks the specific kind', () => {
    expect(diagramSupportedForKind(spec, 'table')).toBe(true)
    expect(diagramSupportedForKind(spec, 'function')).toBe(false)
    expect(diagramSupportedForKind(spec, 'missing')).toBe(false)
  })
})
