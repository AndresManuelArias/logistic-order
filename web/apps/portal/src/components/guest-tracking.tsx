'use client'

// Vista pública de una orden. El token de invitado llega en el fragmento
// (#t=…) y solo autoriza esta orden: el cliente final no tiene sesión ni puede
// consultar flota, tienda u otras órdenes.
import {
  createApiClient,
  orderChannel,
  ApiError,
  useTracking,
} from '@tiendas/tracking-core'
import {
  ConnectionPill,
  ErrorPanel,
  LoadingPanel,
  OrderProgress,
  StatusBadge,
  TrackingMap,
  formatDateTime,
  formatDistance,
  formatFreshness,
  formatSpeed,
  type TrackingMarker,
  type TrackingTrail,
} from '@tiendas/tracking-ui'
import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useCallback, useMemo, useState, useSyncExternalStore } from 'react'
import { useApiResource } from '@/hooks/use-api-resource'
import { parseGuestToken } from '@/lib/guest-token'

const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? 'ws://localhost:8080/ws'

// El fragmento es un sistema externo: `useSyncExternalStore` lo lee sin
// provocar un render en cascada y reacciona si el usuario cambia de enlace.
function subscribeToHash(onChange: () => void): () => void {
  window.addEventListener('hashchange', onChange)
  return () => window.removeEventListener('hashchange', onChange)
}

function readGuestToken(): string | null {
  return parseGuestToken(window.location.hash)
}

const subscribeNothing = () => () => {}
const isClient = () => true
const isServer = () => false

