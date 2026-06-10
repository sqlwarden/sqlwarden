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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function fieldErrorsFrom(value: unknown) {
  if (!isRecord(value) || !isRecord(value.field_errors)) {
    return undefined
  }
  return value.field_errors as Record<string, string>
}

function firstValidationMessage(payload: unknown) {
  if (typeof payload !== 'object' || payload === null) {
    return undefined
  }

  const fieldErrors = fieldErrorsFrom(payload)
  if (fieldErrors) {
    const [message] = Object.values(fieldErrors)
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

export function parseAPIErrorPayload(payload: unknown, fallback: string) {
  const legacyFieldErrors = fieldErrorsFrom(payload)

  if (isRecord(payload) && isRecord(payload.error)) {
    const error = payload.error
    const fieldErrors = fieldErrorsFrom(error)
    const message =
      firstValidationMessage(error) ??
      (typeof error.message === 'string' && error.message.trim() !== '' ? error.message : undefined) ??
      fallback

    return {
      code: typeof error.code === 'string' ? error.code : undefined,
      details: 'details' in error ? error.details : payload,
      fieldErrors,
      message,
    }
  }

  const message =
    ((isRecord(payload) && typeof payload.error === 'string' ? payload.error : undefined) ??
      firstValidationMessage(payload) ??
      fallback) ||
    'Request failed'

  return {
    code: undefined,
    details: payload,
    fieldErrors: legacyFieldErrors,
    message,
  }
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

    const parsedError = parseAPIErrorPayload(payload, response.statusText || 'Request failed')

    throw new ApiError(parsedError.message, response.status, {
      code: parsedError.code,
      details: parsedError.details,
      fieldErrors: parsedError.fieldErrors,
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
