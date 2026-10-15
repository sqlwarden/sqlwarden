import { http, HttpResponse } from 'msw'
import type { ConnectionFieldSpec, SessionResponse, SetupStatusResponse } from '#/lib/api/types'
import { connectionFieldSpecFixture, sessionFixture, setupStatusFixture } from './fixtures'

export function connectionFieldsHandler(
  fields: ConnectionFieldSpec[] = connectionFieldSpecFixture(),
) {
  return http.get('/api/v1/engines/:driver/connection-fields', () => HttpResponse.json({ fields }))
}

export function setupStatusHandler(payload: SetupStatusResponse = setupStatusFixture()) {
  return http.get('/api/setup/status', () => HttpResponse.json(payload))
}

export function sessionHandler(payload: SessionResponse = sessionFixture()) {
  return http.get('/api/v1/session', () => HttpResponse.json(payload))
}

export function apiErrorHandler(
  method: 'get' | 'post' | 'patch' | 'delete',
  path: string,
  status: number,
  message: string,
  code = 'test_error',
) {
  return http[method](path, () => HttpResponse.json({ error: { code, message } }, { status }))
}
