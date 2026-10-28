'use client'

import { useCallback, useEffect, useState } from 'react'

export interface ResourceState<T> {
  data: T | null
  error: unknown
  loading: boolean
  reload: () => void
}

interface InternalState<T> {
  data: T | null
  error: unknown
  loading: boolean
}

// Carga de datos con cancelación lógica: si el componente se desmonta o cambia
// la clave, el resultado tardío no pisa el estado. El `loader` debe ser estable
// (useCallback): es la dependencia que dispara la recarga.
//
// El estado de carga se activa al inicio del efecto a propósito: es la señal
// que usa la UI para mostrar el esqueleto mientras llega la respuesta.
/* eslint-disable react-hooks/set-state-in-effect -- carga de datos en el efecto; el estado de carga es intencional */
export function useApiResource<T>(
  loader: () => Promise<T>,
  options: { enabled?: boolean; reloadKey?: string | number } = {},
): ResourceState<T> {
  const { enabled = true, reloadKey = '' } = options
  const [state, setState] = useState<InternalState<T>>({
    data: null,
    error: null,
    loading: enabled,
  })
  const [nonce, setNonce] = useState(0)

  useEffect(() => {
    if (!enabled) return

    let cancelled = false
    setState((previous) => ({ ...previous, loading: true, error: null }))

    loader()
      .then((result) => {
        if (cancelled) return
        setState({ data: result, error: null, loading: false })
      })
      .catch((reason: unknown) => {
        if (cancelled) return
        setState({ data: null, error: reason, loading: false })
      })

    return () => {
      cancelled = true
    }
  }, [enabled, reloadKey, nonce, loader])

  const reload = useCallback(() => setNonce((value) => value + 1), [])

  // Al deshabilitarse se oculta lo cargado sin provocar un render extra desde
  // el efecto (el estado se deriva del flag).
  if (!enabled) {
    return { data: null, error: null, loading: false, reload }
  }

  return { ...state, reload }
}
/* eslint-enable react-hooks/set-state-in-effect */
