import { describe, expect, it } from 'vitest'

import { computeBackoffDelay } from './backoff'

const POLICY = { baseMs: 1_000, maxMs: 8_000, jitterRatio: 0 }

describe('computeBackoffDelay', () => {
  it('crece exponencialmente desde el primer reintento', () => {
    expect(computeBackoffDelay(1, POLICY)).toBe(1_000)
    expect(computeBackoffDelay(2, POLICY)).toBe(2_000)
    expect(computeBackoffDelay(3, POLICY)).toBe(4_000)
  })

  it('respeta el tope máximo', () => {
    expect(computeBackoffDelay(10, POLICY)).toBe(8_000)
    expect(computeBackoffDelay(999, POLICY)).toBe(8_000)
  })

  it('aplica jitter dentro de los límites', () => {
    const withJitter = { baseMs: 1_000, maxMs: 10_000, jitterRatio: 0.2 }
    expect(computeBackoffDelay(1, withJitter, () => 0)).toBe(800)
    expect(computeBackoffDelay(1, withJitter, () => 1)).toBe(1_200)
    expect(computeBackoffDelay(2, withJitter, () => 0)).toBe(1_600)
  })

  it('normaliza intentos inválidos al primero', () => {
    expect(computeBackoffDelay(0, POLICY)).toBe(1_000)
    expect(computeBackoffDelay(Number.NaN, POLICY)).toBe(1_000)
  })
})
