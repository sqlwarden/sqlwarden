import { describe, expect, it } from 'vitest'
import {
  bytesInUnit,
  bytesToSize,
  durationToSeconds,
  formatBytesValue,
  formatDuration,
  secondsInUnit,
  secondsToDuration,
  sizeToBytes,
} from './units'

describe('secondsToDuration', () => {
  it('picks the largest exact unit', () => {
    expect(secondsToDuration(86_400)).toEqual({ amount: 1, unit: 'days' })
    expect(secondsToDuration(7_200)).toEqual({ amount: 2, unit: 'hours' })
    expect(secondsToDuration(120)).toEqual({ amount: 2, unit: 'minutes' })
    expect(secondsToDuration(90)).toEqual({ amount: 90, unit: 'seconds' })
  })

  it('falls back to seconds for zero', () => {
    expect(secondsToDuration(0)).toEqual({ amount: 0, unit: 'seconds' })
  })

  it('round-trips through durationToSeconds', () => {
    for (const seconds of [1, 59, 60, 3_600, 86_400, 172_800, 9_223_372_036]) {
      const { amount, unit } = secondsToDuration(seconds)
      expect(durationToSeconds(amount, unit)).toBe(seconds)
    }
  })
})

describe('durationToSeconds', () => {
  it('converts and rounds fractional amounts', () => {
    expect(durationToSeconds(2, 'hours')).toBe(7_200)
    expect(durationToSeconds(1.5, 'minutes')).toBe(90)
  })
})

describe('bytesToSize', () => {
  it('picks the largest exact unit', () => {
    expect(bytesToSize(1_024 ** 3)).toEqual({ amount: 1, unit: 'gb' })
    expect(bytesToSize(5 * 1_024 ** 2)).toEqual({ amount: 5, unit: 'mb' })
    expect(bytesToSize(2_048)).toEqual({ amount: 2, unit: 'kb' })
    expect(bytesToSize(500)).toEqual({ amount: 500, unit: 'bytes' })
  })

  it('falls back to bytes for zero', () => {
    expect(bytesToSize(0)).toEqual({ amount: 0, unit: 'bytes' })
  })

  it('round-trips through sizeToBytes', () => {
    for (const bytes of [0, 1, 1_023, 1_024, 52_428_800, 5 * 1_024 ** 3]) {
      const { amount, unit } = bytesToSize(bytes)
      expect(sizeToBytes(amount, unit)).toBe(bytes)
    }
  })
})

describe('sizeToBytes', () => {
  it('converts and rounds fractional amounts', () => {
    expect(sizeToBytes(2, 'mb')).toBe(2 * 1_024 ** 2)
    expect(sizeToBytes(1.5, 'kb')).toBe(1_536)
  })
})

describe('formatDuration', () => {
  it('renders a human-friendly label', () => {
    expect(formatDuration(1)).toBe('1 second')
    expect(formatDuration(120)).toBe('2 minutes')
    expect(formatDuration(86_400)).toBe('1 day')
  })
})

describe('formatBytesValue', () => {
  it('renders a human-friendly label', () => {
    expect(formatBytesValue(1_024)).toBe('1 KB')
    expect(formatBytesValue(5 * 1_024 ** 2)).toBe('5 MB')
  })
})

describe('secondsInUnit', () => {
  it('computes the display amount for a fixed unit', () => {
    expect(secondsInUnit(7_200, 'hours')).toBe(2)
    expect(secondsInUnit(90, 'minutes')).toBe(1.5)
  })
})

describe('bytesInUnit', () => {
  it('computes the display amount for a fixed unit', () => {
    expect(bytesInUnit(2 * 1_024 ** 2, 'mb')).toBe(2)
    expect(bytesInUnit(1_536, 'kb')).toBe(1.5)
  })
})
