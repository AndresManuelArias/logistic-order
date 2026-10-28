import type { TelemetryPointInput } from '../contracts/types'

/**
 * Muestra GPS tal como la entrega el sistema operativo, antes de asignarle una
 * secuencia de dispositivo y persistirla.
 */
export interface GpsSampleInput {
  lat: number
  lon: number
  /** Timestamp del dispositivo en milisegundos epoch. */
  recordedAt: number
  speedKph?: number | null
  accuracyM?: number | null
  heading?: number | null
  vehicleId?: string | null
  orderId?: string | null
}

/** Punto persistido en la cola local; incluye identidad e idempotencia. */
export interface QueuedPoint extends GpsSampleInput {
  id: string
  ownerId: string
  deviceSeq: number
}

export type SampleRejectionCode =
  | 'INVALID_COORDINATES'
  | 'INVALID_TIMESTAMP'
  | 'FUTURE_TIMESTAMP'
  | 'LOW_ACCURACY'
  | 'INVALID_SPEED'
  | 'INVALID_HEADING'

export type SampleValidation =
  | { ok: true }
  | { ok: false; code: SampleRejectionCode; message: string }

export interface SampleValidationContext {
  /** Reloj actual en ms (inyectado para poder probar). */
  now: number
  /** Precisión máxima aceptada en metros; `null` desactiva el filtro. */
  maxAccuracyMeters?: number | null
  /** Tolerancia de reloj adelantado. El backend rechaza > 5 min en el futuro. */
  futureToleranceMs?: number
}

/** El backend rechaza muestras con más de 5 minutos de adelanto. */
export const DEFAULT_FUTURE_TOLERANCE_MS = 5 * 60_000

export interface NormalizedSample {
  lat: number
  lon: number
  recordedAt: number
  speedKph: number | null
  accuracyM: number | null
  heading: number | null
  vehicleId: string | null
  orderId: string | null
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function round(value: number, decimals: number): number {
  const factor = 10 ** decimals
  return Math.round(value * factor) / factor
}

function nonEmpty(value: string | null | undefined): string | null {
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

/** Normaliza grados a [0, 360): 360 → 0, -10 → 350. */
export function normalizeHeading(value: number): number {
  const mod = value % 360
  return mod < 0 ? mod + 360 : mod
}

/** Normaliza después de redondear: 359,96° no debe convertirse en 360°. */
function normalizeRoundedHeading(value: number): number {
  const rounded = round(normalizeHeading(value), 1)
  return rounded >= 360 ? 0 : rounded
}

export function validateSample(
  sample: GpsSampleInput,
  context: SampleValidationContext,
): SampleValidation {
  const tolerance = context.futureToleranceMs ?? DEFAULT_FUTURE_TOLERANCE_MS

  if (!isFiniteNumber(sample.lat) || sample.lat < -90 || sample.lat > 90) {
    return {
      ok: false,
      code: 'INVALID_COORDINATES',
      message: 'Latitud inválida (debe estar entre -90 y 90)',
    }
  }
  if (!isFiniteNumber(sample.lon) || sample.lon < -180 || sample.lon > 180) {
    return {
      ok: false,
      code: 'INVALID_COORDINATES',
      message: 'Longitud inválida (debe estar entre -180 y 180)',
    }
  }
  if (!isFiniteNumber(sample.recordedAt) || sample.recordedAt <= 0) {
    return { ok: false, code: 'INVALID_TIMESTAMP', message: 'Timestamp del GPS inválido' }
  }
  if (sample.recordedAt > context.now + tolerance) {
    return {
      ok: false,
      code: 'FUTURE_TIMESTAMP',
      message: 'El reloj del dispositivo está adelantado más de 5 minutos',
    }
  }
  if (sample.accuracyM != null) {
    if (!isFiniteNumber(sample.accuracyM) || sample.accuracyM < 0) {
      return { ok: false, code: 'LOW_ACCURACY', message: 'Precisión GPS inválida' }
    }
    if (context.maxAccuracyMeters != null && sample.accuracyM > context.maxAccuracyMeters) {
      return {
        ok: false,
        code: 'LOW_ACCURACY',
        message: `Precisión GPS insuficiente (${Math.round(sample.accuracyM)} m > ${context.maxAccuracyMeters} m)`,
      }
    }
  }
  if (sample.speedKph != null && (!isFiniteNumber(sample.speedKph) || sample.speedKph < 0)) {
    return { ok: false, code: 'INVALID_SPEED', message: 'Velocidad inválida' }
  }
  if (sample.heading != null && !isFiniteNumber(sample.heading)) {
    return { ok: false, code: 'INVALID_HEADING', message: 'Rumbo inválido' }
  }
  return { ok: true }
}

/**
 * Redondea coordenadas (~0,1 m) y normaliza opcionales para que el payload sea
 * estable y los duplicados por diferencias de coma flotante no existan.
 */
export function normalizeSample(sample: GpsSampleInput): NormalizedSample {
  return {
    lat: round(sample.lat, 6),
    lon: round(sample.lon, 6),
    recordedAt: Math.round(sample.recordedAt),
    speedKph: sample.speedKph == null ? null : round(sample.speedKph, 1),
    accuracyM: sample.accuracyM == null ? null : round(sample.accuracyM, 1),
    heading: sample.heading == null ? null : normalizeRoundedHeading(sample.heading),
    vehicleId: nonEmpty(sample.vehicleId),
    orderId: nonEmpty(sample.orderId),
  }
}

export function formatTimestamp(ms: number): string {
  return new Date(ms).toISOString()
}

/**
 * Traduce el punto local al contrato `TelemetryPointInput`. `device_seq` viaja
 * siempre: junto a `time` y `vehicle_id` forma la clave de deduplicación del
 * backend.
 */
export function toPointInput(point: QueuedPoint): TelemetryPointInput {
  const input: TelemetryPointInput = {
    time: formatTimestamp(point.recordedAt),
    lat: point.lat,
    lon: point.lon,
    device_seq: point.deviceSeq,
  }
  if (point.speedKph != null) input.speed_kph = point.speedKph
  if (point.accuracyM != null) input.accuracy_m = point.accuracyM
  if (point.heading != null) input.heading = point.heading
  if (point.vehicleId) input.vehicle_id = point.vehicleId
  if (point.orderId) input.order_id = point.orderId
  return input
}
