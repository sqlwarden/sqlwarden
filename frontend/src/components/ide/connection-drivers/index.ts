import { postgresDriver } from './postgres'
import { mysqlDriver } from './mysql'

export type FieldDef = {
  key: string
  label: string
  type: 'text' | 'password' | 'number' | 'select'
  placeholder?: string
  default?: string
  required?: boolean
  options?: { label: string; value: string }[]
}

export type DriverDef = {
  id: string
  label: string
  defaultPort: number
  fields: FieldDef[]
  buildDSN: (values: Record<string, string>) => string
}

export const drivers: DriverDef[] = [postgresDriver, mysqlDriver]
export const driverMap = new Map(drivers.map((d) => [d.id, d]))

export function defaultFieldValues(driver: DriverDef): Record<string, string> {
  const values: Record<string, string> = {}
  for (const field of driver.fields) {
    values[field.key] = field.default ?? ''
  }
  return values
}
