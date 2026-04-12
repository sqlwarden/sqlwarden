export interface ApiFieldErrors {
  [field: string]: string
}

export class ApiError extends Error {
  status: number
  code?: string
  details?: unknown
  fieldErrors?: ApiFieldErrors

  constructor(message: string, status: number, options?: { code?: string; details?: unknown; fieldErrors?: ApiFieldErrors }) {
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
