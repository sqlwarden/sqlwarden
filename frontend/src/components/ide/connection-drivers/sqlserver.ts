import type { DriverDef } from './types'

export const sqlserverDriver: DriverDef = {
  id: 'sqlserver',
  label: 'SQL Server',
  defaultPort: 1433,
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
      default: '1433',
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
      placeholder: 'sa',
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
  // Matches go-mssqldb/msdsn.Parse's accepted URL form:
  // sqlserver://username:password@host:port?database=dbname
  buildDSN: (values) => {
    const { host, port, database, username, password } = values
    const userPart = password
      ? `${encodeURIComponent(username)}:${encodeURIComponent(password)}`
      : encodeURIComponent(username)
    const query = database ? `?database=${encodeURIComponent(database)}` : ''
    return `sqlserver://${userPart}@${host}:${port}${query}`
  },
  parseDSN: (dsn): Record<string, string> => {
    try {
      const url = new URL(dsn)
      if (url.protocol !== 'sqlserver:') return {}
      return {
        host: url.hostname,
        port: url.port || '1433',
        database: url.searchParams.get('database') ?? '',
        username: decodeURIComponent(url.username),
        password: decodeURIComponent(url.password),
      }
    } catch {
      return {}
    }
  },
}
