import type { OrderStatus } from '../contracts/types'
import type { PendingTransition } from '../domain/sync'

export interface EnqueueTransitionInput {
  /** Identificador local de la transición (también base de la clave). */
  id: string
  orderId: string
  expectedStatus: OrderStatus
  targetStatus: OrderStatus
  /** Clave estable por operación; se conserva para auditoría y deduplicación. */
  idempotencyKey: string
}

export interface EnqueueTransitionResult {
  transition: PendingTransition
  /** `true` si ya existía una transición pendiente equivalente. */
  deduped: boolean
}

export interface TransitionQueuePort {
  enqueue(
    ownerId: string,
    input: EnqueueTransitionInput,
    at: number,
  ): Promise<EnqueueTransitionResult>
  /**
   * Transiciones pendientes ordenadas por antigüedad. El servicio comprueba
   * el backoff (`nextAttemptAt`) para respetar el orden estricto por orden.
   */
  next(ownerId: string, limit: number): Promise<PendingTransition[]>
  findPending(ownerId: string, orderId: string): Promise<PendingTransition | null>
  listPending(ownerId: string): Promise<PendingTransition[]>
  counts(ownerId: string): Promise<{ pending: number; failed: number }>
  markApplied(ownerId: string, transitionId: string, at: number): Promise<void>
  markRetry(
    ownerId: string,
    transitionId: string,
    nextAttemptAt: number,
    error: string,
  ): Promise<void>
  markRejected(ownerId: string, transitionId: string, code: string, at: number): Promise<void>
}
