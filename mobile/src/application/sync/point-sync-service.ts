import { SYNC_POLICY, type SyncPolicy } from '../../config/policies'
import { AppError, toAppError, type ErrorCode } from '../../domain/errors'
import { toPointInput, type QueuedPoint } from '../../domain/point'
import type { PendingBatch, SyncTrigger } from '../../domain/sync'
import type { Clock, Logger } from '../../ports/clock'
import type { DeviceMetadataPort } from '../../ports/device'
import type { PointQueueStore, TelemetryTransport } from '../../ports/telemetry'
import { batchIdempotencyKey, newLocalId } from '../../shared/ids'
import { computeBackoffDelay } from './backoff'

export interface PointSyncDeps {
  store: PointQueueStore
  transport: TelemetryTransport
  device: DeviceMetadataPort
  clock: Clock
  logger: Logger
  policy?: SyncPolicy
  random?: () => number
}

export interface PointSyncContext {
  ownerId: string
  trigger: SyncTrigger
}

export interface PointSyncResult {
  outcome: 'idle' | 'synced' | 'retry' | 'paused'
  pointsSynced: number
  pointsRejected: number
  batchesSent: number
  nextRetryAt?: number
  error?: string
  errorCode?: ErrorCode
}

type FailureDecision =
  | { kind: 'pause'; error: AppError }
  | { kind: 'retry'; error: AppError; nextRetryAt: number }
  | { kind: 'reject'; error: AppError }

/**
 * Envía lotes de puntos GPS con garantías de idempotencia:
 *
 * 1. El lote se persiste con su `Idempotency-Key` antes de la primera llamada.
 * 2. Sólo existe un lote abierto por conductor; los reintentos reutilizan la
 *    misma clave, así que un timeout ambiguo nunca duplica puntos.
 * 3. Los fallos permanentes (payload inválido) se ponen en cuarentena sin
 *    borrar los datos y no bloquean los lotes siguientes.
 */
export class PointSyncService {
  private readonly policy: SyncPolicy
  private readonly logger: Logger

  constructor(private readonly deps: PointSyncDeps) {
    this.policy = deps.policy ?? SYNC_POLICY
    this.logger = deps.logger
  }

  async run(context: PointSyncContext): Promise<PointSyncResult> {
    const { ownerId, trigger } = context
    const result: PointSyncResult = {
      outcome: 'idle',
      pointsSynced: 0,
      pointsRejected: 0,
      batchesSent: 0,
    }

    const deviceId = await this.deps.device.getDeviceId()

    for (let iteration = 0; iteration < this.policy.maxBatchesPerRun; iteration += 1) {
      const now = this.deps.clock.now()
      const openBatch = await this.deps.store.openBatch(ownerId)

      if (openBatch && openBatch.nextAttemptAt > now) {
        result.outcome = 'retry'
        result.nextRetryAt = openBatch.nextAttemptAt
        result.error = openBatch.lastError ?? undefined
        return result
      }

      let batch: PendingBatch
      let points: QueuedPoint[]

      if (openBatch) {
        batch = openBatch
        points = await this.loadBatchPoints(ownerId, openBatch)
        if (points.length === 0) {
          // No hay puntos recuperables: cerrar el lote para no bloquear la cola.
          await this.deps.store.rejectBatch(ownerId, openBatch.id, [], 'EMPTY_BATCH', now)
          this.logger.warn('Lote sin puntos recuperables; se cierra', { batchId: openBatch.id })
          continue
        }
      } else {
        const pending = await this.deps.store.pendingPoints(ownerId, this.policy.maxBatchSize)
        if (pending.length === 0) break
        const batchId = newLocalId('bt_')
        batch = {
          id: batchId,
          ownerId,
          idempotencyKey: batchIdempotencyKey(deviceId, batchId),
          pointIds: pending.map((point) => point.id),
          createdAt: now,
          attempts: 0,
          lastAttemptAt: null,
          nextAttemptAt: now,
          lastError: null,
        }
        // Write-ahead: la clave queda persistida antes de tocar la red.
        await this.deps.store.createBatch(batch)
        points = pending
      }

      try {
        const response = await this.deps.transport.sendBulk({
          deviceId,
          idempotencyKey: batch.idempotencyKey,
          points: points.map(toPointInput),
        })
        await this.deps.store.markBatchSynced(
          ownerId,
          batch.id,
          points.map((point) => point.id),
          this.deps.clock.now(),
        )
        result.pointsSynced += points.length
        result.batchesSent += 1
        result.outcome = 'synced'
        this.logger.info('Lote de telemetría sincronizado', {
          batchId: batch.id,
          points: points.length,
          accepted: response.accepted,
          duplicates: response.duplicates,
          alreadyProcessed: response.alreadyProcessed,
          trigger,
        })
      } catch (rawError) {
        const decision = this.classifyFailure(rawError, batch)
        if (decision.kind === 'pause') {
          result.outcome = 'paused'
          result.error = decision.error.message
          result.errorCode = decision.error.code
          this.logger.warn('Sincronización en pausa', {
            code: decision.error.code,
            message: decision.error.message,
          })
          return result
        }

        if (decision.kind === 'retry') {
          const attempts = batch.attempts + 1
          await this.deps.store.markBatchRetry(
            ownerId,
            batch.id,
            decision.nextRetryAt,
            `${decision.error.code}: ${decision.error.message}`,
            this.deps.clock.now(),
          )
          result.outcome = 'retry'
          result.nextRetryAt = decision.nextRetryAt
          result.error = decision.error.message
          result.errorCode = decision.error.code
          this.logger.warn('Reintento de lote programado', {
            batchId: batch.id,
            attempts,
            nextRetryAt: decision.nextRetryAt,
            code: decision.error.code,
          })
          return result
        }

        await this.deps.store.rejectBatch(
          ownerId,
          batch.id,
          points.map((point) => point.id),
          `${decision.error.code}: ${decision.error.message}`,
          this.deps.clock.now(),
        )
        result.pointsRejected += points.length
        result.batchesSent += 1
        result.outcome = 'synced'
        result.error = decision.error.message
        result.errorCode = decision.error.code
        this.logger.error('Lote rechazado por el servidor; queda en cuarentena', {
          batchId: batch.id,
          points: points.length,
          code: decision.error.code,
          message: decision.error.message,
        })
      }
    }

    return result
  }

  private async loadBatchPoints(ownerId: string, batch: PendingBatch): Promise<QueuedPoint[]> {
    const found = await this.deps.store.pointsByIds(ownerId, batch.pointIds)
    const byId = new Map(found.map((point) => [point.id, point]))
    const ordered: QueuedPoint[] = []
    for (const id of batch.pointIds) {
      const point = byId.get(id)
      if (point) ordered.push(point)
    }
    return ordered
  }

  private classifyFailure(rawError: unknown, batch: PendingBatch): FailureDecision {
    const error = toAppError(rawError)

    // Sin sesión o sin permisos no se quema ningún reintento ni se descartan
    // datos: el coordinador pausa hasta que el usuario resuelva.
    if (
      error.code === 'UNAUTHENTICATED' ||
      error.code === 'FORBIDDEN' ||
      error.code === 'NOT_FOUND'
    ) {
      return { kind: 'pause', error }
    }

    if (error.code === 'VALIDATION' || error.code === 'CONFLICT') {
      return { kind: 'reject', error }
    }

    const attempts = batch.attempts + 1
    const delay =
      error.retryAfterMs ?? computeBackoffDelay(attempts, this.policy.retry, this.deps.random)
    return { kind: 'retry', error, nextRetryAt: this.deps.clock.now() + delay }
  }
}
