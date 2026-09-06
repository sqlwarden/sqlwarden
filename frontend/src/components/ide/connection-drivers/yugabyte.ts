import type { DriverDef } from './types'

export const yugabyteDriver: DriverDef = {
  id: 'yugabyte',
  label: 'YugabyteDB',
  defaultPort: 5433,
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
      default: '5433',
      required: true,
      section: 'Server',
      span: 'compact',
    },
    {
      key: 'database',
      label: 'Database (optional)',
      type: 'text',
      placeholder: 'yugabyte',
      section: 'Server',
    },
    {
      key: 'username',
      label: 'Username',
      type: 'text',
      placeholder: 'yugabyte',
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
    const userPart = password
      ? `${encodeURIComponent(username)}:${encodeURIComponent(password)}`
      : encodeURIComponent(username)
    return `postgresql://${userPart}@${host}:${port}/${database}`
  },
  parseDSN: (dsn): Record<string, string> => {
    try {
      const url = new URL(dsn)
      return {
        host: url.hostname,
        port: url.port || '5433',
        database: decodeURIComponent(url.pathname.replace(/^\//, '')),
        username: decodeURIComponent(url.username),
        password: decodeURIComponent(url.password),
      }
    } catch {
      return {}
    }
  },
}
