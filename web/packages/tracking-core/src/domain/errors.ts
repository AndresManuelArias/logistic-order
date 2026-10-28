// Errores tipados del cliente HTTP. `status` 0 representa fallo de red.
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }

  get isNetwork(): boolean {
    return this.status === 0
  }

  get isUnauthorized(): boolean {
    return this.status === 401
  }

  get isForbidden(): boolean {
    return this.status === 403
  }

  get isNotFound(): boolean {
    return this.status === 404
  }
}

export function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  if (error instanceof DOMException && error.name === 'AbortError') {
    return new ApiError(0, 'ABORTED', 'Solicitud cancelada')
  }
  const message = error instanceof Error ? error.message : 'Error de red'
  return new ApiError(0, 'NETWORK', message)
}
