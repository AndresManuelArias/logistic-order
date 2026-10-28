import type {
  BulkSyncResponse,
  Order,
  OrderPage,
  OrderStatus,
  TokenPair,
  User,
} from '../../contracts/types'
import type { BulkSendInput, BulkSendResult } from '../../ports/telemetry'
import type { HttpClient } from './http'

export interface TrackingApi {
  login(email: string, password: string): Promise<TokenPair>
  refresh(refreshToken: string): Promise<TokenPair>
  logout(refreshToken: string): Promise<void>
  me(accessToken?: string): Promise<User>
  listOrders(params?: { limit?: number; offset?: number }): Promise<OrderPage>
  getOrder(orderId: string): Promise<Order>
  changeOrderStatus(orderId: string, status: OrderStatus): Promise<Order>
  sendBulk(input: BulkSendInput): Promise<BulkSendResult>
}

function withQuery(path: string, params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  const qs = search.toString()
  return qs ? `${path}?${qs}` : path
}

/** Endpoints F2–F4 usados por la app del conductor. */
export function createTrackingApi(http: HttpClient): TrackingApi {
  return {
    login: (email, password) =>
      http.request<TokenPair>('/api/v1/auth/login', {
        method: 'POST',
        body: { email, password },
        retryOnUnauthorized: false,
      }),

    refresh: (refreshToken) =>
      http.request<TokenPair>('/api/v1/auth/refresh', {
        method: 'POST',
        body: { refresh_token: refreshToken },
        retryOnUnauthorized: false,
      }),

    logout: (refreshToken) =>
      http.request<void>('/api/v1/auth/logout', {
        method: 'POST',
        body: { refresh_token: refreshToken },
        retryOnUnauthorized: false,
      }),

    me: (accessToken) =>
      http.request<User>('/api/v1/auth/me', {
        ...(accessToken ? { accessToken } : {}),
        retryOnUnauthorized: false,
      }),

    listOrders: (params = {}) =>
      http.request<OrderPage>(
        withQuery('/api/v1/orders', { limit: params.limit, offset: params.offset }),
      ),

    getOrder: (orderId) =>
      http.request<Order>(`/api/v1/orders/${encodeURIComponent(orderId)}`),

    changeOrderStatus: (orderId, status) =>
      http.request<Order>(`/api/v1/orders/${encodeURIComponent(orderId)}/status`, {
        method: 'PATCH',
        body: { status },
      }),

    sendBulk: async ({ deviceId, idempotencyKey, points }) => {
      const response = await http.request<BulkSyncResponse>('/api/v1/telemetry/bulk', {
        method: 'POST',
        body: { device_id: deviceId, points },
        headers: { 'Idempotency-Key': idempotencyKey },
      })
      return {
        accepted: response.accepted,
        duplicates: response.duplicates,
        alreadyProcessed: response.already_processed,
        processedAt: response.processed_at,
      }
    },
  }
}
