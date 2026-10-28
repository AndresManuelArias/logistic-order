import { describe, expect, it } from 'vitest'

import type { Order } from '../contracts/types'
import {
  ALLOWED_TRANSITIONS,
  canTransition,
  compareOrders,
  effectiveStatus,
  isActive,
  isOrderOfDay,
  isTerminal,
  nextDriverAction,
  selectDayOrders,
} from './order'

function order(overrides: Partial<Order> = {}): Order {
  return {
    id: 'order-1',
    external_ref: 'PED-1',
    store_id: 'store-1',
    status: 'CREATED',
    origin: { lat: 4.71, lon: -74.07 },
    destination: { lat: 4.65, lon: -74.05 },
    placed_at: '2026-10-28T15:00:00.000Z',
    ...overrides,
  } as Order
}

describe('máquina de estados', () => {
  it('refleja las transiciones permitidas por el backend', () => {
    expect(ALLOWED_TRANSITIONS.CREATED).toEqual(['IN_TRANSIT', 'CANCELLED'])
    expect(ALLOWED_TRANSITIONS.IN_TRANSIT).toEqual(['DELIVERED', 'CANCELLED'])
    expect(ALLOWED_TRANSITIONS.DELIVERED).toEqual([])
    expect(ALLOWED_TRANSITIONS.CANCELLED).toEqual([])
  })

  it('cancela y entrega sólo desde los estados correctos', () => {
    expect(canTransition('CREATED', 'IN_TRANSIT')).toBe(true)
    expect(canTransition('CREATED', 'DELIVERED')).toBe(false)
    expect(canTransition('IN_TRANSIT', 'DELIVERED')).toBe(true)
    expect(canTransition('DELIVERED', 'CANCELLED')).toBe(false)
    expect(canTransition('CREATED', 'CREATED')).toBe(false)
  })

  it('identifica estados activos y terminales', () => {
    expect(isActive('CREATED')).toBe(true)
    expect(isActive('IN_TRANSIT')).toBe(true)
    expect(isActive('DELIVERED')).toBe(false)
    expect(isTerminal('DELIVERED')).toBe(true)
    expect(isTerminal('CANCELLED')).toBe(true)
    expect(isTerminal('IN_TRANSIT')).toBe(false)
  })
})

describe('acciones del conductor', () => {
  it('ofrece iniciar y confirmar entrega en orden', () => {
    expect(nextDriverAction('CREATED')).toMatchObject({ target: 'IN_TRANSIT' })
    expect(nextDriverAction('IN_TRANSIT')).toMatchObject({ target: 'DELIVERED' })
  })

  it('no ofrece acciones en estados terminales', () => {
    expect(nextDriverAction('DELIVERED')).toBeNull()
    expect(nextDriverAction('CANCELLED')).toBeNull()
  })
})

describe('effectiveStatus', () => {
  it('aplica la transición pendiente de forma optimista', () => {
    expect(effectiveStatus(order({ status: 'CREATED' }), 'IN_TRANSIT')).toBe('IN_TRANSIT')
    expect(effectiveStatus(order({ status: 'IN_TRANSIT' }), 'DELIVERED')).toBe('DELIVERED')
  })

  it('ignora pendientes ilegales (estado cambiado en otro dispositivo)', () => {
    expect(effectiveStatus(order({ status: 'CANCELLED' }), 'IN_TRANSIT')).toBe('CANCELLED')
    expect(effectiveStatus(order({ status: 'CREATED' }), 'DELIVERED')).toBe('CREATED')
    expect(effectiveStatus(order({ status: 'CREATED' }), null)).toBe('CREATED')
  })
})

describe('órdenes del día', () => {
  const now = new Date(2026, 9, 28, 12, 0, 0) // 28-oct-2026 12:00 local

  it('incluye activas aunque se hayan creado ayer', () => {
    const yesterday = new Date(2026, 9, 27, 9, 0, 0)
    expect(
      isOrderOfDay(order({ status: 'IN_TRANSIT', placed_at: yesterday.toISOString() }), now),
    ).toBe(true)
  })

  it('incluye finalizadas de hoy y excluye finalizadas de ayer', () => {
    const today = new Date(2026, 9, 28, 9, 0, 0)
    const yesterday = new Date(2026, 9, 27, 23, 0, 0)
    expect(
      isOrderOfDay(order({ status: 'DELIVERED', placed_at: today.toISOString() }), now),
    ).toBe(true)
    expect(
      isOrderOfDay(order({ status: 'DELIVERED', placed_at: yesterday.toISOString() }), now),
    ).toBe(false)
  })

  it('descarta fechas inválidas de órdenes finalizadas', () => {
    expect(isOrderOfDay(order({ status: 'CANCELLED', placed_at: 'no-date' }), now)).toBe(false)
  })

  it('ordena activas por antigüedad y finalizadas por recencia', () => {
    const activeOld = order({ id: 'a', status: 'IN_TRANSIT', placed_at: new Date(2026, 9, 28, 8).toISOString() })
    const activeNew = order({ id: 'b', status: 'CREATED', placed_at: new Date(2026, 9, 28, 10).toISOString() })
    const doneNew = order({ id: 'c', status: 'DELIVERED', placed_at: new Date(2026, 9, 28, 11).toISOString() })
    const doneOld = order({ id: 'd', status: 'DELIVERED', placed_at: new Date(2026, 9, 28, 9).toISOString() })

    expect(selectDayOrders([doneOld, activeNew, doneNew, activeOld], now).map((o) => o.id)).toEqual([
      'a',
      'b',
      'c',
      'd',
    ])
  })

  it('compareOrders tolera fechas inválidas', () => {
    expect(() => compareOrders(order({ placed_at: 'x' }), order({ placed_at: 'y' }))).not.toThrow()
  })
})
