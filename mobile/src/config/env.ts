import { Platform } from 'react-native'

// `process.env.EXPO_PUBLIC_*` se sustituye en build-time por Metro/Babel; debe
// leerse de forma literal (no mediante una variable intermedia) para que el
// reemplazo funcione.
const configuredUrl = process.env.EXPO_PUBLIC_API_URL

const fallbackUrl = Platform.OS === 'android' ? 'http://10.0.2.2:8080' : 'http://localhost:8080'

export const API_BASE_URL = (configuredUrl && configuredUrl.trim()) || fallbackUrl

export const DEVICE_NAME = process.env.EXPO_PUBLIC_DEVICE_NAME || 'driver-app'

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

const LOG_LEVELS: Record<LogLevel, number> = { debug: 10, info: 20, warn: 30, error: 40 }

function parseLogLevel(value: string | undefined): LogLevel {
  if (value === 'debug' || value === 'info' || value === 'warn' || value === 'error') return value
  return 'info'
}

export const LOG_LEVEL = parseLogLevel(process.env.EXPO_PUBLIC_LOG_LEVEL)
export const LOG_LEVEL_VALUE = LOG_LEVELS[LOG_LEVEL]
