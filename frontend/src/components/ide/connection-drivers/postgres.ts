import type { DriverDef } from './index'

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
    },
    {
      key: 'port',
      label: 'Port',
      type: 'number',
      default: '5432',
      required: true,
    },
    {
      key: 'database',
      label: 'Database',
      type: 'text',
      placeholder: 'mydb',
      required: true,
    },
    {
      key: 'username',
      label: 'Username',
      type: 'text',
      placeholder: 'postgres',
      required: true,
    },
    {
      key: 'password',
      label: 'Password',
      type: 'password',
    },
    {
      key: 'sslmode',
      label: 'SSL Mode',
      type: 'select',
      default: 'prefer',
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
