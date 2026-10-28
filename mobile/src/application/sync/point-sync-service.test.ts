import { describe, expect, it } from 'vitest'

import { SYNC_POLICY, type SyncPolicy } from '../../config/policies'
import { AppError } from '../../domain/errors'
import type { SyncTrigger } from '../../domain/sync'
import {
  FakeClock,
  FakeDeviceMetadata,
  FakeTelemetryServer,
  MemoryPointQueue,
  silentLogger,
} from '../../testing/fakes'
import { PointSyncService } from './point-sync-service'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000

const POLICY: SyncPolicy = {
  ...SYNC_POLICY,
  maxBatchSize: 3,
  maxBatchesPerRun: 10,
  retry: { baseMs: 1_000, maxMs: 8_000, jitterRatio: 0 },
}

function createHarness(policy: SyncPolicy = POLICY) {
  const clock = new FakeClock(NOW)
  const store = new MemoryPointQueue()
  const server = new FakeTelemetryServer(clock)
  const device = new FakeDeviceMetadata('dev-test')
  const service = new PointSyncService({
    store,
    transport: server,
    device,
    clock,
    logger: silentLogger,
    policy,
  })
  return { clock, store, server, device, service }
}

async function seed(store: MemoryPointQueue, count: number, startSeq = 1): Promise<void> {
  for (let index = 0; index < count; index += 1) {
    const seq = startSeq + index
    await store.enqueue({
      id: `pt_${seq}`,
      ownerId: OWNER,
      deviceSeq: seq,
      recordedAt: NOW + index * 15_000,
      lat: 4.7108,
      lon: -74.0721,
      speedKph: 18.5,
      accuracyM: 6.2,
      heading: 90,
    })
  }
}

const CONTEXT = { ownerId: OWNER, trigger: 'manual' as SyncTrigger }

