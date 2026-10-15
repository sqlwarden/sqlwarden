import type { ConnectionFieldSpec, ConnectionParams } from '#/lib/api/types'
import type { FieldLayout } from './types'

export type ResolvedField = ConnectionFieldSpec &
  Pick<FieldLayout, 'placeholder' | 'span' | 'section'>

/**
 * Merges the engine's field spec, which owns keys, types, defaults, and
 * requirements, with the driver's layout, which owns labels, order, and
 * grouping. Spec fields without a layout entry render last at full width, and
 * layout entries without a spec field are dropped.
 */
export function resolveFields(spec: ConnectionFieldSpec[], layout: FieldLayout[]): ResolvedField[] {
  const byKey = new Map(spec.map((field) => [field.key, field]))
  const laidOut = new Set<string>()
  const resolved: ResolvedField[] = []
  for (const entry of layout) {
    const field = byKey.get(entry.key)
    if (!field) continue
    laidOut.add(entry.key)
    resolved.push({
      ...field,
      label: entry.label,
      placeholder: entry.placeholder,
      span: entry.span,
      section: entry.section,
    })
  }
  for (const field of spec) {
    if (!laidOut.has(field.key)) resolved.push({ ...field })
  }
  return resolved
}

export function fieldDefaults(fields: ResolvedField[]): Record<string, string> {
  const values: Record<string, string> = {}
  for (const field of fields) {
    if (!field.secret) values[field.key] = field.default ?? ''
  }
  return values
}

export function requiredFieldErrors(
  fields: ResolvedField[],
  values: Record<string, string>,
): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const field of fields) {
    if (field.required && !field.secret && !values[field.key]?.trim()) {
      errors[field.key] = `${field.label} is required.`
    }
  }
  return errors
}

export function paramsFromValues(
  fields: ResolvedField[],
  values: Record<string, string>,
): ConnectionParams {
  const params: ConnectionParams = {}
  for (const field of fields) {
    const value = values[field.key]
    if (!field.secret && value !== undefined && value !== '') params[field.key] = value
  }
  return params
}

/** Seeds form values from stored params, falling back to spec defaults. */
export function valuesFromParams(
  fields: ResolvedField[],
  params: ConnectionParams,
): Record<string, string> {
  return { ...fieldDefaults(fields), ...params }
}
