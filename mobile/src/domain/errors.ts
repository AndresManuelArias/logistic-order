// Errores tipados de la app. `status` 0 representa un fallo sin respuesta HTTP
// (red, timeout o error inesperado del dispositivo).

export type ErrorCode =
  | 'NETWORK'
  | 'TIMEOUT'
  | 'ABORTED'
  | 'UNAUTHENTICATED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'CONFLICT'
  | 'VALIDATION'
  | 'RATE_LIMITED'
  | 'SERVER'
  | 'UNKNOWN'

export interface AppErrorOptions {
  status?: number
  serverCode?: string
  retryAfterMs?: number
  cause?: unknown
}

export class AppError extends Error {
  readonly code: ErrorCode
  readonly status: number
  readonly serverCode?: string
  readonly retryAfterMs?: number

  constructor(code: ErrorCode, message: string, options: AppErrorOptions = {}) {
    super(message, options.cause === undefined ? undefined : { cause: options.cause })
    this.name = 'AppError'
    this.code = code
    this.status = options.status ?? 0
    this.serverCode = options.serverCode
    this.retryAfterMs = options.retryAfterMs
  }

  /** Sólo los fallos transitorios justifican reintentar el mismo lote. */
  get retryable(): boolean {
    return (
      this.code === 'NETWORK' ||
      this.code === 'TIMEOUT' ||
      this.code === 'SERVER' ||
      this.code === 'RATE_LIMITED'
    )
  }
}

export function isAppError(error: unknown): error is AppError {
  return error instanceof AppError
}

export function toAppError(error: unknown): AppError {
  if (error instanceof AppError) return error
  if (typeof DOMException !== 'undefined' && error instanceof DOMException) {
    if (error.name === 'AbortError') {
      return new AppError('ABORTED', 'Solicitud cancelada', { cause: error })
    }
  }
  if (error instanceof Error) {
    return new AppError('UNKNOWN', error.message, { cause: error })
  }
  return new AppError('UNKNOWN', 'Error inesperado', { cause: error })
}
