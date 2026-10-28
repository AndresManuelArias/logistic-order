import { AppError } from '../../domain/errors'
import type { Logger } from '../../ports/clock'

export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>

export type HttpMethod = 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'

export interface HttpClientDeps {
  baseUrl: string
  getAccessToken: () => Promise<string | null>
  refreshAccessToken: () => Promise<string | null>
  fetcher?: FetchLike
  timeoutMs?: number
  logger?: Logger
}

export interface RequestOptions {
  method?: HttpMethod
  body?: unknown
  headers?: Record<string, string>
  retryOnUnauthorized?: boolean
  timeoutMs?: number
  /**
   * Token explícito para peticiones que ocurren **antes** de publicar la
   * sesión (p. ej. `GET /auth/me` justo después del login). Si se indica, no
   * se consulta ni se refresca la sesión persistida.
   */
  accessToken?: string
}

interface ErrorEnvelope {
  error?: {
    code?: string
    message?: string
  }
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError'
}

function parseRetryAfter(header: string | null, now: number): number | undefined {
  if (!header) return undefined
  const seconds = Number(header)
  if (Number.isFinite(seconds) && seconds >= 0) return Math.round(seconds * 1000)
  const date = Date.parse(header)
  return Number.isNaN(date) ? undefined : Math.max(0, date - now)
}

function defaultMessage(status: number): string {
  if (status === 401) return 'Sesión no válida o expirada'
  if (status === 403) return 'No tienes permisos para esta operación'
  if (status === 404) return 'Recurso no encontrado'
  if (status === 409) return 'La operación entra en conflicto con el estado actual'
  if (status === 429) return 'Demasiadas solicitudes; intenta más tarde'
  if (status >= 500) return 'El servidor no está disponible'
  return `Error del servidor (HTTP ${status})`
}

export function mapHttpError(status: number, payload: unknown, retryAfterMs?: number): AppError {
  const envelope = (payload ?? {}) as ErrorEnvelope
  const serverCode = envelope.error?.code
  const message = envelope.error?.message ?? defaultMessage(status)

  let code: AppError['code']
  if (status === 401) code = 'UNAUTHENTICATED'
  else if (status === 403) code = 'FORBIDDEN'
  else if (status === 404) code = 'NOT_FOUND'
  else if (status === 409) code = 'CONFLICT'
  else if (status === 400 || status === 413 || status === 422) code = 'VALIDATION'
  else if (status === 429) code = 'RATE_LIMITED'
  else if (status >= 500) code = 'SERVER'
  else code = 'UNKNOWN'

  return new AppError(code, message, { status, serverCode, retryAfterMs })
}

/**
 * Cliente HTTP mínimo sobre `fetch`: timeout, bearer, un único reintento tras
 * refrescar la sesión y traducción del sobre `{error:{code,message}}` del
 * backend a `AppError`.
 */
export class HttpClient {
  private readonly fetcher: FetchLike
  private readonly baseUrl: string
  private readonly timeoutMs: number

  constructor(private readonly deps: HttpClientDeps) {
    this.baseUrl = deps.baseUrl.replace(/\/+$/, '')
    this.fetcher = deps.fetcher ?? ((input, init) => fetch(input, init))
    this.timeoutMs = deps.timeoutMs ?? 20_000
  }

  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const hasOverride = typeof options.accessToken === 'string'
    const accessToken = hasOverride
      ? options.accessToken
      : await this.deps.getAccessToken()
    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...options.headers,
    }
    if (accessToken) headers.Authorization = `Bearer ${accessToken}`
    if (options.body !== undefined) headers['Content-Type'] = 'application/json'

    const controller = new AbortController()
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, options.timeoutMs ?? this.timeoutMs)

    let response: Response
    try {
      response = await this.fetcher(`${this.baseUrl}${path}`, {
        method: options.method ?? 'GET',
        headers,
        body: options.body === undefined ? undefined : JSON.stringify(options.body),
        signal: controller.signal,
      })
    } catch (error) {
      if (timedOut) {
        throw new AppError('TIMEOUT', 'La solicitud tardó demasiado; se reintentará', {
          cause: error,
        })
      }
      if (isAbortError(error)) {
        throw new AppError('ABORTED', 'Solicitud cancelada', { cause: error })
      }
      throw new AppError('NETWORK', 'Sin conexión con el servidor', { cause: error })
    } finally {
      clearTimeout(timer)
    }

    if (response.status === 401 && options.retryOnUnauthorized !== false && !hasOverride) {
      try {
        const refreshed = await this.deps.refreshAccessToken()
        if (refreshed) {
          return this.request<T>(path, { ...options, retryOnUnauthorized: false })
        }
      } catch (rawError) {
        this.deps.logger?.warn('No se pudo refrescar la sesión tras un 401', {
          message: rawError instanceof Error ? rawError.message : 'error desconocido',
        })
      }
    }

    const text = await response.text()
    let payload: unknown
    if (text.length > 0) {
      try {
        payload = JSON.parse(text)
      } catch {
        payload = undefined
      }
    }

    if (!response.ok) {
      throw mapHttpError(
        response.status,
        payload,
        parseRetryAfter(response.headers.get('retry-after'), Date.now()),
      )
    }

    return (payload ?? undefined) as T
  }
}
