'use client'

import { ROLE_LABEL, type Role } from '@tiendas/tracking-core'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { useState, type ReactNode } from 'react'
import { useAuth } from './auth-provider'

const NAV: ReadonlyArray<{ href: string; label: string; roles: readonly Role[] }> = [
  { href: '/proveedor', label: 'Flota', roles: ['PROVIDER'] },
  { href: '/proveedor/tiendas', label: 'Tiendas', roles: ['PROVIDER'] },
  { href: '/tienda', label: 'Mi tienda', roles: ['STORE'] },
  { href: '/conductor', label: 'Mis entregas', roles: ['DRIVER'] },
]

// Armazón del portal autenticado: navegación filtrada por rol (la autorización
// real la aplica el backend) y cierre de sesión explícito.
export function AppShell({ children }: { children: ReactNode }) {
  const { status, user, logout } = useAuth()
  const pathname = usePathname()
  const router = useRouter()
  const [leaving, setLeaving] = useState(false)

  async function handleLogout() {
    setLeaving(true)
    await logout()
    router.replace('/login')
  }

  const links = user ? NAV.filter((item) => item.roles.includes(user.role)) : []

  return (
    <div className="tt-shell">
      <a className="tt-skip" href="#contenido">
        Saltar al contenido
      </a>
      <header className="tt-app-header">
        <Link className="tt-brand" href="/">
          <span className="tt-brand__mark" aria-hidden="true">
            TT
          </span>
          <span className="tt-brand__name">Tiendas-Track</span>
        </Link>

        <nav className="tt-nav" aria-label="Navegación principal">
          {links.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="tt-nav__link"
              aria-current={pathname === item.href ? 'page' : undefined}
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="tt-user">
          {user ? (
            <>
              <span className="tt-user__name">{user.full_name}</span>
              <span className="tt-badge tt-badge--neutral">{ROLE_LABEL[user.role]}</span>
              <button type="button" className="tt-button tt-button--ghost" onClick={handleLogout} disabled={leaving}>
                {leaving ? 'Saliendo…' : 'Salir'}
              </button>
            </>
          ) : status === 'loading' ? (
            <span className="tt-user__name">Cargando sesión…</span>
          ) : (
            <Link className="tt-button" href="/login">
              Iniciar sesión
            </Link>
          )}
        </div>
      </header>

      <main className="tt-main" id="contenido">
        {children}
      </main>

      <footer className="tt-footer">
        Tiendas-Track · Portal multi-rol · Posiciones en vivo por WebSocket
      </footer>
    </div>
  )
}
