import { describe, expect, it } from 'vitest'
import { workspaceFileIcon } from './workspaceFileIcon'

describe('workspaceFileIcon', () => {
  it.each([
    ['query.sql', 'sql'],
    ['export.csv', 'csv'],
    ['EXPORT.TSV', 'csv'],
    ['report.xlsx', 'excel'],
    ['legacy.xls', 'excel'],
    ['data.parquet', 'parquet'],
    ['payload.json', 'json'],
    ['config.xml', 'xml'],
    ['notes.md', 'markdown'],
    ['run.sh', 'shell'],
    ['server.log', 'log'],
    ['dump.bin', 'binary'],
  ] as const)('maps %s to the %s icon', (name, icon) => {
    expect(workspaceFileIcon({ name })).toBe(icon)
  })

  it('uses file kind and normalized media type for extensionless files', () => {
    expect(workspaceFileIcon({ name: 'scratch', file_kind: 'SQL' })).toBe('sql')
    expect(workspaceFileIcon({ name: 'results', media_type: 'text/csv; charset=utf-8' })).toBe(
      'csv',
    )
    expect(workspaceFileIcon({ name: 'events', media_type: 'application/x-ndjson' })).toBe('json')
  })

  it('prefers a recognized filename extension over stale metadata', () => {
    expect(
      workspaceFileIcon({ name: 'renamed.sql', file_kind: 'export', media_type: 'text/csv' }),
    ).toBe('sql')
  })

  it('falls back to the generic file icon for unknown and hidden files', () => {
    expect(workspaceFileIcon({ name: 'archive.custom' })).toBe('default')
    expect(workspaceFileIcon({ name: '.env' })).toBe('default')
  })
})
