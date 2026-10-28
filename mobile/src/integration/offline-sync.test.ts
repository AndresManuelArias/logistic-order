import { describe, expect, it } from 'vitest'

import { SYNC_POLICY, GPS_POLICY } from '../config/policies'
import { AppError } from '../domain/errors'
import { RecordGpsSample } from '../application/telemetry/record-gps-sample'
import { PointSyncService } from '../application/sync/point-sync-service'
import { SyncCoordinator } from '../application/sync/sync-coordinator'
import { SyncStatusStore } from '../application/sync/sync-status-store'
import { TransitionSyncService } from '../application/sync/transition-sync-service'
import {
  FakeClock,
  FakeConnectivity,
  FakeDeviceMetadata,
  FakeOrderServer,
  FakeTelemetryServer,
  MemoryOrderCache,
  MemoryPointQueue,
  MemoryTransitionQueue,
  silentLogger,
} from '../testing/fakes'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000

const POLICY = {
  ...SYNC_POLICY,
  maxBatchSize: 2,
  retry: { baseMs: 1_000, maxMs: 8_000, jitterRatio: 0 },
}

/**
 * Escenario de aceptación F4: modo avión acumula puntos y, al reconectar, se
 * sincronizan una sola vez incluso si la respuesta de un lote se pierde.
 */
describe('integración offline → reconexión', () => {
  function createHarness() {
    const clock = new FakeClock(NOW)
    const connectivity = new FakeConnectivity(false)
    const pointStore = new MemoryPointQueue()
    const transitionStore = new MemoryTransitionQueue()
    const cache = new MemoryOrderCache()
    const device = new FakeDeviceMetadata('dev-e2e')
    const telemetry = new FakeTelemetryServer(clock)
    const orders = new FakeOrderServer()

    const recordSample = new RecordGpsSample({
      points: pointStore,
      device,
      clock,
      policy: GPS_POLICY,
    })

    const points = new PointSyncService({
      store: pointStore,
      transport: telemetry,
      device,
      clock,
      logger: silentLogger,
      policy: POLICY,
    })

    const transitions = new TransitionSyncService({
      queue: transitionStore,
      transport: orders,
      cache,
      clock,
      logger: silentLogger,
      policy: POLICY,
    })

    const status = new SyncStatusStore()
    const coordinator = new SyncCoordinator({
      points,
      transitions,
      connectivity,
      status,
      pointStore,
      transitionStore,
      ownerId: () => OWNER,
      clock,
      logger: silentLogger,
    })

    return {
      clock,
      connectivity,
      pointStore,
      cache,
      telemetry,
      orders,
      recordSample,
      coordinator,
      status,
    }
  }

  async function capture(
    harness: ReturnType<typeof createHarness>,
    count: number,
    startOffset = 0,
  ): Promise<void> {
    for (let index = 0; index < count; index += 1) {
      const result = await harness.recordSample.execute(OWNER, {
        lat: 4.7108 + index * 0.0001,
        lon: -74.0721,
        recordedAt: NOW + (startOffset + index) * 15_000,
        accuracyM: 6,
      })
      expect(result.stored).toBe(true)
    }
  }

  it('acumula en modo avión y sincroniza sin duplicar al reconectar', async () => {
    const harness = createHarness()

    await capture(harness, 5)
    const offline = await harness.coordinator.syncNow('manual')

    expect(offline.outcome).toBe('offline')
    expect(harness.telemetry.requests).toHaveLength(0)
    expect(harness.status.getSnapshot().pendingPoints).toBe(5)

    harness.connectivity.setOnline(true)
    const online = await harness.coordinator.syncNow('online')

    expect(online.outcome).toBe('synced')
    expect(online.pointsSynced).toBe(5)
    expect(harness.telemetry.storedCount).toBe(5)
    expect(harness.status.getSnapshot()).toMatchObject({
      online: true,
      pendingPoints: 0,
      rejectedPoints: 0,
    })

    // Un nuevo ciclo sin pendientes no vuelve a enviar nada.
    await harness.coordinator.syncNow('timer')
    expect(harness.telemetry.requests).toHaveLength(3)
  })

  it('un fallo ambiguo no duplica puntos tras reiniciar la app', async () => {
    const harness = createHarness()
    harness.connectivity.setOnline(true)
    await capture(harness, 2)

    harness.telemetry.failNext(new AppError('NETWORK', 'respuesta perdida'), true)
    const first = await harness.coordinator.syncNow('online')

    expect(first.outcome).toBe('retry')
    expect(harness.telemetry.storedCount).toBe(2)
    expect(harness.status.getSnapshot().pendingPoints).toBe(2)

    harness.clock.advance(1_000)
    const second = await harness.coordinator.syncNow('manual')

    expect(second.pointsSynced).toBe(2)
    expect(harness.telemetry.storedCount).toBe(2)
    expect(harness.telemetry.receiptCount).toBe(1)
    expect(harness.telemetry.requests[1]?.idempotencyKey).toBe(
      harness.telemetry.requests[0]?.idempotencyKey,
    )
    expect(harness.status.getSnapshot().pendingPoints).toBe(0)
  })

  it('mantiene separados los pendientes de dos conductores', async () => {
    const harness = createHarness()
    await harness.pointStore.enqueue({
      id: 'other_1',
      ownerId: 'driver-2',
      deviceSeq: 1,
      recordedAt: NOW + 100_000,
      lat: 4.6,
      lon: -74.1,
    })
    await capture(harness, 2)
    harness.connectivity.setOnline(true)

    await harness.coordinator.syncNow('online')

    expect(harness.telemetry.storedCount).toBe(2)
    expect(await harness.pointStore.counts('driver-2')).toMatchObject({ pending: 1 })
  })
})
