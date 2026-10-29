// Cliente HTTP tipado sobre el contrato OpenAPI. Centraliza serialización,
// errores y renovación de sesión (un único reintento tras un 401).
import { ApiError, toApiError } from '../domain/errors'
import type {
  BulkSyncResponse,
  GuestTokenResponse,
  Order,
  OrderPage,
  OrderStatus,
  PointInput,
  Schemas,
  Store,
  TelemetryPoint,
  TokenPair,
  TrackResponse,
  User,
  VehiclePage,
  CreateStoreInput,
  CreateOrderInput,
} from './types'

export interface ApiClientOptions {
  /** Base del proxy same-origin, p. ej. `/api/backend`. */
  baseUrl: string
  getToken: () => string | null
  /** Renueva la sesión y devuelve el nuevo access token (o null). */
  refresh?: () => Promise<string | null>
  fetchImpl?: typeof fetch
}

export interface ListOrdersParams {
  status?: OrderStatus
  limit?: number
  offset?: number
}

export interface TrackRange {
  from: string
  to: string
  limit?: number
}

// Respuesta del agente operativo (F5). `tools` permite mostrar al usuario qué
// herramienta se consultó y si falló; el portal no inventa respuestas.
export interface AiToolInvocation {
  name: string
  arguments?: Record<string, unknown>
  ok: boolean
  error?: string
  duration_ms: number
}

export interface AiAnswer {
  answer?: string
  tool?: string
  tools?: AiToolInvocation[]
  model?: string
  audit_id?: string
}

export interface ApiClient {
  login: (email: string, password: string) => Promise<TokenPair>
  me: () => Promise<User>
  listOrders: (params?: ListOrdersParams) => Promise<OrderPage>
  createOrder: (input: CreateOrderInput) => Promise<Order>
  getOrder: (id: string) => Promise<Order>
  changeOrderStatus: (id: string, status: OrderStatus) => Promise<Order>
  assignOrder: (id: string, input: { vehicle_id: string; driver_id: string }) => Promise<Order>
  getMyStore: () => Promise<Store>
  createStore: (input: CreateStoreInput) => Promise<Store>
  issueGuestToken: (orderId: string) => Promise<GuestTokenResponse>
  listVehicles: (params?: { limit?: number; offset?: number }) => Promise<VehiclePage>
  latest: (vehicleId: string) => Promise<Schemas['LatestPositionResponse']>
  track: (vehicleId: string, range: TrackRange) => Promise<TrackResponse>
  ingestPoint: (point: PointInput) => Promise<Schemas['IngestResponse']>
  syncBulk: (input: {
    deviceId: string
    points: PointInput[]
    idempotencyKey: string
  }) => Promise<BulkSyncResponse>
  queryAi: (question: string) => Promise<AiAnswer>
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
  headers?: Record<string, string>
  retryOnUnauthorized?: boolean
}

function withQuery(path: string, params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  const qs = search.toString()
  return qs ? `${path}?${qs}` : path
}

export function createApiClient(options: ApiClientOptions): ApiClient {
  const doFetch = options.fetchImpl ?? globalThis.fetch.bind(globalThis)
  let refreshInFlight: Promise<string | null> | null = null

  async function refreshOnce(): Promise<string | null> {
    if (!options.refresh) return null
    if (!refreshInFlight) {
      refreshInFlight = options.refresh().finally(() => {
        refreshInFlight = null
      })
    }
    return refreshInFlight
  }

  async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    const token = options.getToken()
    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...opts.headers,
    }
    if (token) headers.Authorization = `Bearer ${token}`
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json'

    let response: Response
    try {
      response = await doFetch(`${options.baseUrl}${path}`, {
        method: opts.method ?? 'GET',
        headers,
        body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
        signal: opts.signal,
        cache: 'no-store',
      })
    } catch (error) {
      throw toApiError(error)
    }

    if (response.status === 401 && opts.retryOnUnauthorized !== false) {
      const fresh = await refreshOnce()
      if (fresh) return request<T>(path, { ...opts, retryOnUnauthorized: false })
    }

    const text = await response.text()
    let payload: unknown
    if (text) {
      try {
        payload = JSON.parse(text)
      } catch {
        payload = undefined
      }
    }

    if (!response.ok) {
      const envelope = payload as { error?: { code?: string; message?: string } } | undefined
      throw new ApiError(
        response.status,
        envelope?.error?.code ?? `HTTP_${response.status}`,
        envelope?.error?.message ?? response.statusText ?? 'Error del servidor',
      )
    }

    return (payload ?? undefined) as T
  }

  return {
    login: (email, password) =>
      request<TokenPair>('/api/v1/auth/login', {
        method: 'POST',
        body: { email, password },
        retryOnUnauthorized: false,
      }),

    me: () => request<User>('/api/v1/auth/me'),

    listOrders: (params = {}) =>
      request<OrderPage>(
        withQuery('/api/v1/orders', {
          status: params.status,
          limit: params.limit,
          offset: params.offset,
        }),
      ),

    getOrder: (id) => request<Order>(`/api/v1/orders/${encodeURIComponent(id)}`),

    createOrder: (input) =>
      request<Order>('/api/v1/orders', { method: 'POST', body: input }),

    changeOrderStatus: (id, status) =>
      request<Order>(`/api/v1/orders/${encodeURIComponent(id)}/status`, {
        method: 'PATCH',
        body: { status },
      }),

    assignOrder: (id, input) =>
      request<Order>(`/api/v1/orders/${encodeURIComponent(id)}/assignment`, {
        method: 'POST',
        body: input,
      }),

    getMyStore: () => request<Store>('/api/v1/stores/me'),

    createStore: (input) =>
      request<Store>('/api/v1/stores', { method: 'POST', body: input }),

    issueGuestToken: (orderId) =>
      request<GuestTokenResponse>('/api/v1/auth/guest-token', {
        method: 'POST',
        body: { order_id: orderId },
      }),

    listVehicles: (params = {}) =>
      request<VehiclePage>(withQuery('/api/v1/vehicles', { limit: params.limit, offset: params.offset })),

    latest: (vehicleId) =>
      request<Schemas['LatestPositionResponse']>(
        `/api/v1/vehicles/${encodeURIComponent(vehicleId)}/latest`,
      ),

    track: (vehicleId, range) =>
      request<TrackResponse>(
        withQuery(`/api/v1/vehicles/${encodeURIComponent(vehicleId)}/track`, {
          from: range.from,
          to: range.to,
          limit: range.limit,
        }),
      ),

    ingestPoint: (point) =>
      request<Schemas['IngestResponse']>('/api/v1/telemetry/points', {
        method: 'POST',
        body: point,
      }),

    syncBulk: ({ deviceId, points, idempotencyKey }) =>
      request<BulkSyncResponse>('/api/v1/telemetry/bulk', {
        method: 'POST',
        body: { device_id: deviceId, points },
        headers: { 'Idempotency-Key': idempotencyKey },
      }),

    queryAi: (question) =>
      request<AiAnswer>('/api/v1/ai/query', { method: 'POST', body: { question } }),
  }
}

// Re-exporta el tipo de punto para consumidores del cliente sin obligarlos a
// importar del módulo generado.
export type { TelemetryPoint }
