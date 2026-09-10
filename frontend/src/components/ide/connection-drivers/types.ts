export type FieldDef = {
  key: string
  label: string
  type: 'text' | 'password' | 'number' | 'select'
  placeholder?: string
  default?: string
  required?: boolean
  options?: { label: string; value: string }[]
  /** Width on the form grid (defaults to 'full'): full · wide · half · compact. */
  span?: 'full' | 'wide' | 'half' | 'compact'
  /** Section heading; a divider renders whenever it differs from the previous field's. */
  section?: string
  /** Requests a platform-native file picker next to this field when available. */
  nativeFilePicker?: 'sqlite'
}

export type DriverDef = {
  id: string
  label: string
  defaultPort: number
  fields: FieldDef[]
  buildDSN: (values: Record<string, string>) => string
  parseDSN: (dsn: string) => Record<string, string>
}
