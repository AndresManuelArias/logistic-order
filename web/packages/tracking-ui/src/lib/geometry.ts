// Proyección geográfica para el mapa SVG (sin proveedor de tiles: R3 pendiente).
// Equirectangular con corrección de longitud por latitud media: suficiente para
// el ámbito urbano del MVP y determinista en pruebas.

export interface GeoPoint {
  lat: number
  lon: number
}

export interface GeoBounds {
  minLat: number
  maxLat: number
  minLon: number
  maxLon: number
}

export interface ScreenPoint {
  x: number
  y: number
}

// Bogotá: encuadre por defecto cuando aún no hay ninguna posición.
export const DEFAULT_BOUNDS: GeoBounds = {
  minLat: 4.55,
  maxLat: 4.75,
  minLon: -74.15,
  maxLon: -73.95,
}

export function computeCenter(points: readonly GeoPoint[]): GeoPoint | null {
  let latSum = 0
  let lonSum = 0
  let count = 0

  for (const point of points) {
    if (!Number.isFinite(point.lat) || !Number.isFinite(point.lon)) continue
    latSum += point.lat
    lonSum += point.lon
    count += 1
  }

  if (count === 0) return null
  return { lat: latSum / count, lon: lonSum / count }
}

export function computeBounds(points: readonly GeoPoint[]): GeoBounds | null {
  if (points.length === 0) return null
  let minLat = Number.POSITIVE_INFINITY
  let maxLat = Number.NEGATIVE_INFINITY
  let minLon = Number.POSITIVE_INFINITY
  let maxLon = Number.NEGATIVE_INFINITY

  for (const point of points) {
    if (!Number.isFinite(point.lat) || !Number.isFinite(point.lon)) continue
    minLat = Math.min(minLat, point.lat)
    maxLat = Math.max(maxLat, point.lat)
    minLon = Math.min(minLon, point.lon)
    maxLon = Math.max(maxLon, point.lon)
  }

  if (!Number.isFinite(minLat) || !Number.isFinite(minLon)) return null
  return { minLat, maxLat, minLon, maxLon }
}

// Añade margen y garantiza un span mínimo para que un único punto no deforme
// la escala (evita división por cero).
export function padBounds(bounds: GeoBounds, ratio = 0.15, minSpan = 0.004): GeoBounds {
  const centerLat = (bounds.minLat + bounds.maxLat) / 2
  const centerLon = (bounds.minLon + bounds.maxLon) / 2
  const latSpan = Math.max(bounds.maxLat - bounds.minLat, minSpan)
  const lonSpan = Math.max(bounds.maxLon - bounds.minLon, minSpan)
  const halfLat = (latSpan * (1 + ratio * 2)) / 2
  const halfLon = (lonSpan * (1 + ratio * 2)) / 2

  return {
    minLat: centerLat - halfLat,
    maxLat: centerLat + halfLat,
    minLon: centerLon - halfLon,
    maxLon: centerLon + halfLon,
  }
}

// Proyecta conservando la relación de aspecto (letterboxing dentro del SVG).
export function projectPoint(
  point: GeoPoint,
  bounds: GeoBounds,
  width: number,
  height: number,
  padding = 28,
): ScreenPoint {
  const innerWidth = Math.max(width - padding * 2, 1)
  const innerHeight = Math.max(height - padding * 2, 1)
  const midLatRad = (((bounds.minLat + bounds.maxLat) / 2) * Math.PI) / 180
  const lonScale = Math.max(Math.cos(midLatRad), 0.2)

  const spanX = Math.max((bounds.maxLon - bounds.minLon) * lonScale, Number.EPSILON)
  const spanY = Math.max(bounds.maxLat - bounds.minLat, Number.EPSILON)
  const scale = Math.min(innerWidth / spanX, innerHeight / spanY)

  const offsetX = padding + (innerWidth - spanX * scale) / 2
  const offsetY = padding + (innerHeight - spanY * scale) / 2

  return {
    x: offsetX + (point.lon - bounds.minLon) * lonScale * scale,
    y: offsetY + (bounds.maxLat - point.lat) * scale,
  }
}
