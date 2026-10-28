import {
  TRACKING_STATUS_LABEL,
  trackingStatusTone,
  type TrackingStatus,
} from '@tiendas/tracking-core'

export interface ConnectionPillProps {
  status: TrackingStatus
  onReconnect?: () => void
}

// Estado de la conexión de tiempo real. `role="status"` + `aria-live` avisa a
// lectores de pantalla cuando se cae o vuelve el canal.
export function ConnectionPill({ status, onReconnect }: ConnectionPillProps) {
  const tone = trackingStatusTone(status)
  const canRetry = status === 'closed' || status === 'reconnecting'
  return (
    <span className={`tt-pill tt-pill--${tone}`} role="status" aria-live="polite">
      <span className="tt-pill__dot" aria-hidden="true" />
      <span>{TRACKING_STATUS_LABEL[status]}</span>
      {canRetry && onReconnect ? (
        <button type="button" className="tt-pill__action" onClick={onReconnect}>
          Reintentar
        </button>
      ) : null}
    </span>
  )
}
