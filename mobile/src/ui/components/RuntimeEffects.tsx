import { useEffect } from 'react'
import { AppState } from 'react-native'

import { SYNC_POLICY } from '../../config/policies'
import { useServices } from '../context/ServicesProvider'

/**
 * Efectos del ciclo de vida autenticado:
 *
 * - Timer de 30 s para vaciar la cola sin intervención del usuario.
 * - Al volver a primer plano: sincroniza y reanuda el GPS de primer plano.
 * - Al pasar a segundo plano: pausa el fallback de primer plano; el servicio en
 *   segundo plano (si está activo) sigue registrando.
 */
export function RuntimeEffects() {
  const { coordinator, tracker } = useServices()

  useEffect(() => {
    void coordinator.syncNow('resume')

    const interval = setInterval(() => {
      void coordinator.syncNow('timer')
    }, SYNC_POLICY.tickMs)

    const subscription = AppState.addEventListener('change', (next) => {
      if (next === 'active') {
        void coordinator.syncNow('resume')
        void tracker.resume()
      } else if (next === 'background') {
        void tracker.suspend()
      }
    })

    return () => {
      clearInterval(interval)
      subscription.remove()
    }
  }, [coordinator, tracker])

  return null
}
