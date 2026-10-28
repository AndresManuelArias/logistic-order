import type Database from '@nozbe/watermelondb/Database'

import { API_BASE_URL, LOG_LEVEL } from '../config/env'
import { GPS_POLICY, HTTP_TIMEOUT_MS } from '../config/policies'
import { DeliveryService } from '../application/delivery/delivery-service'
import { OrderService } from '../application/orders/order-service'
import { SessionService } from '../application/auth/session-service'
import { SyncCoordinator } from '../application/sync/sync-coordinator'
import { PointSyncService } from '../application/sync/point-sync-service'
import { SyncStatusStore } from '../application/sync/sync-status-store'
import { TransitionSyncService } from '../application/sync/transition-sync-service'
import { RecordGpsSample } from '../application/telemetry/record-gps-sample'
import { NetInfoConnectivity } from '../infrastructure/connectivity/netinfo.adapter'
import { ExpoLocationTracker } from '../infrastructure/device/expo-location-tracker'
import { createTrackingApi, type TrackingApi } from '../infrastructure/api/endpoints'
import { HttpClient } from '../infrastructure/api/http'
import { ExpoProofFileStore } from '../infrastructure/files/proof-file.store'
import { createConsoleLogger } from '../infrastructure/logging/console-logger'
import { createDatabase } from '../infrastructure/persistence/watermelon/database'
import { WatermelonDeliveryProofStore } from '../infrastructure/persistence/watermelon/delivery-proof.adapter'
import { WatermelonDeviceMetadata } from '../infrastructure/persistence/watermelon/device-metadata.adapter'
import { WatermelonOrderCache } from '../infrastructure/persistence/watermelon/order-cache.adapter'
import { WatermelonPointQueue } from '../infrastructure/persistence/watermelon/point-queue.adapter'
import { WatermelonTransitionQueue } from '../infrastructure/persistence/watermelon/transition-queue.adapter'
import { SecureSessionRepository } from '../infrastructure/security/secure-session.repository'
import type { ConnectivityPort } from '../ports/connectivity'
import type { RawLocationSample } from '../ports/location'
import { systemClock } from '../ports/clock'

export interface AppServices {
  readonly database: Database
  readonly session: SessionService
  readonly api: TrackingApi
  readonly connectivity: ConnectivityPort
  readonly status: SyncStatusStore
  readonly coordinator: SyncCoordinator
  readonly orders: OrderService
  readonly delivery: DeliveryService
  readonly tracker: ExpoLocationTracker
  /** Captura muestras provenientes del GPS (background o foreground). */
  handleBackgroundSamples(samples: RawLocationSample[]): Promise<void>
  dispose(): void
}

async function createServices(): Promise<AppServices> {
  const logger = createConsoleLogger(LOG_LEVEL)
  const clock = systemClock

  const database = createDatabase((error) => {
    logger.error('Base de datos local no disponible', { message: error.message })
  })

  const status = new SyncStatusStore()
  const connectivity = new NetInfoConnectivity()

  // La sesión se crea después del cliente HTTP, pero éste sólo la usa dentro de
  // funciones diferidas (nunca durante la construcción).
  let sessionRef: SessionService | null = null

  const http = new HttpClient({
    baseUrl: API_BASE_URL,
    logger,
    timeoutMs: HTTP_TIMEOUT_MS,
    getAccessToken: async () => (sessionRef ? sessionRef.ensureAccessToken() : null),
    refreshAccessToken: async () => (sessionRef ? sessionRef.refreshAccessToken() : null),
  })
  const api = createTrackingApi(http)

  const session = new SessionService({
    repository: new SecureSessionRepository(),
    api,
    clock,
    logger,
  })
  sessionRef = session
  await session.hydrate()

  const device = new WatermelonDeviceMetadata(database)
  const pointStore = new WatermelonPointQueue(database)
  const transitionStore = new WatermelonTransitionQueue(database)
  const orderCache = new WatermelonOrderCache(database)
  const proofStore = new WatermelonDeliveryProofStore(database)

  const recordSample = new RecordGpsSample({
    points: pointStore,
    device,
    clock,
    policy: GPS_POLICY,
  })

  const orders = new OrderService({
    cache: orderCache,
    transitions: transitionStore,
    transport: api,
    connectivity,
    clock,
    logger,
  })

  const delivery = new DeliveryService({
    proofs: proofStore,
    files: new ExpoProofFileStore(),
    orders,
    clock,
  })

  const pointSync = new PointSyncService({
    store: pointStore,
    transport: api,
    device,
    clock,
    logger,
  })

  const transitionSync = new TransitionSyncService({
    queue: transitionStore,
    transport: api,
    cache: orderCache,
    clock,
    logger,
  })

  const coordinator = new SyncCoordinator({
    points: pointSync,
    transitions: transitionSync,
    connectivity,
    status,
    pointStore,
    transitionStore,
    ownerId: () => session.currentUserId(),
    clock,
    logger,
  })

  async function handleBackgroundSamples(samples: RawLocationSample[]): Promise<void> {
    const ownerId = session.currentUserId()
    if (!ownerId) {
      logger.warn('Muestras GPS recibidas sin sesión activa; no se persisten')
      return
    }

    let stored = 0
    for (const sample of samples) {
      const outcome = await recordSample.execute(ownerId, {
        lat: sample.lat,
        lon: sample.lon,
        recordedAt: sample.timestamp,
        speedKph: sample.speedMps == null ? null : sample.speedMps * 3.6,
        accuracyM: sample.accuracyM,
        heading: sample.heading,
      })
      if (outcome.stored) stored += 1
      else logger.debug('Muestra GPS descartada', { code: outcome.code, message: outcome.message })
    }

    logger.info('Muestras GPS persistidas', { received: samples.length, stored })
    await coordinator.refreshStatus()
    if (stored > 0) await coordinator.syncNow('capture')
  }

  const tracker = new ExpoLocationTracker({
    onSamples: handleBackgroundSamples,
    policy: GPS_POLICY,
    logger,
  })

  const unsubscribeSession = session.subscribe((snapshot) => {
    void coordinator.refreshStatus()
    if (snapshot.status === 'authenticated') {
      void coordinator.syncNow('manual')
    } else if (snapshot.status === 'anonymous' || snapshot.status === 'expired') {
      void tracker.stop().catch((error: unknown) => {
        logger.warn('No se pudo detener el GPS al cerrar sesión', {
          message: error instanceof Error ? error.message : String(error),
        })
      })
    }
  })

  coordinator.start()

  let disposed = false
  return {
    database,
    session,
    api,
    connectivity,
    status,
    coordinator,
    orders,
    delivery,
    tracker,
    handleBackgroundSamples,
    dispose: () => {
      if (disposed) return
      disposed = true
      unsubscribeSession()
      coordinator.stop()
      void tracker.stop()
    },
  }
}

let servicesPromise: Promise<AppServices> | null = null

/** Composición única de dependencias (también válida en arranque headless). */
export function getServices(): Promise<AppServices> {
  if (!servicesPromise) {
    servicesPromise = createServices().catch((error) => {
      servicesPromise = null
      throw error
    })
  }
  return servicesPromise
}
