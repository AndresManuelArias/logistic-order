import type { Order } from '@tiendas/tracking-core'
import type { ReactNode } from 'react'
import { formatDateTime, formatDistance } from '../lib/format'
import { OrderStatusBadge } from './status-badge'

export interface OrderCardProps {
  order: Order
  vehiclePlate?: string
  selected?: boolean
  onSelect?: (order: Order) => void
  actions?: ReactNode
  extra?: ReactNode
}

// Tarjeta de orden reutilizable por los tres paneles. Las acciones se inyectan
// desde cada rol: la tarjeta no decide qué puede hacer el usuario.
export function OrderCard({ order, vehiclePlate, selected = false, onSelect, actions, extra }: OrderCardProps) {
  return (
    <article className={`tt-order-card${selected ? ' is-selected' : ''}`}>
      <header className="tt-order-card__head">
        <h3 className="tt-order-card__title">{order.external_ref}</h3>
        <OrderStatusBadge status={order.status} />
      </header>

      <p className="tt-order-card__address">{order.destination_address ?? 'Destino sin dirección'}</p>

      <dl className="tt-order-card__meta">
        <div>
          <dt>Distancia</dt>
          <dd>{formatDistance(order.distance_meters)}</dd>
        </div>
        <div>
          <dt>Creada</dt>
          <dd>{formatDateTime(order.placed_at)}</dd>
        </div>
        <div>
          <dt>Vehículo</dt>
          <dd>{vehiclePlate ?? 'Sin asignar'}</dd>
        </div>
      </dl>

      {extra}

      {onSelect || actions ? (
        <footer className="tt-order-card__footer">
          {onSelect ? (
            <button
              type="button"
              className="tt-button tt-button--ghost"
              aria-pressed={selected}
              onClick={() => onSelect(order)}
            >
              {selected ? 'Seleccionada' : 'Ver detalle'}
            </button>
          ) : null}
          {actions}
        </footer>
      ) : null}
    </article>
  )
}
