import type { DriverDef } from './index'

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
    },
    {
      key: 'port',
      label: 'Port',
      type: 'number',
      default: '3306',
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
      placeholder: 'root',
      required: true,
    },
    {
      key: 'password',
      label: 'Password',
      type: 'password',
    },
  ],
  buildDSN: (values) => {
    const { host, port, database, username, password } = values
    const userPart = password ? `${username}:${password}` : username
    return `${userPart}@tcp(${host}:${port})/${database}`
  },
}
