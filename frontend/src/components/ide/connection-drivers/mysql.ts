import type { DriverDef } from './types'

export const mysqlDriver: DriverDef = {
  id: 'mysql',
  label: 'MySQL',
  defaultPort: 3306,
  fields: [
    {
      key: 'host',
      label: 'Host',
      type: 'text',
      placeholder: 'localhost',
      required: true,
      section: 'Server',
      span: 'wide',
    },
    {
      key: 'port',
      label: 'Port',
      type: 'number',
      default: '3306',
      required: true,
      section: 'Server',
      span: 'compact',
    },
    {
      key: 'database',
      label: 'Database (optional)',
      type: 'text',
      placeholder: 'Optional',
      section: 'Server',
    },
    {
      key: 'username',
      label: 'Username',
      type: 'text',
      placeholder: 'root',
      required: true,
      section: 'Credentials',
      span: 'half',
    },
    {
      key: 'password',
      label: 'Password',
      type: 'password',
      section: 'Credentials',
      span: 'half',
    },
  ],
  buildDSN: (values) => {
    const { host, port, database, username, password } = values
    const userPart = password ? `${username}:${password}` : username
    return `${userPart}@tcp(${host}:${port})/${database}`
  },
  parseDSN: (dsn): Record<string, string> => {
    const match = /^(?:([^:@]*)(?::(.*))?@)?tcp\(([^:]+):(\d+)\)\/(.*)$/.exec(dsn)
    if (!match) return {}
    const [, username, password, host, port, database] = match
    return {
      host,
      port,
      database,
      username: username ?? '',
      password: password ?? '',
    }
  },
}
