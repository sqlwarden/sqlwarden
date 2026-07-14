import type { DriverDef } from './types'

export const postgresDriver: DriverDef = {
  id: 'postgres',
  label: 'PostgreSQL',
  defaultPort: 5432,
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
      default: '5432',
      required: true,
      section: 'Server',
      span: 'compact',
    },
    {
      key: 'database',
      label: 'Database',
      type: 'text',
      placeholder: 'mydb',
      required: true,
      section: 'Server',
    },
    {
      key: 'username',
      label: 'Username',
      type: 'text',
      placeholder: 'postgres',
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
      key: 'sslmode',
      label: 'SSL Mode',
      type: 'select',
      default: 'prefer',
      section: 'Security',
      span: 'half',
      options: [
        { label: 'Disable', value: 'disable' },
        { label: 'Prefer', value: 'prefer' },
        { label: 'Require', value: 'require' },
      ],
    },
  ],
  buildDSN: (values) => {
    const { host, port, database, username, password, sslmode } = values
    const userPart = password
      ? `${encodeURIComponent(username)}:${encodeURIComponent(password)}`
      : encodeURIComponent(username)
    return `postgresql://${userPart}@${host}:${port}/${database}?sslmode=${sslmode || 'prefer'}`
  },
}
