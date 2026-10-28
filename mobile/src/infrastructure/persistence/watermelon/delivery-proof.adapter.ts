import { Q } from '@nozbe/watermelondb'
import type Database from '@nozbe/watermelondb/Database'

import type { DeliveryProofRecord, DeliveryProofStorePort } from '../../../ports/proofs'
import { DeliveryProofModel } from './models'

function toRecord(model: DeliveryProofModel): DeliveryProofRecord {
  return {
    id: model.id,
    ownerId: model.ownerId,
    orderId: model.orderId,
    receiverName: model.receiverName,
    notes: model.notes,
    photoUri: model.photoUri,
    capturedAt: model.capturedAt,
    uploadState: 'pending_backend',
  }
}

/** La evidencia POD nunca sale del dispositivo (no hay endpoint en el contrato). */
export class WatermelonDeliveryProofStore implements DeliveryProofStorePort {
  private readonly proofs

  constructor(private readonly database: Database) {
    this.proofs = database.get<DeliveryProofModel>('delivery_proofs')
  }

  async save(record: DeliveryProofRecord): Promise<void> {
    await this.database.write(async () => {
      await this.proofs.create((row) => {
        row._raw.id = record.id
        row.ownerId = record.ownerId
        row.orderId = record.orderId
        row.receiverName = record.receiverName
        row.notes = record.notes
        row.photoUri = record.photoUri
        row.capturedAt = record.capturedAt
        row.uploadState = record.uploadState
      })
    })
  }

  async listByOrder(ownerId: string, orderId: string): Promise<DeliveryProofRecord[]> {
    const rows = await this.proofs
      .query(
        Q.where('owner_id', ownerId),
        Q.where('order_id', orderId),
        Q.sortBy('captured_at', Q.desc),
      )
      .fetch()
    return rows.map(toRecord)
  }
}
