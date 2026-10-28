import type * as Location from 'expo-location'
import * as TaskManager from 'expo-task-manager'

import type { RawLocationSample } from '../../ports/location'

export const LOCATION_TASK = 'tiendas-track-gps'

export type LocationTaskHandler = (samples: RawLocationSample[]) => Promise<void>

let handler: LocationTaskHandler | null = null

export function setLocationTaskHandler(next: LocationTaskHandler | null): void {
  handler = next
}

export function toRawSample(location: Location.LocationObject): RawLocationSample {
  const { coords } = location
  const speed = coords.speed
  const heading = coords.heading
  return {
    lat: coords.latitude,
    lon: coords.longitude,
    timestamp: location.timestamp,
    accuracyM: typeof coords.accuracy === 'number' ? coords.accuracy : null,
    speedMps: typeof speed === 'number' && speed >= 0 ? speed : null,
    heading: typeof heading === 'number' && heading >= 0 ? heading : null,
  }
}

/**
 * Debe invocarse en el ámbito de módulo (antes de montar React) para que el
 * sistema operativo pueda despertar la tarea en background/headless.
 */
export function registerLocationTask(): void {
  if (TaskManager.isTaskDefined(LOCATION_TASK)) return

  TaskManager.defineTask<{ locations?: Location.LocationObject[] }>(
    LOCATION_TASK,
    async ({ data, error }) => {
      if (error) {
        // El logger de la app puede no estar inicializado en headless.
        console.warn('[tiendas:gps] error en la tarea de ubicación', error.message)
        return
      }

      const samples = (data?.locations ?? []).map(toRawSample)
      if (samples.length === 0) return

      const current = handler
      if (!current) {
        console.warn('[tiendas:gps] tarea sin manejador: ¿arranque headless sin servicios?')
        return
      }

      try {
        await current(samples)
      } catch (taskError) {
        console.error('[tiendas:gps] no se pudieron procesar las muestras', taskError)
      }
    },
  )
}
