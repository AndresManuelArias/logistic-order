export interface RawLocationSample {
  lat: number
  lon: number
  /** Epoch ms del sistema operativo. */
  timestamp: number
  accuracyM: number | null
  speedMps: number | null
  heading: number | null
}

export type PermissionState = 'granted' | 'denied' | 'undetermined'

export type TrackingMode = 'off' | 'foreground' | 'background'

export interface LocationTrackingState {
  mode: TrackingMode
  foreground: PermissionState
  background: PermissionState
  /** Explicación apta para mostrar al conductor. */
  message: string
}

export interface LocationTrackerPort {
  getState(): Promise<LocationTrackingState>
  /** Solicita permisos y arranca el seguimiento (segundo plano si es posible). */
  start(): Promise<LocationTrackingState>
  stop(): Promise<LocationTrackingState>
  /** Pausa el fallback de primer plano cuando la app pasa a background. */
  suspend(): Promise<void>
  /** Reanuda el fallback de primer plano al volver a la app. */
  resume(): Promise<void>
}
