import type { NextRequest } from 'next/server'
import { NextResponse } from 'next/server'
import {
  BACKEND_TIMEOUT_MS,
  BACKEND_URL,
  isSameOrigin,
  jsonError,
  refreshCookieOptions,
  REFRESH_COOKIE,
} from '@/lib/backend'

// POST /api/session/logout — revoca el refresh token en el backend y limpia la
// cookie. Es idempotente: siempre responde 204.
export async function POST(request: NextRequest) {
  if (!isSameOrigin(request)) {
    return jsonError(403, 'CROSS_ORIGIN', 'Origen no permitido')
  }

  const current = request.cookies.get(REFRESH_COOKIE)?.value
  if (current) {
    try {
      await fetch(`${BACKEND_URL}/api/v1/auth/logout`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ refresh_token: current }),
        cache: 'no-store',
        signal: AbortSignal.timeout(BACKEND_TIMEOUT_MS),
      })
    } catch {
      // La sesión local se limpia igualmente; el token caducará solo.
    }
  }

  const response = new NextResponse(null, {
    status: 204,
    headers: { 'Cache-Control': 'no-store' },
  })
  response.cookies.set(REFRESH_COOKIE, '', { ...refreshCookieOptions(request), maxAge: 0 })
  return response
}

export const dynamic = 'force-dynamic'
