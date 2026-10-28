import type { TelemetryPointInput } from '../contracts/types'
import type { QueuedPoint } from '../domain/point'
import type { PendingBatch } from '../domain/sync'

export interface BulkSendInput {
  deviceId: string
  /** Debe ser idéntico entre reintentos del mismo lote (8–128 caracteres). */
  idempotencyKey: string
  points: TelemetryPointInput[]
}

export interface BulkSendResult {
  accepted: number
  duplicates: number
  alreadyProcessed: boolean
  processedAt: string
}

export interface TelemetryTransport {
  sendBulk(input: BulkSendInput): Promise<BulkSendResult>
}

/**
 * Cola local de puntos GPS.
 *
 * Contrato clave: `createBatch` persiste el lote y marca sus puntos como
 * "batched" ANTES de cualquier llamada de red. Si la app muere después de
 * enviar, `openBatch` devuelve el mismo lote y la misma clave para que el
 * backend responda `already_processed` en lugar de duplicar.
 */
export interface PointQueueStore {
  enqueue(point: QueuedPoint, at: number): Promise<void>
  pendingPoints(ownerId: string, limit: number): Promise<QueuedPoint[]>
  pointsByIds(ownerId: string, ids: readonly string[]): Promise<QueuedPoint[]>
  openBatch(ownerId: string): Promise<PendingBatch | null>
  createBatch(batch: PendingBatch): Promise<void>
  markBatchSynced(
    ownerId: string,
    batchId: string,
    pointIds: readonly string[],
    at: number,
  ): Promise<void>
  markBatchRetry(
    ownerId: string,
    batchId: string,
    nextAttemptAt: number,
    error: string,
    at: number,
  ): Promise<void>
  rejectBatch(
    ownerId: string,
    batchId: string,
    pointIds: readonly string[],
    code: string,
    at: number,
  ): Promise<void>
  counts(ownerId: string): Promise<{ pending: number; rejected: number }>
}
