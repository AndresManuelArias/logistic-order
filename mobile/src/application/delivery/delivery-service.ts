import {
  DELIVERY_PROOF_UPLOAD_SUPPORTED,
  validateDeliveryProof,
  type DeliveryProofInput,
} from '../../domain/delivery-proof'
import { toAppError } from '../../domain/errors'
import type { PendingTransition } from '../../domain/sync'
import type { Clock } from '../../ports/clock'
import type { DeliveryProofRecord, DeliveryProofStorePort, ProofFileStorePort } from '../../ports/proofs'
import type { OrderService } from '../orders/order-service'
import { newLocalId } from '../../shared/ids'

export interface DeliveryServiceDeps {
  proofs: DeliveryProofStorePort
  files: ProofFileStorePort
  orders: OrderService
  clock: Clock
}

export type CompleteDeliveryResult =
  | {
      ok: true
      proof: DeliveryProofRecord
      transition: PendingTransition | null
      deduped: boolean
    }
  | {
      ok: false
      code: string
      message: string
      /** La evidencia se guardó aunque la transición no pudiera encolarse. */
      proof?: DeliveryProofRecord
    }

/**
 * Prueba de entrega (POD) local + transición `DELIVERED`.
 *
 * El contrato OpenAPI no ofrece endpoint de POD (`DELIVERY_PROOF_UPLOAD_SUPPORTED`
 * es `false`): la foto y el receptor quedan en el dispositivo y se declara
 * explícitamente que no se suben. La transición de estado sí viaja por la cola
 * offline habitual.
 */
export class DeliveryService {
  constructor(private readonly deps: DeliveryServiceDeps) {}

  async completeDelivery(ownerId: string, input: DeliveryProofInput): Promise<CompleteDeliveryResult> {
    const validation = validateDeliveryProof(input)
    if (!validation.ok) {
      return { ok: false, code: validation.code, message: validation.message }
    }

    const value = validation.value
    const proofId = newLocalId('proof_')

    let photoUri: string | null = null
    if (value.photoUri) {
      try {
        photoUri = await this.deps.files.persist(proofId, value.photoUri)
      } catch (rawError) {
        return {
          ok: false,
          code: 'PHOTO_COPY_FAILED',
          message: `No se pudo guardar la foto: ${toAppError(rawError).message}`,
        }
      }
    }

    const record: DeliveryProofRecord = {
      id: proofId,
      ownerId,
      orderId: value.orderId,
      receiverName: value.receiverName,
      notes: value.notes,
      photoUri,
      capturedAt: this.deps.clock.now(),
      uploadState: 'pending_backend',
    }

    await this.deps.proofs.save(record)

    try {
      const result = await this.deps.orders.requestTransition(ownerId, value.orderId, 'DELIVERED')
      return { ok: true, proof: record, transition: result.transition, deduped: result.deduped }
    } catch (rawError) {
      const error = toAppError(rawError)
      return { ok: false, code: error.code, message: error.message, proof: record }
    }
  }

  async listProofs(ownerId: string, orderId: string): Promise<DeliveryProofRecord[]> {
    return this.deps.proofs.listByOrder(ownerId, orderId)
  }

  get uploadSupported(): boolean {
    return DELIVERY_PROOF_UPLOAD_SUPPORTED
  }
}
