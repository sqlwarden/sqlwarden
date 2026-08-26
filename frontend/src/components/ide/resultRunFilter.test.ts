import { describe, expect, it } from 'vitest'
import { resolveSelectedRunId, visibleRuns } from './resultRunFilter'
import type { ResultRun } from './useIdeStore'

function run(id: string, tabId: string, createdAt: number, connectionId?: number): ResultRun {
  return { id, tabId, results: [], selectedIndex: 0, createdAt, connectionId }
}

describe('visibleRuns', () => {
  const resultRuns = {
    t1: [run('r1', 't1', 1, 100), run('r2', 't1', 3, 100)],
    t2: [run('r3', 't2', 2, 200)],
  }

  it('per-editor: only the focused tab’s own runs', () => {
    expect(visibleRuns('per-editor', resultRuns, 't1', 100).map((r) => r.id)).toEqual(['r1', 'r2'])
  })

  it('per-editor: empty when no tab is focused', () => {
    expect(visibleRuns('per-editor', resultRuns, undefined, undefined)).toEqual([])
  })

  it('shared: every run from every tab, oldest first', () => {
    expect(visibleRuns('shared', resultRuns, 't1', 100).map((r) => r.id)).toEqual([
      'r1',
      'r3',
      'r2',
    ])
  })

  it('per-connection: runs across tabs matching the focused connection, oldest first', () => {
    expect(visibleRuns('per-connection', resultRuns, 't1', 100).map((r) => r.id)).toEqual([
      'r1',
      'r2',
    ])
  })

  it('per-connection: empty when the focused tab has no connection', () => {
    expect(visibleRuns('per-connection', resultRuns, 't1', undefined)).toEqual([])
  })
})

describe('resolveSelectedRunId', () => {
  const runs = [run('r1', 't1', 1), run('r2', 't1', 2)]

  it('uses the remembered per-tab selection when still visible', () => {
    expect(resolveSelectedRunId('per-editor', runs, 'r1', undefined, undefined)).toBe('r1')
  })

  it('uses the remembered shared selection when still visible', () => {
    expect(resolveSelectedRunId('shared', runs, undefined, 'r1', undefined)).toBe('r1')
  })

  it('uses the remembered per-connection selection when still visible', () => {
    expect(resolveSelectedRunId('per-connection', runs, undefined, undefined, 'r2')).toBe('r2')
  })

  it('falls back to the latest run when the remembered selection is gone', () => {
    expect(resolveSelectedRunId('shared', runs, undefined, 'evicted', undefined)).toBe('r2')
  })

  it('falls back to undefined when there are no runs', () => {
    expect(resolveSelectedRunId('shared', [], undefined, undefined, undefined)).toBeUndefined()
  })
})
