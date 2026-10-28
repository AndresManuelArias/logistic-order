import { describe, expect, it, vi } from 'vitest'
import { buildSocketUrl, TrackingClient } from '../application/tracking-client'
import type { TelemetryPoint } from '../api/types'
import type { SocketCloseEvent, SocketHandlers, SocketLike, Timers } from '../ports/socket'

// Reloj determinista: avanza los temporizadores sin esperas reales.
class ManualTimers implements Timers {
  private now = 0
  private seq = 1
  private timeouts = new Map<number, { at: number; fn: () => void }>()
  private intervals = new Map<number, { every: number; next: number; fn: () => void }>()

  setTimeout(fn: () => void, ms: number): number {
    const id = this.seq++
    this.timeouts.set(id, { at: this.now + ms, fn })
    return id
  }

  clearTimeout(id: unknown): void {
    this.timeouts.delete(id as number)
  }

  setInterval(fn: () => void, ms: number): number {
    const id = this.seq++
    this.intervals.set(id, { every: ms, next: this.now + ms, fn })
    return id
  }

  clearInterval(id: unknown): void {
    this.intervals.delete(id as number)
  }

  advance(ms: number): void {
    const target = this.now + ms
    for (;;) {
      let due = Number.POSITIVE_INFINITY
      let kind: 'timeout' | 'interval' = 'timeout'
      let id = -1
      for (const [key, item] of this.timeouts) {
        if (item.at <= target && item.at < due) {
          due = item.at
          kind = 'timeout'
          id = key
        }
      }
      for (const [key, item] of this.intervals) {
        if (item.next <= target && item.next < due) {
          due = item.next
          kind = 'interval'
          id = key
        }
      }
      if (id === -1) break
      this.now = due
      if (kind === 'timeout') {
        const item = this.timeouts.get(id)
        this.timeouts.delete(id)
        item?.fn()
      } else {
        const item = this.intervals.get(id)
        if (!item) continue
        item.next += item.every
        item.fn()
      }
    }
    this.now = target
  }

  get pendingTimeouts(): number {
    return this.timeouts.size
  }
}

class FakeSocket implements SocketLike {
  sent: string[] = []
  closed?: { code?: number; reason?: string }
  private handlers?: SocketHandlers

  setHandlers(handlers: SocketHandlers): void {
    this.handlers = handlers
  }
  send(data: string): void {
    this.sent.push(data)
  }
  close(code?: number, reason?: string): void {
    this.closed = { code, reason }
  }
  open(): void {
    this.handlers?.onOpen()
  }
  receive(message: unknown): void {
    this.handlers?.onMessage(JSON.stringify(message))
  }
  receiveRaw(raw: string): void {
    this.handlers?.onMessage(raw)
  }
  serverClose(code: number): void {
    this.handlers?.onClose({ code, reason: '' } satisfies SocketCloseEvent)
  }
  parsed(): Array<Record<string, unknown>> {
    return this.sent.map((raw) => JSON.parse(raw) as Record<string, unknown>)
  }
}

function point(overrides: Partial<TelemetryPoint> = {}): TelemetryPoint {
  return {
    vehicle_id: 'veh-1',
    device_seq: 1,
    lat: 4.65,
    lon: -74.07,
    source: 'DEVICE',
    time: '2026-10-28T10:00:00Z',
    received_at: '2026-10-28T10:00:01Z',
    ...overrides,
  }
}

function setup(channels = ['order:o-1']) {
  const timers = new ManualTimers()
  const sockets: FakeSocket[] = []
  const urls: string[] = []
  const statuses: string[] = []
  const events: unknown[] = []
  const errors: Array<{ code: string; message: string }> = []
  const expired = vi.fn()

  const client = new TrackingClient({
    url: 'ws://localhost:8080/ws',
    token: 'tok+en',
    channels,
    random: () => 0,
    timers,
    socketFactory: (url) => {
      urls.push(url)
      const socket = new FakeSocket()
      sockets.push(socket)
      return socket
    },
    onStatus: (status) => statuses.push(status),
    onEvent: (event) => events.push(event),
    onError: (error) => errors.push(error),
    onSessionExpired: expired,
  })

  return { client, timers, sockets, urls, statuses, events, errors, expired }
}

