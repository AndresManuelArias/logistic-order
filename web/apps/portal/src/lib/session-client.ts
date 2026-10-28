// Llamadas del navegador al BFF de sesión. El refresh token nunca es visible
// aquí: viaja en una cookie httpOnly que solo maneja el servidor de Next.
import { ApiError, toApiError, type User } from '@tiendas/tracking-core'

export interface SessionPayload {
  access_token: string
  expires_in: number
  user: User
}

async function readError(response: Response): Promise<ApiError> {
  let code = `HTTP_${response.status}`
  let message = response.statusText || 'Error del servidor'
  try {
    const body = (await response.json()) as { error?: { code?: string; message?: string } }
    if (body.error?.code) code = body.error.code
    if (body.error?.message) message = body.error.message
  } catch {
    // sin cuerpo JSON: se conserva el mensaje por defecto
  }
  return new ApiError(response.status, code, message)
}

async function postJson(path: string, body?: unknown): Promise<Response> {
  try {
    return await fetch(path, {
      method: 'POST',
      credentials: 'same-origin',
      cache: 'no-store',
      headers: body === undefined ? { Accept: 'application/json' } : { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (error) {
    throw toApiError(error)
  }
}

// Serializa la rotación entre pestañas cuando el navegador soporta Web Locks:
// la segunda pestaña espera y envía la cookie ya rotada por la primera, en
// lugar de reutilizar un token revocado. Sin soporte, se comporta como antes.
async function withRefreshLock<T>(task: () => Promise<T>): Promise<T> {
  if (typeof navigator === 'undefined' || !navigator.locks) return task()
  return navigator.locks.request('tiendas-track-refresh', task)
}

export async function loginRequest(email: string, password: string): Promise<SessionPayload> {
  const response = await postJson('/api/session/login', { email, password })
  if (!response.ok) throw await readError(response)
  return (await response.json()) as SessionPayload
}

// Devuelve null cuando no hay sesión (401); propaga fallos transitorios.
export async function refreshRequest(): Promise<SessionPayload | null> {
  return withRefreshLock(async () => {
    const response = await postJson('/api/session/refresh')
    if (response.status === 401) return null
    if (!response.ok) throw await readError(response)
    return (await response.json()) as SessionPayload
  })
}

export async function logoutRequest(): Promise<void> {
  try {
    await postJson('/api/session/logout')
  } catch {
    // El estado local se limpia igualmente.
  }
}
