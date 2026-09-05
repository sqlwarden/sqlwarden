import { describe, expect, it } from 'vitest'
import {
  connectableEngines,
  dialectFor,
  findFrontendEngine,
  frontendEngines,
  getFrontendEngine,
  UnsupportedFrontendEngineError,
} from './registry'

describe('frontend engine registry — oracle', () => {
  it('resolves the oracle engine', () => {
    expect(findFrontendEngine('oracle')?.label).toBe('Oracle')
  })

  it('exposes oracle as connectable', () => {
    expect(connectableEngines.some((engine) => engine.id === 'oracle')).toBe(true)
  })

  it('returns the oracle dialect', () => {
    expect(dialectFor('oracle').formatColumn('X')).toBe('"X"')
  })

  it('carries a manual-transaction warning for oracle', () => {
    expect(getFrontendEngine('oracle').manualTransactionWarning).toMatch(/DDL/)
  })
})

describe('frontend engine registry', () => {
  it('registers every bundled backend driver explicitly', () => {
    expect(frontendEngines.map((engine) => engine.id)).toEqual([
      'postgres',
      'mysql',
      'oracle',
      'sqlite',
    ])
    for (const engine of frontendEngines) {
      expect(engine.label).not.toBe('')
      expect(engine.dialect).toBeDefined()
      expect(engine.objectDetail).toBeDefined()
      expect(engine.diagram).toBeDefined()
    }
  })

  it('network engines declare a tls spec, sqlite does not', () => {
    const byId = Object.fromEntries(frontendEngines.map((e) => [e.id, e]))
    for (const id of ['postgres', 'mysql', 'oracle']) {
      expect(byId[id].tls, id).toBeDefined()
      expect(byId[id].tls!.modes.length).toBe(4)
    }
    expect(byId['sqlite'].tls).toBeUndefined()
  })

  it('every bundled engine declares backend semantic completion', () => {
    for (const engine of frontendEngines) {
      expect(engine.semanticCompletion, engine.id).toBe(true)
    }
  })

  it('network engines support SSH tunneling, sqlite does not', () => {
    const byId = Object.fromEntries(frontendEngines.map((e) => [e.id, e]))
    for (const id of ['postgres', 'mysql', 'oracle']) {
      expect(byId[id].sshTunnel, id).toBe(true)
    }
    expect(byId['sqlite'].sshTunnel).toBeFalsy()
  })

  it('only exposes drivers with connection forms as connectable', () => {
    expect(connectableEngines.map((engine) => engine.id)).toEqual([
      'postgres',
      'mysql',
      'oracle',
      'sqlite',
    ])
  })

  it('offers explicit strict and optional lookup APIs', () => {
    expect(findFrontendEngine('postgres')?.label).toBe('PostgreSQL')
    expect(dialectFor('postgres')).toBe(findFrontendEngine('postgres')?.dialect)
    expect(findFrontendEngine('unknown')).toBeUndefined()
    expect(() => getFrontendEngine('unknown')).toThrow(UnsupportedFrontendEngineError)
    expect(() => dialectFor('unknown')).toThrow(UnsupportedFrontendEngineError)
  })
})