export function GuestTracking() {
  const params = useParams<{ orderId: string }>()
  const orderId = params.orderId

  const token = useSyncExternalStore(subscribeToHash, readGuestToken, () => null)
  const hydrated = useSyncExternalStore(subscribeNothing, isClient, isServer)
  const [expired, setExpired] = useState(false)

  const api = useMemo(
    () => createApiClient({ baseUrl: '/api/backend', getToken: () => token }),
    [token],
  )
  const loadOrder = useCallback(() => api.getOrder(orderId), [api, orderId])

  const order = useApiResource(loadOrder, {
    enabled: Boolean(token) && Boolean(orderId),
  })

  const tracking = useTracking({
    url: WS_URL,
    token,
    channels: token ? [orderChannel(orderId)] : [],
    enabled: Boolean(token) && !expired,
    onSessionExpired: () => setExpired(true),
  })

  const latest = useMemo(() => {
    const points = Object.values(tracking.latest)
    if (points.length === 0) return null
    return points.reduce((newest, point) =>
      Date.parse(point.time) > Date.parse(newest.time) ? point : newest,
    )
  }, [tracking.latest])

  const trail = useMemo<TrackingTrail[]>(() => {
    const seen = new Set<string>()
    const points = [...tracking.events]
      .reverse()
      .flatMap((event) => {
        const key = `${event.data.vehicle_id}|${event.data.time}|${event.data.device_seq}`
        if (seen.has(key)) return []
        seen.add(key)
        return [{ lat: event.data.lat, lon: event.data.lon }]
      })
      .slice(-300)
    return points.length > 1 ? [{ id: 'order-trail', points, tone: 'live' }] : []
  }, [tracking.events])

  const markers = useMemo<TrackingMarker[]>(() => {
    const result: TrackingMarker[] = []
    const current = order.data
    if (!current) return result

    result.push({
      id: 'origin',
      label: 'Tienda',
      detail: 'Punto de origen',
      lat: current.origin.lat,
      lon: current.origin.lon,
      tone: 'neutral',
    })
    result.push({
      id: 'destination',
      label: 'Entrega',
      detail: 'Tu dirección',
      lat: current.destination.lat,
      lon: current.destination.lon,
      tone: 'pending',
    })
    if (latest) {
      result.push({
        id: 'vehicle',
        label: current.assigned_vehicle_plate ?? 'Vehículo',
        detail: `${formatSpeed(latest.speed_kph)} · ${formatFreshness(latest.time)}`,
        lat: latest.lat,
        lon: latest.lon,
        tone: 'live',
      })
    }
    return result
  }, [order.data, latest])

  const invalidLink =
    expired ||
    (order.error instanceof ApiError &&
      (order.error.isUnauthorized || order.error.isForbidden || order.error.isNotFound))

  return (
    <div className="tt-guest">
      <header className="tt-guest__header">
        <Link className="tt-brand" href="/">
          <span className="tt-brand__mark" aria-hidden="true">
            TT
          </span>
          <span className="tt-brand__name">Tiendas-Track</span>
        </Link>
        {token && !invalidLink ? (
          <ConnectionPill status={tracking.status} onReconnect={tracking.reconnect} />
        ) : null}
      </header>

      <main className="tt-guest__main">
        <div>
          <p className="tt-badge tt-badge--neutral">Seguimiento de pedido</p>
          <h1 className="tt-guest__title">{order.data ? order.data.external_ref : 'Tu pedido'}</h1>
          <p className="tt-muted">
            Vista de invitado con acceso exclusivo a esta orden. El enlace caduca y no requiere crear una cuenta.
          </p>
        </div>

        {!hydrated ? <LoadingPanel label="Preparando el seguimiento…" /> : null}

        {hydrated && !token ? (
          <div className="tt-state tt-state--error" role="alert">
            <p className="tt-state__title">Enlace incompleto</p>
            <p className="tt-state__text">
              Falta el token de invitado. Abre el enlace completo que te compartió la tienda, incluida la parte
              posterior al símbolo <code>#</code>.
            </p>
          </div>
        ) : null}

        {token && invalidLink ? (
          <div className="tt-state tt-state--error" role="alert">
            <p className="tt-state__title">Este enlace ya no es válido</p>
            <p className="tt-state__text">
              El token de invitado caducó o fue revocado. Pide a la tienda un enlace de seguimiento nuevo.
            </p>
          </div>
        ) : null}

        {token && !invalidLink && order.error ? (
          <ErrorPanel error={order.error} onRetry={order.reload} title="No se pudo cargar tu pedido" />
        ) : null}

        {token && !invalidLink && order.loading && !order.data ? (
          <LoadingPanel label="Cargando tu pedido…" />
        ) : null}

        {order.data && !invalidLink ? (
          <>
            <section className="tt-card" aria-labelledby="guest-order-title">
              <div className="tt-section__head">
                <h2 className="tt-section__title" id="guest-order-title">
                  {order.data.destination_address ?? 'Entrega a domicilio'}
                </h2>
                <StatusBadge
                  label={
                    order.data.status === 'CREATED'
                      ? 'Preparando'
                      : order.data.status === 'IN_TRANSIT'
                        ? 'En camino'
                        : order.data.status === 'DELIVERED'
                          ? 'Entregado'
                          : 'Cancelado'
                  }
                  tone={
                    order.data.status === 'IN_TRANSIT'
                      ? 'live'
                      : order.data.status === 'CANCELLED'
                        ? 'danger'
                        : order.data.status === 'DELIVERED'
                          ? 'neutral'
                          : 'pending'
                  }
                />
              </div>

              <dl className="tt-detail">
                <div className="tt-detail__row">
                  <dt>Distancia del recorrido</dt>
                  <dd>{formatDistance(order.data.distance_meters)}</dd>
                </div>
                <div className="tt-detail__row">
                  <dt>Creado</dt>
                  <dd>{formatDateTime(order.data.placed_at)}</dd>
                </div>
                <div className="tt-detail__row">
                  <dt>Última posición</dt>
                  <dd>{latest ? formatFreshness(latest.time) : 'Sin señal todavía'}</dd>
                </div>
              </dl>

              <OrderProgress order={order.data} />

              {order.data.status === 'CREATED' && !latest ? (
                <p className="tt-muted">
                  Tu pedido aún no tiene un vehículo reportando posiciones. El mapa se actualizará automáticamente.
                </p>
              ) : null}

              <div className="tt-form__actions">
                <button type="button" className="tt-button tt-button--ghost" onClick={order.reload}>
                  Actualizar pedido
                </button>
              </div>
            </section>

            <section className="tt-card" aria-labelledby="guest-map-title">
              <div className="tt-section__head">
                <h2 className="tt-section__title" id="guest-map-title">
                  Recorrido
                </h2>
                <span className="tt-section__hint">Solo se muestra tu pedido</span>
              </div>
              <TrackingMap
                markers={markers}
                trails={trail}
                summary={`${markers.length} punto(s) en el mapa`}
                emptyLabel="Aún no hay posiciones para mostrar."
              />
              {tracking.error ? (
                <p className="tt-form__error" role="status">
                  Canal en vivo: {tracking.error.message}
                </p>
              ) : null}
            </section>
          </>
        ) : null}
      </main>

      <footer className="tt-guest__foot">
        Tiendas-Track · Este enlace es personal y temporal. No lo compartas fuera de tu entrega.
      </footer>
    </div>
  )
}
