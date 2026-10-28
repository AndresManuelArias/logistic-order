// Un segmento válido no puede ser "." ni ".." (path traversal) ni contener
// caracteres que cambien el host o la ruta (":", "/", "%", "\", "@").
const ALLOWED_SEGMENT = /^(?!\.{1,2}$)[A-Za-z0-9._~-]+$/

const DEFAULT_BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:8080'

// Construye la URL del backend para el proxy. Devuelve null si la ruta no
// pertenece al prefijo permitido o intenta escapar de él.
export function buildBackendUrl(
  path: readonly string[],
  search = '',
  baseUrl = DEFAULT_BACKEND_URL,
): URL | null {
  if (path.length === 0) return null
  if (path.some((segment) => !ALLOWED_SEGMENT.test(segment))) return null

  const joined = path.join('/')
  if (!joined.startsWith('api/v1/')) return null

  const base = new URL(baseUrl)
  const target = new URL(`${base.origin}/${path.map(encodeURIComponent).join('/')}`)
  // Defensa en profundidad: aunque los segmentos ya se validaron, la
  // normalización de URL no debe sacar la petición de /api/v1/.
  if (!target.pathname.startsWith('/api/v1/') || target.origin !== base.origin) {
    return null
  }

  target.search = search
  return target
}
