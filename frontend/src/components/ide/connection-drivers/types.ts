/** Presentation of one connection field; its type, default, and requirement come from the engine's field spec. */
export type FieldLayout = {
  key: string
  label: string
  placeholder?: string
  /** Width on the form grid (defaults to 'full'): full · wide · half · compact. */
  span?: 'full' | 'wide' | 'half' | 'compact'
  /** Section heading; a divider renders whenever it differs from the previous field's. */
  section?: string
}

export type DriverDef = {
  id: string
  label: string
  fields: FieldLayout[]
}
