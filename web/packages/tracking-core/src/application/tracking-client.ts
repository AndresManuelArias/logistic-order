// Caso de uso de tiempo real: conexión, suscripción, reconexión con backoff,
// deduplicación y descarte de eventos obsoletos. Sin React ni `window`.
import type { TelemetryPoint } from '../api/types'
import {
  isNewerPoint,
  mergeLatestPoint,
  pointKey,
  type FleetEvent,
  type TrackingStatus,
} from '../domain/telemetry'
import type { SocketFactory, SocketLike, Timers } from '../ports/socket'

export interface TrackingClientOptions {
  url: string
  token: string
  channels: string[]
  socketFactory: SocketFactory
  onStatus: (status: TrackingStatus) => void
  onEvent: (event: FleetEvent) => void
  onError: (error: { code: string; message: string }) => void
  onSessionExpired: () => void
  timers?: Timers
  random?: () => number
  baseBackoffMs?: number
  maxBackoffMs?: number
  pingIntervalMs?: number
  maxSeenKeys?: number
}

const DEFAULT_BASE_BACKOFF_MS = 500
const DEFAULT_MAX_BACKOFF_MS = 10_000
const DEFAULT_PING_MS = 25_000
const DEFAULT_MAX_SEEN = 1_000

const systemTimers: Timers = {
  setTimeout: (fn, ms) => globalThis.setTimeout(fn, ms),
  clearTimeout: (id) => globalThis.clearTimeout(id as ReturnType<typeof setTimeout>),
  setInterval: (fn, ms) => globalThis.setInterval(fn, ms),
  clearInterval: (id) => globalThis.clearInterval(id as ReturnType<typeof setInterval>),
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// El backend garantiza el schema; aquí solo se comprueba lo mínimo para no
// romper la UI con un mensaje malformado.
function isTelemetryPoint(value: unknown): value is TelemetryPoint {
  if (!isRecord(value)) return false
  return (
    typeof value.vehicle_id === 'string' &&
    typeof value.time === 'string' &&
    typeof value.lat === 'number' &&
    typeof value.lon === 'number'
  )
}

export function buildSocketUrl(baseUrl: string, token: string): string {
  const separator = baseUrl.includes('?') ? '&' : '?'
  return `${baseUrl}${separator}access_token=${encodeURIComponent(token)}`
}

export class TrackingClient {
  private readonly opts: TrackingClientOptions
  private readonly timers: Timers
  private readonly channels: Set<string>
  private readonly seen = new Set<string>()
  private readonly seenOrder: string[] = []
  private socket?: SocketLike
  private reconnectTimer?: unknown
  private pingTimer?: unknown
  private attempts = 0
  private stopped = false
  private status: TrackingStatus = 'idle'
  private latest: Record<string, TelemetryPoint> = {}

  constructor(options: TrackingClientOptions) {
    this.opts = options
    this.timers = options.timers ?? systemTimers
    // Los canales repetidos no generan suscripciones duplicadas.
    this.channels = new Set(options.channels.filter(Boolean))
  }

  start(): void {
    if (this.socket || this.stopped) return
    this.connect()
  }

  stop(): void {
    this.stopped = true
    this.clearReconnect()
    this.clearPing()
    const socket = this.socket
    this.socket = undefined
    socket?.close(1000, 'cliente cerrado')
    this.setStatus('closed')
  }

  // Reconexión manual (botón "Reintentar" de la UI).
  reconnectNow(): void {
    if (this.stopped) return
    this.clearReconnect()
    this.attempts = 0
    this.socket?.close(1000, 'reconexión manual')
    this.socket = undefined
    this.connect()
  }

  getStatus(): TrackingStatus {
    return this.status
  }

  getLatest(): Record<string, TelemetryPoint> {
    return this.latest
  }

  private connect(): void {
    if (this.stopped) return
    this.setStatus(this.attempts === 0 ? 'connecting' : 'reconnecting')
    const socket = this.opts.socketFactory(buildSocketUrl(this.opts.url, this.opts.token))
    this.socket = socket
    socket.setHandlers({
      onOpen: () => this.handleOpen(),
      onMessage: (raw) => this.handleMessage(raw),
      onClose: (event) => this.handleClose(event.code),
      onError: () => {
        // El cierre siempre llega después; la reconexión se agenda allí.
      },
    })
  }

  private handleOpen(): void {
    if (this.stopped) return
    this.attempts = 0
    this.setStatus('open')
    for (const channel of this.channels) {
      this.send({ action: 'subscribe', channel })
    }
    this.startPing()
  }

  private handleClose(code: number): void {
    this.socket = undefined
    this.clearPing()
    if (this.stopped) return

    // 4401: el token expiró. No se reintenta con el mismo token.
    if (code === 4401) {
      this.setStatus('session-expired')
      this.opts.onSessionExpired()
      return
    }
    this.scheduleReconnect()
  }

  private scheduleReconnect(): void {
    this.attempts += 1
    const base = Math.min(
      this.opts.maxBackoffMs ?? DEFAULT_MAX_BACKOFF_MS,
      (this.opts.baseBackoffMs ?? DEFAULT_BASE_BACKOFF_MS) * 2 ** (this.attempts - 1),
    )
    const jitter = Math.floor((this.opts.random ?? Math.random)() * base * 0.25)
    this.setStatus('reconnecting')
    this.reconnectTimer = this.timers.setTimeout(() => {
      this.reconnectTimer = undefined
      if (!this.stopped) this.connect()
    }, base + jitter)
  }

  private startPing(): void {
    this.clearPing()
    this.pingTimer = this.timers.setInterval(() => {
      this.send({ action: 'ping' })
    }, this.opts.pingIntervalMs ?? DEFAULT_PING_MS)
  }

  private clearPing(): void {
    if (this.pingTimer !== undefined) {
      this.timers.clearInterval(this.pingTimer)
      this.pingTimer = undefined
    }
  }

  private clearReconnect(): void {
    if (this.reconnectTimer !== undefined) {
      this.timers.clearTimeout(this.reconnectTimer)
      this.reconnectTimer = undefined
    }
  }

  private send(message: Record<string, unknown>): void {
    try {
      this.socket?.send(JSON.stringify(message))
    } catch {
      // Un socket cerrado lanza al enviar; el ciclo de cierre se encarga.
    }
  }

  private handleMessage(raw: string): void {
    let parsed: unknown
    try {
      parsed = JSON.parse(raw)
    } catch {
      return
    }
    if (!isRecord(parsed)) return

    switch (parsed.type) {
      case 'event': {
        const channel = typeof parsed.channel === 'string' ? parsed.channel : ''
        if (!this.channels.has(channel)) return
        if (!isTelemetryPoint(parsed.data)) return
        this.handleEvent({
          channel,
          at: typeof parsed.at === 'string' ? parsed.at : new Date().toISOString(),
          data: parsed.data,
        })
        return
      }
      case 'error':
        this.opts.onError({
          code: typeof parsed.code === 'string' ? parsed.code : 'WS_ERROR',
          message: typeof parsed.message === 'string' ? parsed.message : 'Error del canal',
        })
        return
      default:
        // `subscribed`, `unsubscribed`, `pong`: acuses sin estado de UI.
        return
    }
  }

  private handleEvent(event: FleetEvent): void {
    const key = pointKey(event.data)
    if (this.seen.has(key)) return
    this.remember(key)

    // Un evento más antiguo que el último pintado se descarta (protección
    // contra reordenamientos tras una reconexión).
    if (!isNewerPoint(event.data, this.latest[event.data.vehicle_id])) return

    this.latest = mergeLatestPoint(this.latest, event.data)
    this.opts.onEvent(event)
  }

  private remember(key: string): void {
    this.seen.add(key)
    this.seenOrder.push(key)
    const max = this.opts.maxSeenKeys ?? DEFAULT_MAX_SEEN
    while (this.seenOrder.length > max) {
      const oldest = this.seenOrder.shift()
      if (oldest) this.seen.delete(oldest)
    }
  }

  private setStatus(status: TrackingStatus): void {
    if (this.status === status) return
    this.status = status
    this.opts.onStatus(status)
  }
}
