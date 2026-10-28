import type { Order, OrderStatus } from '../contracts/types'

export const ACTIVE_STATUSES: readonly OrderStatus[] = ['CREATED', 'IN_TRANSIT']

/** Máquina de estados del backend (docs/OpenAPI). Delivered y cancelled son finales. */
export const ALLOWED_TRANSITIONS: Record<OrderStatus, readonly OrderStatus[]> = {
  CREATED: ['IN_TRANSIT', 'CANCELLED'],
  IN_TRANSIT: ['DELIVERED', 'CANCELLED'],
  DELIVERED: [],
  CANCELLED: [],
}

export interface DriverAction {
  target: OrderStatus
  label: string
  /** Texto de confirmación mostrado en el detalle. */
  confirmation: string
}

/**
 * Acciones que la app móvil ofrece al conductor. Cancelar una orden existe en
 * el backend (tienda dueña o conductor asignado) pero no se expone aquí porque
 * el alcance F4 del conductor es iniciar/entregar; ver mobile/README.md.
 */
const DRIVER_ACTIONS: Partial<Record<OrderStatus, DriverAction>> = {
  CREATED: {
    target: 'IN_TRANSIT',
    label: 'Iniciar entrega',
    confirmation: '¿Confirmas que recogiste el pedido y vas en camino?',
  },
  IN_TRANSIT: {
    target: 'DELIVERED',
    label: 'Confirmar entrega',
    confirmation: '¿Confirmas que el pedido fue entregado?',
  },
}

export function isTerminal(status: OrderStatus): boolean {
  return status === 'DELIVERED' || status === 'CANCELLED'
}

export function isActive(status: OrderStatus): boolean {
  return status === 'CREATED' || status === 'IN_TRANSIT'
}

export function canTransition(from: OrderStatus, to: OrderStatus): boolean {
  return ALLOWED_TRANSITIONS[from].includes(to)
}

export function nextDriverAction(status: OrderStatus): DriverAction | null {
  return DRIVER_ACTIONS[status] ?? null
}

/**
 * Estado que ve el conductor: el del servidor con la transición pendiente
 * aplicada de forma optimista. Si la transición encolada no es legal desde el
 * estado actual (p. ej. cambió en otro dispositivo) se devuelve el del servidor.
 */
export function effectiveStatus(order: Order, pendingTarget?: OrderStatus | null): OrderStatus {
  if (pendingTarget && canTransition(order.status, pendingTarget)) return pendingTarget
  return order.status
}

function sameLocalDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

/** Una orden es "del día" si se creó hoy o sigue activa (aunque venga de ayer). */
export function isOrderOfDay(order: Order, now: Date): boolean {
  if (isActive(order.status)) return true
  const placed = new Date(order.placed_at)
  if (Number.isNaN(placed.getTime())) return false
  return sameLocalDay(placed, now)
}

export function selectDayOrders(orders: readonly Order[], now: Date): Order[] {
  return orders
    .filter((order) => isOrderOfDay(order, now))
    .slice()
    .sort(compareOrders)
}

export function compareOrders(a: Order, b: Order): number {
  const aActive = isActive(a.status) ? 0 : 1
  const bActive = isActive(b.status) ? 0 : 1
  if (aActive !== bActive) return aActive - bActive
  const aTime = new Date(a.placed_at).getTime()
  const bTime = new Date(b.placed_at).getTime()
  const safeA = Number.isNaN(aTime) ? 0 : aTime
  const safeB = Number.isNaN(bTime) ? 0 : bTime
  // Activas: la más antigua primero. Finalizadas: la más reciente primero.
  return aActive === 0 ? safeA - safeB : safeB - safeA
}
