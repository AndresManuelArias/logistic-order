// Utilidades del BFF (solo servidor). El navegador nunca habla directo con Go:
// el proxy same-origin evita CORS y permite guardar el refresh token en una
// cookie httpOnly fuera del alcance de JavaScript.
import { NextResponse } from 'next/server'

export const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:8080'
export const REFRESH_COOKIE = 'tt_refresh'
export const SESSION_COOKIE_PATH = '/api/session'
export const BACKEND_TIMEOUT_MS = 20_000

export function jsonError(status: number, code: string, message: string): NextResponse {
  return NextResponse.json(
    { error: { code, message } },
    { status, headers: { 'Cache-Control': 'no-store' } },
  )
}

export function noStoreJson(payload: unknown, status = 200): NextResponse {
  return NextResponse.json(payload, { status, headers: { 'Cache-Control': 'no-store' } })
}

// Solo se aceptan peticiones del mismo origen en las rutas que mutan sesión.
export function isSameOrigin(request: Request): boolean {
  const origin = request.headers.get('origin')
  if (!origin) return true // clientes no navegador (curl, pruebas)
  try {
    return new URL(origin).host === request.headers.get('host')
  } catch {
    return false
  }
}

export function isSecureRequest(request: Request): boolean {
  const forwarded = request.headers.get('x-forwarded-proto')
  if (forwarded) return forwarded.split(',')[0]?.trim() === 'https'
  return new URL(request.url).protocol === 'https:'
}

export function refreshCookieOptions(request: Request, maxAgeSeconds = 30 * 24 * 60 * 60) {
  return {
    httpOnly: true,
    sameSite: 'lax' as const,
    secure: isSecureRequest(request),
    path: SESSION_COOKIE_PATH,
    maxAge: maxAgeSeconds,
  }
}

export async function readJson<T>(request: Request): Promise<T | null> {
  try {
    return (await request.json()) as T
  } catch {
    return null
  }
}

// Normaliza la respuesta de error del backend sin filtrar HTML ni detalles
// internos si el upstream devuelve algo inesperado.
export async function upstreamError(
  response: Response,
  fallbackCode: string,
  fallbackMessage: string,
): Promise<NextResponse> {
  const text = await response.text()
  try {
    const parsed = JSON.parse(text) as { error?: { code?: string; message?: string } }
    if (parsed.error?.code && parsed.error.message) {
      return NextResponse.json(parsed, {
        status: response.status,
        headers: { 'Cache-Control': 'no-store' },
      })
    }
  } catch {
    // respuesta no JSON: se normaliza abajo
  }
  return jsonError(response.status, fallbackCode, fallbackMessage)
}