describe('TrackingClient', () => {
  it('conecta, se suscribe una sola vez por canal y reporta open', () => {
    const { client, sockets, urls, statuses } = setup(['order:o-1', 'order:o-1', 'vehicle:v-1'])
    client.start()
    expect(statuses).toEqual(['connecting'])
    expect(urls[0]).toBe(buildSocketUrl('ws://localhost:8080/ws', 'tok+en'))
    expect(urls[0]).toContain('access_token=tok%2Ben')

    sockets[0]!.open()
    expect(statuses.at(-1)).toBe('open')
    const subs = sockets[0]!.parsed().filter((m) => m.action === 'subscribe')
    expect(subs.map((m) => m.channel)).toEqual(['order:o-1', 'vehicle:v-1'])
  })

  it('deduplica eventos y descarta los más antiguos que el último pintado', () => {
    const { client, sockets, events } = setup()
    client.start()
    sockets[0]!.open()

    const newer = point({ time: '2026-10-28T10:05:00Z' })
    sockets[0]!.receive({ type: 'event', channel: 'order:o-1', at: 'x', data: newer })
    expect(events).toHaveLength(1)

    // Reintento del dispositivo con la misma PK: no se pinta dos veces.
    sockets[0]!.receive({ type: 'event', channel: 'order:o-1', at: 'x', data: newer })
    expect(events).toHaveLength(1)

    // Evento reordenado (más antiguo): se ignora.
    sockets[0]!.receive({
      type: 'event',
      channel: 'order:o-1',
      at: 'x',
      data: point({ time: '2026-10-28T10:04:00Z' }),
    })
    expect(events).toHaveLength(1)

    // Evento nuevo: se entrega y actualiza el mapa interno.
    sockets[0]!.receive({
      type: 'event',
      channel: 'order:o-1',
      at: 'x',
      data: point({ time: '2026-10-28T10:06:00Z' }),
    })
    expect(events).toHaveLength(2)
    expect(client.getLatest()['veh-1']?.time).toBe('2026-10-28T10:06:00Z')
  })

  it('ignora canales no suscritos, mensajes inválidos y errores no JSON', () => {
    const { client, sockets, events } = setup()
    client.start()
    sockets[0]!.open()
    sockets[0]!.receive({ type: 'event', channel: 'order:ajena', at: 'x', data: point() })
    sockets[0]!.receive({ type: 'event', channel: 'order:o-1', at: 'x', data: { nope: true } })
    sockets[0]!.receiveRaw('no-json')
    expect(events).toHaveLength(0)
  })

  it('reconecta con backoff y vuelve a suscribirse', () => {
    const { client, timers, sockets, statuses } = setup()
    client.start()
    sockets[0]!.open()

    sockets[0]!.serverClose(1006)
    expect(statuses.at(-1)).toBe('reconnecting')
    expect(sockets).toHaveLength(1)

    timers.advance(499)
    expect(sockets).toHaveLength(1)
    timers.advance(1)
    expect(sockets).toHaveLength(2)
    sockets[1]!.open()
    expect(sockets[1]!.parsed().some((m) => m.action === 'subscribe')).toBe(true)
  })

  it('el backoff se duplica mientras la conexión no llega a abrirse', () => {
    const { client, timers, sockets } = setup()
    client.start()
    sockets[0]!.open()

    // Primer fallo tras una sesión abierta: 500 ms.
    sockets[0]!.serverClose(1006)
    timers.advance(500)
    expect(sockets).toHaveLength(2)

    // Segundo fallo sin abrir: 1000 ms, no 500.
    sockets[1]!.serverClose(1006)
    timers.advance(999)
    expect(sockets).toHaveLength(2)
    timers.advance(1)
    expect(sockets).toHaveLength(3)

    // Tercer fallo sin abrir: 2000 ms.
    sockets[2]!.serverClose(1006)
    timers.advance(1999)
    expect(sockets).toHaveLength(3)
    timers.advance(1)
    expect(sockets).toHaveLength(4)
  })

  it('no reconecta cuando el token expiró (4401) y avisa a la sesión', () => {
    const { client, timers, sockets, statuses, expired } = setup()
    client.start()
    sockets[0]!.open()
    sockets[0]!.serverClose(4401)
    expect(statuses.at(-1)).toBe('session-expired')
    expect(expired).toHaveBeenCalledTimes(1)
    timers.advance(60_000)
    expect(sockets).toHaveLength(1)
  })

  it('reconnectNow reinicia el backoff y abre una conexión nueva', () => {
    const { client, sockets } = setup()
    client.start()
    sockets[0]!.open()
    sockets[0]!.serverClose(1006)
    client.reconnectNow()
    expect(sockets).toHaveLength(2)
  })

  it('stop cierra el socket y cancela temporizadores pendientes', () => {
    const { client, timers, sockets, statuses } = setup()
    client.start()
    sockets[0]!.open()
    client.stop()
    expect(sockets[0]!.closed?.code).toBe(1000)
    expect(statuses.at(-1)).toBe('closed')
    timers.advance(60_000)
    expect(sockets).toHaveLength(1)
    expect(timers.pendingTimeouts).toBe(0)
  })

  it('reporta errores del protocolo (p. ej. FORBIDDEN_SCOPE)', () => {
    const { client, sockets, errors } = setup()
    client.start()
    sockets[0]!.open()
    sockets[0]!.receive({ type: 'error', code: 'FORBIDDEN_SCOPE', message: 'canal ajeno' })
    expect(errors).toEqual([{ code: 'FORBIDDEN_SCOPE', message: 'canal ajeno' }])
  })
})
