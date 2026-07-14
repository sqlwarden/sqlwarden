import { describe, expect, it } from 'vitest'
import { isTerminalJobStatus, nextEventCursor, type EventCursor } from './useExportJobs'
import type { JobEventPage } from '#/lib/api/types'

describe('isTerminalJobStatus', () => {
  it('treats succeeded, failed, and cancelled as terminal', () => {
    expect(isTerminalJobStatus('succeeded')).toBe(true)
    expect(isTerminalJobStatus('failed')).toBe(true)
    expect(isTerminalJobStatus('cancelled')).toBe(true)
  })

  it('treats queued and running as non-terminal', () => {
    expect(isTerminalJobStatus('queued')).toBe(false)
    expect(isTerminalJobStatus('running')).toBe(false)
  })
})

describe('nextEventCursor', () => {
  it('keeps the current cursor when the page has no items', () => {
    const current: EventCursor = { afterId: 'evt-5', lastMessage: 'previous' }
    const page: JobEventPage = { items: [] }

    expect(nextEventCursor(current, page)).toEqual(current)
  })

  it('advances afterId and lastMessage from a non-empty page', () => {
    const current: EventCursor = {}
    const page: JobEventPage = {
      items: [
        {
          id: 'evt-1',
          job_id: 'job-1',
          level: 'info',
          code: 'target_connected',
          message: 'Connected',
          created_at: '2026-07-05T00:00:00Z',
        },
        {
          id: 'evt-2',
          job_id: 'job-1',
          level: 'info',
          code: 'rows_streamed',
          message: '10,000 rows streamed',
          created_at: '2026-07-05T00:00:01Z',
        },
      ],
      next_after_id: 'evt-2',
    }

    expect(nextEventCursor(current, page)).toEqual({
      afterId: 'evt-2',
      lastMessage: '10,000 rows streamed',
    })
  })

  it('falls back to the current afterId when the page omits next_after_id', () => {
    const current: EventCursor = { afterId: 'evt-2' }
    const page: JobEventPage = {
      items: [
        {
          id: 'evt-3',
          job_id: 'job-1',
          level: 'info',
          code: 'export_succeeded',
          message: 'Done',
          created_at: '2026-07-05T00:00:02Z',
        },
      ],
    }

    expect(nextEventCursor(current, page)).toEqual({ afterId: 'evt-2', lastMessage: 'Done' })
  })
})
