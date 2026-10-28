import type { LogContext, Logger } from '../../ports/clock'

const LEVEL_VALUE = { debug: 10, info: 20, warn: 30, error: 40 } as const

export type LogLevelName = keyof typeof LEVEL_VALUE

/**
 * Logger mínimo. No se registran coordenadas ni tokens: sólo identificadores y
 * metadatos operativos.
 */
export function createConsoleLogger(minLevel: LogLevelName = 'info'): Logger {
  const threshold = LEVEL_VALUE[minLevel]

  const emit = (level: LogLevelName, message: string, context?: LogContext) => {
    if (LEVEL_VALUE[level] < threshold) return
    const prefix = `[tiendas:${level}]`
    if (context) {
      // eslint-disable-next-line no-console
      console[level === 'debug' ? 'log' : level](prefix, message, context)
    } else {
      // eslint-disable-next-line no-console
      console[level === 'debug' ? 'log' : level](prefix, message)
    }
  }

  return {
    debug: (message, context) => emit('debug', message, context),
    info: (message, context) => emit('info', message, context),
    warn: (message, context) => emit('warn', message, context),
    error: (message, context) => emit('error', message, context),
  }
}
