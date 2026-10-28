import { NextResponse } from 'next/server'
import { BACKEND_TIMEOUT_MS, jsonError } from '@/lib/backend'
import { buildBackendUrl } from '@/lib/backend-proxy'

// Proxy same-origin hacia la API Go (ADR-10). Motivos:
// - el backend no habilita CORS para REST;
// - el navegador no necesita conocer la URL interna del backend;
// - la autorización sigue siendo del servidor (aquí solo se reenvía el token).
//
// Solo se permite el prefijo /api/v1/ y segmentos seguros: no hay forma de
// construir una URL hacia otro host (defensa SSRF).

interface RouteContext {
  params: Promise<{ path?: string[] }>
}

const FORWARDED_HEADERS = ['authorization', 'content-type', 'accept', 'idempotency-key']

async function proxy(request: Request, context: RouteContext): Promise<Response> {
  const { path = [] } = await context.params
  const target = buildBackendUrl(path, new URL(request.url).search)

  if (!target) {
    return jsonError(400, 'INVALID_PATH', 'Ruta de API inválida')
  }

  const headers = new Headers()
  for (const name of FORWARDED_HEADERS) {
    const value = request.headers.get(name)
    if (value) headers.set(name, value)
  }
  if (!headers.has('accept')) headers.set('accept', 'application/json')

  const hasBody = request.method !== 'GET' && request.method !== 'HEAD'
  let body: ArrayBuffer | undefined
  if (hasBody) {
    body = await request.arrayBuffer()
    if (body.byteLength === 0) body = undefined
  }

  let upstream: Response
  try {
    upstream = await fetch(target, {
      method: request.method,
      headers,
      body,
      cache: 'no-store',
      redirect: 'manual',
      signal: AbortSignal.timeout(BACKEND_TIMEOUT_MS),
    })
  } catch {
    return jsonError(502, 'BACKEND_UNAVAILABLE', 'No se pudo contactar con el servidor')
  }

  const payload = await upstream.arrayBuffer()
  const responseHeaders: Record<string, string> = {
    'Cache-Control': 'no-store',
    'Content-Type': upstream.headers.get('content-type') ?? 'application/json',
  }
  const location = upstream.headers.get('location')
  if (location) responseHeaders.Location = location

  return new NextResponse(payload.byteLength === 0 ? null : payload, {
    status: upstream.status,
    headers: responseHeaders,
  })
}

export const GET = proxy
export const POST = proxy
export const PATCH = proxy
export const PUT = proxy
export const DELETE = proxy

export const dynamic = 'force-dynamic'
