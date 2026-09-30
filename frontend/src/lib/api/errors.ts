export interface ApiFieldErrors {
  [field: string]: string
}

export class ApiError extends Error {
  status: number
  code?: string
  details?: unknown
  fieldErrors?: ApiFieldErrors

  constructor(
    message: string,
    status: number,
    options?: { code?: string; details?: unknown; fieldErrors?: ApiFieldErrors },
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = options?.code
    this.details = options?.details
    this.fieldErrors = options?.fieldErrors
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError
}

export function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message.trim() !== '' ? error.message : fallback
}

/** True when the backend refused a schema read because the connection has no live session. */
export function isSessionRequired(error: unknown) {
  return isApiError(error) && error.status === 409 && error.code === 'session_required'
}
