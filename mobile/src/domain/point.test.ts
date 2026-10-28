import { describe, expect, it } from 'vitest'

import {
  DEFAULT_FUTURE_TOLERANCE_MS,
  formatTimestamp,
  normalizeHeading,
  normalizeSample,
  toPointInput,
  validateSample,
  type QueuedPoint,
} from './point'

const NOW = 1_700_000_000_000

describe('validateSample', () => {
  const base = { lat: 4.7108, lon: -74.0721, recordedAt: NOW }

  it('acepta una muestra válida', () => {
    expect(validateSample({ ...base, accuracyM: 8, speedKph: 20, heading: 90 }, { now: NOW })).toEqual({
      ok: true,
    })
  })

  it('rechaza coordenadas fuera de rango', () => {
    expect(validateSample({ ...base, lat: 91 }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'INVALID_COORDINATES',
    })
    expect(validateSample({ ...base, lon: -181 }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'INVALID_COORDINATES',
    })
  })

  it('rechaza timestamps futuros más allá de la tolerancia del backend', () => {
    const future = NOW + DEFAULT_FUTURE_TOLERANCE_MS + 1
    expect(validateSample({ ...base, recordedAt: future }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'FUTURE_TIMESTAMP',
    })
  })

  it('acepta un adelanto de reloj dentro de los 5 minutos', () => {
    expect(
      validateSample({ ...base, recordedAt: NOW + DEFAULT_FUTURE_TOLERANCE_MS - 10 }, { now: NOW }),
    ).toEqual({ ok: true })
  })

  it('rechaza fechas inválidas', () => {
    expect(validateSample({ ...base, recordedAt: 0 }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'INVALID_TIMESTAMP',
    })
    expect(validateSample({ ...base, recordedAt: Number.NaN }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'INVALID_TIMESTAMP',
    })
  })

  it('descarta muestras con precisión insuficiente sólo si hay umbral', () => {
    expect(
      validateSample({ ...base, accuracyM: 150 }, { now: NOW, maxAccuracyMeters: 100 }),
    ).toMatchObject({ ok: false, code: 'LOW_ACCURACY' })

    expect(validateSample({ ...base, accuracyM: 150 }, { now: NOW })).toEqual({ ok: true })
    expect(
      validateSample({ ...base, accuracyM: null }, { now: NOW, maxAccuracyMeters: 100 }),
    ).toEqual({ ok: true })
  })

  it('rechaza velocidad y rumbo inválidos', () => {
    expect(validateSample({ ...base, speedKph: -1 }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'INVALID_SPEED',
    })
    expect(validateSample({ ...base, heading: Number.NaN }, { now: NOW })).toMatchObject({
      ok: false,
      code: 'INVALID_HEADING',
    })
  })
})

describe('normalizeSample', () => {
  it('redondea coordenadas y normaliza el rumbo', () => {
    expect(normalizeHeading(360)).toBe(0)
    expect(normalizeHeading(-10)).toBe(350)
    expect(normalizeHeading(725)).toBe(5)

    const normalized = normalizeSample({
      lat: 4.7108123456,
      lon: -74.072198765,
      recordedAt: NOW + 0.4,
      speedKph: 20.26,
      accuracyM: 7.84,
      heading: 359.96,
      vehicleId: '  veh-1  ',
      orderId: '',
    })

    expect(normalized).toEqual({
      lat: 4.710812,
      lon: -74.072199,
      recordedAt: NOW,
      speedKph: 20.3,
      accuracyM: 7.8,
      heading: 0,
      vehicleId: 'veh-1',
      orderId: null,
    })
  })
})

describe('toPointInput', () => {
  const point: QueuedPoint = {
    id: 'pt_1',
    ownerId: 'driver-1',
    deviceSeq: 42,
    lat: 4.710812,
    lon: -74.072199,
    recordedAt: NOW,
    speedKph: 20.3,
    accuracyM: 7.8,
    heading: 0,
    vehicleId: 'vehicle-1',
    orderId: null,
  }

  it('traduce al contrato TelemetryPointInput', () => {
    expect(toPointInput(point)).toEqual({
      time: formatTimestamp(NOW),
      lat: 4.710812,
      lon: -74.072199,
      device_seq: 42,
      speed_kph: 20.3,
      accuracy_m: 7.8,
      heading: 0,
      vehicle_id: 'vehicle-1',
    })
  })

  it('omite los opcionales nulos', () => {
    const minimal = toPointInput({
      ...point,
      speedKph: null,
      accuracyM: null,
      heading: null,
      vehicleId: null,
    })
    expect(minimal).toEqual({
      time: formatTimestamp(NOW),
      lat: point.lat,
      lon: point.lon,
      device_seq: 42,
    })
  })

  it('usa ISO-8601 UTC para el timestamp', () => {
    expect(formatTimestamp(NOW)).toBe('2023-11-14T22:13:20.000Z')
  })
})
