import { ApiError } from '#/lib/api/errors'
import { getAccessToken, clearAccessToken } from '#/lib/auth/access-token'
import { notifyAuthInvalidated } from '#/lib/auth/invalidation'
import type { ListQuery } from '#/lib/api/types'

export interface ApiClientOptions extends Omit<RequestInit, 'body'> {
  query?: ListQuery
  body?: unknown
  skipAuth?: boolean
}

function buildHeaders(options: ApiClientOptions) {
  const headers = new Headers(options.headers ?? {})
  if (!headers.has('Accept')) {
    headers.set('Accept', 'application/json')
  }
  if (options.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (!options.skipAuth) {
    const token = getAccessToken()
    if (token && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${token}`)
    }
  }
  return headers
}

export function buildSearchParams(query?: ListQuery) {
  const params = new URLSearchParams()
  if (!query) {
    return params
  }

  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === '') {
      continue
    }
    params.set(key, String(value))
  }

  return params
}

async function parseJson(response: Response) {
  const contentType = response.headers.get('content-type') ?? ''
  if (!contentType.includes('application/json')) {
    return undefined
  }
  return response.json()
}

function firstValidationMessage(payload: unknown) {
  if (typeof payload !== 'object' || payload === null) {
    return undefined
  }

  if ('field_errors' in payload && typeof payload.field_errors === 'object' && payload.field_errors !== null) {
    const [message] = Object.values(payload.field_errors as Record<string, unknown>)
    if (typeof message === 'string' && message.trim() !== '') {
      return message
    }
  }

  if ('errors' in payload && Array.isArray(payload.errors)) {
    const [message] = payload.errors
    if (typeof message === 'string' && message.trim() !== '') {
      return message
    }
  }

  return undefined
}

export async function apiRequest<T>(path: string, options: ApiClientOptions = {}): Promise<T> {
  const url = new URL(path, window.location.origin)
  const params = buildSearchParams(options.query)
  if ([...params.keys()].length > 0) {
    url.search = params.toString()
  }

  const response = await fetch(url.toString(), {
    ...options,
    headers: buildHeaders(options),
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  })

  if (response.status === 204) {
    return undefined as T
  }

  const payload = await parseJson(response)
  if (!response.ok) {
    if (response.status === 401 && !options.skipAuth) {
      clearAccessToken()
      notifyAuthInvalidated()
    }

    const fieldErrors = typeof payload === 'object' && payload !== null && 'field_errors' in payload
      ? (payload.field_errors as Record<string, string>)
      : undefined
    const message =
      ((typeof payload === 'object' && payload !== null && 'error' in payload && typeof payload.error === 'string'
        ? payload.error
        : undefined) ??
        firstValidationMessage(payload) ??
        response.statusText) ||
      'Request failed'

    throw new ApiError(message, response.status, {
      details: payload,
      fieldErrors,
    })
  }

  return payload as T
}

export const api = {
  get: <T>(path: string, options?: Omit<ApiClientOptions, 'method' | 'body'>) =>
    apiRequest<T>(path, { ...options, method: 'GET' }),
  post: <T>(path: string, body?: unknown, options?: Omit<ApiClientOptions, 'method' | 'body'>) =>
    apiRequest<T>(path, { ...options, method: 'POST', body }),
  patch: <T>(path: string, body?: unknown, options?: Omit<ApiClientOptions, 'method' | 'body'>) =>
    apiRequest<T>(path, { ...options, method: 'PATCH', body }),
  delete: <T>(path: string, options?: Omit<ApiClientOptions, 'method' | 'body'>) =>
    apiRequest<T>(path, { ...options, method: 'DELETE' }),
}
