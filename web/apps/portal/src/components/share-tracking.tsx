'use client'

// Emisión del enlace de invitado. El token viaja en el fragmento de la URL
// (#t=…): no se envía al servidor del portal ni queda en logs de acceso.
import { formatDateTime } from '@tiendas/tracking-ui'
import { toApiError } from '@tiendas/tracking-core'
import { useState } from 'react'
import { useAuth } from './auth-provider'

export function ShareTracking({ orderId }: { orderId: string }) {
  const { api } = useAuth()
  const [link, setLink] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [pending, setPending] = useState(false)

  async function handleGenerate() {
    setPending(true)
    setError(null)
    setCopied(false)
    try {
      const issued = await api.issueGuestToken(orderId)
      const hash = `#t=${encodeURIComponent(issued.token)}`
      setLink(`${window.location.origin}/seguimiento/${orderId}${hash}`)
      setExpiresAt(issued.expires_at)
    } catch (reason) {
      const apiError = toApiError(reason)
      setError(
        apiError.isForbidden
          ? 'Tu rol no puede emitir enlaces para esta orden.'
          : apiError.message || 'No se pudo generar el enlace.',
      )
    } finally {
      setPending(false)
    }
  }

  async function handleCopy() {
    if (!link) return
    if (!navigator.clipboard) {
      setError('No se pudo copiar automáticamente. Selecciona el enlace y cópialo manualmente.')
      return
    }
    try {
      await navigator.clipboard.writeText(link)
      setCopied(true)
      setError(null)
    } catch {
      setError('No se pudo copiar automáticamente. Selecciona el enlace y cópialo manualmente.')
    }
  }

  return (
    <div className="tt-share">
      <div>
        <button type="button" className="tt-button tt-button--ghost" onClick={handleGenerate} disabled={pending}>
          {pending ? 'Generando…' : link ? 'Generar otro enlace' : 'Compartir seguimiento'}
        </button>
      </div>

      {link ? (
        <div className="tt-share" aria-live="polite">
          <label className="tt-field__label" htmlFor={`share-${orderId}`}>
            Enlace de invitado
          </label>
          <div className="tt-share__row">
            <input
              id={`share-${orderId}`}
              className="tt-input tt-share__input"
              type="text"
              readOnly
              value={link}
              onFocus={(event) => event.currentTarget.select()}
            />
            <button type="button" className="tt-button" onClick={handleCopy}>
              {copied ? 'Copiado' : 'Copiar'}
            </button>
          </div>
          <p className="tt-share__note">
            El cliente final ve solo esta orden.
            {expiresAt ? ` El enlace caduca el ${formatDateTime(expiresAt)}.` : ''}
          </p>
        </div>
      ) : null}

      {error ? (
        <p className="tt-form__error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  )
}
