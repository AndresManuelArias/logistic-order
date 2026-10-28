import { describe, expect, it } from 'vitest'

import {
  IDEMPOTENCY_KEY_MAX,
  IDEMPOTENCY_KEY_MIN,
  batchIdempotencyKey,
  buildIdempotencyKey,
  newLocalId,
  transitionIdempotencyKey,
} from './ids'

describe('newLocalId', () => {
  it('incluye el prefijo y no repite valores en llamadas consecutivas', () => {
    const ids = new Set(Array.from({ length: 200 }, () => newLocalId('pt_')))
    expect(ids.size).toBe(200)
    for (const id of ids) expect(id.startsWith('pt_')).toBe(true)
  })
})

describe('buildIdempotencyKey', () => {
  it('respeta el mínimo y el máximo del backend', () => {
    const empty = buildIdempotencyKey([])
    expect(empty.length).toBeGreaterThanOrEqual(IDEMPOTENCY_KEY_MIN)
    expect(empty.length).toBeLessThanOrEqual(IDEMPOTENCY_KEY_MAX)

    const long = buildIdempotencyKey(['x'.repeat(500)])
    expect(long.length).toBe(IDEMPOTENCY_KEY_MAX)
  })

  it('sanea caracteres no permitidos y elimina partes vacías', () => {
    const key = buildIdempotencyKey(['batch', 'dev 1/2', '  ', 'bt-3'])
    expect(key).toBe('batch:dev-1-2:bt-3')
  })

  it('es determinista para las mismas entradas', () => {
    expect(buildIdempotencyKey(['batch', 'dev', 'bt'])).toBe(
      buildIdempotencyKey(['batch', 'dev', 'bt']),
    )
  })
})

describe('claves compuestas', () => {
  it('la clave de lote incluye dispositivo e identificador del lote', () => {
    expect(batchIdempotencyKey('dev-1', 'bt-1')).toBe('batch:dev-1:bt-1')
    expect(batchIdempotencyKey('dev-1', 'bt-1')).toBe(batchIdempotencyKey('dev-1', 'bt-1'))
  })

  it('la clave de transición es estable por operación', () => {
    const key = transitionIdempotencyKey('tr-1', 'order-1', 'IN_TRANSIT')
    expect(key).toBe('order:order-1:IN_TRANSIT:tr-1')
    expect(key.length).toBeGreaterThanOrEqual(IDEMPOTENCY_KEY_MIN)
  })
})
