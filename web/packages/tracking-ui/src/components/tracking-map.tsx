'use client'

import { useCallback, useEffect, useRef } from 'react'
import * as maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import {
  DEFAULT_BOUNDS,
  computeBounds,
  padBounds,
  type GeoPoint,
} from '../lib/geometry'
import { createTrackingMap } from '../lib/map'
import type { Tone } from './status-badge'

export interface TrackingMarker extends GeoPoint {
  id: string
  label: string
  detail?: string
  tone?: Tone
}

export interface TrackingTrail {
  id: string
  points: readonly GeoPoint[]
  tone?: Tone
}

export interface TrackingMapProps {
  markers: readonly TrackingMarker[]
  trails?: readonly TrackingTrail[]
  selectedId?: string | null
  onSelect?: (id: string) => void
  emptyLabel?: string
  summary?: string
}

const VIEW_HEIGHT = 420

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;')
}

// Color mapping for tone -> marker colors
const toneColors: Record<Tone, string> = {
  live: '#0f7b4a',
  pending: '#92400e',
  neutral: '#52627a',
  offline: '#5b6b82',
  danger: '#b42318',
}

export function TrackingMap({
  markers,
  trails = [],
  selectedId,
  onSelect,
  emptyLabel = 'Sin posiciones para mostrar todavía.',
  summary,
}: TrackingMapProps) {
  const mapContainerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<Map<string, maplibregl.Marker>>(new Map())
  const fittedMarkerIdsRef = useRef<string | null>(null)
  const hasData = markers.length > 0 || trails.some((trail) => trail.points.length > 0)

  // Compute bounds for the map view
  const bounds = useCallback(() => {
    const points: GeoPoint[] = [...markers]
    for (const trail of trails) points.push(...trail.points)
    return padBounds(computeBounds(points) ?? DEFAULT_BOUNDS)
  }, [markers, trails])

  // Initialize map on first render
  useEffect(() => {
    if (!mapContainerRef.current || mapRef.current) return

    const map = createTrackingMap(mapContainerRef.current)

    map.addControl(new maplibregl.NavigationControl(), 'top-left')

    mapRef.current = map

    return () => {
      map.remove()
      mapRef.current = null
    }
  }, [])

  // Update map view when bounds change
  useEffect(() => {
    if (!mapRef.current) return

    const markerIds = markers.map((marker) => marker.id).sort().join('|')
    if (fittedMarkerIdsRef.current === markerIds) return
    fittedMarkerIdsRef.current = markerIds

    const mapBounds = bounds()
    const southWest = new maplibregl.LngLat(mapBounds.minLon, mapBounds.minLat)
    const northEast = new maplibregl.LngLat(mapBounds.maxLon, mapBounds.maxLat)

    mapRef.current.fitBounds([southWest, northEast], {
      padding: 50,
      animate: true,
    })
  }, [bounds])

  // Update markers
  useEffect(() => {
    if (!mapRef.current) return

    const map = mapRef.current

    // Remove markers not in current list
    const currentIds = new Set(markers.map((m) => m.id))
    markersRef.current.forEach((marker, id) => {
      if (!currentIds.has(id)) {
        marker.remove()
        markersRef.current.delete(id)
      }
    })

    // Add or update markers
    for (const marker of markers) {
      const el = document.createElement('div')
      el.className = 'tt-map-marker'
      el.innerHTML = `
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <circle cx="12" cy="12" r="8" fill="${toneColors[marker.tone ?? 'neutral']}" stroke="#fff" stroke-width="2"/>
          ${selectedId === marker.id ? '<circle cx="12" cy="12" r="6" fill="#1d4ed8"/>' : ''}
        </svg>
        <div class="tt-map-marker-label">${escapeHtml(marker.label)}</div>
      `

      el.style.cursor = 'pointer'
      el.onclick = () => {
        if (onSelect) onSelect(marker.id)
      }

      let mapMarker = markersRef.current.get(marker.id)
      if (!mapMarker) {
        mapMarker = new maplibregl.Marker({ element: el, anchor: 'center' })
          .setLngLat([marker.lon, marker.lat])
          .addTo(map)
        markersRef.current.set(marker.id, mapMarker)
      } else {
        const existingElement = mapMarker.getElement()
        existingElement.innerHTML = el.innerHTML
        existingElement.style.cursor = 'pointer'
        existingElement.onclick = el.onclick
        mapMarker.setLngLat([marker.lon, marker.lat])
      }
    }

    return () => {
      markersRef.current.forEach((marker) => marker.remove())
      markersRef.current.clear()
    }
  }, [markers, selectedId, onSelect])

  // Update trails using GeoJSON source
  useEffect(() => {
    if (!mapRef.current) return

    const map = mapRef.current

    // Remove old trail layers and sources
    if (map.getSource('trails-source')) {
      map.removeSource('trails-source')
    }

    const style = map.getStyle()
    if (style && style.layers) {
      const layersToRemove: string[] = []
      style.layers.forEach((layer) => {
        if (layer.id.startsWith('trail-')) {
          layersToRemove.push(layer.id)
        }
      })
      layersToRemove.forEach((id) => map.removeLayer(id))
    }

    if (trails.length === 0) return

    // Create GeoJSON data for trails
    const features = trails
      .filter((t) => t.points.length >= 2)
      .map((trail) => ({
        type: 'Feature' as const,
        properties: {
          id: trail.id,
          tone: trail.tone ?? 'live',
        },
        geometry: {
          type: 'LineString' as const,
          coordinates: trail.points.map((p) => [p.lon, p.lat]),
        },
      }))

    map.addSource('trails-source', {
      type: 'geojson',
      data: {
        type: 'FeatureCollection' as const,
        features,
      },
    })

    // Add trail layer for all trails
    map.addLayer({
      id: 'trail-all',
      type: 'line',
      source: 'trails-source',
      paint: {
        'line-color': '#888',
        'line-width': 3,
        'line-opacity': 0.5,
      },
    })

    // Add selected trail layer
    map.addLayer({
      id: 'trail-selected',
      type: 'line',
      source: 'trails-source',
      filter: ['==', ['get', 'id'], selectedId ?? 'none'],
      paint: {
        'line-color': toneColors.live,
        'line-width': 4,
        'line-opacity': 1,
      },
    })

    return () => {
      const style = map.getStyle()
      if (style && style.layers) {
        const layersToRemove: string[] = []
        style.layers.forEach((layer) => {
          if (layer.id.startsWith('trail-')) {
            layersToRemove.push(layer.id)
          }
        })
        layersToRemove.forEach((id) => map.removeLayer(id))
      }
    }
  }, [trails, selectedId])

  const heading = summary ?? `${markers.length} vehículo(s) en el mapa`

  return (
    <div className="tt-map">
      <div
        ref={mapContainerRef}
        className="tt-map__viewport tt-map__canvas"
        aria-hidden="true"
        style={{ width: '100%', height: `${VIEW_HEIGHT}px` }}
      />
      {!hasData ? (
        <p className="tt-map__empty" role="status">
          {emptyLabel}
        </p>
      ) : null}

      <div className="tt-map__legend">
        <p className="tt-map__summary">{heading}</p>
        <ul className="tt-map__list">
          {markers.map((marker) => {
            const tone = marker.tone ?? 'neutral'
            const content = (
              <>
                <span className={`tt-dot tt-dot--${tone}`} aria-hidden="true" />
                <span className="tt-map__item-text">
                  <strong>{marker.label}</strong>
                  {marker.detail ? <span>{marker.detail}</span> : null}
                </span>
              </>
            )
            return (
              <li key={marker.id}>
                {onSelect ? (
                  <button
                    type="button"
                    className={`tt-map__item${marker.id === selectedId ? ' is-selected' : ''}`}
                    aria-pressed={marker.id === selectedId}
                    onClick={() => onSelect(marker.id)}
                  >
                    {content}
                  </button>
                ) : (
                  <span className="tt-map__item">{content}</span>
                )}
              </li>
            )
          })}
        </ul>
      </div>
    </div>
  )
}