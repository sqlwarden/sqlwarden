import { postgresDriver } from './postgres'
import { mysqlDriver } from './mysql'

export const driverBrands: Record<string, { color: string; abbr: string; description: string }> = {
  postgres: { color: '#336791', abbr: 'PG', description: 'Open-source relational database' },
  mysql: { color: '#E97B00', abbr: 'MY', description: 'MySQL / MariaDB database' },
}

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
