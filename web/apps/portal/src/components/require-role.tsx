'use client'

import { ROLE_LABEL, homeForRole, type Role } from '@tiendas/tracking-core'
import { LoadingPanel } from '@tiendas/tracking-ui'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { useEffect, type ReactNode } from 'react'
import { useAuth } from './auth-provider'

export interface RequireRoleProps {
  roles: readonly Role[]
  children: ReactNode
}

// Guarda de navegación. Mejora la UX, pero no es la frontera de seguridad: el
// backend vuelve a comprobar el rol y el alcance en cada petición (RF-02).
export function RequireRole({ roles, children }: RequireRoleProps) {
  const { status, user } = useAuth()
  const router = useRouter()
  const pathname = usePathname()

  useEffect(() => {
    if (status === 'anonymous') {
      router.replace(`/login?next=${encodeURIComponent(pathname)}`)
    }
  }, [status, router, pathname])

  if (status === 'loading') {
    return (
      <div className="tt-page">
        <LoadingPanel label="Comprobando sesión…" />
      </div>
    )
  }

  if (!user) return null

  if (!roles.includes(user.role)) {
    return (
      <div className="tt-page">
        <div className="tt-state tt-state--error" role="alert">
          <p className="tt-state__title">Sin acceso a esta sección</p>
          <p className="tt-state__text">
            Tu rol ({ROLE_LABEL[user.role]}) no puede ver este panel. El servidor aplica la autorización final.
          </p>
          <div className="tt-state__action">
            <Link className="tt-button" href={homeForRole(user.role)}>
              Ir a mi panel
            </Link>
          </div>
        </div>
      </div>
    )
  }

  return <>{children}</>
}
