// Superficie pública de @tiendas/tracking-core: dominio, puertos, caso de uso
// de tiempo real, cliente HTTP tipado y adaptadores.
export * from './api/types'
export * from './api/client'
export * from './domain/errors'
export * from './domain/session'
export * from './domain/telemetry'
export * from './ports/socket'
export * from './application/tracking-client'
export * from './infrastructure/browser-socket'
export * from './hooks/use-tracking'
