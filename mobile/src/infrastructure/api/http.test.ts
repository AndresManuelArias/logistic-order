import { describe, expect, it } from 'vitest'

import { HttpClient, type FetchLike } from './http'

interface Harness {
  http: HttpClient
  calls: Array<{ url: string; authorization: string | null }>
  tokenReads: () => number
  refreshes: () => number
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function createHarness(
  handler: (url: string, authorization: string | null, call: number) => Response,
): Harness {
  const calls: Harness['calls'] = []
  let tokenReads = 0
  let refreshes = 0

  const fetcher: FetchLike = async (url, init) => {
    const authorization = new Headers(init?.headers).get('authorization')
    calls.push({ url, authorization })
    return handler(url, authorization, calls.length)
  }

  const http = new HttpClient({
    baseUrl: 'http://localhost:8080',
    fetcher,
    getAccessToken: async () => {
      tokenReads += 1
      // Igual que `SessionService`: tras renovar, devuelve el token nuevo.
      return refreshes > 0 ? 'access-new' : 'access-old'
    },
    refreshAccessToken: async () => {
      refreshes += 1
      return 'access-new'
    },
  })

  return { http, calls, tokenReads: () => tokenReads, refreshes: () => refreshes }
}

describe('HttpClient', () => {
  it('con auth:false no resuelve el access token (regresión del ciclo de refresh)', async () => {
    const harness = createHarness(() => jsonResponse({ access_token: 'a', refresh_token: 'r' }))

    await harness.http.request('/api/v1/auth/refresh', {
      method: 'POST',
      body: { refresh_token: 'r' },
      retryOnUnauthorized: false,
      auth: false,
    })

    expect(harness.tokenReads()).toBe(0)
    expect(harness.calls[0]?.authorization).toBeNull()
    expect(harness.calls).toHaveLength(1)
  })

  it('envía el bearer y reintenta una vez tras un 401 renovando la sesión', async () => {
    const harness = createHarness((_url, authorization) =>
      authorization === 'Bearer access-new'
        ? jsonResponse({ ok: true })
        : jsonResponse({ error: { code: 'UNAUTHENTICATED' } }, 401),
    )

    const payload = await harness.http.request<{ ok: boolean }>('/api/v1/orders')

    expect(payload.ok).toBe(true)
    expect(harness.refreshes()).toBe(1)
    expect(harness.calls.map((call) => call.authorization)).toEqual([
      'Bearer access-old',
      'Bearer access-new',
    ])
  })

  it('con auth:false no reintenta ni refresca tras un 401', async () => {
    const harness = createHarness(() =>
      jsonResponse({ error: { code: 'UNAUTHENTICATED', message: 'Refresh revocado' } }, 401),
    )

    await expect(
      harness.http.request('/api/v1/auth/refresh', {
        method: 'POST',
        auth: false,
        retryOnUnauthorized: false,
      }),
    ).rejects.toMatchObject({ code: 'UNAUTHENTICATED' })

    expect(harness.refreshes()).toBe(0)
    expect(harness.calls).toHaveLength(1)
  })
})
