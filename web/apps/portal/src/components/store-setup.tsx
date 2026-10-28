'use client'

import { toApiError } from '@tiendas/tracking-core'
import { useState, type FormEvent } from 'react'
import { useAuth } from './auth-provider'

interface CoordinateValues {
  lat: number
  lon: number
}

function parseCoordinate(latRaw: string, lonRaw: string): CoordinateValues | null {
  const lat = Number(latRaw.replace(',', '.'))
  const lon = Number(lonRaw.replace(',', '.'))
  if (!Number.isFinite(lat) || lat < -90 || lat > 90) return null
  if (!Number.isFinite(lon) || lon < -180 || lon > 180) return null
  return { lat, lon }
}

// Alta de tienda (R11): paso separado del registro. Si el tendero ya tiene
// tienda, el backend responde 409 y no se duplica.
export function StoreSetup({ onCreated }: { onCreated: () => void }) {
  const { api } = useAuth()
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [lat, setLat] = useState('4.7108')
  const [lon, setLon] = useState('-74.0721')
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const coordinates = parseCoordinate(lat, lon)
    if (!name.trim()) {
      setError('El nombre de la tienda es obligatorio.')
      return
    }
    if (!coordinates) {
      setError('Latitud o longitud fuera de rango.')
      return
    }

    setPending(true)
    setError(null)
    try {
      await api.createStore({
        name: name.trim(),
        address: address.trim() || undefined,
        location: coordinates,
      })
      onCreated()
    } catch (reason) {
      const apiError = toApiError(reason)
      if (apiError.status === 409) setError('Ya tienes una tienda registrada.')
      else if (apiError.status === 422) setError('Revisa los datos: el nombre y la ubicación son obligatorios.')
      else setError(apiError.message || 'No se pudo crear la tienda.')
    } finally {
      setPending(false)
    }
  }

  return (
    <section className="tt-card" aria-labelledby="store-setup-title">
      <div>
        <h2 className="tt-section__title" id="store-setup-title">
          Configura tu tienda
        </h2>
        <p className="tt-section__hint">
          Tu usuario aún no tiene tienda. Crea una para poder recibir y despachar órdenes.
        </p>
      </div>

      <form className="tt-form" onSubmit={handleSubmit} noValidate>
        <div className="tt-field">
          <label className="tt-field__label" htmlFor="store-name">
            Nombre de la tienda
          </label>
          <input
            id="store-name"
            className="tt-input"
            type="text"
            required
            maxLength={120}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </div>

        <div className="tt-field">
          <label className="tt-field__label" htmlFor="store-address">
            Dirección (opcional)
          </label>
          <input
            id="store-address"
            className="tt-input"
            type="text"
            value={address}
            onChange={(event) => setAddress(event.target.value)}
          />
        </div>

        <div className="tt-field-grid">
          <div className="tt-field">
            <label className="tt-field__label" htmlFor="store-lat">
              Latitud
            </label>
            <input
              id="store-lat"
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
            <label className="tt-field__label" htmlFor="store-lon">
              Longitud
            </label>
            <input
              id="store-lon"
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
        <p className="tt-field__hint">La ubicación se guarda con PostGIS y se usa como origen de tus órdenes.</p>

        {error ? (
          <p className="tt-form__error" role="alert">
            {error}
          </p>
        ) : null}

        <div className="tt-form__actions">
          <button className="tt-button" type="submit" disabled={pending}>
            {pending ? 'Creando…' : 'Crear tienda'}
          </button>
        </div>
      </form>
    </section>
  )
}
