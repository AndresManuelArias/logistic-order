import { describe, expect, it } from 'vitest'

import { AppError } from '../../domain/errors'
import { selectDayOrders } from '../../domain/order'
import {
  FakeClock,
  FakeConnectivity,
  FakeOrderServer,
  MemoryOrderCache,
  MemoryTransitionQueue,
  makeOrder,
  silentLogger,
} from '../../testing/fakes'
import { OrderService } from './order-service'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000
const ORDER_ID = '22222222-2222-4222-8222-222222222222'

async function createHarness(options: { online?: boolean } = {}) {
  const clock = new FakeClock(NOW)
  const cache = new MemoryOrderCache()
  const transitions = new MemoryTransitionQueue()
  const order = makeOrder({ id: ORDER_ID, status: 'CREATED' })
  const server = new FakeOrderServer([order])
  const connectivity = new FakeConnectivity(options.online ?? true)

  const service = new OrderService({
    cache,
    transitions,
    transport: server,
    connectivity,
    clock,
    logger: silentLogger,
  })

  return { clock, cache, transitions, server, connectivity, service, order }
}

describe('OrderService', () => {
  it('devuelve la caché local sin tocar la red cuando está offline', async () => {
    const harness = await createHarness({ online: false })
    await harness.cache.upsert(OWNER, harness.order, NOW)

    const result = await harness.service.load(OWNER)

    expect(result.offline).toBe(true)
    expect(result.views).toHaveLength(1)
    expect(result.views[0]?.order.id).toBe(ORDER_ID)
    expect(harness.server.calls).toHaveLength(0)
  })

  it('refresca desde el servidor y actualiza la caché al recuperar red', async () => {
    const harness = await createHarness()
    const fresh = makeOrder({ id: ORDER_ID, status: 'IN_TRANSIT' })
    harness.server.seed(fresh)

    const result = await harness.service.load(OWNER)

    expect(result.offline).toBe(false)
    expect(result.views[0]?.order.status).toBe('IN_TRANSIT')
    expect((await harness.cache.get(OWNER, ORDER_ID))?.order.status).toBe('IN_TRANSIT')
    expect(harness.server.calls).toEqual(['list'])
  })

  it('no pierde la caché si el refresh remoto falla', async () => {
    const harness = await createHarness()
    await harness.cache.upsert(OWNER, harness.order, NOW)
    harness.server.failNext(new AppError('NETWORK', 'sin red'))

    const result = await harness.service.load(OWNER)

    expect(result.views).toHaveLength(1)
    expect(result.offline).toBe(true)
    expect(result.error?.code).toBe('NETWORK')
  })

  it('muestra de forma optimista el estado de una transición pendiente', async () => {
    const harness = await createHarness({ online: false })
    await harness.cache.upsert(OWNER, harness.order, NOW)
    await harness.service.requestTransition(OWNER, ORDER_ID, 'IN_TRANSIT')

    const views = await harness.service.listCached(OWNER)
    const view = views[0]

    expect(view?.order.status).toBe('CREATED')
    expect(view?.effectiveStatus).toBe('IN_TRANSIT')
    expect(view?.pendingTransition?.targetStatus).toBe('IN_TRANSIT')
    expect(harness.service.nextAction(view!)?.target).toBe('DELIVERED')
  })

  it('deduplica una transición repetida al mismo objetivo', async () => {
    const harness = await createHarness({ online: false })
    await harness.cache.upsert(OWNER, harness.order, NOW)

    const first = await harness.service.requestTransition(OWNER, ORDER_ID, 'IN_TRANSIT')
    const second = await harness.service.requestTransition(OWNER, ORDER_ID, 'IN_TRANSIT')

    expect(first.deduped).toBe(false)
    expect(second.deduped).toBe(true)
    expect(harness.transitions.all()).toHaveLength(1)
  })

  it('no encola una transición ilegal para el estado actual', async () => {
    const harness = await createHarness({ online: false })
    await harness.cache.upsert(OWNER, harness.order, NOW)

    await expect(
      harness.service.requestTransition(OWNER, ORDER_ID, 'DELIVERED'),
    ).rejects.toMatchObject({ code: 'CONFLICT' })
    expect(harness.transitions.all()).toHaveLength(0)
  })

  it('rechaza operar sobre una orden que no está en el dispositivo', async () => {
    const harness = await createHarness({ online: false })

    await expect(
      harness.service.requestTransition(OWNER, 'desconocida', 'IN_TRANSIT'),
    ).rejects.toMatchObject({ code: 'VALIDATION' })
  })

  it('no repite la transición si el servidor ya está en el objetivo', async () => {
    const harness = await createHarness({ online: false })
    await harness.cache.upsert(OWNER, makeOrder({ id: ORDER_ID, status: 'IN_TRANSIT' }), NOW)

    const result = await harness.service.requestTransition(OWNER, ORDER_ID, 'IN_TRANSIT')

    expect(result).toMatchObject({ transition: null, deduped: true })
    expect(harness.transitions.all()).toHaveLength(0)
  })

  it('aísla las órdenes por conductor', async () => {
    const harness = await createHarness({ online: false })
    await harness.cache.upsert(OWNER, harness.order, NOW)

    expect(await harness.service.get('driver-2', ORDER_ID)).toBeNull()
    expect(await harness.service.listCached('driver-2')).toHaveLength(0)
  })

  it('ordena las órdenes del día con el criterio de dominio', async () => {
    const harness = await createHarness({ online: false })
    const created = makeOrder({ id: 'a', status: 'CREATED', placed_at: '2026-10-28T08:00:00.000Z' })
    const delivered = makeOrder({
      id: 'b',
      status: 'DELIVERED',
      placed_at: '2026-10-28T09:00:00.000Z',
    })
    await harness.cache.upsertMany(OWNER, [delivered, created], NOW)

    const views = await harness.service.listCached(OWNER)
    const selected = selectDayOrders(
      views.map((view) => view.order),
      new Date('2026-10-28T12:00:00.000Z'),
    )

    expect(selected.map((order) => order.id)).toEqual(['a', 'b'])
  })
})
