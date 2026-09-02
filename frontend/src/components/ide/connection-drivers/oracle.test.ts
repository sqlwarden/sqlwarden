import { describe, expect, it } from 'vitest'
import { oracleDriver } from './oracle'

describe('oracle connection driver', () => {
  it('builds a service-name DSN', () => {
    expect(
      oracleDriver.buildDSN({
        host: 'db.example.com',
        port: '1521',
        serviceName: 'ORCLPDB1',
        username: 'hr',
        password: 's3cr3t',
      }),
    ).toBe('oracle://hr:s3cr3t@db.example.com:1521/ORCLPDB1')
  })

  it('does not emit an SSL query parameter', () => {
    expect(
      oracleDriver.buildDSN({
        host: 'h',
        port: '1521',
        serviceName: 'S',
        username: 'u',
        password: '',
      }),
    ).toBe('oracle://u@h:1521/S')
  })

  it('round-trips through parseDSN', () => {
    const values = {
      host: 'h',
      port: '1600',
      serviceName: 'PDB',
      username: 'u',
      password: 'p',
    }
    const parsed = oracleDriver.parseDSN(oracleDriver.buildDSN(values))
    expect(parsed).toEqual(values)
  })

  it('defaults the port when the DSN omits it', () => {
    expect(oracleDriver.parseDSN('oracle://u:p@h/PDB').port).toBe('1521')
  })
})
