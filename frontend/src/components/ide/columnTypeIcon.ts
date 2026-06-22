import type { AppIcon } from '#/lib/icons'

/**
 * Maps a driver-specific column data type string (Postgres `int8`/`timestamptz`,
 * MySQL `varchar(255)`/`datetime`, SQLite affinities, …) to a category icon.
 * Order matters: more specific patterns are checked before broader ones
 * (e.g. boolean before number, timestamp/datetime before plain "time", varbinary
 * before var-string). Unknown types fall back to the generic `column` icon.
 */
export function columnTypeIcon(dataType: string): AppIcon {
  const t = dataType.trim().toLowerCase()
  if (!t) return 'column'

  // Boolean — before number so `bit`/`tinyint(1)` aren't read as numeric.
  if (t === 'bool' || t === 'boolean' || t === 'bit' || t.startsWith('tinyint(1)')) return 'type-boolean'

  // Date / time — before number, and timestamp/datetime before plain "time".
  if (t.includes('timestamp') || t.includes('datetime')) return 'type-timestamp'
  if (t.startsWith('date')) return 'type-date'
  if (t.startsWith('time')) return 'type-time'

  // Binary — before string so `varbinary` isn't caught by the "var" string rule.
  if (t.includes('binary') || t === 'bytea' || t.includes('blob')) return 'type-binary'

  if (t === 'uuid' || t.includes('uniqueidentifier')) return 'type-uuid'
  if (t.includes('json')) return 'type-json'

  // Numbers — the integer family (incl. MySQL display-width / unsigned forms
  // like `bigint(20)` and `bigint unsigned`) plus fixed/floating-point types.
  // The integer pattern is anchored so `point` (ends in "int") is NOT matched.
  if (
    /^(tiny|small|medium|big)?int(eger)?(\b|\d|\()/.test(t) ||
    /^(numeric|decimal|dec\b|number|real|double|float|money|smallmoney|fixed|(big|small)?serial)/.test(t)
  ) {
    return 'type-number'
  }

  // Strings — char/text families.
  if (
    t.includes('char') || t.includes('text') || t === 'name' ||
    t.startsWith('string') || t.startsWith('clob')
  ) {
    return 'type-string'
  }

  return 'column'
}
