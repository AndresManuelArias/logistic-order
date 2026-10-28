export interface DeliveryProofRecord {
  id: string
  ownerId: string
  orderId: string
  receiverName: string
  notes: string | null
  /** URI definitiva dentro del almacenamiento de la app (no la caché del picker). */
  photoUri: string | null
  capturedAt: number
  /**
   * Siempre `pending_backend` mientras no exista endpoint de POD en el
   * contrato OpenAPI. Se declara explícitamente para no fingir una subida.
   */
  uploadState: 'pending_backend'
}

export interface DeliveryProofStorePort {
  save(record: DeliveryProofRecord): Promise<void>
  listByOrder(ownerId: string, orderId: string): Promise<DeliveryProofRecord[]>
}

/** Copia la foto a un directorio permanente de la app. */
export interface ProofFileStorePort {
  persist(proofId: string, sourceUri: string): Promise<string>
}
