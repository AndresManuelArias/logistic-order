import { describe, expect, it } from 'vitest'

import { SYNC_POLICY, type SyncPolicy } from '../../config/policies'
import { AppError } from '../../domain/errors'
import type { SyncTrigger } from '../../domain/sync'
import {
  FakeClock,
  FakeOrderServer,
  MemoryOrderCache,
  MemoryTransitionQueue,
  makeOrder,
  silentLogger,
} from '../../testing/fakes'
import { TransitionSyncService } from './transition-sync-service'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000
const ORDER_ID = '22222222-2222-4222-8222-222222222222'

const POLICY: SyncPolicy = {
  ...SYNC_POLICY,
  maxTransitionsPerRun: 10,
  retry: { baseMs: 1_000, maxMs: 8_000, jitterRatio: 0 },
}

const CONTEXT = { ownerId: OWNER, trigger: 'manual' as SyncTrigger }

async function createHarness(policy: SyncPolicy = POLICY) {
  const clock = new FakeClock(NOW)
  const queue = new MemoryTransitionQueue()
  const cache = new MemoryOrderCache()
  const order = makeOrder({ id: ORDER_ID, status: 'CREATED' })
  const server = new FakeOrderServer([order])
  await cache.upsert(OWNER, order, NOW)

  const service = new TransitionSyncService({
    queue,
    transport: server,
    cache,
    clock,
    logger: silentLogger,
    policy,
  })

  return { clock, queue, cache, server, service, order }
}

async function enqueue(
  queue: MemoryTransitionQueue,
  options: {
    id?: string
    target?: 'IN_TRANSIT' | 'DELIVERED' | 'CANCELLED'
    expected?: 'CREATED' | 'IN_TRANSIT'
    at?: number
  } = {},
) {
  const id = options.id ?? 'tr_1'
  return queue.enqueue(
    OWNER,
    {
      id,
      orderId: ORDER_ID,
      expectedStatus: options.expected ?? 'CREATED',
      targetStatus: options.target ?? 'IN_TRANSIT',
      idempotencyKey: `order:${ORDER_ID}:${options.target ?? 'IN_TRANSIT'}:${id}`,
    },
    options.at ?? NOW,
  )
}

