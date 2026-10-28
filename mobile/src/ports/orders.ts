import type { Order, OrderPage, OrderStatus } from '../contracts/types'

export interface CachedOrder {
  order: Order
  /** Epoch ms de la última vez que el servidor confirmó estos datos. */
  cachedAt: number
}

export interface OrderCachePort {
  list(ownerId: string): Promise<CachedOrder[]>
  get(ownerId: string, orderId: string): Promise<CachedOrder | null>
  upsertMany(ownerId: string, orders: readonly Order[], at: number): Promise<void>
  upsert(ownerId: string, order: Order, at: number): Promise<void>
}

/**
 * Transporte de órdenes (REST). `changeOrderStatus` no admite cabecera de
 * idempotencia en el contrato: la ambigüedad se resuelve releyendo el estado
 * del servidor (ver `TransitionSyncService`).
 */
export interface OrderTransport {
  listOrders(params?: { limit?: number; offset?: number }): Promise<OrderPage>
  getOrder(orderId: string): Promise<Order>
  changeOrderStatus(orderId: string, status: OrderStatus): Promise<Order>
}
