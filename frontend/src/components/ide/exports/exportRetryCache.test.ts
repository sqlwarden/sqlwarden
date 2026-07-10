import { describe, expect, it } from 'vitest'
import { getExportRetryEntry, rememberExportRetry } from './exportRetryCache'

describe('exportRetryCache', () => {
  it('returns undefined for an unknown job id', () => {
    expect(getExportRetryEntry('unknown-job')).toBeUndefined()
  })

  it('returns the remembered entry for a known job id', () => {
    rememberExportRetry('job-1', { connectionId: 42, sql: 'SELECT 1', filename: 'report', format: 'csv' })

    expect(getExportRetryEntry('job-1')).toEqual({
      connectionId: 42,
      sql: 'SELECT 1',
      filename: 'report',
      format: 'csv',
    })
  })

  it('overwrites an existing entry for the same job id', () => {
    rememberExportRetry('job-2', { connectionId: 1, sql: 'SELECT 1', filename: 'a', format: 'csv' })
    rememberExportRetry('job-2', { connectionId: 1, sql: 'SELECT 2', filename: 'b', format: 'csv' })

    expect(getExportRetryEntry('job-2')).toEqual({ connectionId: 1, sql: 'SELECT 2', filename: 'b', format: 'csv' })
  })
})
