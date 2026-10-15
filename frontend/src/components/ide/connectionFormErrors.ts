export type MappedFieldErrors = {
  name?: string
  environmentId?: string
  fields: Record<string, string>
  form?: string
}

/**
 * Splits the backend's field-error map into the form's slots. Errors keyed
 * `params.<key>` attach to the matching input; anything else without its own
 * input (secrets, TLS, SSH, driver) surfaces as a form-level message.
 */
export function mapConnectionFieldErrors(fieldErrors: Record<string, string>): MappedFieldErrors {
  const mapped: MappedFieldErrors = { fields: {} }
  for (const [key, message] of Object.entries(fieldErrors)) {
    if (key === 'name') mapped.name = message
    else if (key === 'environment_id') mapped.environmentId = message
    else if (key.startsWith('params.')) mapped.fields[key.slice('params.'.length)] = message
    else mapped.form ??= message
  }
  return mapped
}
