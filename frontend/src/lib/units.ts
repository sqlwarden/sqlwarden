// Human-friendly value/unit pairs for backend fields stored as raw seconds or bytes.
// Conversion always round-trips exactly for values loaded from the API: the display
// unit is chosen as the largest unit that divides the raw value evenly, falling back
// to the base unit (seconds/bytes) otherwise. User-entered amounts are rounded to the
// nearest whole base unit on submit so saved values are always exact integers.

export type DurationUnit = 'seconds' | 'minutes' | 'hours' | 'days'

export const DURATION_UNIT_SECONDS: Record<DurationUnit, number> = {
  seconds: 1,
  minutes: 60,
  hours: 3_600,
  days: 86_400,
}

export const durationUnitOptions: { value: DurationUnit; label: string }[] = [
  { value: 'seconds', label: 'Seconds' },
  { value: 'minutes', label: 'Minutes' },
  { value: 'hours', label: 'Hours' },
  { value: 'days', label: 'Days' },
]

const durationUnitsLargestFirst: DurationUnit[] = ['days', 'hours', 'minutes', 'seconds']

export function secondsToDuration(totalSeconds: number): { amount: number; unit: DurationUnit } {
  for (const unit of durationUnitsLargestFirst) {
    const factor = DURATION_UNIT_SECONDS[unit]
    if (totalSeconds !== 0 && totalSeconds % factor === 0) {
      return { amount: totalSeconds / factor, unit }
    }
  }
  return { amount: totalSeconds, unit: 'seconds' }
}

export function durationToSeconds(amount: number, unit: DurationUnit): number {
  return Math.round(amount * DURATION_UNIT_SECONDS[unit])
}

/** Amount displayed for a fixed unit choice, e.g. while a user is actively editing. */
export function secondsInUnit(totalSeconds: number, unit: DurationUnit): number {
  return totalSeconds / DURATION_UNIT_SECONDS[unit]
}

export function formatDuration(totalSeconds: number): string {
  const { amount, unit } = secondsToDuration(totalSeconds)
  const label = durationUnitOptions.find((option) => option.value === unit)?.label ?? unit
  return `${amount.toLocaleString()} ${amount === 1 ? label.replace(/s$/, '').toLowerCase() : label.toLowerCase()}`
}

export type ByteUnit = 'bytes' | 'kb' | 'mb' | 'gb'

export const BYTE_UNIT_BYTES: Record<ByteUnit, number> = {
  bytes: 1,
  kb: 1_024,
  mb: 1_024 ** 2,
  gb: 1_024 ** 3,
}

export const byteUnitOptions: { value: ByteUnit; label: string }[] = [
  { value: 'bytes', label: 'Bytes' },
  { value: 'kb', label: 'KB' },
  { value: 'mb', label: 'MB' },
  { value: 'gb', label: 'GB' },
]

const byteUnitsLargestFirst: ByteUnit[] = ['gb', 'mb', 'kb', 'bytes']

export function bytesToSize(totalBytes: number): { amount: number; unit: ByteUnit } {
  for (const unit of byteUnitsLargestFirst) {
    const factor = BYTE_UNIT_BYTES[unit]
    if (totalBytes !== 0 && totalBytes % factor === 0) {
      return { amount: totalBytes / factor, unit }
    }
  }
  return { amount: totalBytes, unit: 'bytes' }
}

export function sizeToBytes(amount: number, unit: ByteUnit): number {
  return Math.round(amount * BYTE_UNIT_BYTES[unit])
}

/** Amount displayed for a fixed unit choice, e.g. while a user is actively editing. */
export function bytesInUnit(totalBytes: number, unit: ByteUnit): number {
  return totalBytes / BYTE_UNIT_BYTES[unit]
}

export function formatBytesValue(totalBytes: number): string {
  const { amount, unit } = bytesToSize(totalBytes)
  const label = byteUnitOptions.find((option) => option.value === unit)?.label ?? unit
  return `${amount.toLocaleString()} ${label}`
}
