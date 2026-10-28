export type ConnectivityListener = (online: boolean) => void

/**
 * Abstracción de NetInfo para que la lógica de sincronización no dependa de
 * React Native y pueda probarse con un doble en memoria.
 */
export interface ConnectivityPort {
  isOnline(): Promise<boolean>
  /** Devuelve la función para cancelar la suscripción. */
  subscribe(listener: ConnectivityListener): () => void
}
