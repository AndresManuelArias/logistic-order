// Puerto de socket: aísla el caso de uso de `window.WebSocket` y permite
// pruebas deterministas con un doble en memoria.
export interface SocketCloseEvent {
  code: number
  reason: string
}

export interface SocketHandlers {
  onOpen: () => void
  onMessage: (raw: string) => void
  onClose: (event: SocketCloseEvent) => void
  onError: () => void
}

export interface SocketLike {
  send: (data: string) => void
  close: (code?: number, reason?: string) => void
  setHandlers: (handlers: SocketHandlers) => void
}

export type SocketFactory = (url: string) => SocketLike

// Reloj inyectable: los tests usan temporizadores falsos sin esperas reales.
export interface Timers {
  setTimeout: (fn: () => void, ms: number) => unknown
  clearTimeout: (id: unknown) => void
  setInterval: (fn: () => void, ms: number) => unknown
  clearInterval: (id: unknown) => void
}
