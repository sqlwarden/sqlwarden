import type { DriverDef } from './types'

export const oracleDriver: DriverDef = {
  id: 'oracle',
  label: 'Oracle',
  defaultPort: 1521,
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
      default: '1521',
      required: true,
      section: 'Server',
      span: 'compact',
    },
    {
      key: 'serviceName',
      label: 'Service name',
      type: 'text',
      placeholder: 'ORCLPDB1',
      required: true,
      section: 'Server',
    },
    {
      key: 'username',
      label: 'Username',
      type: 'text',
      placeholder: 'system',
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
    {
      key: 'ssl',
      label: 'SSL',
      type: 'select',
      default: 'disable',
      section: 'Security',
      span: 'half',
      options: [
        { label: 'Disable', value: 'disable' },
        { label: 'Require', value: 'require' },
      ],
    },
  ],
  buildDSN: (values) => {
    const { host, port, serviceName, username, password, ssl } = values
    const userPart = password
      ? `${encodeURIComponent(username)}:${encodeURIComponent(password)}`
      : encodeURIComponent(username)
    const query = ssl === 'require' ? '?SSL=true' : ''
    return `oracle://${userPart}@${host}:${port || '1521'}/${serviceName}${query}`
  },
  parseDSN: (dsn): Record<string, string> => {
    try {
      const url = new URL(dsn)
      return {
        host: url.hostname,
        port: url.port || '1521',
        serviceName: decodeURIComponent(url.pathname.replace(/^\//, '')),
        username: decodeURIComponent(url.username),
        password: decodeURIComponent(url.password),
        ssl: url.searchParams.get('SSL') === 'true' ? 'require' : 'disable',
      }
    } catch {
      return {}
    }
  },
}
