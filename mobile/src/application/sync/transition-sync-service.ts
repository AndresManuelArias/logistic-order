import { SYNC_POLICY, type SyncPolicy } from '../../config/policies'
import type { Order } from '../../contracts/types'
import { AppError, toAppError, type ErrorCode } from '../../domain/errors'
import { canTransition } from '../../domain/order'
import type { PendingTransition, SyncTrigger } from '../../domain/sync'
import type { Clock, Logger } from '../../ports/clock'
import type { OrderCachePort, OrderTransport } from '../../ports/orders'
import type { TransitionQueuePort } from '../../ports/transitions'
import { computeBackoffDelay } from './backoff'

export interface TransitionSyncDeps {
  queue: TransitionQueuePort
  transport: OrderTransport
  cache: OrderCachePort
  clock: Clock
  logger: Logger
  policy?: SyncPolicy
  random?: () => number
}

export interface TransitionSyncContext {
  ownerId: string
  trigger: SyncTrigger
}

export interface TransitionSyncResult {
  outcome: 'idle' | 'synced' | 'retry' | 'paused'
  applied: number
  rejected: number
  nextRetryAt?: number
  error?: string
  errorCode?: ErrorCode
}

/**
 * Aplica transiciones de orden encoladas offline.
 *
 * El endpoint `PATCH /orders/{id}/status` no acepta `Idempotency-Key`, así que
 * la idempotencia se resuelve comparando el estado del servidor:
 *
 * - Si el PATCH falla de forma ambigua (timeout/5xx), en el siguiente intento
 *   se hace `GET` antes de reenviar.
 * - Un `409` se reconcilia releyendo: si el servidor ya está en el estado
 *   objetivo la operación se da por aplicada; si no, se marca rechazada.
 *
 * Las transiciones se procesan en orden de encolado y de una en una para no
 * adelantar un cambio sobre otro.
 */
export class TransitionSyncService {
  private readonly policy: SyncPolicy

  constructor(private readonly deps: TransitionSyncDeps) {
    this.policy = deps.policy ?? SYNC_POLICY
  }

  async run(context: TransitionSyncContext): Promise<TransitionSyncResult> {
    const { ownerId, trigger } = context
    const result: TransitionSyncResult = { outcome: 'idle', applied: 0, rejected: 0 }

    for (let iteration = 0; iteration < this.policy.maxTransitionsPerRun; iteration += 1) {
      const pending = await this.deps.queue.next(ownerId, 1)
      const transition = pending[0]
      if (!transition) break

      const now = this.deps.clock.now()
      // `nextAttemptAt` sólo limita reintentos: una transición recién encolada
      // se envía de inmediato aunque su marca de tiempo sea milimétricamente
      // posterior al reloj del coordinador.
      if (transition.attempts > 0 && transition.nextAttemptAt > now) {
        result.outcome = 'retry'
        result.nextRetryAt = transition.nextAttemptAt
        result.error = transition.lastError ?? undefined
        return result
      }

      const outcome = await this.processTransition(ownerId, transition, trigger)
      if (outcome.kind === 'applied') {
        result.applied += 1
        result.outcome = 'synced'
        continue
      }
      if (outcome.kind === 'rejected') {
        result.rejected += 1
        result.outcome = 'synced'
        continue
      }

      result.outcome = outcome.kind === 'paused' ? 'paused' : 'retry'
      result.error = outcome.error.message
      result.errorCode = outcome.error.code
      if (outcome.kind === 'retry') result.nextRetryAt = outcome.nextRetryAt
      return result
    }

    return result
  }

