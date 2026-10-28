'use client'

import {
  fleetProviderChannel,
  type Order,
  type Vehicle,
} from '@tiendas/tracking-core'
import {
  ConnectionPill,
  EmptyState,
  ErrorPanel,
  KpiCard,
  LoadingPanel,
  OrderCard,
  StatusBadge,
  TrackingMap,
  formatDateTime,
  formatDistance,
  formatFreshness,
  formatSpeed,
  positionTone,
  type Tone,
  type TrackingMarker,
  type TrackingTrail,
} from '@tiendas/tracking-ui'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useAuth } from '@/components/auth-provider'
import { AiAssistant } from '@/components/ai-assistant'
import { useApiResource } from '@/hooks/use-api-resource'
import { usePageTracking } from '@/hooks/use-page-tracking'

const TRACK_WINDOW_HOURS = 6

function vehicleTone(vehicle: Vehicle): Tone {
  if (vehicle.status === 'INACTIVE') return 'offline'
  return positionTone(vehicle.last_position)
}

export function ProviderDashboard() {
  const { user, api } = useAuth()
  const loadFleet = useCallback(() => api.listVehicles({ limit: 200 }), [api])
  const loadOrders = useCallback(() => api.listOrders({ limit: 100 }), [api])

  const fleet = useApiResource(loadFleet, {
    enabled: Boolean(user),
    reloadKey: user?.id ?? '',
  })
  const orders = useApiResource(loadOrders, {
    enabled: Boolean(user),
    reloadKey: user?.id ?? '',
  })
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const tracking = usePageTracking(user ? [fleetProviderChannel(user.id)] : [])

  const vehicles = useMemo(() => {
    const items = fleet.data?.items ?? []
    return items.map((vehicle) => {
      const live = tracking.latest[vehicle.id]
      return live ? { ...vehicle, last_position: live } : vehicle
    })
  }, [fleet.data, tracking.latest])

  // Si llega telemetría de un vehículo que no está en la página cargada, se
  // refresca la flota una sola vez por id (evita bucles con el canal vivo).
  const requestedIds = useRef(new Set<string>())
  const reloadFleet = fleet.reload
  useEffect(() => {
    const known = new Set(vehicles.map((vehicle) => vehicle.id))
    for (const id of Object.keys(tracking.latest)) {
      if (!known.has(id) && !requestedIds.current.has(id)) {
        requestedIds.current.add(id)
        reloadFleet()
        break
      }
    }
  }, [tracking.latest, vehicles, reloadFleet])

  const loadTrack = useCallback(() => {
    if (!selectedId) return Promise.resolve(null)
    const to = new Date()
    const from = new Date(to.getTime() - TRACK_WINDOW_HOURS * 60 * 60 * 1000)
    return api.track(selectedId, {
      from: from.toISOString(),
      to: to.toISOString(),
      limit: 1000,
    })
  }, [api, selectedId])

  const track = useApiResource(loadTrack, {
    enabled: Boolean(selectedId),
    reloadKey: selectedId ?? '',
  })

  const selected = vehicles.find((vehicle) => vehicle.id === selectedId) ?? null

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
            tone: vehicleTone(vehicle),
            detail: `${formatSpeed(point.speed_kph)} · ${formatFreshness(point.time)}`,
          },
        ]
      }),
    [vehicles],
  )

  const trails = useMemo<TrackingTrail[]>(() => {
    if (!track.data || track.data.points.length === 0) return []
    return [
      {
        id: track.data.vehicle_id,
        tone: 'live',
        points: track.data.points.map((point) => ({ lat: point.lat, lon: point.lon })),
      },
    ]
  }, [track.data])

  const orderItems = orders.data?.items ?? []
  const inTransit = orderItems.filter((order) => order.status === 'IN_TRANSIT').length
  const delivered = orderItems.filter((order) => order.status === 'DELIVERED').length
  const withoutSignal = vehicles.filter((vehicle) => vehicleTone(vehicle) !== 'live').length
  const plates = new Map(vehicles.map((vehicle) => [vehicle.id, vehicle.plate]))

  function refreshAll() {
    fleet.reload()
    orders.reload()
    if (selectedId) track.reload()
  }

  return (
    <div className="tt-page">
      <header className="tt-page__head">
        <div>
          <h1 className="tt-page__title">Flota del proveedor</h1>
          <p className="tt-page__subtitle">
            Todas las posiciones de tu flota B2B, en vivo. Toca un vehículo en el mapa para ver su traza de las
            últimas {TRACK_WINDOW_HOURS} horas.
          </p>
        </div>
        <div className="tt-page__actions">
          <ConnectionPill status={tracking.status} onReconnect={tracking.reconnect} />
          <button type="button" className="tt-button tt-button--ghost" onClick={refreshAll} disabled={fleet.loading}>
            {fleet.loading ? 'Actualizando…' : 'Actualizar'}
          </button>
        </div>
      </header>

      {tracking.error ? (
        <p className="tt-form__error" role="status">
          Canal en vivo: {tracking.error.message}
        </p>
      ) : null}

      <section className="tt-grid" aria-label="Indicadores de la flota">
        <KpiCard label="Vehículos" value={vehicles.length} hint="En tu flota" />
        <KpiCard label="En tránsito" value={inTransit} hint="Órdenes activas" tone="live" />
        <KpiCard label="Entregadas" value={delivered} hint="Histórico visible" tone="neutral" />
        <KpiCard
          label="Sin señal"
          value={withoutSignal}
          hint="Sin posición reciente"
          tone={withoutSignal > 0 ? 'pending' : 'live'}
        />
      </section>

      {fleet.error ? <ErrorPanel error={fleet.error} onRetry={fleet.reload} /> : null}
      {fleet.loading && !fleet.data ? <LoadingPanel label="Cargando flota…" /> : null}

      {fleet.data ? (
        <div className="tt-columns">
          <section className="tt-card" aria-labelledby="fleet-map-title">
            <div className="tt-section__head">
              <h2 className="tt-section__title" id="fleet-map-title">
                Mapa en vivo
              </h2>
              <span className="tt-section__hint">Última posición reportada</span>
            </div>
            <TrackingMap
              markers={markers}
              trails={trails}
              selectedId={selectedId}
              onSelect={setSelectedId}
              summary={`${markers.length} de ${vehicles.length} vehículo(s) con posición`}
              emptyLabel="Tu flota aún no reporta posiciones. Los vehículos aparecerán al recibir telemetría."
            />
          </section>

          <div className="tt-page">
            <section className="tt-card" aria-labelledby="vehicle-detail-title">
              <h2 className="tt-section__title" id="vehicle-detail-title">
                Detalle del vehículo
              </h2>
              {selected ? (
                <>
                  <h3 className="tt-detail__title">
                    {selected.plate}
                    <StatusBadge
                      label={selected.status === 'ACTIVE' ? 'Activo' : 'Inactivo'}
                      tone={selected.status === 'ACTIVE' ? 'live' : 'offline'}
                    />
                  </h3>
                  {selected.last_position ? (
                    <dl className="tt-detail">
                      <div className="tt-detail__row">
                        <dt>Última señal</dt>
                        <dd>{formatFreshness(selected.last_position.time)}</dd>
                      </div>
                      <div className="tt-detail__row">
                        <dt>Recibida</dt>
                        <dd>{formatDateTime(selected.last_position.received_at)}</dd>
                      </div>
                      <div className="tt-detail__row">
                        <dt>Velocidad</dt>
                        <dd>{formatSpeed(selected.last_position.speed_kph)}</dd>
                      </div>
                      <div className="tt-detail__row">
                        <dt>Precisión</dt>
                        <dd>{formatDistance(selected.last_position.accuracy_m)}</dd>
                      </div>
                      <div className="tt-detail__row">
                        <dt>Rumbo</dt>
                        <dd>
                          {selected.last_position.heading === undefined
                            ? '—'
                            : `${Math.round(selected.last_position.heading)}°`}
                        </dd>
                      </div>
                      <div className="tt-detail__row">
                        <dt>Secuencia</dt>
                        <dd>{selected.last_position.device_seq}</dd>
                      </div>
                      <div className="tt-detail__row">
                        <dt>Traza {TRACK_WINDOW_HOURS} h</dt>
                        <dd>
                          {track.loading
                            ? 'Cargando…'
                            : track.data
                              ? `${track.data.count} puntos`
                              : '—'}
                        </dd>
                      </div>
                    </dl>
                  ) : (
                    <p className="tt-muted">Este vehículo todavía no tiene posiciones registradas.</p>
                  )}
                  {track.error ? (
                    <p className="tt-form__error" role="alert">
                      No se pudo cargar la traza del vehículo.
                    </p>
                  ) : null}
                </>
              ) : (
                <EmptyState title="Ningún vehículo seleccionado" description="Elige un vehículo en el mapa o en la lista." />
              )}
            </section>

            <section className="tt-card" aria-labelledby="vehicle-list-title">
              <h2 className="tt-section__title" id="vehicle-list-title">
                Vehículos
              </h2>
              {vehicles.length === 0 ? (
                <EmptyState title="Sin vehículos" description="Tu flota no tiene vehículos registrados." />
              ) : (
                <div className="tt-table-wrap">
                  <table className="tt-table">
                    <caption className="tt-visually-hidden">Vehículos de tu flota y su última posición</caption>
                    <thead>
                      <tr>
                        <th scope="col">Placa</th>
                        <th scope="col">Estado</th>
                        <th scope="col">Última señal</th>
                        <th scope="col">Velocidad</th>
                        <th scope="col">
                          <span className="tt-visually-hidden">Acciones</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {vehicles.map((vehicle) => (
                        <tr key={vehicle.id} className={vehicle.id === selectedId ? 'is-selected' : undefined}>
                          <th scope="row">{vehicle.plate}</th>
                          <td>
                            <StatusBadge
                              label={vehicle.status === 'ACTIVE' ? 'Activo' : 'Inactivo'}
                              tone={vehicle.status === 'ACTIVE' ? 'live' : 'offline'}
                            />
                          </td>
                          <td>{formatFreshness(vehicle.last_position?.time)}</td>
                          <td>{formatSpeed(vehicle.last_position?.speed_kph)}</td>
                          <td>
                            <button
                              type="button"
                              className="tt-table__action"
                              aria-pressed={vehicle.id === selectedId}
                              onClick={() => setSelectedId(vehicle.id)}
                            >
                              Ver
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          </div>
        </div>
      ) : null}

      <section className="tt-card" aria-labelledby="fleet-orders-title">
        <div className="tt-section__head">
          <h2 className="tt-section__title" id="fleet-orders-title">
            Órdenes de mi flota
          </h2>
          <span className="tt-section__hint">{orderItems.length} órdenes visibles</span>
        </div>

        {orders.error ? <ErrorPanel error={orders.error} onRetry={orders.reload} /> : null}
        {orders.loading && !orders.data ? <LoadingPanel label="Cargando órdenes…" /> : null}

        {orders.data && orderItems.length === 0 ? (
          <EmptyState
            title="Sin órdenes asignadas"
            description="Cuando los vehículos de tu flota tengan órdenes asignadas, aparecerán aquí con su estado."
          />
        ) : null}

        {orderItems.length > 0 ? (
          <ul className="tt-list">
            {orderItems.map((order: Order) => (
              <li key={order.id}>
                <OrderCard
                  order={order}
                  vehiclePlate={order.assigned_vehicle_id ? plates.get(order.assigned_vehicle_id) : undefined}
                />
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      <AiAssistant />
    </div>
  )
}
