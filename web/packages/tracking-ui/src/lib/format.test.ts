import { describe, expect, it } from 'vitest'
import { formatDateTime, formatDistance, formatFreshness, formatSpeed, minutesSince } from './format'

describe('formatDistance', () => {
  it('usa metros por debajo de 1 km y kilómetros por encima', () => {
    expect(formatDistance(840)).toBe('840 m')
    expect(formatDistance(1500)).toBe('1.5 km')
    expect(formatDistance(null)).toBe('—')
    expect(formatDistance(Number.NaN)).toBe('—')
  })
})

describe('formatDateTime', () => {
  it('devuelve un guion para valores ausentes o inválidos', () => {
    expect(formatDateTime(undefined)).toBe('—')
    expect(formatDateTime('no-es-fecha')).toBe('—')
  })

  it('formatea una fecha ISO en es-CO', () => {
    expect(formatDateTime('2026-10-28T15:04:00Z')).toMatch(/2026/)
  })
})

describe('formatSpeed', () => {
  it('redondea y etiqueta km/h', () => {
    expect(formatSpeed(18.4)).toBe('18 km/h')
    expect(formatSpeed(undefined)).toBe('—')
  })
})

describe('formatFreshness', () => {
  const now = Date.parse('2026-10-28T15:00:00Z')

  it('describe la antigüedad en minutos y horas', () => {
    expect(minutesSince('2026-10-28T14:59:40Z', now)).toBe(0)
    expect(formatFreshness('2026-10-28T14:59:40Z', now)).toBe('Ahora mismo')
    expect(formatFreshness('2026-10-28T14:59:00Z', now)).toBe('Hace 1 min')
    expect(formatFreshness('2026-10-28T14:30:00Z', now)).toBe('Hace 30 min')
    expect(formatFreshness('2026-10-28T12:00:00Z', now)).toBe('Hace 3 h')
  })

  it('no inventa frescura sin marca temporal', () => {
    expect(formatFreshness(null, now)).toBe('Sin datos')
    expect(minutesSince('inválida', now)).toBeNull()
  })
})
