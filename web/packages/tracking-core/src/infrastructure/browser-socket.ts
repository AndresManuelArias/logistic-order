import type { SocketFactory, SocketHandlers, SocketLike } from '../ports/socket'

// Adaptador de navegador para el puerto SocketLike.
export const browserSocketFactory: SocketFactory = (url: string): SocketLike => {
  const socket = new WebSocket(url)
  let handlers: SocketHandlers | undefined

  socket.onopen = () => handlers?.onOpen()
  socket.onmessage = (event: MessageEvent) => {
    if (typeof event.data === 'string') handlers?.onMessage(event.data)
  }
  socket.onclose = (event: CloseEvent) => handlers?.onClose({ code: event.code, reason: event.reason })
  socket.onerror = () => handlers?.onError()

  return {
    send: (data) => socket.send(data),
    close: (code, reason) => socket.close(code, reason),
    setHandlers: (next) => {
      handlers = next
    },
  }
}
