import { describe, expect, it } from 'vitest'
import { workspaceFileIcon } from './workspaceFileIcon'

describe('workspaceFileIcon', () => {
  it.each([
    ['query.sql', 'database'],
    ['export.csv', 'table'],
    ['EXPORT.TSV', 'table'],
    ['payload.json', 'type-json'],
    ['notes.md', 'book-open-02'],
    ['run.sh', 'terminal'],
    ['server.log', 'subject'],
    ['dump.bin', 'type-binary'],
  ] as const)('maps %s to the %s icon', (name, icon) => {
    expect(workspaceFileIcon({ name })).toBe(icon)
  })

  it('uses file kind and normalized media type for extensionless files', () => {
    expect(workspaceFileIcon({ name: 'scratch', file_kind: 'SQL' })).toBe('database')
    expect(workspaceFileIcon({ name: 'results', media_type: 'text/csv; charset=utf-8' })).toBe(
      'table',
    )
    expect(workspaceFileIcon({ name: 'events', media_type: 'application/x-ndjson' })).toBe(
      'type-json',
    )
  })

  it('prefers a recognized filename extension over stale metadata', () => {
    expect(
      workspaceFileIcon({ name: 'renamed.sql', file_kind: 'export', media_type: 'text/csv' }),
    ).toBe('database')
  })

  it('falls back to the generic file icon for unknown and hidden files', () => {
    expect(workspaceFileIcon({ name: 'archive.custom' })).toBe('file-01')
    expect(workspaceFileIcon({ name: '.env' })).toBe('file-01')
  })
})
