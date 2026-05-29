import { postgresDriver } from './postgres'
import { mysqlDriver } from './mysql'
import postgresIcon from '#/assets/drivers/postgresql.svg'
import mysqlIcon from '#/assets/drivers/mysql.svg'

export const driverBrands: Record<string, { icon: string; description: string }> = {
  postgres: { icon: postgresIcon, description: 'Open-source relational database' },
  mysql: { icon: mysqlIcon, description: 'MySQL / MariaDB database' },
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
