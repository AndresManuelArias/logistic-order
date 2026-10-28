import type { TokenPair, User } from '../contracts/types'

export interface StoredSession {
  accessToken: string
  refreshToken: string
  /** Epoch ms en que expira el access token. */
  accessExpiresAt: number
  user: User
}

/**
 * Persistencia segura (Keychain/Keystore). Nunca debe implementarse con
 * AsyncStorage ni con archivos en claro.
 */
export interface SessionRepository {
  read(): Promise<StoredSession | null>
  write(session: StoredSession): Promise<void>
  clear(): Promise<void>
}

/** Endpoints de autenticación; el transporte HTTP inyecta la base URL. */
export interface AuthApi {
  login(email: string, password: string): Promise<TokenPair>
  refresh(refreshToken: string): Promise<TokenPair>
  logout(refreshToken: string): Promise<void>
  /** `accessToken` permite consultar el perfil antes de publicar la sesión. */
  me(accessToken?: string): Promise<User>
}

export type SessionStatus = 'unknown' | 'anonymous' | 'authenticated' | 'expired'

export interface SessionSnapshot {
  status: SessionStatus
  user: User | null
}

export type SessionListener = (snapshot: SessionSnapshot) => void

/**
 * Devuelve el `sub` del usuario autenticado. Se usa para aislar los datos
 * locales por conductor: al cerrar sesión no se sirve nada de otra cuenta.
 */
export type OwnerIdProvider = () => string | null
