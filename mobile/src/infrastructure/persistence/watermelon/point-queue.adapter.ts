import { Q } from '@nozbe/watermelondb'
import type Database from '@nozbe/watermelondb/Database'

import type { QueuedPoint } from '../../../domain/point'
import type { PendingBatch } from '../../../domain/sync'
import type { PointQueueStore } from '../../../ports/telemetry'
import { LocalPointModel, SyncBatchModel } from './models'

function parsePointIds(raw: string): string[] {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((value): value is string => typeof value === 'string')
  } catch {
    return []
  }
}

function toQueuedPoint(model: LocalPointModel): QueuedPoint {
  return {
    id: model.id,
    ownerId: model.ownerId,
    deviceSeq: model.deviceSeq,
    recordedAt: model.recordedAt,
    lat: model.lat,
    lon: model.lon,
    speedKph: model.speedKph,
    accuracyM: model.accuracyM,
    heading: model.heading,
    vehicleId: model.vehicleId,
    orderId: model.orderId,
  }
}

function toPendingBatch(model: SyncBatchModel): PendingBatch {
  return {
    id: model.id,
    ownerId: model.ownerId,
    idempotencyKey: model.idempotencyKey,
    pointIds: parsePointIds(model.pointIds),
    createdAt: model.createdAt,
    attempts: model.attempts,
    lastAttemptAt: model.lastAttemptAt,
    nextAttemptAt: model.nextAttemptAt,
    lastError: model.lastError,
  }
}

/** Implementación WatermelonDB de la cola de puntos y sus lotes. */
export class WatermelonPointQueue implements PointQueueStore {
  private readonly points
  private readonly batches

  constructor(private readonly database: Database) {
    this.points = database.get<LocalPointModel>('local_points')
    this.batches = database.get<SyncBatchModel>('sync_batches')
  }

  async enqueue(point: QueuedPoint, at: number): Promise<void> {
    await this.database.write(async () => {
      await this.points.create((record) => {
        record._raw.id = point.id
        record.ownerId = point.ownerId
        record.deviceSeq = point.deviceSeq
        record.recordedAt = point.recordedAt
        record.lat = point.lat
        record.lon = point.lon
        record.speedKph = point.speedKph ?? null
        record.accuracyM = point.accuracyM ?? null
        record.heading = point.heading ?? null
        record.vehicleId = point.vehicleId ?? null
        record.orderId = point.orderId ?? null
        record.syncState = 'pending'
        record.batchId = null
        record.attempts = 0
        record.lastError = null
        record.createdAt = at
        record.syncedAt = null
        record.rejectedAt = null
      })
    })
  }

  async pendingPoints(ownerId: string, limit: number): Promise<QueuedPoint[]> {
    const rows = await this.points
      .query(
        Q.where('owner_id', ownerId),
        Q.where('sync_state', 'pending'),
        Q.sortBy('device_seq', Q.asc),
        Q.sortBy('id', Q.asc),
        Q.take(limit),
      )
      .fetch()
    return rows.map(toQueuedPoint)
  }

  async pointsByIds(ownerId: string, ids: readonly string[]): Promise<QueuedPoint[]> {
    if (ids.length === 0) return []
    const rows = await this.points
      .query(Q.where('owner_id', ownerId), Q.where('id', Q.oneOf([...ids])))
      .fetch()
    return rows.map(toQueuedPoint)
  }

  async openBatch(ownerId: string): Promise<PendingBatch | null> {
    const rows = await this.batches
      .query(
        Q.where('owner_id', ownerId),
        Q.where('state', 'open'),
        Q.sortBy('created_at', Q.asc),
        Q.take(1),
      )
      .fetch()
    const row = rows[0]
    return row ? toPendingBatch(row) : null
  }

