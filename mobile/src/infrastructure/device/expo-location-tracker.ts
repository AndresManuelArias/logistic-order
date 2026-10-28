import * as Location from 'expo-location'

import type { GpsPolicy } from '../../config/policies'
import type { Logger } from '../../ports/clock'
import type {
  LocationTrackingState,
  LocationTrackerPort,
  PermissionState,
  RawLocationSample,
  TrackingMode,
} from '../../ports/location'
import { LOCATION_TASK, toRawSample } from './location-task'

const ACCURACY: Record<GpsPolicy['accuracy'], Location.LocationAccuracy> = {
  Low: Location.Accuracy.Low,
  Balanced: Location.Accuracy.Balanced,
  High: Location.Accuracy.High,
  Highest: Location.Accuracy.Highest,
  BestForNavigation: Location.Accuracy.BestForNavigation,
}

function toPermission(status: string): PermissionState {
  if (status === 'granted') return 'granted'
  if (status === 'denied') return 'denied'
  return 'undetermined'
}

const MESSAGES = {
  denied: 'Permiso de ubicación denegado. Actívalo en los ajustes del sistema para registrar la ruta.',
  foregroundOnly:
    'Seguimiento activo sólo con la app abierta. Concede "Permitir siempre" para registrar la ruta en segundo plano.',
  background: 'Seguimiento GPS activo en segundo plano. Verás una notificación mientras esté en marcha.',
  off: 'Seguimiento GPS desactivado.',
} as const

export interface ExpoLocationTrackerDeps {
  onSamples: (samples: RawLocationSample[]) => Promise<void>
  policy: GpsPolicy
  logger: Logger
}

/**
 * Seguimiento GPS con dos modos:
 *
 * - `background`: tarea registrada con `expo-location` + foreground service en
 *   Android (notificación persistente) y `UIBackgroundModes: location` en iOS.
 * - `foreground`: si el usuario sólo concede "mientras uso la app", se usa
 *   `watchPositionAsync` y se pausa al pasar a background.
 *
 * Se pide `Balanced` + 15 s/25 m para equilibrar precisión y batería.
 */
export class ExpoLocationTracker implements LocationTrackerPort {
  private watch: Location.LocationSubscription | null = null
  private mode: TrackingMode = 'off'
  private suspended = false

  constructor(private readonly deps: ExpoLocationTrackerDeps) {}

  async getState(): Promise<LocationTrackingState> {
    const [foreground, background] = await Promise.all([
      Location.getForegroundPermissionsAsync(),
      Location.getBackgroundPermissionsAsync(),
    ])

    let mode = this.mode
    if (mode === 'background' && !(await this.isBackgroundRunning())) {
      mode = 'off'
      this.mode = 'off'
    }
    if (mode === 'off' && (await this.isBackgroundRunning())) {
      mode = 'background'
      this.mode = 'background'
    }

    const fg = toPermission(foreground.status)
    const bg = toPermission(background.status)
    let message: string = MESSAGES.off
    if (fg === 'denied') message = MESSAGES.denied
    else if (mode === 'background') message = MESSAGES.background
    else if (mode === 'foreground') message = MESSAGES.foregroundOnly

    return { mode, foreground: fg, background: bg, message }
  }

  async start(): Promise<LocationTrackingState> {
    const foreground = await Location.requestForegroundPermissionsAsync()
    if (foreground.status !== 'granted') {
      this.mode = 'off'
      return this.getState()
    }

    const background = await Location.requestBackgroundPermissionsAsync()
    if (background.status === 'granted') {
      await this.startBackground()
      this.mode = 'background'
      this.suspended = false
    } else {
      await this.startForegroundWatch()
      this.mode = 'foreground'
    }

    const state = await this.getState()
    this.deps.logger.info('Seguimiento GPS iniciado', {
      mode: state.mode,
      foreground: state.foreground,
      background: state.background,
    })
    return state
  }

  async stop(): Promise<LocationTrackingState> {
    this.stopForegroundWatch()
    if (await this.isBackgroundRunning()) {
      try {
        await Location.stopLocationUpdatesAsync(LOCATION_TASK)
      } catch (error) {
        this.deps.logger.warn('No se pudo detener la tarea de ubicación', {
          message: error instanceof Error ? error.message : String(error),
        })
      }
    }
    this.mode = 'off'
    this.suspended = false
    this.deps.logger.info('Seguimiento GPS detenido')
    return this.getState()
  }

  async suspend(): Promise<void> {
    if (this.mode !== 'foreground' || !this.watch) return
    this.stopForegroundWatch()
    this.suspended = true
  }

  async resume(): Promise<void> {
    if (!this.suspended || this.mode !== 'foreground') return
    this.suspended = false
    await this.startForegroundWatch()
  }

  private async startBackground(): Promise<void> {
    this.stopForegroundWatch()
    if (await this.isBackgroundRunning()) return

    await Location.startLocationUpdatesAsync(LOCATION_TASK, {
      accuracy: ACCURACY[this.deps.policy.accuracy],
      timeInterval: this.deps.policy.timeIntervalMs,
      distanceInterval: this.deps.policy.distanceIntervalM,
      pausesUpdatesAutomatically: false,
      showsBackgroundLocationIndicator: true,
      foregroundService: {
        notificationTitle: 'Tiendas-Track en ruta',
        notificationBody: 'Registrando tu ubicación para las entregas en curso.',
        notificationColor: '#2563eb',
        killServiceOnDestroy: false,
      },
    })
  }

  private async startForegroundWatch(): Promise<void> {
    if (this.watch) return
    this.watch = await Location.watchPositionAsync(
      {
        accuracy: ACCURACY[this.deps.policy.accuracy],
        timeInterval: this.deps.policy.timeIntervalMs,
        distanceInterval: this.deps.policy.distanceIntervalM,
      },
      (location) => {
        void this.emit([toRawSample(location)])
      },
      (error) => {
        this.deps.logger.warn('Error del GPS en primer plano', { message: String(error) })
      },
    )
  }

  private stopForegroundWatch(): void {
    this.watch?.remove()
    this.watch = null
  }

  private async emit(samples: RawLocationSample[]): Promise<void> {
    try {
      await this.deps.onSamples(samples)
    } catch (error) {
      this.deps.logger.error('No se pudieron registrar las muestras GPS', {
        message: error instanceof Error ? error.message : String(error),
      })
    }
  }

  private async isBackgroundRunning(): Promise<boolean> {
    try {
      return await Location.hasStartedLocationUpdatesAsync(LOCATION_TASK)
    } catch {
      return false
    }
  }
}
