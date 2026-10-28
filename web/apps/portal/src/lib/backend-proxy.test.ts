import { describe, expect, it } from 'vitest'
import { buildBackendUrl } from '@/lib/backend-proxy'

const BASE = 'http://backend:8080'

describe('buildBackendUrl', () => {
  it('construye la URL dentro de /api/v1 y conserva la query', () => {
    const url = buildBackendUrl(
      ['api', 'v1', 'orders', 'abc-123', 'status'],
      '?limit=10&offset=0',
      BASE,
    )
    expect(url?.toString()).toBe('http://backend:8080/api/v1/orders/abc-123/status?limit=10&offset=0')
  })

  it('rechaza rutas vacías o fuera de /api/v1', () => {
    expect(buildBackendUrl([], '', BASE)).toBeNull()
    expect(buildBackendUrl(['healthz'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v2', 'orders'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v1'], '', BASE)).toBeNull()
  })

  it('rechaza path traversal con puntos, barras o codificación', () => {
    expect(buildBackendUrl(['api', 'v1', '..', 'admin'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v1', '.', 'orders'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v1', 'orders', '..', '..', 'healthz'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v1', 'a%2fb'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v1', 'a/b'], '', BASE)).toBeNull()
  })

  it('rechaza segmentos que intenten cambiar de host', () => {
    expect(buildBackendUrl(['api', 'v1', 'http:', 'evil.example'], '', BASE)).toBeNull()
    expect(buildBackendUrl(['api', 'v1', '@evil.example'], '', BASE)).toBeNull()
  })

  it('solo apunta al origen configurado', () => {
    const url = buildBackendUrl(['api', 'v1', 'vehicles'], '', 'https://api.internal:8443')
    expect(url?.origin).toBe('https://api.internal:8443')
    expect(url?.pathname).toBe('/api/v1/vehicles')
  })
})
