// Formateadores de presentación compartidos por el portal. No contienen reglas
// de negocio: solo traducen valores del contrato a texto legible (es-CO).

const DATE_FORMATTER = new Intl.DateTimeFormat('es-CO', {
  dateStyle: 'medium',
  timeStyle: 'short',
  timeZone: 'America/Bogota',
})

const TIME_FORMATTER = new Intl.DateTimeFormat('es-CO', {
  hour: '2-digit',
  minute: '2-digit',
  timeZone: 'America/Bogota',
})

export function formatDateTime(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return DATE_FORMATTER.format(date)
}

export function formatTime(value: string | null | undefined): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return TIME_FORMATTER.format(date)
}

export function formatDistance(meters: number | null | undefined): string {
  if (meters === null || meters === undefined || !Number.isFinite(meters)) return '—'
  if (meters < 1000) return `${Math.round(meters)} m`
  return `${(meters / 1000).toFixed(1)} km`
}

export function formatSpeed(kph: number | null | undefined): string {
  if (kph === null || kph === undefined || !Number.isFinite(kph)) return '—'
  return `${Math.round(kph)} km/h`
}

// Minutos transcurridos desde una marca temporal; null si no es válida.
export function minutesSince(value: string | null | undefined, now = Date.now()): number | null {
  if (!value) return null
  const time = Date.parse(value)
  if (Number.isNaN(time)) return null
  return Math.max(0, Math.round((now - time) / 60_000))
}

export function formatFreshness(value: string | null | undefined, now = Date.now()): string {
  const minutes = minutesSince(value, now)
  if (minutes === null) return 'Sin datos'
  if (minutes < 1) return 'Ahora mismo'
  if (minutes === 1) return 'Hace 1 min'
  if (minutes < 60) return `Hace ${minutes} min`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `Hace ${hours} h`
  return formatDateTime(value)
}
