import { renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useApiResource } from '@/hooks/use-api-resource'

function deferred<T>() {
  let resolve: (value: T) => void = () => {}
  let reject: (reason: unknown) => void = () => {}
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('useApiResource', () => {
  it('carga los datos y termina el estado de carga', async () => {
    const loader = vi.fn().mockResolvedValue({ id: 'o-1' })
    const { result } = renderHook(() => useApiResource(loader))

    expect(result.current.loading).toBe(true)
    await waitFor(() => expect(result.current.data).toEqual({ id: 'o-1' }))
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBeNull()
    expect(loader).toHaveBeenCalledTimes(1)
  })

  it('expone el error y permite reintentar', async () => {
    const loader = vi
      .fn()
      .mockRejectedValueOnce(new Error('sin red'))
      .mockResolvedValueOnce('ok')
    const { result } = renderHook(() => useApiResource(loader))

    await waitFor(() => expect(result.current.error).toBeInstanceOf(Error))
    expect(result.current.data).toBeNull()

    result.current.reload()
    await waitFor(() => expect(result.current.data).toBe('ok'))
    expect(result.current.error).toBeNull()
  })

  it('no llama al cargador mientras está deshabilitado', () => {
    const loader = vi.fn().mockResolvedValue('dato')
    const { result } = renderHook(() => useApiResource(loader, { enabled: false }))

    expect(loader).not.toHaveBeenCalled()
    expect(result.current).toMatchObject({ data: null, error: null, loading: false })
  })

  it('descarta la respuesta obsoleta al cambiar la clave', async () => {
    const first = deferred<string>()
    const second = deferred<string>()
    const loader = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)

    const { result, rerender } = renderHook(
      ({ key }: { key: string }) => useApiResource(loader, { reloadKey: key }),
      { initialProps: { key: 'a' } },
    )

    rerender({ key: 'b' })
    second.resolve('nuevo')
    await waitFor(() => expect(result.current.data).toBe('nuevo'))

    // La primera petición termina después: no debe pisar el resultado nuevo.
    first.resolve('viejo')
    await waitFor(() => expect(loader).toHaveBeenCalledTimes(2))
    expect(result.current.data).toBe('nuevo')
  })

  it('conserva los datos previos mientras recarga', async () => {
    const first = deferred<string>()
    const second = deferred<string>()
    const loader = vi
      .fn()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)

    const { result } = renderHook(() => useApiResource(loader))

    first.resolve('inicial')
    await waitFor(() => expect(result.current.data).toBe('inicial'))

    result.current.reload()
    await waitFor(() => expect(result.current.loading).toBe(true))
    expect(result.current.data).toBe('inicial')

    second.resolve('actualizado')
    await waitFor(() => expect(result.current.data).toBe('actualizado'))
  })
})
