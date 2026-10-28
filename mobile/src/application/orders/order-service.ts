import type { Order, OrderStatus } from '../../contracts/types'
import { AppError, toAppError } from '../../domain/errors'
import { canTransition, effectiveStatus, nextDriverAction } from '../../domain/order'
import type { PendingTransition } from '../../domain/sync'
import type { Clock, Logger } from '../../ports/clock'
import type { ConnectivityPort } from '../../ports/connectivity'
import type { CachedOrder, OrderCachePort, OrderTransport } from '../../ports/orders'
import type { EnqueueTransitionResult, TransitionQueuePort } from '../../ports/transitions'
import { transitionIdempotencyKey, newLocalId } from '../../shared/ids'

export interface OrderView {
  order: Order
  effectiveStatus: OrderStatus
  pendingTransition: PendingTransition | null
  cachedAt: number
}

export interface OrderListResult {
  views: OrderView[]
  offline: boolean
  error?: AppError
}

export interface OrderServiceDeps {
  cache: OrderCachePort
  transitions: TransitionQueuePort
  transport: OrderTransport
  connectivity: ConnectivityPort
  clock: Clock
  logger: Logger
}

/** Órdenes del conductor combinando caché local, servidor y cola offline. */
export class OrderService {
  constructor(private readonly deps: OrderServiceDeps) {}

  async listCached(ownerId: string): Promise<OrderView[]> {
    const [cached, pending] = await Promise.all([
      this.deps.cache.list(ownerId),
      this.deps.transitions.listPending(ownerId),
    ])
    return this.buildViews(cached, pending)
  }

  /**
   * Cache-first: siempre devuelve lo local y, si hay red, intenta actualizar.
   * Un fallo remoto no vacía la lista ni pierde datos.
   */
  async load(ownerId: string): Promise<OrderListResult> {
    const localViews = await this.listCached(ownerId)
    const online = await this.deps.connectivity.isOnline().catch(() => false)
    if (!online) return { views: localViews, offline: true }

    try {
      const page = await this.deps.transport.listOrders({ limit: 100, offset: 0 })
      await this.deps.cache.upsertMany(ownerId, page.items, this.deps.clock.now())
      return { views: await this.listCached(ownerId), offline: false }
    } catch (rawError) {
      const error = toAppError(rawError)
      this.deps.logger.warn('No se pudieron refrescar las órdenes', {
        code: error.code,
        message: error.message,
      })
      return { views: localViews, offline: error.code === 'NETWORK' || error.code === 'TIMEOUT', error }
    }
  }

  async get(ownerId: string, orderId: string): Promise<OrderView | null> {
    const [cached, pending] = await Promise.all([
      this.deps.cache.get(ownerId, orderId),
      this.deps.transitions.listPending(ownerId),
    ])
    if (!cached) return null
    return this.buildViews([cached], pending)[0] ?? null
  }

  /** Refresca una orden concreta; devuelve la copia local si no hay red. */
  async refreshOne(ownerId: string, orderId: string): Promise<{ view: OrderView | null; error?: AppError }> {
    const online = await this.deps.connectivity.isOnline().catch(() => false)
    if (online) {
      try {
        const order = await this.deps.transport.getOrder(orderId)
        await this.deps.cache.upsert(ownerId, order, this.deps.clock.now())
      } catch (rawError) {
        const error = toAppError(rawError)
        if (error.code !== 'NOT_FOUND') {
          this.deps.logger.warn('No se pudo refrescar la orden', {
            orderId,
            code: error.code,
            message: error.message,
          })
        }
        const cached = await this.get(ownerId, orderId)
        return { view: cached, error }
      }
    }
    return { view: await this.get(ownerId, orderId) }
  }

  async requestTransition(
    ownerId: string,
    orderId: string,
    targetStatus: OrderStatus,
  ): Promise<EnqueueTransitionResult | { transition: null; deduped: true }> {
    const cached = await this.deps.cache.get(ownerId, orderId)
    if (!cached) {
      throw new AppError(
        'VALIDATION',
        'La orden no está disponible en este dispositivo. Conéctate y vuelve a abrirla.',
      )
    }

    const pending = await this.deps.transitions.findPending(ownerId, orderId)
    if (pending && pending.targetStatus === targetStatus) {
      return { transition: pending, deduped: true }
    }

    const current = effectiveStatus(cached.order, pending?.targetStatus)
    if (current === targetStatus) {
      return { transition: null, deduped: true }
    }
    if (!canTransition(current, targetStatus)) {
      throw new AppError(
        'CONFLICT',
        `No se puede pasar de ${current} a ${targetStatus}. Actualiza la orden.`,
      )
    }

    const transitionId = newLocalId('tr_')
    return this.deps.transitions.enqueue(
      ownerId,
      {
        id: transitionId,
        orderId,
        expectedStatus: current,
        targetStatus,
        idempotencyKey: transitionIdempotencyKey(transitionId, orderId, targetStatus),
      },
      this.deps.clock.now(),
    )
  }

  /** Próxima acción ofrecida al conductor para el estado efectivo. */
  nextAction(view: OrderView) {
    return nextDriverAction(view.effectiveStatus)
  }

  private buildViews(cached: readonly CachedOrder[], pending: readonly PendingTransition[]): OrderView[] {
    const byOrder = new Map(pending.map((transition) => [transition.orderId, transition]))
    return cached.map(({ order, cachedAt }) => {
      const transition = byOrder.get(order.id) ?? null
      return {
        order,
        cachedAt,
        pendingTransition: transition,
        effectiveStatus: effectiveStatus(order, transition?.targetStatus),
      }
    })
  }
}
