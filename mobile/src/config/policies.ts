// Políticas de captura y sincronización. Son decisiones operativas (batería,
// red y límites del backend), no reglas de negocio: cualquier cambio debe
// quedar documentado en mobile/README.md.

export interface RetryPolicy {
  /** Espera base del primer reintento. */
  baseMs: number
  /** Tope de la espera exponencial. */
  maxMs: number
  /** Proporción de jitter aplicada (0.2 = ±20 %). */
  jitterRatio: number
}

export interface SyncPolicy {
  /** Máximo de puntos por lote (el backend acepta hasta 1000). */
  maxBatchSize: number
  /** Lotes enviados por ciclo para no bloquear la UI indefinidamente. */
  maxBatchesPerRun: number
  /** Transiciones de orden enviadas por ciclo. */
  maxTransitionsPerRun: number
  /** Reintentos antes de cuarentena para errores permanentes por lotes. */
  retry: RetryPolicy
  /** Intervalo del temporizador de sincronización en primer plano. */
  tickMs: number
}

export const SYNC_POLICY: SyncPolicy = {
  // 200 puntos ~ 15 min de captura a 15 s. Mantiene los cuerpos pequeños en
  // redes 3G y reduce la ventana de pérdida ante cierres de la app.
  maxBatchSize: 200,
  maxBatchesPerRun: 5,
  maxTransitionsPerRun: 10,
  retry: { baseMs: 5_000, maxMs: 5 * 60_000, jitterRatio: 0.2 },
  tickMs: 30_000,
}

export interface GpsPolicy {
  /** Frecuencia mínima entre muestras (batería). */
  timeIntervalMs: number
  /** Distancia mínima para forzar una muestra. */
  distanceIntervalM: number
  /** Precisión solicitada al sistema operativo. */
  accuracy: 'Low' | 'Balanced' | 'High' | 'Highest' | 'BestForNavigation'
  /** Se descartan muestras con peor precisión (metros). 100 m ≈ manzana urbana. */
  maxAccuracyMeters: number
}

export const GPS_POLICY: GpsPolicy = {
  // 15 s / 25 m es el compromiso usado por apps de reparto urbanas: suficiente
  // resolución para la traza y ~4 muestras/min en lugar de 1/s.
  timeIntervalMs: 15_000,
  distanceIntervalM: 25,
  // `High` (GPS) y no `Balanced`: en Android `Balanced` se traduce a
  // PRIORITY_BALANCED_POWER_ACCURACY, que FusedLocationProvider resuelve con
  // localización por red. En un equipo sin ese proveedor (o sin datos de red)
  // no llega nunca una muestra, aunque el GPS esté disponible. La traza de
  // ruta necesita GPS; el ahorro de batería se mantiene con 15 s / 25 m.
  accuracy: 'High',
  maxAccuracyMeters: 100,
}

/** Margen para considerar válido el access token antes de refrescarlo. */
export const SESSION_CLOCK_SKEW_MS = 30_000

/** Timeout por petición HTTP (incluye subida de lotes). */
export const HTTP_TIMEOUT_MS = 20_000
