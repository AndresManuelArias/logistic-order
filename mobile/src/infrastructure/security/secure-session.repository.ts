import * as SecureStore from 'expo-secure-store'

import type { User } from '../../contracts/types'
import type { SessionRepository, StoredSession } from '../../ports/auth'

const KEYS = {
  accessToken: 'tiendas.access_token',
  refreshToken: 'tiendas.refresh_token',
  expiresAt: 'tiendas.access_expires_at',
  user: 'tiendas.user',
} as const

// `AFTER_FIRST_UNLOCK` permite que la tarea GPS en segundo plano lea el token
// con la pantalla bloqueada tras el primer desbloqueo. Es el mínimo necesario
// para sincronizar sin pedir biometría en cada muestra.
const OPTIONS: SecureStore.SecureStoreOptions = {
  keychainAccessible: SecureStore.AFTER_FIRST_UNLOCK,
}

/**
 * Tokens en Keychain (iOS) / Keystore (Android). Nunca AsyncStorage ni SQLite.
 * La sesión se escribe en orden refresh → access → expiración → usuario para
 * que un cierre inesperado no deje un refresh token nuevo con metadatos viejos.
 */
export class SecureSessionRepository implements SessionRepository {
  async read(): Promise<StoredSession | null> {
    const [accessToken, refreshToken, expiresAtRaw, userRaw] = await Promise.all([
      SecureStore.getItemAsync(KEYS.accessToken, OPTIONS),
      SecureStore.getItemAsync(KEYS.refreshToken, OPTIONS),
      SecureStore.getItemAsync(KEYS.expiresAt, OPTIONS),
      SecureStore.getItemAsync(KEYS.user, OPTIONS),
    ])

    if (!accessToken || !refreshToken || !expiresAtRaw || !userRaw) return null

    const accessExpiresAt = Number(expiresAtRaw)
    if (!Number.isFinite(accessExpiresAt)) return null

    let user: User
    try {
      user = JSON.parse(userRaw) as User
    } catch {
      return null
    }
    if (!user || typeof user.id !== 'string' || typeof user.role !== 'string') return null

    return { accessToken, refreshToken, accessExpiresAt, user }
  }

  async write(session: StoredSession): Promise<void> {
    await SecureStore.setItemAsync(KEYS.refreshToken, session.refreshToken, OPTIONS)
    await SecureStore.setItemAsync(KEYS.accessToken, session.accessToken, OPTIONS)
    await SecureStore.setItemAsync(KEYS.expiresAt, String(session.accessExpiresAt), OPTIONS)
    await SecureStore.setItemAsync(KEYS.user, JSON.stringify(session.user), OPTIONS)
  }

  async clear(): Promise<void> {
    await Promise.all([
      SecureStore.deleteItemAsync(KEYS.accessToken, OPTIONS),
      SecureStore.deleteItemAsync(KEYS.refreshToken, OPTIONS),
      SecureStore.deleteItemAsync(KEYS.expiresAt, OPTIONS),
      SecureStore.deleteItemAsync(KEYS.user, OPTIONS),
    ])
  }
}