  async createBatch(batch: PendingBatch): Promise<void> {
    await this.database.write(async () => {
      const pointModels = await this.points
        .query(Q.where('owner_id', batch.ownerId), Q.where('id', Q.oneOf(batch.pointIds)))
        .fetch()

      const batchRecord = this.batches.prepareCreate((record) => {
        record._raw.id = batch.id
        record.ownerId = batch.ownerId
        record.idempotencyKey = batch.idempotencyKey
        record.pointIds = JSON.stringify(batch.pointIds)
        record.createdAt = batch.createdAt
        record.attempts = batch.attempts
        record.lastAttemptAt = batch.lastAttemptAt
        record.nextAttemptAt = batch.nextAttemptAt
        record.lastError = batch.lastError
        record.state = 'open'
        record.closedAt = null
      })

      const pointUpdates = pointModels.map((model) =>
        model.prepareUpdate((record) => {
          record.batchId = batch.id
          record.syncState = 'batched'
        }),
      )

      await this.database.batch([batchRecord, ...pointUpdates])
    })
  }

  async markBatchSynced(
    ownerId: string,
    batchId: string,
    pointIds: readonly string[],
    at: number,
  ): Promise<void> {
    await this.database.write(async () => {
      const batch = await this.findBatch(ownerId, batchId)
      if (!batch) return

      const pointModels =
        pointIds.length > 0
          ? await this.points
              .query(Q.where('owner_id', ownerId), Q.where('id', Q.oneOf([...pointIds])))
              .fetch()
          : []

      await this.database.batch([
        batch.prepareUpdate((record) => {
          record.state = 'closed'
          record.closedAt = at
          record.lastAttemptAt = at
          record.lastError = null
        }),
        ...pointModels.map((model) =>
          model.prepareUpdate((record) => {
            record.syncState = 'synced'
            record.syncedAt = at
            record.lastError = null
          }),
        ),
      ])
    })
  }

  async markBatchRetry(
    ownerId: string,
    batchId: string,
    nextAttemptAt: number,
    error: string,
    at: number,
  ): Promise<void> {
    await this.database.write(async () => {
      const batch = await this.findBatch(ownerId, batchId)
      if (!batch) return
      await batch.update((record) => {
        record.attempts = record.attempts + 1
        record.lastAttemptAt = at
        record.nextAttemptAt = nextAttemptAt
        record.lastError = error
      })
    })
  }

  async rejectBatch(
    ownerId: string,
    batchId: string,
    pointIds: readonly string[],
    code: string,
    at: number,
  ): Promise<void> {
    await this.database.write(async () => {
      const batch = await this.findBatch(ownerId, batchId)
      if (!batch) return

      const pointModels =
        pointIds.length > 0
          ? await this.points
              .query(Q.where('owner_id', ownerId), Q.where('id', Q.oneOf([...pointIds])))
              .fetch()
          : []

      await this.database.batch([
        batch.prepareUpdate((record) => {
          record.state = 'rejected'
          record.closedAt = at
          record.lastAttemptAt = at
          record.lastError = code
        }),
        ...pointModels.map((model) =>
          model.prepareUpdate((record) => {
            record.syncState = 'rejected'
            record.rejectedAt = at
            record.lastError = code
            record.attempts = record.attempts + 1
          }),
        ),
      ])
    })
  }

  async counts(ownerId: string): Promise<{ pending: number; rejected: number }> {
    const [pending, rejected] = await Promise.all([
      this.points
        .query(
          Q.where('owner_id', ownerId),
          Q.where('sync_state', Q.oneOf(['pending', 'batched'])),
        )
        .fetchCount(),
      this.points.query(Q.where('owner_id', ownerId), Q.where('sync_state', 'rejected')).fetchCount(),
    ])
    return { pending, rejected }
  }

  private async findBatch(ownerId: string, batchId: string): Promise<SyncBatchModel | null> {
    const rows = await this.batches
      .query(Q.where('owner_id', ownerId), Q.where('id', batchId), Q.take(1))
      .fetch()
    return rows[0] ?? null
  }
}
