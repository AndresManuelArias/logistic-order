import React, { createContext, useContext, useSyncExternalStore } from 'react'

import type { AppServices } from '../../composition/services'
import type { SessionSnapshot } from '../../ports/auth'
import type { SyncStatusSnapshot } from '../../domain/sync'

const ServicesContext = createContext<AppServices | null>(null)

export function ServicesProvider({
  services,
  children,
}: {
  services: AppServices
  children: React.ReactNode
}) {
  return <ServicesContext.Provider value={services}>{children}</ServicesContext.Provider>
}

export function useServices(): AppServices {
  const services = useContext(ServicesContext)
  if (!services) {
    throw new Error('useServices debe usarse dentro de <ServicesProvider>')
  }
  return services
}

/** Suscripción al estado de sesión (login/logout/refresh). */
export function useSession(): SessionSnapshot {
  const { session } = useServices()
  return useSyncExternalStore(session.subscribe, session.getSnapshot, session.getSnapshot)
}

/** Suscripción al estado de sincronización (pendientes, conectividad, errores). */
export function useSyncStatus(): SyncStatusSnapshot {
  const { status } = useServices()
  return useSyncExternalStore(status.subscribe, status.getSnapshot, status.getSnapshot)
}

/**
 * Datos locales aislados por conductor. Mientras no haya usuario autenticado no
 * se consulta ninguna tabla, de modo que una sesión cerrada no expone la ruta
 * de la cuenta anterior.
 */
export function useOwnerId(): string | null {
  const session = useSession()
  return session.user?.id ?? null
}
