import type { TokenPair } from '@tiendas/tracking-core'
import {
  BACKEND_TIMEOUT_MS,
  BACKEND_URL,
  isSameOrigin,
  jsonError,
  noStoreJson,
  readJson,
  refreshCookieOptions,
  REFRESH_COOKIE,
  upstreamError,
} from '@/lib/backend'

interface LoginBody {
  email?: unknown
  password?: unknown
}

// POST /api/session/login — valida credenciales contra Go y deja el refresh
// token en cookie httpOnly. El access token se devuelve para memoria del SPA.
export async function POST(request: Request) {
  if (!isSameOrigin(request)) {
    return jsonError(403, 'CROSS_ORIGIN', 'Origen no permitido')
  }

  const body = await readJson<LoginBody>(request)
  const email = typeof body?.email === 'string' ? body.email.trim() : ''
  const password = typeof body?.password === 'string' ? body.password : ''
  if (!email || !password) {
    return jsonError(400, 'INVALID_BODY', 'Email y contraseña son obligatorios')
  }

  let upstream: Response
  try {
    upstream = await fetch(`${BACKEND_URL}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ email, password }),
      cache: 'no-store',
      signal: AbortSignal.timeout(BACKEND_TIMEOUT_MS),
    })
  } catch {
    return jsonError(502, 'BACKEND_UNAVAILABLE', 'No se pudo contactar con el servidor')
  }

  if (!upstream.ok) {
    return upstream.status === 401
      ? jsonError(401, 'INVALID_CREDENTIALS', 'Correo o contraseña incorrectos')
      : upstreamError(upstream, 'LOGIN_FAILED', 'No se pudo iniciar sesión')
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
