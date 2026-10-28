import { Q } from '@nozbe/watermelondb'
import type Database from '@nozbe/watermelondb/Database'

import type { Order } from '../../../contracts/types'
import type { CachedOrder, OrderCachePort } from '../../../ports/orders'
import { OrderModel } from './models'

function toEpoch(value: string | undefined | null): number {
  if (!value) return 0
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}

function parseOrder(payload: string): Order | null {
  try {
    return JSON.parse(payload) as Order
  } catch {
    return null
  }
}

/**
 * Caché de órdenes. Guarda el `Order` completo del contrato (JSON) y duplica
 * `status`/`placed_at` como columnas indexadas para filtrar sin deserializar.
 */
export class WatermelonOrderCache implements OrderCachePort {
  private readonly orders

  constructor(private readonly database: Database) {
    this.orders = database.get<OrderModel>('orders')
  }

  async list(ownerId: string): Promise<CachedOrder[]> {
    const rows = await this.orders.query(Q.where('owner_id', ownerId)).fetch()
    const cached: CachedOrder[] = []
    for (const row of rows) {
      const order = parseOrder(row.payload)
      if (order) cached.push({ order, cachedAt: row.cachedAt })
    }
    return cached
  }

  async get(ownerId: string, orderId: string): Promise<CachedOrder | null> {
    const rows = await this.orders
      .query(Q.where('owner_id', ownerId), Q.where('id', orderId), Q.take(1))
      .fetch()
    const row = rows[0]
    if (!row) return null
    const order = parseOrder(row.payload)
    return order ? { order, cachedAt: row.cachedAt } : null
  }

  async upsertMany(ownerId: string, orders: readonly Order[], at: number): Promise<void> {
    if (orders.length === 0) return
    await this.database.write(async () => {
      const existing = await this.orders
        .query(Q.where('id', Q.oneOf(orders.map((order) => order.id))))
        .fetch()
      const byId = new Map(existing.map((row) => [row.id, row]))

      const prepared = orders.map((order) => {
        const payload = JSON.stringify(order)
        const placedAt = toEpoch(order.placed_at)
        const row = byId.get(order.id)
        if (row) {
          return row.prepareUpdate((record) => {
            record.payload = payload
            record.status = order.status
            record.placedAt = placedAt
            record.cachedAt = at
            // Si la orden se reasignó a otro conductor en este mismo
            // dispositivo, la fila completa cambia de dueño.
            record.ownerId = ownerId
          })
        }
        return this.orders.prepareCreate((record) => {
          record._raw.id = order.id
          record.ownerId = ownerId
          record.payload = payload
          record.status = order.status
          record.placedAt = placedAt
          record.cachedAt = at
        })
      })

      await this.database.batch(prepared)
    })
  }

  async upsert(ownerId: string, order: Order, at: number): Promise<void> {
    await this.upsertMany(ownerId, [order], at)
  }
}
