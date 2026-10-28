const ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz'

function randomChunk(length: number): string {
  let out = ''
  for (let i = 0; i < length; i += 1) {
    out += ALPHABET.charAt(Math.floor(Math.random() * ALPHABET.length))
  }
  return out
}

/**
 * Identificador local único y ordenable por tiempo. No se usa para firmar ni
 * como secreto: sólo para claves de fila y de idempotencia.
 */
export function newLocalId(prefix: string): string {
  return `${prefix}${Date.now().toString(36)}${randomChunk(8)}`
}

/** El backend exige `Idempotency-Key` de 8 a 128 caracteres. */
export const IDEMPOTENCY_KEY_MIN = 8
export const IDEMPOTENCY_KEY_MAX = 128

function sanitizeKeyPart(value: string): string {
  return value
    .replace(/[^A-Za-z0-9:_-]/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function buildIdempotencyKey(parts: readonly string[]): string {
  const key = parts
    .map((part) => sanitizeKeyPart(part))
    .filter((part) => part.length > 0)
    .join(':')
    .slice(0, IDEMPOTENCY_KEY_MAX)
  if (key.length >= IDEMPOTENCY_KEY_MIN) return key
  // Mantiene el mínimo contractual incluso con entradas vacías.
  return `key:${key}`.padEnd(IDEMPOTENCY_KEY_MIN, '0').slice(0, IDEMPOTENCY_KEY_MAX)
}

export function batchIdempotencyKey(deviceId: string, batchId: string): string {
  return buildIdempotencyKey(['batch', deviceId, batchId])
}

export function transitionIdempotencyKey(transitionId: string, orderId: string, target: string): string {
  return buildIdempotencyKey(['order', orderId, target, transitionId])
}
