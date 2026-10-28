'use client'

import { useId, useMemo } from 'react'
import {
  DEFAULT_BOUNDS,
  computeBounds,
  padBounds,
  projectPoint,
  type GeoPoint,
} from '../lib/geometry'
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

const VIEW_WIDTH = 800
const VIEW_HEIGHT = 420

// Mapa SVG sin dependencia de proveedores de tiles (R3 pendiente). El SVG es
// decorativo: la lista inferior es la alternativa accesible y operable con
// teclado, con la misma selección que el mapa.
export function TrackingMap({
  markers,
  trails = [],
  selectedId,
  onSelect,
  emptyLabel = 'Sin posiciones para mostrar todavía.',
  summary,
}: TrackingMapProps) {
  const rawId = useId()
  const gridId = `tt-grid-${rawId.replace(/[^a-zA-Z0-9_-]/g, '')}`

  const bounds = useMemo(() => {
    const points: GeoPoint[] = [...markers]
    for (const trail of trails) points.push(...trail.points)
    return padBounds(computeBounds(points) ?? DEFAULT_BOUNDS)
  }, [markers, trails])

  if (markers.length === 0 && trails.every((trail) => trail.points.length === 0)) {
    return (
      <div className="tt-map tt-map--empty" role="status">
        <p>{emptyLabel}</p>
      </div>
    )
  }

  const project = (point: GeoPoint) => projectPoint(point, bounds, VIEW_WIDTH, VIEW_HEIGHT)
  const heading = summary ?? `${markers.length} vehículo(s) en el mapa`

  return (
    <div className="tt-map">
      <div className="tt-map__viewport" aria-hidden="true">
        <svg viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`} className="tt-map__canvas" focusable="false">
          <defs>
            <pattern id={gridId} width="50" height="50" patternUnits="userSpaceOnUse">
              <path d="M 50 0 L 0 0 0 50" className="tt-map__grid-line" fill="none" />
            </pattern>
          </defs>
          <rect width={VIEW_WIDTH} height={VIEW_HEIGHT} className="tt-map__bg" />
          <rect width={VIEW_WIDTH} height={VIEW_HEIGHT} fill={`url(#${gridId})`} />

          {trails.map((trail) => {
            if (trail.points.length < 2) return null
            const path = trail.points
              .map((point) => {
                const { x, y } = project(point)
                return `${x.toFixed(1)},${y.toFixed(1)}`
              })
              .join(' ')
            return (
              <polyline
                key={trail.id}
                points={path}
                className={`tt-map__trail tt-map__trail--${trail.tone ?? 'live'}`}
                fill="none"
              />
            )
          })}

          {markers.map((marker) => {
            const { x, y } = project(marker)
            const tone = marker.tone ?? 'neutral'
            const selected = marker.id === selectedId
            return (
              <g key={marker.id} className={`tt-map__marker-group tt-map__marker--${tone}`}>
                {selected ? <circle cx={x} cy={y} r={16} className="tt-map__ring" /> : null}
                <circle cx={x} cy={y} r={selected ? 10 : 8} className="tt-map__dot" />
                <text x={x} y={y - 18} textAnchor="middle" className="tt-map__label">
                  {marker.label}
                </text>
              </g>
            )
          })}
        </svg>
      </div>

      <div className="tt-map__legend">
        <p className="tt-map__summary">{heading}</p>
        <ul className="tt-map__list">
          {markers.map((marker) => {
            const content = (
              <>
                <span className={`tt-dot tt-dot--${marker.tone ?? 'neutral'}`} aria-hidden="true" />
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
