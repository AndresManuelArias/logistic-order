import type { OrderStatus } from '../contracts/types'

/** Lote de puntos persistido antes de enviarse (write-ahead de idempotencia). */
export interface PendingBatch {
  id: string
  ownerId: string
  /** Clave enviada en `Idempotency-Key`; estable entre reintentos. */
  idempotencyKey: string
  pointIds: string[]
  createdAt: number
  attempts: number
  lastAttemptAt: number | null
  nextAttemptAt: number
  lastError: string | null
}

/** Transición de orden pendiente de aplicar en el servidor. */
export interface PendingTransition {
  id: string
  ownerId: string
  orderId: string
  /** Estado que la app creía vigente al encolar (para detectar carreras). */
  expectedStatus: OrderStatus
  targetStatus: OrderStatus
  idempotencyKey: string
  requestedAt: number
  attempts: number
  nextAttemptAt: number
  lastError: string | null
}

export interface PendingCounts {
  pendingPoints: number
  rejectedPoints: number
  pendingTransitions: number
  failedTransitions: number
}

export type SyncTrigger =
  | 'startup'
  | 'online'
  | 'resume'
  | 'timer'
  | 'capture'
  | 'transition'
  | 'manual'
  | 'background'

/**
 * `retry` = hay un lote abierto esperando backoff; `paused` = falta sesión o
 * permisos (no se quema ningún reintento).
 */
export type SyncOutcome = 'idle' | 'synced' | 'offline' | 'paused' | 'retry' | 'error'

export interface SyncCycleResult {
  outcome: SyncOutcome
  trigger: SyncTrigger
  pointsSynced: number
  pointsRejected: number
  transitionsApplied: number
  transitionsRejected: number
  batchesSent: number
  nextRetryAt?: number
  error?: string
}

export interface SyncStatusSnapshot {
  online: boolean
  syncing: boolean
  pendingPoints: number
  rejectedPoints: number
  pendingTransitions: number
  failedTransitions: number
  lastSyncedAt: number | null
  lastError: string | null
  sessionExpired: boolean
  lastTrigger: SyncTrigger | null
}

export const EMPTY_SYNC_STATUS: SyncStatusSnapshot = {
  online: false,
  syncing: false,
  pendingPoints: 0,
  rejectedPoints: 0,
  pendingTransitions: 0,
  failedTransitions: 0,
  lastSyncedAt: null,
  lastError: null,
  sessionExpired: false,
  lastTrigger: null,
}
