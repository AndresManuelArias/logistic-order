import { describe, expect, it } from 'vitest'

import { AppError } from '../../domain/errors'
import {
  FakeClock,
  FakeConnectivity,
  FakeOrderServer,
  FakeProofFileStore,
  MemoryOrderCache,
  MemoryProofStore,
  MemoryTransitionQueue,
  makeOrder,
  silentLogger,
} from '../../testing/fakes'
import { OrderService } from '../orders/order-service'
import { DeliveryService } from './delivery-service'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000
const ORDER_ID = '22222222-2222-4222-8222-222222222222'

async function createHarness() {
  const clock = new FakeClock(NOW)
  const cache = new MemoryOrderCache()
  const transitions = new MemoryTransitionQueue()
  const server = new FakeOrderServer([makeOrder({ id: ORDER_ID, status: 'IN_TRANSIT' })])
  const connectivity = new FakeConnectivity(false)
  const proofs = new MemoryProofStore()
  const files = new FakeProofFileStore()

  const orders = new OrderService({
    cache,
    transitions,
    transport: server,
    connectivity,
    clock,
    logger: silentLogger,
  })
  await cache.upsert(OWNER, makeOrder({ id: ORDER_ID, status: 'IN_TRANSIT' }), NOW)

  const delivery = new DeliveryService({ proofs, files, orders, clock })

  return { clock, cache, transitions, server, proofs, files, orders, delivery }
}

describe('DeliveryService', () => {
  it('guarda la prueba, copia la foto y encola DELIVERED', async () => {
    const harness = await createHarness()

    const result = await harness.delivery.completeDelivery(OWNER, {
      orderId: ORDER_ID,
      receiverName: '  Ana Pérez  ',
      notes: '  Entregado en portería  ',
      photoUri: 'file:///cache/photo.jpg',
    })

    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.proof).toMatchObject({
      orderId: ORDER_ID,
      receiverName: 'Ana Pérez',
      notes: 'Entregado en portería',
      uploadState: 'pending_backend',
    })
    expect(result.proof.photoUri).toContain('delivery-proofs')
    expect(result.transition?.targetStatus).toBe('DELIVERED')
    expect(harness.files.calls).toHaveLength(1)
    expect(harness.proofs.records).toHaveLength(1)
    expect(harness.transitions.all()[0]?.targetStatus).toBe('DELIVERED')
  })

  it('valida el receptor antes de guardar nada', async () => {
    const harness = await createHarness()

    const result = await harness.delivery.completeDelivery(OWNER, {
      orderId: ORDER_ID,
      receiverName: '   ',
    })

    expect(result).toMatchObject({ ok: false, code: 'MISSING_RECEIVER' })
    expect(harness.proofs.records).toHaveLength(0)
    expect(harness.files.calls).toHaveLength(0)
    expect(harness.transitions.all()).toHaveLength(0)
  })

  it('no guarda evidencia si la copia de la foto falla', async () => {
    const harness = await createHarness()
    harness.files.failure = new AppError('UNKNOWN', 'disco lleno')

    const result = await harness.delivery.completeDelivery(OWNER, {
      orderId: ORDER_ID,
      receiverName: 'Ana Pérez',
      photoUri: 'file:///cache/photo.jpg',
    })

    expect(result).toMatchObject({ ok: false, code: 'PHOTO_COPY_FAILED' })
    expect(harness.proofs.records).toHaveLength(0)
  })

  it('conserva la prueba aunque la transición de estado no pueda encolarse', async () => {
    const harness = await createHarness()
    await harness.cache.upsert(OWNER, makeOrder({ id: ORDER_ID, status: 'CANCELLED' }), NOW)

    const result = await harness.delivery.completeDelivery(OWNER, {
      orderId: ORDER_ID,
      receiverName: 'Ana Pérez',
    })

    expect(result).toMatchObject({ ok: false })
    if (result.ok) return
    expect(result.proof?.orderId).toBe(ORDER_ID)
    expect(harness.proofs.records).toHaveLength(1)
  })

  it('lista las pruebas por orden y conductor', async () => {
    const harness = await createHarness()
    await harness.delivery.completeDelivery(OWNER, {
      orderId: ORDER_ID,
      receiverName: 'Ana Pérez',
    })

    expect(await harness.delivery.listProofs(OWNER, ORDER_ID)).toHaveLength(1)
    expect(await harness.delivery.listProofs('driver-2', ORDER_ID)).toHaveLength(0)
  })

  it('declara que la subida de POD no está soportada por el contrato actual', async () => {
    const harness = await createHarness()
    expect(harness.delivery.uploadSupported).toBe(false)
  })
})
