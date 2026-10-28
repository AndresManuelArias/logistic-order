'use client'

import { toApiError, type Store } from '@tiendas/tracking-core'
import { useState, type FormEvent } from 'react'
import { useAuth } from './auth-provider'

// Creación de una orden desde la tienda. El origen es siempre la ubicación de
// la tienda: el backend rechaza cualquier store_id ajeno al token (RF-02).
export function CreateOrderForm({ store, onCreated }: { store: Store; onCreated: () => void }) {
  const { api } = useAuth()
  const [open, setOpen] = useState(false)
  const [externalRef, setExternalRef] = useState('')
  const [address, setAddress] = useState('')
  const [lat, setLat] = useState('4.6578')
  const [lon, setLon] = useState('-74.0561')
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  function reset() {
    setExternalRef('')
    setAddress('')
    setLat('4.6578')
    setLon('-74.0561')
    setError(null)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const latitude = Number(lat.replace(',', '.'))
    const longitude = Number(lon.replace(',', '.'))
    if (!externalRef.trim()) {
      setError('La referencia externa es obligatoria.')
      return
    }
    if (!Number.isFinite(latitude) || latitude < -90 || latitude > 90) {
      setError('La latitud del destino no es válida.')
      return
    }
    if (!Number.isFinite(longitude) || longitude < -180 || longitude > 180) {
      setError('La longitud del destino no es válida.')
      return
    }

    setPending(true)
    setError(null)
    try {
      await api.createOrder({
        external_ref: externalRef.trim(),
        origin: { lat: store.lat, lon: store.lon },
        destination: { lat: latitude, lon: longitude },
        destination_address: address.trim() || undefined,
      })
      reset()
      setOpen(false)
      onCreated()
    } catch (reason) {
      const apiError = toApiError(reason)
      if (apiError.status === 409) setError('Ya existe una orden con esa referencia externa.')
      else if (apiError.status === 422) setError('Revisa los datos de la orden y las coordenadas.')
      else setError(apiError.message || 'No se pudo crear la orden.')
    } finally {
      setPending(false)
    }
  }

  if (!open) {
    return (
      <button type="button" className="tt-button" onClick={() => setOpen(true)}>
        Nueva orden
      </button>
    )
  }

  return (
    <section className="tt-card tt-card--flat" aria-labelledby="new-order-title">
      <div className="tt-section__head">
        <h2 className="tt-section__title" id="new-order-title">
          Nueva orden
        </h2>
        <button
          type="button"
          className="tt-button tt-button--ghost"
          onClick={() => {
            setOpen(false)
            setError(null)
          }}
        >
          Cerrar
        </button>
      </div>

      <form className="tt-form" onSubmit={handleSubmit} noValidate>
        <div className="tt-field">
          <label className="tt-field__label" htmlFor="order-ref">
            Referencia externa
          </label>
          <input
            id="order-ref"
            className="tt-input"
            type="text"
            required
            maxLength={80}
            placeholder="PED-1001"
            value={externalRef}
            onChange={(event) => setExternalRef(event.target.value)}
          />
          <p className="tt-field__hint">Es la clave de idempotencia: no se puede repetir.</p>
        </div>

        <div className="tt-field">
          <label className="tt-field__label" htmlFor="order-address">
            Dirección de entrega (opcional)
          </label>
          <input
            id="order-address"
            className="tt-input"
            type="text"
            value={address}
            onChange={(event) => setAddress(event.target.value)}
          />
        </div>

        <div className="tt-field-grid">
          <div className="tt-field">
            <label className="tt-field__label" htmlFor="order-lat">
              Latitud destino
            </label>
            <input
              id="order-lat"
              className="tt-input"
              type="number"
              inputMode="decimal"
              step="0.000001"
              min={-90}
              max={90}
              required
              value={lat}
              onChange={(event) => setLat(event.target.value)}
            />
          </div>
          <div className="tt-field">
            <label className="tt-field__label" htmlFor="order-lon">
              Longitud destino
            </label>
            <input
              id="order-lon"
              className="tt-input"
              type="number"
              inputMode="decimal"
              step="0.000001"
              min={-180}
              max={180}
              required
              value={lon}
              onChange={(event) => setLon(event.target.value)}
            />
          </div>
        </div>
        <p className="tt-field__hint">
          Origen: {store.name} ({store.lat.toFixed(4)}, {store.lon.toFixed(4)}).
        </p>

        {error ? (
          <p className="tt-form__error" role="alert">
            {error}
          </p>
        ) : null}

        <div className="tt-form__actions">
          <button className="tt-button" type="submit" disabled={pending}>
            {pending ? 'Creando…' : 'Crear orden'}
          </button>
        </div>
      </form>
    </section>
  )
}