describe('TransitionSyncService', () => {
  it('no hace nada si no hay transiciones pendientes', async () => {
    const { service, server } = await createHarness()
    const result = await service.run(CONTEXT)
    expect(result).toMatchObject({ outcome: 'idle', applied: 0, rejected: 0 })
    expect(server.calls).toHaveLength(0)
  })

  it('aplica una transición y actualiza la caché', async () => {
    const { service, queue, cache, server } = await createHarness()
    await enqueue(queue)

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'synced', applied: 1, rejected: 0 })
    expect(server.orders.get(ORDER_ID)?.status).toBe('IN_TRANSIT')
    expect((await cache.get(OWNER, ORDER_ID))?.order.status).toBe('IN_TRANSIT')
    expect(queue.stateOf('tr_1')).toBe('applied')
    expect(server.calls).toEqual([`patch:${ORDER_ID}:IN_TRANSIT`])
  })

  it('reconcilia sin reenviar si el servidor ya aplicó un intento ambiguo', async () => {
    const { service, queue, server, clock } = await createHarness()
    await enqueue(queue)
    await queue.markRetry(OWNER, 'tr_1', NOW, 'NETWORK: timeout')
    server.orders.set(ORDER_ID, makeOrder({ id: ORDER_ID, status: 'IN_TRANSIT' }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'synced', applied: 1 })
    expect(server.calls).toEqual([`get:${ORDER_ID}`])
    expect(queue.stateOf('tr_1')).toBe('applied')
    expect(clock.now()).toBe(NOW)
  })

  it('da por aplicada la transición si un 409 revela que el servidor ya está en el objetivo', async () => {
    const { service, queue, server } = await createHarness()
    await enqueue(queue)
    server.orders.set(ORDER_ID, makeOrder({ id: ORDER_ID, status: 'IN_TRANSIT' }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'synced', applied: 1, rejected: 0 })
    expect(server.calls).toEqual([`patch:${ORDER_ID}:IN_TRANSIT`, `get:${ORDER_ID}`])
    expect(queue.stateOf('tr_1')).toBe('applied')
  })

  it('rechaza la transición si el estado cambió fuera de la app', async () => {
    const { service, queue, cache, server } = await createHarness()
    await enqueue(queue)
    server.orders.set(ORDER_ID, makeOrder({ id: ORDER_ID, status: 'CANCELLED' }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'synced', applied: 0, rejected: 1 })
    expect(queue.stateOf('tr_1')).toBe('rejected')
    expect((await cache.get(OWNER, ORDER_ID))?.order.status).toBe('CANCELLED')
  })

  it('reintenta con backoff ante errores transitorios y conserva la transición', async () => {
    const { service, queue, server, clock } = await createHarness()
    await enqueue(queue)
    server.failNext(new AppError('SERVER', 'boom', { status: 503 }))

    const first = await service.run(CONTEXT)

    expect(first).toMatchObject({ outcome: 'retry', applied: 0 })
    expect(first.nextRetryAt).toBe(clock.now() + 1_000)
    expect(queue.stateOf('tr_1')).toBe('pending')
    expect(queue.all()[0]?.attempts).toBe(1)

    clock.advance(1_000)
    const second = await service.run(CONTEXT)

    expect(second).toMatchObject({ outcome: 'synced', applied: 1 })
    expect(queue.stateOf('tr_1')).toBe('applied')
    expect(server.calls).toEqual([`patch:${ORDER_ID}:IN_TRANSIT`, `get:${ORDER_ID}`, `patch:${ORDER_ID}:IN_TRANSIT`])
  })

  it('pausa ante 401 sin descartar la transición', async () => {
    const { service, queue, server } = await createHarness()
    await enqueue(queue)
    server.failNext(new AppError('UNAUTHENTICATED', 'expirado', { status: 401 }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'paused', errorCode: 'UNAUTHENTICATED' })
    expect(queue.stateOf('tr_1')).toBe('pending')
    expect(queue.all()[0]?.attempts).toBe(0)
  })

  it('rechaza de forma permanente una orden inexistente', async () => {
    const { service, queue, server } = await createHarness()
    await enqueue(queue)
    server.failNext(new AppError('NOT_FOUND', 'no existe', { status: 404 }))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'synced', rejected: 1 })
    expect(queue.stateOf('tr_1')).toBe('rejected')
  })

  it('procesa dos transiciones encoladas en orden', async () => {
    const { service, queue, server } = await createHarness()
    await enqueue(queue, { id: 'tr_1', target: 'IN_TRANSIT', expected: 'CREATED', at: NOW })
    await enqueue(queue, { id: 'tr_2', target: 'DELIVERED', expected: 'IN_TRANSIT', at: NOW + 1 })

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ applied: 2, rejected: 0 })
    expect(server.calls).toEqual([
      `patch:${ORDER_ID}:IN_TRANSIT`,
      `patch:${ORDER_ID}:DELIVERED`,
    ])
    expect(server.orders.get(ORDER_ID)?.status).toBe('DELIVERED')
    expect(queue.stateOf('tr_1')).toBe('applied')
    expect(queue.stateOf('tr_2')).toBe('applied')
  })

  it('respeta el máximo de transiciones por ciclo', async () => {
    const policy: SyncPolicy = { ...POLICY, maxTransitionsPerRun: 1 }
    const { service, queue, server } = await createHarness(policy)
    await enqueue(queue, { id: 'tr_1', target: 'IN_TRANSIT', expected: 'CREATED', at: NOW })
    await enqueue(queue, { id: 'tr_2', target: 'DELIVERED', expected: 'IN_TRANSIT', at: NOW + 1 })

    const result = await service.run(CONTEXT)

    expect(result.applied).toBe(1)
    expect(server.orders.get(ORDER_ID)?.status).toBe('IN_TRANSIT')
    expect(queue.stateOf('tr_1')).toBe('applied')
    expect(queue.stateOf('tr_2')).toBe('pending')
  })

  it('reintenta si falla el GET de reconciliación', async () => {
    const { service, queue, server, clock } = await createHarness()
    await enqueue(queue)
    await queue.markRetry(OWNER, 'tr_1', NOW, 'NETWORK: timeout')
    server.failNext(new AppError('NETWORK', 'sin red'))

    const result = await service.run(CONTEXT)

    expect(result).toMatchObject({ outcome: 'retry' })
    expect(queue.stateOf('tr_1')).toBe('pending')
    expect(queue.all()[0]?.attempts).toBe(2)
    expect(result.nextRetryAt).toBe(clock.now() + 2_000)
  })
})
