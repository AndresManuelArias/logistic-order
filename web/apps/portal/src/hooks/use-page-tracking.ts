'use client'

// Suscripción de tiempo real atada a la sesión: cuando el canal cierra por
// token caducado (4401), intenta renovar una vez; si no hay sesión, manda al
// login. El hook de la librería se encarga del backoff y la limpieza.
import { useTracking, type UseTrackingResult } from '@tiendas/tracking-core'
import { useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { useAuth } from '@/components/auth-provider'

export const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? 'ws://localhost:8080/ws'

export function usePageTracking(channels: string[]): UseTrackingResult {
  const { token, status, refreshSession } = useAuth()
  const router = useRouter()

  const handleExpired = useCallback(async () => {
    const fresh = await refreshSession()
    if (!fresh) {
      router.replace('/login?expired=1')
      return
    }
    // `token` cambia y useTracking recrea la conexión con el token nuevo.
  }, [refreshSession, router])

  return useTracking({
    url: WS_URL,
    token,
    channels,
    enabled: status === 'authenticated' && Boolean(token),
    onSessionExpired: handleExpired,
  })
}
