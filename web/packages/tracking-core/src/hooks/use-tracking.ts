'use client'

// Adaptador React del caso de uso de tiempo real. Gestiona el ciclo de vida
// (una sola suscripción por combinación token+canales) y limpia al desmontar.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { TelemetryPoint } from '../api/types'
import { TrackingClient } from '../application/tracking-client'
import {
  mergeLatestPoint,
  type FleetEvent,
  type TrackingStatus,
} from '../domain/telemetry'
import { browserSocketFactory } from '../infrastructure/browser-socket'

export interface UseTrackingParams {
  url: string
  token: string | null
  channels: string[]
  enabled?: boolean
  onSessionExpired?: () => void
}

export interface UseTrackingResult {
  status: TrackingStatus
  latest: Record<string, TelemetryPoint>
  events: FleetEvent[]
  error: { code: string; message: string } | null
  reconnect: () => void
}

export function useTracking({
  url,
  token,
  channels,
  enabled = true,
  onSessionExpired,
}: UseTrackingParams): UseTrackingResult {
  const [status, setStatus] = useState<TrackingStatus>('idle')
  const [latest, setLatest] = useState<Record<string, TelemetryPoint>>({})
  const [events, setEvents] = useState<FleetEvent[]>([])
  const [error, setError] = useState<{ code: string; message: string } | null>(null)

  const clientRef = useRef<TrackingClient | null>(null)
  const expiredRef = useRef(onSessionExpired)
  useEffect(() => {
    expiredRef.current = onSessionExpired
  }, [onSessionExpired])

  // Clave estable: el orden de los canales no debe recrear la conexión.
  const channelsKey = useMemo(() => [...new Set(channels)].sort().join('|'), [channels])

  useEffect(() => {
    if (!enabled || !token || channelsKey === '') {
      setStatus('idle')
      return
    }

    setLatest({})
    setEvents([])
    setError(null)

    const client = new TrackingClient({
      url,
      token,
      channels: channelsKey.split('|'),
      socketFactory: browserSocketFactory,
      onStatus: setStatus,
      onEvent: (event) => {
        setLatest((prev) => mergeLatestPoint(prev, event.data))
        setEvents((prev) => [event, ...prev].slice(0, 100))
      },
      onError: setError,
      onSessionExpired: () => expiredRef.current?.(),
    })
    clientRef.current = client
    client.start()

    return () => {
      clientRef.current = null
      client.stop()
    }
  }, [url, token, channelsKey, enabled])

  const reconnect = useCallback(() => {
    clientRef.current?.reconnectNow()
  }, [])

  return { status, latest, events, error, reconnect }
}
