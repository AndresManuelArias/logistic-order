import { describe, expect, it, vi } from 'vitest'
import { createApiClient } from '../api/client'
import { ApiError } from '../domain/errors'
import type { Order } from '../api/types'

const order: Order = {
  id: 'o-1',
  external_ref: 'PED-1',
  store_id: 's-1',
  status: 'CREATED',
  origin: { lat: 4.7, lon: -74.1 },
  destination: { lat: 4.6, lon: -74.0 },
  placed_at: '2026-10-28T10:00:00Z',
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function headersOf(init?: RequestInit): Headers {
  return new Headers(init?.headers)
}

describe('createApiClient', () => {
  it('inyecta el token y devuelve el JSON tipado', async () => {
    const fetchImpl = vi.fn(async () => jsonResponse(order))
    const client = createApiClient({
      baseUrl: '/api/backend',
      getToken: () => 'tok',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    const result = await client.getOrder('o-1')

    expect(result.id).toBe('o-1')
    const [url, init] = fetchImpl.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/backend/api/v1/orders/o-1')
    expect(headersOf(init).get('authorization')).toBe('Bearer tok')
  })

  it('serializa filtros de listado y omite valores vacíos', async () => {
    const fetchImpl = vi.fn(async () => jsonResponse({ items: [order], total: 1 }))
    const client = createApiClient({
      baseUrl: '',
      getToken: () => 'tok',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    await client.listOrders({ status: 'IN_TRANSIT', limit: 50 })

    const [url] = fetchImpl.mock.calls[0] as unknown as [string]
    expect(url).toBe('/api/v1/orders?status=IN_TRANSIT&limit=50')
  })

  it('convierte el sobre de error en ApiError tipado', async () => {
    const fetchImpl = vi.fn(async () =>
      jsonResponse({ error: { code: 'FORBIDDEN_SCOPE', message: 'ajeno' } }, 403),
    )
    const client = createApiClient({
      baseUrl: '',
      getToken: () => 'tok',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    await expect(client.getOrder('o-1')).rejects.toMatchObject({
      name: 'ApiError',
      status: 403,
      code: 'FORBIDDEN_SCOPE',
      message: 'ajeno',
    })
  })

  it('renueva la sesión una sola vez tras un 401 y reintenta', async () => {
    let token = 'viejo'
    const calls: string[] = []
    const fetchImpl = vi.fn(async (_url: string, init?: RequestInit) => {
      const auth = headersOf(init).get('authorization') ?? ''
      calls.push(auth)
      if (auth === 'Bearer viejo') {
        return jsonResponse({ error: { code: 'UNAUTHENTICATED', message: 'expiró' } }, 401)
      }
      return jsonResponse(order)
    })
    const refresh = vi.fn(async () => {
      token = 'nuevo'
      return 'nuevo'
    })
    const client = createApiClient({
      baseUrl: '',
      getToken: () => token,
      refresh,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    const result = await client.getOrder('o-1')

    expect(result.id).toBe('o-1')
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(calls).toEqual(['Bearer viejo', 'Bearer nuevo'])
  })

  it('si la renovación falla propaga el 401 sin bucles', async () => {
    const fetchImpl = vi.fn(async () =>
      jsonResponse({ error: { code: 'UNAUTHENTICATED', message: 'expiró' } }, 401),
    )
    const refresh = vi.fn(async () => null)
    const client = createApiClient({
      baseUrl: '',
      getToken: () => 'tok',
      refresh,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    await expect(client.me()).rejects.toBeInstanceOf(ApiError)
    expect(fetchImpl).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('comparte una única renovación entre peticiones concurrentes', async () => {
    let resolveRefresh: (value: string | null) => void = () => {}
    const refreshDeferred = new Promise<string | null>((resolve) => {
      resolveRefresh = resolve
    })
    const refresh = vi.fn(() => refreshDeferred)
    let refreshed = false
    const fetchImpl = vi.fn(async (_url: string, init?: RequestInit) => {
      if (!refreshed && headersOf(init).get('authorization') === 'Bearer viejo') {
        return jsonResponse({ error: { code: 'UNAUTHENTICATED', message: 'expiró' } }, 401)
      }
      return jsonResponse(order)
    })
    const client = createApiClient({
      baseUrl: '',
      getToken: () => (refreshed ? 'nuevo' : 'viejo'),
      refresh,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    const pending = Promise.all([client.getOrder('o-1'), client.getOrder('o-2')])
    // Ambas peticiones reciben 401 y comparten la misma renovación en vuelo.
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(1))
    refreshed = true
    resolveRefresh('nuevo')

    const [a, b] = await pending
    expect(a.id).toBe('o-1')
    expect(b.id).toBe('o-1')
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('envía la Idempotency-Key del bulk sync y el cuerpo normalizado', async () => {
    const fetchImpl = vi.fn(async () =>
      jsonResponse({ accepted: 1, duplicates: 0, already_processed: false, processed_at: 'x' }, 201),
    )
    const client = createApiClient({
      baseUrl: '',
      getToken: () => 'tok',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    await client.syncBulk({
      deviceId: 'dev-1',
      idempotencyKey: 'dev-1-batch-1',
      points: [{ time: '2026-10-28T10:00:00Z', lat: 4.6, lon: -74.1, device_seq: 1 }],
    })

    const [, init] = fetchImpl.mock.calls[0] as unknown as [string, RequestInit]
    expect(headersOf(init).get('idempotency-key')).toBe('dev-1-batch-1')
    expect(JSON.parse(String(init.body))).toMatchObject({ device_id: 'dev-1' })
  })

  it('marca 404 del asistente como no disponible (F5)', async () => {
    const fetchImpl = vi.fn(async () =>
      jsonResponse({ error: { code: 'NOT_FOUND', message: 'sin ruta' } }, 404),
    )
    const client = createApiClient({
      baseUrl: '',
      getToken: () => 'tok',
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    await expect(client.queryAi('¿vehículos detenidos?')).rejects.toSatisfy(
      (error: unknown) => error instanceof ApiError && error.isNotFound,
    )
  })

  it('traduce fallos de red a ApiError status 0', async () => {
    const fetchImpl = vi.fn(async () => {
      throw new TypeError('Failed to fetch')
    })
    const client = createApiClient({
      baseUrl: '',
      getToken: () => null,
      fetchImpl: fetchImpl as unknown as typeof fetch,
    })

    await expect(client.listVehicles()).rejects.toMatchObject({ status: 0, code: 'NETWORK' })
  })
})
