import type { OrderStatus, TelemetryPoint } from '../api/types'

// Canales del hub WS (ADR-08). El backend rechaza cualquier otro formato.
export const fleetProviderChannel = (providerId: string): string => `fleet:provider:${providerId}`
export const fleetStoreChannel = (storeId: string): string => `fleet:store:${storeId}`
export const orderChannel = (orderId: string): string => `order:${orderId}`
export const vehicleChannel = (vehicleId: string): string => `vehicle:${vehicleId}`

export type TrackingStatus =
  | 'idle'
  | 'connecting'
  | 'open'
  | 'reconnecting'
  | 'closed'
  | 'session-expired'

export const TRACKING_STATUS_LABEL: Record<TrackingStatus, string> = {
  idle: 'Sin conexión',
  connecting: 'Conectando…',
  open: 'En vivo',
  reconnecting: 'Reconectando…',
  closed: 'Desconectado',
  'session-expired': 'Sesión caducada',
}

export type TrackingStatusTone = 'live' | 'pending' | 'offline' | 'danger'

export function trackingStatusTone(status: TrackingStatus): TrackingStatusTone {
  switch (status) {
    case 'open':
      return 'live'
    case 'connecting':
    case 'reconnecting':
      return 'pending'
    case 'session-expired':
      return 'danger'
    default:
      return 'offline'
  }
}

// Evento de posición entregado por el canal `event` del WS.
export interface FleetEvent {
  channel: string
  at: string
  data: TelemetryPoint
}

// Clave de deduplicación: la PK del backend es (time, vehicle_id, device_seq),
// así que un reintento del dispositivo no debe pintarse dos veces.
export function pointKey(p: Pick<TelemetryPoint, 'vehicle_id' | 'time' | 'device_seq'>): string {
  return `${p.vehicle_id}|${p.time}|${p.device_seq}`
}

export function isNewerPoint(incoming: TelemetryPoint, current: TelemetryPoint | undefined): boolean {
  if (!current) return true
  return Date.parse(incoming.time) > Date.parse(current.time)
}

export function mergeLatestPoint(
  current: Record<string, TelemetryPoint>,
  incoming: TelemetryPoint,
): Record<string, TelemetryPoint> {
  if (!isNewerPoint(incoming, current[incoming.vehicle_id])) return current
  return { ...current, [incoming.vehicle_id]: incoming }
}

export const ORDER_STATUS_LABEL: Record<OrderStatus, string> = {
  CREATED: 'Creada',
  IN_TRANSIT: 'En tránsito',
  DELIVERED: 'Entregada',
  CANCELLED: 'Cancelada',
}

// Estados que representan una entrega ya cerrada.
export function isFinalStatus(status: OrderStatus): boolean {
  return status === 'DELIVERED' || status === 'CANCELLED'
}
