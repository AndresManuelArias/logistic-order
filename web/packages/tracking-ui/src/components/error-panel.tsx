import { ApiError, toApiError } from '@tiendas/tracking-core'
import type { ReactNode } from 'react'

export interface ErrorPanelProps {
  error: unknown
  title?: string
  onRetry?: () => void
  action?: ReactNode
}

function describe(error: ApiError): string {
  if (error.isNetwork) return 'No pudimos conectar con el servidor. Revisa tu red e inténtalo de nuevo.'
  if (error.isUnauthorized) return 'Tu sesión caducó. Vuelve a iniciar sesión.'
  if (error.isForbidden) return 'Tu rol no tiene acceso a esta información.'
  if (error.isNotFound) return 'No encontramos la información solicitada.'
  if (error.status === 409) return 'La operación no es válida en el estado actual. Recarga los datos.'
  if (error.status === 422) return error.message || 'Revisa los datos enviados.'
  if (error.status === 429) return 'Demasiadas solicitudes. Espera unos segundos antes de reintentar.'
  if (error.status >= 500) return 'El servidor no está disponible en este momento. Inténtalo más tarde.'
  return error.message || 'Ocurrió un error inesperado.'
}

// Panel de error con mensaje accionable. No sustituye la autorización del
// backend: solo explica el resultado que el servidor ya decidió.
export function ErrorPanel({ error, title = 'No se pudo cargar la información', onRetry, action }: ErrorPanelProps) {
  const apiError = toApiError(error)
  return (
    <div className="tt-state tt-state--error" role="alert">
      <p className="tt-state__title">{title}</p>
      <p className="tt-state__text">{describe(apiError)}</p>
      {apiError.code ? <p className="tt-state__code">Código: {apiError.code}</p> : null}
      {onRetry || action ? (
        <div className="tt-state__action">
          {onRetry ? (
            <button type="button" className="tt-button" onClick={onRetry}>
              Reintentar
            </button>
          ) : null}
          {action}
        </div>
      ) : null}
    </div>
  )
}
