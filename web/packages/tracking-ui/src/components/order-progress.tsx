import type { Order, OrderStatus } from '@tiendas/tracking-core'
import { formatDateTime } from '../lib/format'
import { StatusBadge } from './status-badge'

type ProgressOrder = Pick<Order, 'status' | 'placed_at' | 'picked_up_at' | 'delivered_at'>

interface Step {
  status: Exclude<OrderStatus, 'CANCELLED'>
  label: string
  dateKey: 'placed_at' | 'picked_up_at' | 'delivered_at'
}

const STEPS: readonly Step[] = [
  { status: 'CREATED', label: 'Creada', dateKey: 'placed_at' },
  { status: 'IN_TRANSIT', label: 'En tránsito', dateKey: 'picked_up_at' },
  { status: 'DELIVERED', label: 'Entregada', dateKey: 'delivered_at' },
]

// Barra de progreso del ciclo de vida. Los estados se anuncian también con
// texto oculto: el color nunca es el único portador de la información.
export function OrderProgress({ order }: { order: ProgressOrder }) {
  if (order.status === 'CANCELLED') {
    return (
      <div className="tt-progress tt-progress--cancelled" role="status">
        <StatusBadge label="Cancelada" tone="danger" />
        <span className="tt-progress__note">
          La orden se canceló. Creada el {formatDateTime(order.placed_at)}.
        </span>
      </div>
    )
  }

  const currentIndex = STEPS.findIndex((step) => step.status === order.status)

  return (
    <ol className="tt-progress" aria-label="Progreso de la orden">
      {STEPS.map((step, index) => {
        const state = index < currentIndex ? 'done' : index === currentIndex ? 'current' : 'pending'
        const date = order[step.dateKey]
        return (
          <li
            key={step.status}
            className={`tt-progress__step is-${state}`}
            aria-current={state === 'current' ? 'step' : undefined}
          >
            <span className="tt-progress__bullet" aria-hidden="true">
              {index + 1}
            </span>
            <span className="tt-progress__body">
              <span className="tt-progress__label">{step.label}</span>
              <span className="tt-progress__date">{date ? formatDateTime(date) : 'Pendiente'}</span>
            </span>
            {state === 'done' ? <span className="tt-visually-hidden">Completado</span> : null}
            {state === 'current' ? <span className="tt-visually-hidden">Estado actual</span> : null}
          </li>
        )
      })}
    </ol>
  )
}
