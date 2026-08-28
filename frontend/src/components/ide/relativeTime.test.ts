import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatDateGroup, formatExactTime, formatRelativeTime } from './relativeTime'

describe('formatRelativeTime', () => {
  const now = new Date('2026-08-20T12:00:00.000Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(now)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows "Just now" for timestamps under a minute old', () => {
    expect(formatRelativeTime(new Date(now.getTime() - 30_000).toISOString())).toBe('Just now')
  })

  it('shows minutes for timestamps under an hour old', () => {
    expect(formatRelativeTime(new Date(now.getTime() - 5 * 60_000).toISOString())).toBe('5m ago')
  })

  it('shows hours for timestamps under a day old', () => {
    expect(formatRelativeTime(new Date(now.getTime() - 3 * 3_600_000).toISOString())).toBe('3h ago')
  })

  it('shows days for timestamps under a week old', () => {
    expect(formatRelativeTime(new Date(now.getTime() - 2 * 86_400_000).toISOString())).toBe(
      '2d ago',
    )
  })

  it('falls back to a short date for timestamps a week or older', () => {
    expect(formatRelativeTime(new Date('2026-08-01T12:00:00.000Z').toISOString())).toBe(
      'Aug 1, 2026',
    )
  })

  it('returns a placeholder for an invalid timestamp', () => {
    expect(formatRelativeTime('not-a-date')).toBe('Unknown')
  })
})

describe('formatExactTime', () => {
  it('includes the full date and time', () => {
    const formatted = formatExactTime('2026-08-01T12:34:56.000Z')
    expect(formatted).toContain('2026')
    expect(formatted).toMatch(/\d{1,2}:\d{2}/)
  })

  it('returns a placeholder for an invalid timestamp', () => {
    expect(formatExactTime('not-a-date')).toBe('Unknown')
  })
})

describe('formatDateGroup', () => {
  const now = new Date('2026-08-20T09:00:00.000Z')

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(now)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('labels timestamps from today as "Today", even earlier in the day', () => {
    expect(formatDateGroup('2026-08-20T01:00:00.000Z')).toBe('Today')
  })

  it('labels timestamps from the calendar day before as "Yesterday"', () => {
    expect(formatDateGroup('2026-08-19T23:00:00.000Z')).toBe('Yesterday')
  })

  it('falls back to a short date for anything older', () => {
    expect(formatDateGroup('2026-08-01T12:00:00.000Z')).toBe('Aug 1, 2026')
  })

  it('returns a placeholder for an invalid timestamp', () => {
    expect(formatDateGroup('not-a-date')).toBe('Unknown')
  })
})
