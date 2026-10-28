import type { TokenPair } from '@tiendas/tracking-core'
import type { NextRequest } from 'next/server'
import {
  BACKEND_TIMEOUT_MS,
  BACKEND_URL,
  isSameOrigin,
  jsonError,
  noStoreJson,
  refreshCookieOptions,
  REFRESH_COOKIE,
} from '@/lib/backend'

// POST /api/session/refresh — rota el refresh token (cookie httpOnly) y
// devuelve un access token nuevo. El SPA lo llama al arrancar y tras un 401.
export async function POST(request: NextRequest) {
  if (!isSameOrigin(request)) {
    return jsonError(403, 'CROSS_ORIGIN', 'Origen no permitido')
  }

  const current = request.cookies.get(REFRESH_COOKIE)?.value
  if (!current) {
    return jsonError(401, 'SESSION_EXPIRED', 'No hay sesión activa')
  }

  let upstream: Response
  try {
    upstream = await fetch(`${BACKEND_URL}/api/v1/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ refresh_token: current }),
      cache: 'no-store',
      signal: AbortSignal.timeout(BACKEND_TIMEOUT_MS),
    })
  } catch {
    return jsonError(502, 'BACKEND_UNAVAILABLE', 'No se pudo contactar con el servidor')
  }

  if (!upstream.ok) {
    // Token revocado o reutilizado: se limpia la cookie para no reintentar.
    const response = jsonError(401, 'SESSION_EXPIRED', 'La sesión caducó. Inicia sesión de nuevo.')
    response.cookies.set(REFRESH_COOKIE, '', { ...refreshCookieOptions(request), maxAge: 0 })
    return response
  }

  const pair = (await upstream.json()) as TokenPair
  const response = noStoreJson({
    access_token: pair.access_token,
    expires_in: pair.expires_in,
    user: pair.user,
  })
  response.cookies.set(REFRESH_COOKIE, pair.refresh_token, refreshCookieOptions(request))
  return response
}

// Evita que un GET accidental (p. ej. prefetch) consuma la rotación.
export async function GET() {
  return jsonError(405, 'METHOD_NOT_ALLOWED', 'Usa POST para renovar la sesión')
}

export const dynamic = 'force-dynamic'
