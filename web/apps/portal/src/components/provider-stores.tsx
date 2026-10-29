'use client'

import { useCallback } from 'react'
import { EmptyState, ErrorPanel, LoadingPanel } from '@tiendas/tracking-ui'
import { useAuth } from '@/components/auth-provider'
import { useApiResource } from '@/hooks/use-api-resource'

export function ProviderStores() {
  const { api } = useAuth()
  const loadStores = useCallback(() => api.listStores({ limit: 50, offset: 0 }), [api])
  const stores = useApiResource(loadStores)

  return (
    <section className="tt-page tt-page--stack">
      <div className="tt-page-heading">
        <div>
          <p className="tt-eyebrow">Proveedor</p>
          <h1>Tiendas disponibles</h1>
        </div>
      </div>

      {stores.loading ? (
        <LoadingPanel label="Cargando tiendas…" />
      ) : stores.error ? (
        <ErrorPanel error={stores.error} />
      ) : !stores.data || stores.data.items.length === 0 ? (
        <EmptyState title="No hay tiendas disponibles" />
      ) : (
        <div className="tt-grid tt-grid--cards">
          {stores.data.items.map((store) => (
            <article className="tt-card" key={store.id}>
              <h2>{store.name}</h2>
              {store.address && <p>{store.address}</p>}
              <p className="tt-muted">
                Lat {store.lat.toFixed(5)} · Lon {store.lon.toFixed(5)}
              </p>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}
