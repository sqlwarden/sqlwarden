export interface ExportFormat {
  value: string
  label: string
  enabled: boolean
}

export const EXPORT_FORMATS: ExportFormat[] = [
  { value: 'csv', label: 'CSV', enabled: true },
]

export function selectableFormats(formats: ExportFormat[] = EXPORT_FORMATS): ExportFormat[] {
  return formats.filter((f) => f.enabled)
}
