import type { TokenPair, User } from '../../contracts/types'
import { AppError, toAppError } from '../../domain/errors'
import type { Clock, Logger } from '../../ports/clock'
import type {
  AuthApi,
  SessionListener,
  SessionRepository,
  SessionSnapshot,
  SessionStatus,
  StoredSession,
} from '../../ports/auth'

export interface SessionServiceDeps {
  repository: SessionRepository
  api: AuthApi
  clock: Clock
  logger: Logger
  /** Margen de seguridad antes de la expiración del access token. */
  skewMs?: number
}

const DEFAULT_SKEW_MS = 30_000

/**
 * Gestión de sesión del conductor:
 *
 * - Tokens sólo en Keychain/Keystore (`SessionRepository`).
 * - Refresh single-flight: varias peticiones simultáneas comparten la misma
 *   rotación, evitando invalidar el refresh token por doble uso.
 * - Un fallo de red al renovar **no** cierra la sesión: la app sigue operando
 *   offline y reintenta al recuperar conexión.
 */
export class SessionService {
  private session: StoredSession | null = null
  private status: SessionStatus = 'unknown'
  /**
   * Instantánea cacheada: `useSyncExternalStore` exige que `getSnapshot`
   * devuelva siempre la misma referencia mientras el estado no cambie. Crear el
   * objeto en cada llamada provoca un bucle infinito de renders
   * («Maximum update depth exceeded»).
   */
  private snapshot: SessionSnapshot = { status: 'unknown', user: null }
  private readonly listeners = new Set<SessionListener>()
  private refreshInFlight: Promise<string | null> | null = null
  private hydrateInFlight: Promise<void> | null = null
  private readonly skewMs: number

  constructor(private readonly deps: SessionServiceDeps) {
    this.skewMs = deps.skewMs ?? DEFAULT_SKEW_MS
  }

  getSnapshot = (): SessionSnapshot => this.snapshot

  subscribe = (listener: SessionListener): (() => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  currentUserId(): string | null {
    return this.session?.user.id ?? null
  }

  currentUser(): User | null {
    return this.session?.user ?? null
  }

  /** Carga la sesión persistida; idempotente y tolerante a fallos de red. */
  async hydrate(): Promise<void> {
    if (this.hydrateInFlight) return this.hydrateInFlight
    this.hydrateInFlight = this.doHydrate()
    return this.hydrateInFlight
  }

  async login(email: string, password: string): Promise<User> {
    const pair = await this.deps.api.login(email.trim(), password)
    // Si el contrato no devolviera el perfil, se consulta con el token recién
    // emitido: todavía no hay sesión publicada ni token en memoria.
    const user = pair.user ?? (await this.deps.api.me(pair.access_token))

    if (user.role !== 'DRIVER') {
      // Revoca el refresh emitido para no dejar una sesión de otro rol viva
      // aunque la app no la guarde. Es best-effort: el error real es el rol.
      try {
        await this.deps.api.logout(pair.refresh_token)
      } catch (rawError) {
        this.deps.logger.warn('No se pudo revocar el refresh de una cuenta no autorizada', {
          code: toAppError(rawError).code,
        })
      }
      throw new AppError('FORBIDDEN', 'Esta aplicación es exclusiva para conductores')
    }

    await this.persist(pair, user)
    return user
  }

  async ensureAccessToken(): Promise<string | null> {
    if (this.status === 'unknown') await this.hydrate()
    const session = this.session
    if (!session) return null
    if (session.accessExpiresAt - this.skewMs > this.deps.clock.now()) return session.accessToken
    return this.refreshAccessToken()
  }

  /**
   * Renueva el par de tokens. Devuelve `null` sólo si ya no hay sesión o el
   * refresh token fue revocado; los fallos transitorios se propagan para que
   * la capa HTTP los trate como error de red.
   */
  async refreshAccessToken(): Promise<string | null> {
    if (this.refreshInFlight) return this.refreshInFlight
    this.refreshInFlight = this.doRefresh().finally(() => {
      this.refreshInFlight = null
    })
    return this.refreshInFlight
  }

  async logout(): Promise<void> {
    const current = this.session
    if (current) {
      try {
        await this.deps.api.logout(current.refreshToken)
      } catch (rawError) {
        // Best-effort: aunque el servidor no responda, la sesión local se borra.
        this.deps.logger.warn('No se pudo revocar el refresh token en el servidor', {
          code: toAppError(rawError).code,
        })
      }
    }
    await this.clearSession('anonymous')
  }

  private async doHydrate(): Promise<void> {
    let stored: StoredSession | null = null
    try {
      stored = await this.deps.repository.read()
    } catch (rawError) {
      this.deps.logger.error('No se pudo leer la sesión segura', {
        message: toAppError(rawError).message,
      })
    }

    if (!stored) {
      this.setSnapshot('anonymous', null)
      return
    }

    this.session = stored
    if (stored.accessExpiresAt - this.skewMs > this.deps.clock.now()) {
      this.setSnapshot('authenticated', stored.user)
      return
    }

    try {
      await this.refreshAccessToken()
    } catch (rawError) {
      const error = toAppError(rawError)
      if (error.code === 'UNAUTHENTICATED') {
        await this.clearSession('expired')
        return
      }
      // Sin red: conservar la sesión permite consultar datos locales.
      this.deps.logger.warn('Sesión expirada sin conexión; se mantiene para uso offline', {
        code: error.code,
      })
      this.setSnapshot('authenticated', stored.user)
    }
  }

  private async doRefresh(): Promise<string | null> {
    const current = this.session
    if (!current) return null

    try {
      const pair = await this.deps.api.refresh(current.refreshToken)
      const user = pair.user ?? current.user
      await this.persist(pair, user)
      return pair.access_token
    } catch (rawError) {
      const error = toAppError(rawError)
      if (error.code === 'UNAUTHENTICATED' || error.code === 'FORBIDDEN') {
        this.deps.logger.warn('Refresh token inválido; se cierra la sesión', {
          code: error.code,
        })
        await this.clearSession('expired')
        return null
      }
      this.deps.logger.warn('Fallo transitorio al renovar la sesión', {
        code: error.code,
        message: error.message,
      })
      throw error
    }
  }

  private async persist(pair: TokenPair, user: User): Promise<void> {
    const next: StoredSession = {
      accessToken: pair.access_token,
      refreshToken: pair.refresh_token,
      accessExpiresAt: this.deps.clock.now() + pair.expires_in * 1000,
      user,
    }

    try {
      await this.deps.repository.write(next)
    } catch (rawError) {
      this.deps.logger.error('No se pudo guardar la sesión de forma segura', {
        message: toAppError(rawError).message,
      })
      throw new AppError(
        'UNKNOWN',
        'No se pudo guardar la sesión en el almacenamiento seguro del dispositivo',
        { cause: rawError },
      )
    }

    this.session = next
    this.setSnapshot('authenticated', next.user)
  }

  private async clearSession(status: SessionStatus): Promise<void> {
    try {
      await this.deps.repository.clear()
    } catch (rawError) {
      this.deps.logger.error('No se pudo borrar la sesión segura', {
        message: toAppError(rawError).message,
      })
    }
    this.session = null
    this.setSnapshot(status, null)
  }

  private setSnapshot(status: SessionStatus, user: User | null): void {
    this.status = status
    this.snapshot = { status, user }
    for (const listener of this.listeners) listener(this.snapshot)
  }
}
