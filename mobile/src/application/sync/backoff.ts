import type { RetryPolicy } from '../../config/policies'

/**
 * Backoff exponencial con jitter. `attempt` empieza en 1 (primer reintento).
 * El jitter evita que muchos dispositivos reconecten exactamente a la vez.
 */
export function computeBackoffDelay(
  attempt: number,
  policy: RetryPolicy,
  random: () => number = Math.random,
): number {
  const safeAttempt = Number.isFinite(attempt) ? Math.max(1, Math.floor(attempt)) : 1
  const exponential = Math.min(policy.maxMs, policy.baseMs * 2 ** (safeAttempt - 1))
  const jitter = exponential * policy.jitterRatio * (random() * 2 - 1)
  return Math.min(policy.maxMs, Math.max(0, Math.round(exponential + jitter)))
}
