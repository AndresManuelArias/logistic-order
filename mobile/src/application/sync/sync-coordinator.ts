import { toAppError } from '../../domain/errors'
import type { SyncCycleResult, SyncOutcome, SyncTrigger } from '../../domain/sync'
import type { OwnerIdProvider } from '../../ports/auth'
import type { Clock, Logger } from '../../ports/clock'
import type { ConnectivityPort } from '../../ports/connectivity'
import type { PointQueueStore } from '../../ports/telemetry'
import type { TransitionQueuePort } from '../../ports/transitions'
import type { PointSyncResult } from './point-sync-service'
import { PointSyncService } from './point-sync-service'
import type { SyncStatusStore } from './sync-status-store'
import type { TransitionSyncResult } from './transition-sync-service'
import { TransitionSyncService } from './transition-sync-service'

export interface SyncCoordinatorDeps {
  points: PointSyncService
  transitions: TransitionSyncService
  connectivity: ConnectivityPort
  status: SyncStatusStore
  pointStore: PointQueueStore
  transitionStore: TransitionQueuePort
  ownerId: OwnerIdProvider
  clock: Clock
  logger: Logger
}

function combineOutcomes(outcomes: readonly SyncOutcome[]): SyncOutcome {
  if (outcomes.includes('paused')) return 'paused'
  if (outcomes.includes('retry')) return 'retry'
  if (outcomes.includes('synced')) return 'synced'
  return 'idle'
}

/**
 * Orquesta puntos + transiciones. Garantiza single-flight: dos disparadores
 * simultáneos (timer, reconexión, captura) comparten la misma promesa y no
 * generan dos lotes con claves distintas.
 */
export class SyncCoordinator {
  private inFlight: Promise<SyncCycleResult> | null = null
  private unsubscribe: (() => void) | null = null

  constructor(private readonly deps: SyncCoordinatorDeps) {}

  async syncNow(trigger: SyncTrigger): Promise<SyncCycleResult> {
    if (this.inFlight) {
      this.deps.logger.debug('Sync ya en curso; se reutiliza el ciclo', { trigger })
      return this.inFlight
    }
    this.inFlight = this.runCycle(trigger).finally(() => {
      this.inFlight = null
    })
    return this.inFlight
  }

  /** Se suscribe a la conectividad y lanza el primer ciclo. */
  start(): void {
    if (this.unsubscribe) return
    this.unsubscribe = this.deps.connectivity.subscribe((online) => {
      this.deps.status.update({ online })
      if (online) {
        void this.syncNow('online')
      } else {
        void this.refreshStatus()
      }
    })
    void this.refreshStatus()
    void this.syncNow('startup')
  }

  stop(): void {
    this.unsubscribe?.()
    this.unsubscribe = null
  }

  /** Recalcula contadores y conectividad sin enviar nada (p. ej. tras logout). */
  async refreshStatus(): Promise<void> {
    let online = false
    try {
      online = await this.deps.connectivity.isOnline()
    } catch (error) {
      this.deps.logger.warn('No se pudo consultar la conectividad', {
        error: toAppError(error).message,
      })
    }
    this.deps.status.update({ online })
    await this.refreshCounts()
  }

  private async runCycle(trigger: SyncTrigger): Promise<SyncCycleResult> {
    const ownerId = this.deps.ownerId()
    const result: SyncCycleResult = {
      outcome: 'idle',
      trigger,
      pointsSynced: 0,
      pointsRejected: 0,
      transitionsApplied: 0,
      transitionsRejected: 0,
      batchesSent: 0,
    }

    const online = await this.deps.connectivity.isOnline().catch(() => false)
    this.deps.status.update({ online })

    if (!ownerId) {
      await this.refreshCounts()
      return result
    }

    if (!online) {
      await this.refreshCounts()
      return { ...result, outcome: 'offline' }
    }

    this.deps.status.update({
      syncing: true,
      lastTrigger: trigger,
      sessionExpired: false,
    })

    let pointsResult: PointSyncResult | null = null
    let transitionsResult: TransitionSyncResult | null = null
    let unexpected: string | null = null

    try {
      pointsResult = await this.deps.points.run({ ownerId, trigger })

      // Sin credenciales válidas no se intenta el resto: se evita quemar
      // reintentos y se pide reautenticación.
      if (!(pointsResult.outcome === 'paused' && pointsResult.errorCode === 'UNAUTHENTICATED')) {
        transitionsResult = await this.deps.transitions.run({ ownerId, trigger })
      }
    } catch (rawError) {
      const error = toAppError(rawError)
      unexpected = error.message
      this.deps.logger.error('Ciclo de sincronización inesperado', {
        code: error.code,
        message: error.message,
      })
    }

    result.pointsSynced = pointsResult?.pointsSynced ?? 0
    result.pointsRejected = pointsResult?.pointsRejected ?? 0
    result.transitionsApplied = transitionsResult?.applied ?? 0
    result.transitionsRejected = transitionsResult?.rejected ?? 0
    result.batchesSent = pointsResult?.batchesSent ?? 0

    const outcomes: SyncOutcome[] = []
    if (pointsResult) outcomes.push(pointsResult.outcome)
    if (transitionsResult) outcomes.push(transitionsResult.outcome)
    result.outcome = unexpected ? 'error' : combineOutcomes(outcomes)

    const retryCandidates = [
      pointsResult?.nextRetryAt,
      transitionsResult?.nextRetryAt,
    ].filter((value): value is number => typeof value === 'number')
    if (retryCandidates.length > 0) result.nextRetryAt = Math.min(...retryCandidates)

    const errorMessage =
      unexpected ?? pointsResult?.error ?? transitionsResult?.error ?? undefined
    result.error = errorMessage

    const sessionExpired =
      pointsResult?.errorCode === 'UNAUTHENTICATED' ||
      transitionsResult?.errorCode === 'UNAUTHENTICATED'

    const didWork =
      result.pointsSynced > 0 ||
      result.batchesSent > 0 ||
      result.transitionsApplied > 0 ||
      result.transitionsRejected > 0

    const hasRejections = result.pointsRejected > 0 || result.transitionsRejected > 0
    const cleanCycle = (result.outcome === 'synced' || result.outcome === 'idle') && !hasRejections

    this.deps.status.update({
      sessionExpired,
      lastError: cleanCycle ? null : (errorMessage ?? null),
      ...(didWork && !sessionExpired ? { lastSyncedAt: this.deps.clock.now() } : {}),
    })

    await this.refreshCounts()
    this.deps.status.update({ syncing: false })

    return result
  }

  private async refreshCounts(): Promise<void> {
    const ownerId = this.deps.ownerId()
    if (!ownerId) {
      this.deps.status.update({
        pendingPoints: 0,
        rejectedPoints: 0,
        pendingTransitions: 0,
        failedTransitions: 0,
      })
      return
    }
    try {
      const [points, transitions] = await Promise.all([
        this.deps.pointStore.counts(ownerId),
        this.deps.transitionStore.counts(ownerId),
      ])
      this.deps.status.update({
        pendingPoints: points.pending,
        rejectedPoints: points.rejected,
        pendingTransitions: transitions.pending,
        failedTransitions: transitions.failed,
      })
    } catch (rawError) {
      this.deps.logger.warn('No se pudieron contar los pendientes', {
        error: toAppError(rawError).message,
      })
    }
  }
}
