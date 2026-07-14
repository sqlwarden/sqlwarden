import { describe, expect, it } from 'vitest'
import {
  connectableEngines,
  dialectFor,
  findFrontendEngine,
  frontendEngines,
  getFrontendEngine,
  UnsupportedFrontendEngineError,
} from './registry'

describe('frontend engine registry', () => {
  it('registers every bundled backend driver explicitly', () => {
    expect(frontendEngines.map((engine) => engine.id)).toEqual(['postgres', 'mysql', 'sqlite'])
    for (const engine of frontendEngines) {
      expect(engine.label).not.toBe('')
      expect(engine.dialect).toBeDefined()
      expect(engine.objectDetail).toBeDefined()
      expect(engine.diagram).toBeDefined()
    }
  })

  it('only exposes drivers with connection forms as connectable', () => {
    expect(connectableEngines.map((engine) => engine.id)).toEqual(['postgres', 'mysql'])
  })

  it('offers explicit strict and optional lookup APIs', () => {
    expect(findFrontendEngine('postgres')?.label).toBe('PostgreSQL')
    expect(dialectFor('postgres')).toBe(findFrontendEngine('postgres')?.dialect)
    expect(findFrontendEngine('unknown')).toBeUndefined()
    expect(() => getFrontendEngine('unknown')).toThrow(UnsupportedFrontendEngineError)
    expect(() => dialectFor('unknown')).toThrow(UnsupportedFrontendEngineError)
  })
})
