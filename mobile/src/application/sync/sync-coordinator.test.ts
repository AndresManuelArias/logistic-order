import { describe, expect, it } from 'vitest'

import { SYNC_POLICY } from '../../config/policies'
import { AppError } from '../../domain/errors'
import {
  FakeClock,
  FakeConnectivity,
  FakeDeviceMetadata,
  FakeOrderServer,
  FakeTelemetryServer,
  MemoryOrderCache,
  MemoryPointQueue,
  MemoryTransitionQueue,
  makeOrder,
  silentLogger,
} from '../../testing/fakes'
import { PointSyncService } from './point-sync-service'
import { SyncCoordinator } from './sync-coordinator'
import { SyncStatusStore } from './sync-status-store'
import { TransitionSyncService } from './transition-sync-service'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 20))
}

async function createHarness(options: { online?: boolean; owner?: string | null } = {}) {
  const clock = new FakeClock(NOW)
  const pointStore = new MemoryPointQueue()
  const transitionStore = new MemoryTransitionQueue()
  const cache = new MemoryOrderCache()
  const server = new FakeTelemetryServer(clock)
  const orderServer = new FakeOrderServer()
  const device = new FakeDeviceMetadata('dev-test')
  const connectivity = new FakeConnectivity(options.online ?? true)
  const status = new SyncStatusStore()
  let ownerId: string | null = options.owner === undefined ? OWNER : options.owner

  const points = new PointSyncService({
    store: pointStore,
    transport: server,
    device,
    clock,
    logger: silentLogger,
    policy: SYNC_POLICY,
  })

  const transitions = new TransitionSyncService({
    queue: transitionStore,
    transport: orderServer,
    cache,
    clock,
    logger: silentLogger,
    policy: SYNC_POLICY,
  })

  const coordinator = new SyncCoordinator({
    points,
    transitions,
    connectivity,
    status,
    pointStore,
    transitionStore,
    ownerId: () => ownerId,
    clock,
    logger: silentLogger,
  })

  return {
    clock,
    pointStore,
    transitionStore,
    cache,
    server,
    orderServer,
    connectivity,
    status,
    coordinator,
    setOwner: (value: string | null) => {
      ownerId = value
    },
  }
}

async function seedPoints(store: MemoryPointQueue, count: number): Promise<void> {
  for (let index = 0; index < count; index += 1) {
    const seq = index + 1
    await store.enqueue({
      id: `pt_${seq}`,
      ownerId: OWNER,
      deviceSeq: seq,
      recordedAt: NOW + index * 15_000,
      lat: 4.7108,
      lon: -74.0721,
      accuracyM: 5,
    })
  }
}

describe('SyncCoordinator', () => {
  it('no envía nada sin conexión y conserva los pendientes', async () => {
    const harness = await createHarness({ online: false })
    await seedPoints(harness.pointStore, 3)

    const result = await harness.coordinator.syncNow('manual')

    expect(result.outcome).toBe('offline')
    expect(harness.server.requests).toHaveLength(0)
    expect(harness.status.getSnapshot()).toMatchObject({
      online: false,
      pendingPoints: 3,
      lastSyncedAt: null,
    })
  })

  it('no sincroniza sin sesión y no filtra datos de otros conductores', async () => {
    const harness = await createHarness({ owner: null })
    await seedPoints(harness.pointStore, 2)

    const result = await harness.coordinator.syncNow('manual')

    expect(result.outcome).toBe('idle')
    expect(harness.server.requests).toHaveLength(0)
    expect(harness.status.getSnapshot().pendingPoints).toBe(0)
  })

  it('sincroniza puntos y transiciones y actualiza el estado', async () => {
    const harness = await createHarness()
    await seedPoints(harness.pointStore, 3)

    const order = makeOrder()
    await harness.cache.upsert(OWNER, order, NOW)
    harness.orderServer.seed(order)
    await harness.transitionStore.enqueue(
      OWNER,
      {
        id: 'tr_1',
        orderId: order.id,
        expectedStatus: 'CREATED',
        targetStatus: 'IN_TRANSIT',
        idempotencyKey: `order:${order.id}:IN_TRANSIT:tr_1`,
      },
      NOW,
    )

    const result = await harness.coordinator.syncNow('manual')

    expect(result).toMatchObject({
      outcome: 'synced',
      pointsSynced: 3,
      transitionsApplied: 1,
      batchesSent: 1,
    })
    expect(harness.status.getSnapshot()).toMatchObject({
      online: true,
      syncing: false,
      pendingPoints: 0,
      pendingTransitions: 0,
      lastSyncedAt: NOW,
      lastError: null,
      sessionExpired: false,
    })
    expect(harness.orderServer.orders.get(order.id)?.status).toBe('IN_TRANSIT')
  })

  it('single-flight: dos disparadores simultáneos envían un solo lote', async () => {
    const harness = await createHarness()
    await seedPoints(harness.pointStore, 2)

    let release!: () => void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const original = harness.server.sendBulk.bind(harness.server)
    let calls = 0
    harness.server.sendBulk = async (input) => {
      calls += 1
      await gate
      return original(input)
    }

    const first = harness.coordinator.syncNow('manual')
    const second = harness.coordinator.syncNow('online')
    release()
    const [a, b] = await Promise.all([first, second])

    expect(calls).toBe(1)
    expect(a).toEqual(b)
    expect(harness.server.storedCount).toBe(2)
  })

  it('marca la sesión expirada y no envía transiciones tras un 401', async () => {
    const harness = await createHarness()
    await seedPoints(harness.pointStore, 2)
    const order = makeOrder()
    await harness.cache.upsert(OWNER, order, NOW)
    await harness.transitionStore.enqueue(
      OWNER,
      {
        id: 'tr_1',
        orderId: order.id,
        expectedStatus: 'CREATED',
        targetStatus: 'IN_TRANSIT',
        idempotencyKey: `order:${order.id}:IN_TRANSIT:tr_1`,
      },
      NOW,
    )
    harness.server.failNext(new AppError('UNAUTHENTICATED', 'expirado', { status: 401 }))

    const result = await harness.coordinator.syncNow('manual')

    expect(result.outcome).toBe('paused')
    expect(harness.status.getSnapshot()).toMatchObject({
      sessionExpired: true,
      pendingPoints: 2,
      pendingTransitions: 1,
    })
    expect(harness.orderServer.calls).toHaveLength(0)
  })

  it('reacciona a la reconexión y al cambio de conectividad', async () => {
    const harness = await createHarness({ online: false })
    await seedPoints(harness.pointStore, 2)
    harness.coordinator.start()
    await flush()

    expect(harness.status.getSnapshot().online).toBe(false)
    expect(harness.server.requests).toHaveLength(0)

    harness.connectivity.setOnline(true)
    await flush()

    expect(harness.status.getSnapshot().online).toBe(true)
    expect(harness.status.getSnapshot().pendingPoints).toBe(0)
    expect(harness.server.storedCount).toBe(2)
    harness.coordinator.stop()
  })

  it('expone los puntos rechazados en el estado', async () => {
    const harness = await createHarness()
    await seedPoints(harness.pointStore, 2)
    harness.server.failNext(new AppError('VALIDATION', 'inválido', { status: 422 }))

    await harness.coordinator.syncNow('manual')

    expect(harness.status.getSnapshot()).toMatchObject({
      pendingPoints: 0,
      rejectedPoints: 2,
      lastError: 'inválido',
    })
  })
})
