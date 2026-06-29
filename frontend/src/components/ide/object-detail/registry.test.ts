import { describe, it, expect } from 'vitest'
import { getObjectRenderer, type ObjectViewModel } from './registry'
import { dialectFor } from '../sqlDialect'
import type { ObjectDetail } from '#/lib/api/types'

function vm(detail: ObjectDetail, driver = 'postgres'): ObjectViewModel {
  return { detail, dialect: dialectFor(driver), driver, spec: undefined, orgSlug: 'o', workspaceId: 1, connectionId: 1, sessionId: 's' }
}

const tableDetail: ObjectDetail = {
  ref: { namespace: 'public', kind: 'table', name: 'users' },
  relational: { columns: [{ name: 'id', data_type: 'int8', nullable: false, ordinal: 1 }], primary_key: ['id'] },
}

describe('getObjectRenderer', () => {
  it('falls back to the base renderer for unknown drivers', () => {
    const r = getObjectRenderer('does-not-exist')
    const ids = r.sections(vm(tableDetail, 'does-not-exist')).map((s) => s.id)
    expect(ids).toEqual(['columns', 'keys', 'ddl', 'data'])
  })

  it('base renderer shows no header badges or column extras', () => {
    const r = getObjectRenderer('sqlite')
    expect(r.headerBadges(vm(tableDetail, 'sqlite'))).toEqual([])
    expect(r.columnExtras(vm(tableDetail, 'sqlite'))).toEqual([])
  })

  it('renders a single Overview section for non-relational objects without source', () => {
    const fnDetail: ObjectDetail = {
      ref: { namespace: 'public', kind: 'function', name: 'f' },
      descriptors: [{ kind: 'fields', title: 'Function', fields: [{ name: 'returns', value: 'int' }] }],
    }
    expect(getObjectRenderer('postgres').sections(vm(fnDetail)).map((s) => s.id)).toEqual(['overview'])
  })

  it('adds a source section (labeled by title) for non-relational objects carrying SQL', () => {
    const fnDetail: ObjectDetail = {
      ref: { namespace: 'public', kind: 'function', name: 'f' },
      descriptors: [
        { kind: 'fields', title: 'Signature', fields: [{ name: 'returns', value: 'int' }] },
        { kind: 'source', title: 'Definition', source: { language: 'plpgsql', body: 'BEGIN RETURN 1; END' } },
      ],
    }
    const sections = getObjectRenderer('postgres').sections(vm(fnDetail))
    expect(sections.map((s) => s.id)).toEqual(['overview', 'source-0'])
    expect(sections.map((s) => s.label)).toContain('Definition')
  })

  it('uses the descriptor title for the source label (e.g. mysql trigger Statement)', () => {
    const trigDetail: ObjectDetail = {
      ref: { namespace: 'db', kind: 'trigger', name: 't' },
      descriptors: [{ kind: 'source', title: 'Statement', source: { language: 'sql', body: 'BEGIN END' } }],
    }
    expect(getObjectRenderer('mysql').sections(vm(trigDetail, 'mysql')).map((s) => s.label)).toEqual(['Statement'])
  })

  it('mysql renderer surfaces engine/collation badges but not the approx row estimate', () => {
    const detail: ObjectDetail = { ...tableDetail, attributes: { engine: 'InnoDB', collation: 'utf8mb4', row_estimate: '42' } }
    const badges = getObjectRenderer('mysql').headerBadges(vm(detail, 'mysql')).map((b) => `${b.label}=${b.value}`)
    expect(badges).toContain('Engine=InnoDB')
    expect(badges).toContain('Collation=utf8mb4')
    expect(badges.some((b) => b.startsWith('Rows='))).toBe(false)
  })

  it('mysql renderer adds Comment and Extra column extras', () => {
    const headers = getObjectRenderer('mysql').columnExtras(vm(tableDetail, 'mysql')).map((c) => c.header)
    expect(headers).toEqual(['Comment', 'Extra'])
  })

  it('postgres renderer surfaces a table comment badge', () => {
    const detail: ObjectDetail = { ...tableDetail, attributes: { comment: 'people' } }
    const badges = getObjectRenderer('postgres').headerBadges(vm(detail, 'postgres')).map((b) => b.value)
    expect(badges).toContain('people')
  })
})
