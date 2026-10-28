'use client'

import {
  ApiError,
  fleetStoreChannel,
  type Order,
  type OrderStatus,
} from '@tiendas/tracking-core'
import {
  ConnectionPill,
  EmptyState,
  ErrorPanel,
  KpiCard,
  LoadingPanel,
  OrderCard,
  OrderProgress,
  SegmentedTabs,
  TrackingMap,
  formatFreshness,
  formatSpeed,
  positionTone,
  tabDomId,
  tabPanelDomId,
  type TrackingMarker,
} from '@tiendas/tracking-ui'
import { useCallback, useMemo, useState } from 'react'
import { AiAssistant } from '@/components/ai-assistant'
import { useAuth } from '@/components/auth-provider'
import { CreateOrderForm } from '@/components/create-order-form'
import { ShareTracking } from '@/components/share-tracking'
import { StoreSetup } from '@/components/store-setup'
import { useApiResource } from '@/hooks/use-api-resource'
import { usePageTracking } from '@/hooks/use-page-tracking'

type StoreTab = 'entrada' | 'salida'
type OrderAction = Exclude<OrderStatus, 'CREATED'>

export function StoreDashboard() {
  const { user, api } = useAuth()
  const loadStore = useCallback(() => api.getMyStore(), [api])
  const loadOrders = useCallback(() => api.listOrders({ limit: 100 }), [api])
  const loadFleet = useCallback(() => api.listVehicles({ limit: 200 }), [api])

  const store = useApiResource(loadStore, { enabled: Boolean(user) })
  const storeId = store.data?.id ?? ''
  const orders = useApiResource(loadOrders, {
    enabled: Boolean(storeId),
    reloadKey: storeId,
  })
  const fleet = useApiResource(loadFleet, {
    enabled: Boolean(storeId),
    reloadKey: storeId,
  })
  const tracking = usePageTracking(storeId ? [fleetStoreChannel(storeId)] : [])

  const [tab, setTab] = useState<StoreTab>('entrada')
  const [pendingId, setPendingId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<unknown>(null)

  const orderItems = orders.data?.items ?? []
  const entrada = orderItems.filter((order) => order.status === 'CREATED')
  const salida = orderItems.filter((order) => order.status !== 'CREATED')

  const vehicles = useMemo(() => {
    const items = fleet.data?.items ?? []
    return items.map((vehicle) => {
      const live = tracking.latest[vehicle.id]
      return live ? { ...vehicle, last_position: live } : vehicle
    })
  }, [fleet.data, tracking.latest])

  const markers = useMemo<TrackingMarker[]>(
    () =>
      vehicles.flatMap((vehicle) => {
        const point = vehicle.last_position
        if (!point) return []
        return [
          {
            id: vehicle.id,
            label: vehicle.plate,
            lat: point.lat,
            lon: point.lon,
            tone: positionTone(point),
            detail: `${formatSpeed(point.speed_kph)} · ${formatFreshness(point.time)}`,
          },
        ]
      }),
    [vehicles],
  )

  const delivered = orderItems.filter((order) => order.status === 'DELIVERED').length

  async function transition(order: Order, status: OrderAction) {
    if (status === 'DELIVERED' && !window.confirm(`¿Marcar la orden ${order.external_ref} como entregada?`)) return
    if (status === 'CANCELLED' && !window.confirm(`¿Cancelar la orden ${order.external_ref}?`)) return

    setPendingId(order.id)
    setActionError(null)
    try {
      await api.changeOrderStatus(order.id, status)
      orders.reload()
      fleet.reload()
    } catch (reason) {
      setActionError(reason)
    } finally {
      setPendingId(null)
    }
  }

  const storeMissing = store.error instanceof ApiError && store.error.isNotFound

  if (store.loading && !store.data) {
    return (
      <div className="tt-page">
        <LoadingPanel label="Cargando tu tienda…" />
      </div>
    )
  }

  if (storeMissing) {
    return (
      <div className="tt-page">
        <header className="tt-page__head">
          <div>
            <h1 className="tt-page__title">Mi tienda</h1>
            <p className="tt-page__subtitle">
              La vista dual de entrada y salida se habilita cuando registras la ubicación de tu tienda.
            </p>
          </div>
        </header>
        <StoreSetup onCreated={store.reload} />
      </div>
    )
  }

  if (store.error) {
    return (
      <div className="tt-page">
        <ErrorPanel
          error={store.error}
          title="No se pudo cargar tu tienda"
          onRetry={store.reload}
        />
      </div>
    )
  }

  if (!store.data) return null
  const myStore = store.data

  function renderOrders(items: Order[], emptyTitle: string, emptyDescription: string) {
    if (orders.loading && !orders.data) return <LoadingPanel label="Cargando órdenes…" />
    if (items.length === 0) {
      return <EmptyState title={emptyTitle} description={emptyDescription} />
    }
    return (
      <ul className="tt-list">
        {items.map((order) => {
          const busy = pendingId === order.id
          const canTransition = order.status === 'CREATED' || order.status === 'IN_TRANSIT'
          return (
            <li key={order.id}>
              <OrderCard
                order={order}
                vehiclePlate={
                  order.assigned_vehicle_id
                    ? vehicles.find((vehicle) => vehicle.id === order.assigned_vehicle_id)?.plate
                    : undefined
                }
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
                        {busy ? 'Guardando…' : 'Marcar en tránsito'}
                      </button>
                    ) : null}
                    {order.status === 'IN_TRANSIT' ? (
                      <button
                        type="button"
                        className="tt-button"
                        disabled={busy}
                        onClick={() => transition(order, 'DELIVERED')}
                      >
                        {busy ? 'Guardando…' : 'Marcar entregada'}
                      </button>
                    ) : null}
                    {canTransition ? (
                      <button
                        type="button"
                        className="tt-button tt-button--danger"
                        disabled={busy}
                        onClick={() => transition(order, 'CANCELLED')}
                      >
                        Cancelar
                      </button>
                    ) : null}
                    {order.status !== 'CANCELLED' ? <ShareTracking orderId={order.id} /> : null}
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
          <h1 className="tt-page__title">{myStore.name}</h1>
          <p className="tt-page__subtitle">
            Vista dual: <strong>entrada</strong> son las órdenes creadas por tu tienda pendientes de despacho;{' '}
            <strong>salida</strong> son las que ya están en tránsito o cerradas.
          </p>
        </div>
        <div className="tt-page__actions">
          <ConnectionPill status={tracking.status} onReconnect={tracking.reconnect} />
          <button
            type="button"
            className="tt-button tt-button--ghost"
            onClick={() => {
              orders.reload()
              fleet.reload()
            }}
            disabled={orders.loading}
          >
            {orders.loading ? 'Actualizando…' : 'Actualizar'}
          </button>
        </div>
      </header>

      {tracking.error ? (
        <p className="tt-form__error" role="status">
          Canal en vivo: {tracking.error.message}
        </p>
      ) : null}

      <section className="tt-grid" aria-label="Indicadores de la tienda">
        <KpiCard label="Entradas" value={entrada.length} hint="Pendientes de despacho" tone="pending" />
        <KpiCard label="En tránsito" value={salida.filter((order) => order.status === 'IN_TRANSIT').length} hint="Hacia clientes" tone="live" />
        <KpiCard label="Entregadas" value={delivered} hint="Histórico visible" />
        <KpiCard label="Vehículos" value={vehicles.length} hint="Con órdenes de tu tienda" />
      </section>

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

      <div className="tt-columns">
        <section className="tt-card" aria-labelledby="store-orders-title">
          <div className="tt-section__head">
            <h2 className="tt-section__title" id="store-orders-title">
              Órdenes
            </h2>
            <CreateOrderForm store={myStore} onCreated={orders.reload} />
          </div>

          <SegmentedTabs
            ariaLabel="Vista dual de órdenes"
            value={tab}
            onChange={(value) => setTab(value as StoreTab)}
            items={[
              { id: 'entrada', label: 'Entrada', count: entrada.length },
              { id: 'salida', label: 'Salida', count: salida.length },
            ]}
          />

          <div
            id={tabPanelDomId('entrada')}
            role="tabpanel"
            aria-labelledby={tabDomId('entrada')}
            hidden={tab !== 'entrada'}
          >
            {renderOrders(
              entrada,
              'Sin entradas pendientes',
              'Las órdenes creadas aparecerán aquí hasta que se despachen.',
            )}
          </div>

          <div
            id={tabPanelDomId('salida')}
            role="tabpanel"
            aria-labelledby={tabDomId('salida')}
            hidden={tab !== 'salida'}
          >
            {renderOrders(
              salida,
              'Sin salidas registradas',
              'Cuando una orden entre en tránsito o se entregue aparecerá en esta vista.',
            )}
          </div>

          {orders.error ? <ErrorPanel error={orders.error} onRetry={orders.reload} /> : null}
        </section>

        <section className="tt-card" aria-labelledby="store-map-title">
          <div className="tt-section__head">
            <h2 className="tt-section__title" id="store-map-title">
              Vehículos de tus órdenes
            </h2>
            <span className="tt-section__hint">Última posición conocida</span>
          </div>
          <TrackingMap
            markers={markers}
            summary={`${markers.length} de ${vehicles.length} vehículo(s) con posición`}
            emptyLabel="Aún no hay vehículos reportando para tus órdenes."
          />
          {fleet.error ? <ErrorPanel error={fleet.error} onRetry={fleet.reload} /> : null}
        </section>
      </div>

      <AiAssistant />
    </div>
  )
}
