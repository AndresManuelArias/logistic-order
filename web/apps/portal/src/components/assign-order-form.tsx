'use client'

import { toApiError, type Driver, type Order, type Vehicle } from '@tiendas/tracking-core'
import { useEffect, useState } from 'react'
import { useAuth } from './auth-provider'

interface AssignOrderFormProps {
  /** Orden a asignar. Debe estar en estado CREATED. */
  order: Order
  /** Vehículos de la flota del proveedor para seleccionar. */
  vehicles: Vehicle[]
  /** Callback al completar la asignación exitosamente. */
  onAssigned: () => void
  /** Callback para cerrar/cancelar el formulario. */
  onCancel: () => void
}

// Formulario de asignación de vehículo y conductor a una orden.
// Solo visible para el rol PROVIDER (RF-06).
// Al seleccionar un vehículo se cargan automáticamente sus conductores.
export function AssignOrderForm({ order, vehicles, onAssigned, onCancel }: AssignOrderFormProps) {
  const { api } = useAuth()

  const [vehicleId, setVehicleId] = useState<string>('')
  const [driverId, setDriverId] = useState<string>('')
  // Última carga de conductores junto al vehículo al que pertenece. Guardar el
  // vehículo permite derivar "cargando" y "error" durante el render en lugar de
  // resetear estado sincrónicamente dentro del efecto
  // (react-hooks/set-state-in-effect).
  const [driversResult, setDriversResult] = useState<{
    vehicleId: string
    items: Driver[]
    error: string | null
  }>({ vehicleId: '', items: [], error: null })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Al cambiar el vehículo se cargan sus conductores. La limpieza del conductor
  // anterior sale de la derivación: si no está en la lista del vehículo actual,
  // no cuenta como seleccionado.
  useEffect(() => {
    if (!vehicleId) return

    let cancelled = false

    api
      .listDrivers(vehicleId)
      .then((page) => {
        if (cancelled) return
        setDriversResult({ vehicleId, items: page.items, error: null })
        if (page.items.length === 1) {
          // Solo hay un conductor: pre-seleccionar.
          setDriverId(page.items[0]?.id ?? '')
        }
      })
      .catch((reason) => {
        if (cancelled) return
        const apiError = toApiError(reason)
        setDriversResult({
          vehicleId,
          items: [],
          error: apiError.message || 'No se pudieron cargar los conductores.',
        })
      })

    return () => {
      cancelled = true
    }
  }, [vehicleId, api])

  const driversLoaded = driversResult.vehicleId === vehicleId
  const drivers = driversLoaded ? driversResult.items : []
  const driversLoading = Boolean(vehicleId) && !driversLoaded
  const driversError = driversLoaded ? driversResult.error : null
  // El conductor sólo cuenta como seleccionado si pertenece a la lista actual.
  const selectedDriverId = drivers.some((driver) => driver.id === driverId) ? driverId : ''

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!vehicleId || !selectedDriverId) {
      setError('Selecciona un vehículo y un conductor.')
      return
    }

    setPending(true)
    setError(null)
    try {
      await api.assignOrder(order.id, { vehicle_id: vehicleId, driver_id: selectedDriverId })
      onAssigned()
    } catch (reason) {
      const apiError = toApiError(reason)
      if (apiError.status === 403) {
        setError('El vehículo o conductor no pertenece a tu flota.')
      } else if (apiError.status === 422) {
        setError('El conductor no está asignado a ese vehículo.')
      } else {
        setError(apiError.message || 'No se pudo asignar la orden.')
      }
    } finally {
      setPending(false)
    }
  }

  const activeVehicles = vehicles.filter((v) => v.status === 'ACTIVE')

  return (
    <section className="tt-card tt-card--flat" aria-labelledby={`assign-title-${order.id}`}>
      <div className="tt-section__head">
        <h3 className="tt-section__title" id={`assign-title-${order.id}`}>
          Asignar orden <span className="tt-muted">{order.external_ref}</span>
        </h3>
        <button
          type="button"
          className="tt-button tt-button--ghost"
          onClick={onCancel}
          aria-label="Cancelar asignación"
        >
          Cancelar
        </button>
      </div>

      <p className="tt-order-card__address" aria-label="Dirección de entrega">
        {order.destination_address || 'Sin dirección registrada'}
      </p>

      <form className="tt-form" onSubmit={handleSubmit} noValidate>
        {/* Selección de vehículo */}
        <div className="tt-field">
          <label className="tt-field__label" htmlFor={`assign-vehicle-${order.id}`}>
            Vehículo
          </label>
          {activeVehicles.length === 0 ? (
            <p className="tt-muted">No tienes vehículos activos en tu flota.</p>
          ) : (
            <select
              id={`assign-vehicle-${order.id}`}
              className="tt-input"
              value={vehicleId}
              onChange={(e) => setVehicleId(e.target.value)}
              required
              aria-required="true"
            >
              <option value="">— Selecciona un vehículo —</option>
              {activeVehicles.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.plate}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Selección de conductor — se muestra solo cuando hay un vehículo seleccionado */}
        {vehicleId ? (
          <div className="tt-field">
            <label className="tt-field__label" htmlFor={`assign-driver-${order.id}`}>
              Conductor
            </label>

            {driversLoading ? (
              <p className="tt-muted" aria-live="polite">
                Cargando conductores…
              </p>
            ) : driversError ? (
              <p className="tt-form__error" role="alert">
                {driversError}
              </p>
            ) : drivers.length === 0 ? (
              <p className="tt-muted">Este vehículo no tiene conductores asignados.</p>
            ) : (
              <select
                id={`assign-driver-${order.id}`}
                className="tt-input"
                value={selectedDriverId}
                onChange={(e) => setDriverId(e.target.value)}
                required
                aria-required="true"
              >
                <option value="">— Selecciona un conductor —</option>
                {drivers.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.full_name}
                    {d.license_number ? ` · ${d.license_number}` : ''}
                  </option>
                ))}
              </select>
            )}
          </div>
        ) : null}

        {error ? (
          <p className="tt-form__error" role="alert">
            {error}
          </p>
        ) : null}

        <div className="tt-form__actions">
          <button
            className="tt-button"
            type="submit"
            disabled={pending || !vehicleId || !selectedDriverId || driversLoading}
          >
            {pending ? 'Asignando…' : 'Asignar'}
          </button>
        </div>
      </form>
    </section>
  )
}
