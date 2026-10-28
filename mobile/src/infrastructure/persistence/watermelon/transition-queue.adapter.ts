import { Q } from '@nozbe/watermelondb'
import type Database from '@nozbe/watermelondb/Database'

import type { OrderStatus } from '../../../contracts/types'
import type { PendingTransition } from '../../../domain/sync'
import type {
  EnqueueTransitionInput,
  EnqueueTransitionResult,
  TransitionQueuePort,
} from '../../../ports/transitions'
import { OrderTransitionModel } from './models'

function toPendingTransition(model: OrderTransitionModel): PendingTransition {
  return {
    id: model.id,
    ownerId: model.ownerId,
    orderId: model.orderId,
    expectedStatus: model.expectedStatus as OrderStatus,
    targetStatus: model.targetStatus as OrderStatus,
    idempotencyKey: model.idempotencyKey,
    requestedAt: model.requestedAt,
    attempts: model.attempts,
    nextAttemptAt: model.nextAttemptAt,
    lastError: model.lastError,
  }
}

/** Cola persistente de transiciones de orden (offline-first). */
export class WatermelonTransitionQueue implements TransitionQueuePort {
  private readonly transitions

  constructor(private readonly database: Database) {
    this.transitions = database.get<OrderTransitionModel>('order_transitions')
  }

  async enqueue(
    ownerId: string,
    input: EnqueueTransitionInput,
    at: number,
  ): Promise<EnqueueTransitionResult> {
    let result: EnqueueTransitionResult | null = null

    await this.database.write(async () => {
      const existing = await this.transitions
        .query(
          Q.where('owner_id', ownerId),
          Q.where('order_id', input.orderId),
          Q.where('state', 'pending'),
          Q.sortBy('requested_at', Q.desc),
          Q.sortBy('id', Q.desc),
          Q.take(1),
        )
        .fetch()

      const latest = existing[0]
      if (latest && latest.targetStatus === input.targetStatus) {
        result = { transition: toPendingTransition(latest), deduped: true }
        return
      }

      const created = await this.transitions.create((record) => {
        record._raw.id = input.id
        record.ownerId = ownerId
        record.orderId = input.orderId
        record.expectedStatus = input.expectedStatus
        record.targetStatus = input.targetStatus
        record.idempotencyKey = input.idempotencyKey
        record.requestedAt = at
        record.attempts = 0
        record.nextAttemptAt = at
        record.lastError = null
        record.state = 'pending'
        record.resolvedAt = null
        record.resolutionCode = null
      })

      result = { transition: toPendingTransition(created), deduped: false }
    })

    if (!result) throw new Error('No se pudo encolar la transición de la orden')
    return result
  }

  async next(ownerId: string, limit: number): Promise<PendingTransition[]> {
    const rows = await this.transitions
      .query(
        Q.where('owner_id', ownerId),
        Q.where('state', 'pending'),
        Q.sortBy('requested_at', Q.asc),
        Q.sortBy('id', Q.asc),
        Q.take(limit),
      )
      .fetch()
    return rows.map(toPendingTransition)
  }

  async findPending(ownerId: string, orderId: string): Promise<PendingTransition | null> {
    const rows = await this.transitions
      .query(
        Q.where('owner_id', ownerId),
        Q.where('order_id', orderId),
        Q.where('state', 'pending'),
        Q.sortBy('requested_at', Q.desc),
        Q.sortBy('id', Q.desc),
        Q.take(1),
      )
      .fetch()
    const row = rows[0]
    return row ? toPendingTransition(row) : null
  }

  async listPending(ownerId: string): Promise<PendingTransition[]> {
    const rows = await this.transitions
      .query(
        Q.where('owner_id', ownerId),
        Q.where('state', 'pending'),
        Q.sortBy('requested_at', Q.asc),
        Q.sortBy('id', Q.asc),
      )
      .fetch()
    return rows.map(toPendingTransition)
  }

  async counts(ownerId: string): Promise<{ pending: number; failed: number }> {
    const [pending, failed] = await Promise.all([
      this.transitions
        .query(Q.where('owner_id', ownerId), Q.where('state', 'pending'))
        .fetchCount(),
      this.transitions
        .query(Q.where('owner_id', ownerId), Q.where('state', 'rejected'))
        .fetchCount(),
    ])
    return { pending, failed }
  }

  async markApplied(ownerId: string, transitionId: string, at: number): Promise<void> {
    await this.resolve(ownerId, transitionId, 'applied', at, 'APPLIED')
  }

  async markRejected(ownerId: string, transitionId: string, code: string, at: number): Promise<void> {
    await this.resolve(ownerId, transitionId, 'rejected', at, code)
  }

  async markRetry(
    ownerId: string,
    transitionId: string,
    nextAttemptAt: number,
    error: string,
  ): Promise<void> {
    await this.database.write(async () => {
      const row = await this.find(ownerId, transitionId)
      if (!row) return
      await row.update((record) => {
        record.attempts = record.attempts + 1
        record.nextAttemptAt = nextAttemptAt
        record.lastError = error
      })
    })
  }

  private async resolve(
    ownerId: string,
    transitionId: string,
    state: 'applied' | 'rejected',
    at: number,
    code: string,
  ): Promise<void> {
    await this.database.write(async () => {
      const row = await this.find(ownerId, transitionId)
      if (!row) return
      await row.update((record) => {
        record.state = state
        record.resolvedAt = at
        record.resolutionCode = code
        if (state === 'applied') record.lastError = null
      })
    })
  }

  private async find(ownerId: string, transitionId: string): Promise<OrderTransitionModel | null> {
    const rows = await this.transitions
      .query(Q.where('owner_id', ownerId), Q.where('id', transitionId), Q.take(1))
      .fetch()
    return rows[0] ?? null
  }
}