describe('PointSyncService', () => {
  it('no envía nada si no hay puntos pendientes', async () => {
    const { service, server } = createHarness()
    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'idle', pointsSynced: 0, batchesSent: 0 })
    expect(server.requests).toHaveLength(0)
  })

  it('envía todos los pendientes respetando el tamaño de lote', async () => {
    const { service, server, store } = createHarness()
    await seed(store, 7)

    const result = await service.run(CONTEXT)

    expect(result.outcome).toBe('synced')
    expect(result.pointsSynced).toBe(7)
    expect(result.batchesSent).toBe(3)
    expect(server.requests.map((request) => request.points.length)).toEqual([3, 3, 1])
    expect(new Set(server.requests.map((r) => r.idempotencyKey)).size).toBe(3)
    expect(await store.counts(OWNER)).toEqual({ pending: 0, rejected: 0 })
    expect(server.storedCount).toBe(7)
    expect(server.requests[0]?.deviceId).toBe('dev-test')
  })

  it('incluye device_seq, timestamp ISO y opcionales en el payload', async () => {
    const { service, server, store } = createHarness()
    await store.enqueue({
      id: 'pt_1',
      ownerId: OWNER,
      deviceSeq: 9,
      recordedAt: NOW,
      lat: 4.71,
      lon: -74.07,
      speedKph: 21.4,
      accuracyM: 5.5,
      heading: 0,
      vehicleId: 'vehicle-1',
      orderId: 'order-1',
    })

    await service.run(CONTEXT)

    expect(server.requests[0]?.points[0]).toEqual({
      time: new Date(NOW).toISOString(),
      lat: 4.71,
      lon: -74.07,
      device_seq: 9,
      speed_kph: 21.4,
      accuracy_m: 5.5,
      heading: 0,
      vehicle_id: 'vehicle-1',
      order_id: 'order-1',
    })
  })

  it('limita los lotes por ciclo y deja el resto pendiente', async () => {
    const policy: SyncPolicy = { ...POLICY, maxBatchSize: 2, maxBatchesPerRun: 2 }
    const { service, server, store } = createHarness(policy)
    await seed(store, 10)

    const result = await service.run(CONTEXT)

    expect(server.requests).toHaveLength(2)
    expect(result.pointsSynced).toBe(4)
    expect((await store.counts(OWNER)).pending).toBe(6)
  })

  it('reutiliza la misma Idempotency-Key tras un fallo ambiguo (sin duplicar)', async () => {
    const { service, server, store, clock } = createHarness()
    await seed(store, 3)

    // El servidor procesa el lote pero la respuesta se pierde.
    server.failNext(new AppError('NETWORK', 'sin red'), true)
    const first = await service.run(CONTEXT)

    expect(first).toMatchObject({ outcome: 'retry', pointsSynced: 0 })
    expect(server.storedCount).toBe(3)
    const open = await store.openBatch(OWNER)
    expect(open).not.toBeNull()
    expect(open?.attempts).toBe(1)
    expect(open?.nextAttemptAt).toBe(clock.now() + 1_000)
    expect((await store.counts(OWNER)).pending).toBe(3)
    const firstKey = server.requests[0]?.idempotencyKey
    expect(firstKey).toBeDefined()

    // Reintento inmediato: no se envía (backoff).
    const blocked = await service.run(CONTEXT)
    expect(blocked.outcome).toBe('retry')
    expect(server.requests).toHaveLength(1)

    clock.advance(1_000)

    // Simula reinicio de la app: nuevo servicio sobre el mismo almacén.
    const restarted = new PointSyncService({
      store,
      transport: server,
      device: new FakeDeviceMetadata('dev-test'),
      clock,
      logger: silentLogger,
      policy: POLICY,
    })
    const second = await restarted.run(CONTEXT)

    expect(second.outcome).toBe('synced')
    expect(second.pointsSynced).toBe(3)
    expect(server.requests).toHaveLength(2)
    expect(server.requests[1]?.idempotencyKey).toBe(firstKey)
    expect(server.receiptCount).toBe(1)
    expect(server.storedCount).toBe(3)
    expect((await store.counts(OWNER)).pending).toBe(0)
  })

  it('no abre otro lote mientras el primero espera backoff', async () => {
    const { service, server, store, clock } = createHarness()
    await seed(store, 3)
    server.failNext(new AppError('SERVER', 'boom', { status: 503 }))
    await service.run(CONTEXT)

    await seed(store, 2, 10)
    const result = await service.run(CONTEXT)

    expect(result.outcome).toBe('retry')
    expect(server.requests).toHaveLength(1)
    expect((await store.counts(OWNER)).pending).toBe(5)

    clock.advance(1_000)
    const after = await service.run(CONTEXT)
    expect(after.outcome).toBe('synced')
    // El segundo ciclo vacía el lote abierto y el nuevo.
    expect(server.storedCount).toBe(5)
  })

  it('pone en cuarentena un lote inválido y continúa con el siguiente', async () => {
    const { service, server, store } = createHarness()
    await seed(store, 5)
    server.failNext(new AppError('VALIDATION', 'payload inválido', { status: 422 }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'synced', pointsRejected: 3, pointsSynced: 2 })
    expect(server.storedCount).toBe(2)
    expect(await store.counts(OWNER)).toEqual({ pending: 0, rejected: 3 })
  })

  it('pausa sin descartar datos cuando falta autenticación', async () => {
    const { service, server, store } = createHarness()
    await seed(store, 3)
    server.failNext(new AppError('UNAUTHENTICATED', 'token expirado', { status: 401 }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({
      outcome: 'paused',
      errorCode: 'UNAUTHENTICATED',
      pointsRejected: 0,
    })
    expect((await store.counts(OWNER)).pending).toBe(3)
    const open = await store.openBatch(OWNER)
    expect(open?.attempts).toBe(0)
  })

  it('pausa ante un 403 sin quemar reintentos', async () => {
    const { service, server, store } = createHarness()
    await seed(store, 2)
    server.failNext(new AppError('FORBIDDEN', 'sin permisos', { status: 403 }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'paused', errorCode: 'FORBIDDEN' })
    expect((await store.counts(OWNER)).pending).toBe(2)
    expect((await store.openBatch(OWNER))?.attempts).toBe(0)
  })

  it('respeta Retry-After del servidor', async () => {
    const { service, server, store, clock } = createHarness()
    await seed(store, 2)
    server.failNext(
      new AppError('RATE_LIMITED', 'demasiadas peticiones', {
        status: 429,
        retryAfterMs: 7_000,
      }),
    )

    const result = await service.run(CONTEXT)

    expect(result.outcome).toBe('retry')
    expect(result.nextRetryAt).toBe(clock.now() + 7_000)
    expect((await store.openBatch(OWNER))?.nextAttemptAt).toBe(clock.now() + 7_000)
  })

  it('aísla los puntos por conductor', async () => {
    const { service, server, store } = createHarness()
    await seed(store, 2)
    await store.enqueue({
      id: 'other_1',
      ownerId: 'driver-2',
      deviceSeq: 1,
      recordedAt: NOW,
      lat: 4.6,
      lon: -74.1,
    })

    await service.run(CONTEXT)

    expect(server.storedCount).toBe(2)
    expect((await store.counts('driver-2')).pending).toBe(1)
  })
})
