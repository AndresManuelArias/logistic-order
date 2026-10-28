'use client'

import { homeForRole } from '@tiendas/tracking-core'
import Link from 'next/link'
import { useAuth } from '@/components/auth-provider'

const ROLES = [
  {
    title: 'Proveedor',
    description: 'Flota B2B completa, última posición de cada vehículo y traza histórica sobre el mapa.',
  },
  {
    title: 'Tendero',
    description: 'Vista dual de entrada y salida, creación de órdenes y enlaces de seguimiento para tus clientes.',
  },
  {
    title: 'Cliente final',
    description: 'Seguimiento temporal de su pedido con un enlace de invitado, sin crear cuenta.',
  },
] as const

export default function HomePage() {
  const { status, user } = useAuth()
  const primaryHref = user ? homeForRole(user.role) : '/login'
  const primaryLabel = user ? 'Ir a mi panel' : 'Iniciar sesión'

  return (
    <div className="tt-landing">
      <header className="tt-landing__header">
        <Link className="tt-brand" href="/" aria-label="Tiendas-Track, inicio">
          <span className="tt-brand__mark" aria-hidden="true">
            TT
          </span>
          <span className="tt-brand__name">Tiendas-Track</span>
        </Link>
        <div className="tt-inline">
          {status === 'authenticated' && user ? (
            <span className="tt-badge tt-badge--neutral">{user.full_name}</span>
          ) : null}
          <Link className="tt-button" href={primaryHref}>
            {primaryLabel}
          </Link>
        </div>
      </header>

      <main className="tt-landing__main">
        <div>
          <p className="tt-badge tt-badge--live">Visibilidad logística en tiempo real</p>
          <h1 className="tt-landing__title">Cada pedido, en el mapa correcto.</h1>
          <p className="tt-landing__lead">
            Tiendas-Track conecta proveedores, tiendas y clientes finales sobre una misma traza GPS.
            Cada rol ve exclusivamente su ámbito: la autorización se aplica en el servidor, no en el navegador.
          </p>
        </div>

        <div className="tt-role-grid">
          {ROLES.map((role) => (
            <section className="tt-role-card" key={role.title}>
              <h3>{role.title}</h3>
              <p>{role.description}</p>
            </section>
          ))}
        </div>

        <div className="tt-inline">
          <Link className="tt-button" href={primaryHref}>
            {primaryLabel}
          </Link>
          <span className="tt-muted">¿Tienes un enlace de seguimiento? Ábrelo directamente.</span>
        </div>
      </main>

      <footer className="tt-landing__foot">
        Tiendas-Track · API REST + WebSocket · Datos GPS con PostGIS y TimescaleDB
      </footer>
    </div>
  )
}
