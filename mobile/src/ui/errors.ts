import { isAppError } from '../domain/errors'

/** Traduce un error técnico a un mensaje accionable para el conductor. */
export function describeError(error: unknown): string {
  if (!isAppError(error)) {
    return error instanceof Error ? error.message : 'Ocurrió un error inesperado'
  }
  switch (error.code) {
    case 'NETWORK':
      return 'Sin conexión con el servidor. Revisa tu red e inténtalo de nuevo.'
    case 'TIMEOUT':
      return 'El servidor tardó demasiado en responder. Inténtalo de nuevo.'
    case 'UNAUTHENTICATED':
      return 'Correo o contraseña incorrectos, o la sesión caducó.'
    case 'FORBIDDEN':
      return error.message || 'No tienes permisos para esta operación.'
    case 'NOT_FOUND':
      return 'No se encontró la información solicitada.'
    case 'CONFLICT':
      return error.message || 'La orden cambió en otro dispositivo. Actualiza la pantalla.'
    case 'VALIDATION':
      return error.message || 'Los datos enviados no son válidos.'
    case 'RATE_LIMITED':
      return 'Demasiadas solicitudes. Espera un momento y vuelve a intentarlo.'
    case 'SERVER':
      return 'El servidor no está disponible. Tus datos quedaron guardados.'
    default:
      return error.message || 'Ocurrió un error inesperado.'
  }
}
