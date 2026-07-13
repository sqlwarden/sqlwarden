import { http, HttpResponse } from 'msw'
import type { SessionResponse, SetupStatusResponse } from '#/lib/api/types'
import { sessionFixture, setupStatusFixture } from './fixtures'

export function setupStatusHandler(payload: SetupStatusResponse = setupStatusFixture()) {
  return http.get('/api/setup/status', () => HttpResponse.json(payload))
}

export function sessionHandler(payload: SessionResponse = sessionFixture()) {
  return http.get('/api/v1/session', () => HttpResponse.json(payload))
}

export function apiErrorHandler(method: 'get' | 'post' | 'patch' | 'delete', path: string, status: number, message: string) {
  return http[method](path, () =>
    HttpResponse.json({ error: { code: 'test_error', message } }, { status }),
  )
}