  private async processTransition(
    ownerId: string,
    transition: PendingTransition,
    trigger: SyncTrigger,
  ): Promise<
    | { kind: 'applied' }
    | { kind: 'rejected' }
    | { kind: 'paused'; error: AppError }
    | { kind: 'retry'; error: AppError; nextRetryAt: number }
  > {
    // Tras un intento ambiguo, leer el estado real evita reenviar el PATCH.
    if (transition.attempts > 0) {
      let serverOrder: Order
      try {
        serverOrder = await this.deps.transport.getOrder(transition.orderId)
      } catch (rawError) {
        return this.failure(ownerId, transition, rawError)
      }

      await this.deps.cache.upsert(ownerId, serverOrder, this.deps.clock.now())
      if (serverOrder.status === transition.targetStatus) {
        await this.deps.queue.markApplied(ownerId, transition.id, this.deps.clock.now())
        this.deps.logger.info('Transición ya aplicada en el servidor; reconciliada', {
          orderId: transition.orderId,
          target: transition.targetStatus,
          trigger,
        })
        return { kind: 'applied' }
      }
      if (
        !canTransition(serverOrder.status, transition.targetStatus) ||
        serverOrder.status !== transition.expectedStatus
      ) {
        await this.deps.queue.markRejected(
          ownerId,
          transition.id,
          'STATE_CHANGED_OUTSIDE',
          this.deps.clock.now(),
        )
        this.deps.logger.warn('Transición descartada: el estado cambió fuera de la app', {
          orderId: transition.orderId,
          serverStatus: serverOrder.status,
          expected: transition.expectedStatus,
          target: transition.targetStatus,
        })
        return { kind: 'rejected' }
      }
    }

    try {
      const updated = await this.deps.transport.changeOrderStatus(
        transition.orderId,
        transition.targetStatus,
      )
      await this.deps.cache.upsert(ownerId, updated, this.deps.clock.now())
      await this.deps.queue.markApplied(ownerId, transition.id, this.deps.clock.now())
      this.deps.logger.info('Transición aplicada', {
        orderId: transition.orderId,
        target: transition.targetStatus,
        trigger,
      })
      return { kind: 'applied' }
    } catch (rawError) {
      const error = toAppError(rawError)
      if (error.code === 'CONFLICT') {
        return this.reconcileConflict(ownerId, transition)
      }
      return this.failure(ownerId, transition, error)
    }
  }

  private async reconcileConflict(
    ownerId: string,
    transition: PendingTransition,
  ): Promise<
    | { kind: 'applied' }
    | { kind: 'rejected' }
    | { kind: 'paused'; error: AppError }
    | { kind: 'retry'; error: AppError; nextRetryAt: number }
  > {
    try {
      const serverOrder = await this.deps.transport.getOrder(transition.orderId)
      await this.deps.cache.upsert(ownerId, serverOrder, this.deps.clock.now())
      if (serverOrder.status === transition.targetStatus) {
        await this.deps.queue.markApplied(ownerId, transition.id, this.deps.clock.now())
        return { kind: 'applied' }
      }
      await this.deps.queue.markRejected(ownerId, transition.id, 'ILLEGAL_TRANSITION', this.deps.clock.now())
      this.deps.logger.warn('Conflicto de estado no recuperable', {
        orderId: transition.orderId,
        serverStatus: serverOrder.status,
        target: transition.targetStatus,
      })
      return { kind: 'rejected' }
    } catch (rawError) {
      return this.failure(ownerId, transition, rawError)
    }
  }

  private async failure(
    ownerId: string,
    transition: PendingTransition,
    rawError: unknown,
  ): Promise<
    | { kind: 'rejected' }
    | { kind: 'paused'; error: AppError }
    | { kind: 'retry'; error: AppError; nextRetryAt: number }
  > {
    const error = toAppError(rawError)

    if (error.code === 'UNAUTHENTICATED' || error.code === 'FORBIDDEN') {
      return { kind: 'paused', error }
    }

    if (error.code === 'NOT_FOUND' || error.code === 'VALIDATION') {
      // Orden eliminada o transición inválida para el servidor: no se puede
      // reintentar, pero el registro se conserva como fallido.
      await this.deps.queue.markRejected(ownerId, transition.id, error.code, this.deps.clock.now())
      this.deps.logger.error('Transición rechazada por el servidor', {
        orderId: transition.orderId,
        code: error.code,
        message: error.message,
      })
      return { kind: 'rejected' }
    }

    const attempts = transition.attempts + 1
    const delay =
      error.retryAfterMs ?? computeBackoffDelay(attempts, this.policy.retry, this.deps.random)
    const nextRetryAt = this.deps.clock.now() + delay
    await this.deps.queue.markRetry(
      ownerId,
      transition.id,
      nextRetryAt,
      `${error.code}: ${error.message}`,
    )
    return { kind: 'retry', error, nextRetryAt }
  }
}
