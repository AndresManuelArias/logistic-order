import type { OrderStatus, TelemetryPoint } from '@tiendas/tracking-core'
import { ORDER_STATUS_LABEL } from '@tiendas/tracking-core'

// Tonos visuales compartidos por badges, píldoras y marcadores del mapa.
export type Tone = 'live' | 'pending' | 'offline' | 'danger' | 'neutral'

export interface StatusBadgeProps {
  label: string
  tone?: Tone
  className?: string
}

export function StatusBadge({ label, tone = 'neutral', className }: StatusBadgeProps) {
  const classes = ['tt-badge', `tt-badge--${tone}`, className].filter(Boolean).join(' ')
  return <span className={classes}>{label}</span>
}

export function orderStatusTone(status: OrderStatus): Tone {
  switch (status) {
    case 'CREATED':
      return 'pending'
    case 'IN_TRANSIT':
      return 'live'
    case 'DELIVERED':
      return 'neutral'
    case 'CANCELLED':
      return 'danger'
    default:
      return 'neutral'
  }
}

export function OrderStatusBadge({ status }: { status: OrderStatus }) {
  return <StatusBadge label={ORDER_STATUS_LABEL[status]} tone={orderStatusTone(status)} />
}

// Frescura de una posición: más de 5 min sin reportar es "sin señal".
const STALE_AFTER_MS = 5 * 60_000

export function positionTone(point: Pick<TelemetryPoint, 'time'> | null | undefined, now = Date.now()): Tone {
  if (!point) return 'offline'
  const time = Date.parse(point.time)
  if (Number.isNaN(time)) return 'offline'
  return now - time > STALE_AFTER_MS ? 'pending' : 'live'
}
