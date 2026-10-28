'use client'

// Proveedor de sesión del portal. El access token vive solo en memoria; al
// recargar se recupera con el refresh token httpOnly. Cualquier 401 del cliente
// HTTP pasa por `refreshSession` (single-flight en el propio cliente).
import { createApiClient, type ApiClient, type User } from '@tiendas/tracking-core'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  loginRequest,
  logoutRequest,
  refreshRequest,
  type SessionPayload,
} from '@/lib/session-client'

export type AuthStatus = 'loading' | 'authenticated' | 'anonymous'

export interface AuthContextValue {
  status: AuthStatus
  user: User | null
  token: string | null
  api: ApiClient
  login: (email: string, password: string) => Promise<User>
  logout: () => Promise<void>
  refreshSession: () => Promise<string | null>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [status, setStatus] = useState<AuthStatus>('loading')

  const tokenRef = useRef<string | null>(null)
  const bootstrapRef = useRef<Promise<string | null> | null>(null)

  const applySession = useCallback((payload: SessionPayload | null) => {
    tokenRef.current = payload?.access_token ?? null
    setToken(payload?.access_token ?? null)
    setUser(payload?.user ?? null)
    setStatus(payload ? 'authenticated' : 'anonymous')
  }, [])

  const refreshSession = useCallback(async (): Promise<string | null> => {
    try {
      const payload = await refreshRequest()
      applySession(payload)
      return payload?.access_token ?? null
    } catch {
      // Fallo transitorio: la cookie sigue ahí, pero la UI no puede operar.
      applySession(null)
      return null
    }
  }, [applySession])

  // Bootstrap único incluso con el doble montaje de React Strict Mode: dos
  // refresh simultáneos consumirían la rotación del token.
  useEffect(() => {
    if (!bootstrapRef.current) bootstrapRef.current = refreshSession()
    void bootstrapRef.current
  }, [refreshSession])

  const api = useMemo(
    () =>
      // El getter se ejecuta solo al enviar una petición (imperativo), nunca
      // durante el render; la regla no puede distinguir el closure.
      // eslint-disable-next-line react-hooks/refs -- lectura del token en peticiones, no en render
      createApiClient({
        baseUrl: '/api/backend',
        getToken: () => tokenRef.current,
        refresh: refreshSession,
      }),
    [refreshSession],
  )

  const login = useCallback(
    async (email: string, password: string) => {
      const payload = await loginRequest(email, password)
      applySession(payload)
      return payload.user
    },
    [applySession],
  )

  const logout = useCallback(async () => {
    await logoutRequest()
    applySession(null)
  }, [applySession])

  const value = useMemo<AuthContextValue>(
    () => ({ status, user, token, api, login, logout, refreshSession }),
    [status, user, token, api, login, logout, refreshSession],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth debe usarse dentro de <AuthProvider>')
  return context
}
