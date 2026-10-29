'use client'

import { toApiError, type Driver, type Store, type Vehicle } from '@tiendas/tracking-core'
import { useEffect, useState, type FormEvent } from 'react'
import { useAuth } from './auth-provider'

export function ProviderCreateOrderForm({
  vehicles,
  onCreated,
}: {
  vehicles: Vehicle[]
  onCreated: () => void
}) {
  const { api } = useAuth()
  const [open, setOpen] = useState(false)
  const [externalRef, setExternalRef] = useState('')
  const [storeId, setStoreId] = useState('')
  const [address, setAddress] = useState('')
  const [originLat, setOriginLat] = useState('4.6578')
  const [originLon, setOriginLon] = useState('-74.0561')
  const [destinationLat, setDestinationLat] = useState('4.6097')
  const [destinationLon, setDestinationLon] = useState('-74.0817')
  const [vehicleId, setVehicleId] = useState('')
  const [driverId, setDriverId] = useState('')
  const [drivers, setDrivers] = useState<Driver[]>([])
  const [stores, setStores] = useState<Store[]>([])
  const [storesLoading, setStoresLoading] = useState(true)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setStoresLoading(true)
    api.listStores({ limit: 200 }).then((page) => {
      if (!cancelled) setStores(page.items)
    }).catch((reason) => {
      if (!cancelled) setError(toApiError(reason).message || 'No se pudieron cargar las tiendas.')
    }).finally(() => {
      if (!cancelled) setStoresLoading(false)
    })
    return () => { cancelled = true }
  }, [api])

  useEffect(() => {
    setDriverId('')
    setDrivers([])
    if (!vehicleId) return
    let cancelled = false
    api.listDrivers(vehicleId).then((page) => {
      if (!cancelled) {
        setDrivers(page.items)
        if (page.items.length === 1) setDriverId(page.items[0]?.id ?? '')
      }
    }).catch((reason) => {
      if (!cancelled) setError(toApiError(reason).message || 'No se pudieron cargar los conductores.')
    })
    return () => { cancelled = true }
  }, [api, vehicleId])

  function reset() {
    setExternalRef(''); setStoreId(''); setAddress('')
    setOriginLat('4.6578'); setOriginLon('-74.0561')
    setDestinationLat('4.6097'); setDestinationLon('-74.0817')
    setVehicleId(''); setDriverId(''); setError(null)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const olat = Number(originLat.replace(',', '.'))
    const olon = Number(originLon.replace(',', '.'))
    const dlat = Number(destinationLat.replace(',', '.'))
    const dlon = Number(destinationLon.replace(',', '.'))
    if (!externalRef.trim() || !storeId.trim()) {
      setError('La referencia y el ID de la tienda son obligatorios.')
      return
    }
    if (!Number.isFinite(olat) || !Number.isFinite(olon) || olat < -90 || olat > 90 || olon < -180 || olon > 180) {
      setError('Las coordenadas de origen no son válidas.')
      return
    }
    if (!Number.isFinite(dlat) || !Number.isFinite(dlon) || dlat < -90 || dlat > 90 || dlon < -180 || dlon > 180) {
      setError('Las coordenadas de destino no son válidas.')
      return
    }
    if (!vehicleId || !driverId) {
      setError('Selecciona un vehículo y un conductor.')
      return
    }

    setPending(true)
    setError(null)
    try {
      const order = await api.createOrder({
        external_ref: externalRef.trim(),
        store_id: storeId.trim(),
        origin: { lat: olat, lon: olon },
        destination: { lat: dlat, lon: dlon },
        destination_address: address.trim() || undefined,
      })
      await api.assignOrder(order.id, { vehicle_id: vehicleId, driver_id: driverId })
      reset()
      setOpen(false)
      onCreated()
    } catch (reason) {
      const apiError = toApiError(reason)
      setError(apiError.status === 409
        ? 'Ya existe una orden con esa referencia externa.'
        : apiError.message || 'No se pudo crear y asignar la orden.')
    } finally {
      setPending(false)
    }
  }

  const activeVehicles = vehicles.filter((vehicle) => vehicle.status === 'ACTIVE')
  if (!open) return <button type="button" className="tt-button tt-button--primary" onClick={() => setOpen(true)}>Nueva orden</button>

  return (
    <section className="tt-card tt-card--flat" aria-labelledby="provider-new-order-title">
      <div className="tt-section__head">
        <h2 className="tt-section__title" id="provider-new-order-title">Crear y asignar orden</h2>
        <button type="button" className="tt-button tt-button--ghost" onClick={() => { setOpen(false); reset() }}>Cerrar</button>
      </div>
      <form className="tt-form" onSubmit={handleSubmit} noValidate>
        <div className="tt-field-grid">
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-order-ref">Referencia externa</label><input id="provider-order-ref" className="tt-input" value={externalRef} onChange={(e) => setExternalRef(e.target.value)} required /></div>
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-store-id">Tienda</label><select id="provider-store-id" className="tt-input" value={storeId} onChange={(e) => {
            const selectedStore = stores.find((store) => store.id === e.target.value)
            setStoreId(e.target.value)
            if (selectedStore) {
              setOriginLat(String(selectedStore.lat))
              setOriginLon(String(selectedStore.lon))
            }
          }} required disabled={storesLoading}><option value="">— Selecciona una tienda —</option>{stores.map((store) => <option key={store.id} value={store.id}>{store.name}{store.address ? ` · ${store.address}` : ''}</option>)}</select></div>
        </div>
        <div className="tt-field"><label className="tt-field__label" htmlFor="provider-order-address">Dirección de entrega</label><input id="provider-order-address" className="tt-input" value={address} onChange={(e) => setAddress(e.target.value)} /></div>
        <div className="tt-field-grid">
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-origin-lat">Latitud origen</label><input id="provider-origin-lat" className="tt-input" type="number" step="0.000001" value={originLat} onChange={(e) => setOriginLat(e.target.value)} /></div>
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-origin-lon">Longitud origen</label><input id="provider-origin-lon" className="tt-input" type="number" step="0.000001" value={originLon} onChange={(e) => setOriginLon(e.target.value)} /></div>
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-destination-lat">Latitud destino</label><input id="provider-destination-lat" className="tt-input" type="number" step="0.000001" value={destinationLat} onChange={(e) => setDestinationLat(e.target.value)} /></div>
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-destination-lon">Longitud destino</label><input id="provider-destination-lon" className="tt-input" type="number" step="0.000001" value={destinationLon} onChange={(e) => setDestinationLon(e.target.value)} /></div>
        </div>
        <div className="tt-field-grid">
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-order-vehicle">Vehículo</label><select id="provider-order-vehicle" className="tt-input" value={vehicleId} onChange={(e) => setVehicleId(e.target.value)} required><option value="">— Selecciona un vehículo —</option>{activeVehicles.map((vehicle) => <option key={vehicle.id} value={vehicle.id}>{vehicle.plate}</option>)}</select></div>
          <div className="tt-field"><label className="tt-field__label" htmlFor="provider-order-driver">Conductor</label><select id="provider-order-driver" className="tt-input" value={driverId} onChange={(e) => setDriverId(e.target.value)} disabled={!vehicleId} required><option value="">— Selecciona un conductor —</option>{drivers.map((driver) => <option key={driver.id} value={driver.id}>{driver.full_name}</option>)}</select></div>
        </div>
        {error ? <p className="tt-form__error" role="alert">{error}</p> : null}
        <div className="tt-form__actions"><button className="tt-button" type="submit" disabled={pending}>{pending ? 'Creando…' : 'Crear y asignar'}</button></div>
      </form>
    </section>
  )
}
