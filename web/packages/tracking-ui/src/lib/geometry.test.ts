import { describe, expect, it } from 'vitest'
import {
  DEFAULT_BOUNDS,
  computeBounds,
  padBounds,
  projectPoint,
} from './geometry'

describe('computeBounds', () => {
  it('devuelve null sin puntos válidos', () => {
    expect(computeBounds([])).toBeNull()
    expect(computeBounds([{ lat: Number.NaN, lon: 1 }])).toBeNull()
  })

  it('calcula el rectángulo que contiene todos los puntos', () => {
    const bounds = computeBounds([
      { lat: 4.6, lon: -74.1 },
      { lat: 4.7, lon: -74.0 },
      { lat: 4.65, lon: -74.05 },
    ])
    expect(bounds).toEqual({ minLat: 4.6, maxLat: 4.7, minLon: -74.1, maxLon: -74.0 })
  })
})

describe('padBounds', () => {
  it('añade margen alrededor del centro', () => {
    const padded = padBounds({ minLat: 4.6, maxLat: 4.7, minLon: -74.1, maxLon: -74.0 })
    expect(padded.minLat).toBeLessThan(4.6)
    expect(padded.maxLat).toBeGreaterThan(4.7)
    expect(padded.minLon).toBeLessThan(-74.1)
    expect(padded.maxLon).toBeGreaterThan(-74.0)
    expect((padded.minLat + padded.maxLat) / 2).toBeCloseTo(4.65, 10)
  })

  it('garantiza un span mínimo para un único punto', () => {
    const padded = padBounds({ minLat: 4.65, maxLat: 4.65, minLon: -74.05, maxLon: -74.05 }, 0.15, 0.004)
    expect(padded.maxLat - padded.minLat).toBeGreaterThan(0)
    expect(padded.maxLon - padded.minLon).toBeGreaterThan(0)
  })
})

describe('projectPoint', () => {
  const width = 800
  const height = 420
  const padding = 28

  it('coloca noroeste arriba-izquierda y sureste abajo-derecha', () => {
    const bounds = DEFAULT_BOUNDS
    const northWest = projectPoint(
      { lat: bounds.maxLat, lon: bounds.minLon },
      bounds,
      width,
      height,
      padding,
    )
    const southEast = projectPoint(
      { lat: bounds.minLat, lon: bounds.maxLon },
      bounds,
      width,
      height,
      padding,
    )

    expect(northWest.x).toBeLessThan(southEast.x)
    expect(northWest.y).toBeLessThan(southEast.y)
  })

  it('mantiene todos los puntos dentro del área útil', () => {
    const bounds = DEFAULT_BOUNDS
    const corners = [
      { lat: bounds.minLat, lon: bounds.minLon },
      { lat: bounds.minLat, lon: bounds.maxLon },
      { lat: bounds.maxLat, lon: bounds.minLon },
      { lat: bounds.maxLat, lon: bounds.maxLon },
    ]
    for (const corner of corners) {
      const screen = projectPoint(corner, bounds, width, height, padding)
      expect(screen.x).toBeGreaterThanOrEqual(padding - 0.001)
      expect(screen.x).toBeLessThanOrEqual(width - padding + 0.001)
      expect(screen.y).toBeGreaterThanOrEqual(padding - 0.001)
      expect(screen.y).toBeLessThanOrEqual(height - padding + 0.001)
    }
  })
})
