export interface DeliveryProofInput {
  orderId: string
  receiverName: string
  notes?: string | null
  /** URI local de la foto elegida/capturada. */
  photoUri?: string | null
}

export interface NormalizedDeliveryProof {
  orderId: string
  receiverName: string
  notes: string | null
  photoUri: string | null
}

export type DeliveryProofValidation =
  | { ok: true; value: NormalizedDeliveryProof }
  | { ok: false; code: 'MISSING_ORDER' | 'MISSING_RECEIVER' | 'NOTES_TOO_LONG'; message: string }

export const MAX_RECEIVER_NAME_LENGTH = 120
export const MAX_NOTES_LENGTH = 500

/**
 * El backend F2–F4 no expone un endpoint de prueba de entrega (POD), así que
 * la evidencia se guarda sólo en el dispositivo. Esta constante permite que la
 * UI explique la limitación sin inventar un endpoint.
 */
export const DELIVERY_PROOF_UPLOAD_SUPPORTED = false

export function validateDeliveryProof(input: DeliveryProofInput): DeliveryProofValidation {
  const orderId = input.orderId.trim()
  if (orderId.length === 0) {
    return { ok: false, code: 'MISSING_ORDER', message: 'Falta la orden de la entrega' }
  }

  const receiverName = input.receiverName.trim()
  if (receiverName.length === 0) {
    return {
      ok: false,
      code: 'MISSING_RECEIVER',
      message: 'Escribe el nombre de quien recibe',
    }
  }
  if (receiverName.length > MAX_RECEIVER_NAME_LENGTH) {
    return {
      ok: false,
      code: 'MISSING_RECEIVER',
      message: `El nombre no puede superar ${MAX_RECEIVER_NAME_LENGTH} caracteres`,
    }
  }

  const notes = input.notes?.trim() ? input.notes.trim() : null
  if (notes && notes.length > MAX_NOTES_LENGTH) {
    return {
      ok: false,
      code: 'NOTES_TOO_LONG',
      message: `Las notas no pueden superar ${MAX_NOTES_LENGTH} caracteres`,
    }
  }

  const photoUri = input.photoUri?.trim() ? input.photoUri.trim() : null

  return { ok: true, value: { orderId, receiverName, notes, photoUri } }
}
