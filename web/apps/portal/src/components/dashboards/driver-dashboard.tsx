'use client'

// Vista de respaldo del conductor en el portal. La operación en campo (GPS en
// segundo plano y cola offline) vive en la app móvil; aquí solo se consultan y
// cierran las órdenes asignadas, con la misma autorización del servidor.
import type { Order, OrderStatus } from '@tiendas/tracking-core'
import {
  EmptyState,
  ErrorPanel,
  LoadingPanel,
  OrderCard,
  OrderProgress,
} from '@tiendas/tracking-ui'
import { useCallback, useState } from 'react'
import { useAuth } from '@/components/auth-provider'
import { useApiResource } from '@/hooks/use-api-resource'

type OrderAction = Exclude<OrderStatus, 'CREATED'>

export function DriverDashboard() {
  const { user, api } = useAuth()
  const loadOrders = useCallback(() => api.listOrders({ limit: 100 }), [api])
  const orders = useApiResource(loadOrders, {
    enabled: Boolean(user),
    reloadKey: user?.id ?? '',
  })
  const [pendingId, setPendingId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<unknown>(null)

  const items = orders.data?.items ?? []
  const active = items.filter((order) => order.status === 'CREATED' || order.status === 'IN_TRANSIT')
  const closed = items.filter((order) => order.status === 'DELIVERED' || order.status === 'CANCELLED')

  async function transition(order: Order, status: OrderAction) {
    if (status === 'DELIVERED' && !window.confirm(`¿Confirmas la entrega de ${order.external_ref}?`)) return
    setPendingId(order.id)
    setActionError(null)
    try {
      await api.changeOrderStatus(order.id, status)
      orders.reload()
    } catch (reason) {
      setActionError(reason)
    } finally {
      setPendingId(null)
    }
  }

  function renderOrders(list: Order[], emptyTitle: string, emptyDescription: string) {
    if (list.length === 0) {
      return <EmptyState title={emptyTitle} description={emptyDescription} />
    }
    return (
      <ul className="tt-list">
        {list.map((order) => {
          const busy = pendingId === order.id
          return (
            <li key={order.id}>
              <OrderCard
                order={order}
                extra={<OrderProgress order={order} />}
                actions={
                  <div className="tt-inline">
                    {order.status === 'CREATED' ? (
                      <button
                        type="button"
                        className="tt-button"
                        disabled={busy}
                        onClick={() => transition(order, 'IN_TRANSIT')}
                      >
                        {busy ? 'Guardando…' : 'Iniciar ruta'}
                      </button>
                    ) : null}
                    {order.status === 'IN_TRANSIT' ? (
                      <button
                        type="button"
                        className="tt-button"
                        disabled={busy}
                        onClick={() => transition(order, 'DELIVERED')}
                      >
                        {busy ? 'Guardando…' : 'Confirmar entrega'}
                      </button>
                    ) : null}
                  </div>
                }
              />
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <div className="tt-page">
      <header className="tt-page__head">
        <div>
          <h1 className="tt-page__title">Mis entregas</h1>
          <p className="tt-page__subtitle">
            Órdenes asignadas a tu vehículo. La captura GPS y el trabajo sin conexión se realizan en la app móvil del
            conductor.
          </p>
        </div>
        <button
          type="button"
          className="tt-button tt-button--ghost"
          onClick={orders.reload}
          disabled={orders.loading}
        >
          {orders.loading ? 'Actualizando…' : 'Actualizar'}
        </button>
      </header>

      {actionError ? (
        <ErrorPanel
          error={actionError}
          title="No se pudo actualizar la orden"
          action={
            <button type="button" className="tt-button tt-button--ghost" onClick={() => setActionError(null)}>
              Cerrar
            </button>
          }
        />
      ) : null}

      {orders.error ? <ErrorPanel error={orders.error} onRetry={orders.reload} /> : null}
      {orders.loading && !orders.data ? <LoadingPanel label="Cargando tus órdenes…" /> : null}

      {orders.data ? (
        <>
          <section className="tt-card" aria-labelledby="active-orders-title">
            <h2 className="tt-section__title" id="active-orders-title">
              En curso ({active.length})
            </h2>
            {renderOrders(active, 'Sin entregas en curso', 'Cuando te asignen una orden aparecerá aquí.')}
          </section>

          <section className="tt-card" aria-labelledby="closed-orders-title">
            <h2 className="tt-section__title" id="closed-orders-title">
              Cerradas ({closed.length})
            </h2>
            {renderOrders(closed, 'Sin entregas cerradas', 'Las órdenes entregadas o canceladas se listan aquí.')}
          </section>
        </>
      ) : null}
    </div>
  )
}
