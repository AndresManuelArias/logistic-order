'use client'

// Panel del asistente operativo. El contrato (`POST /api/v1/ai/query`) es
// provisional y pertenece a F5: si el backend responde 404 se informa y no se
// inventan respuestas.
import { ApiError, toApiError } from '@tiendas/tracking-core'
import { useRef, useState, type FormEvent } from 'react'
import { useAuth } from './auth-provider'

interface Message {
  id: number
  role: 'user' | 'assistant'
  text: string
}

export function AiAssistant() {
  const { api } = useAuth()
  const nextId = useRef(1)
  const [question, setQuestion] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [unavailable, setUnavailable] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const text = question.trim()
    if (!text || pending || unavailable) return

    setMessages((current) => [...current, { id: nextId.current++, role: 'user', text }])
    setQuestion('')
    setError(null)
    setPending(true)

    try {
      const answer = await api.queryAi(text)
      setMessages((current) => [
        ...current,
        {
          id: nextId.current++,
          role: 'assistant',
          text: answer.answer?.trim() || 'El asistente no devolvió una respuesta.',
        },
      ])
    } catch (reason) {
      if (reason instanceof ApiError && reason.isNotFound) {
        setUnavailable(true)
        setMessages((current) => [
          ...current,
          {
            id: nextId.current++,
            role: 'assistant',
            text: 'El asistente operativo estará disponible en la fase F5.',
          },
        ])
      } else {
        const apiError = toApiError(reason)
        setError(
          apiError.isForbidden
            ? 'Tu rol no puede consultar el asistente.'
            : apiError.message || 'No se pudo consultar al asistente.',
        )
      }
    } finally {
      setPending(false)
    }
  }

  return (
    <section className="tt-card tt-ai" aria-labelledby="ai-title">
      <div className="tt-section__head">
        <div>
          <h2 className="tt-section__title" id="ai-title">
            Asistente operativo
          </h2>
          <p className="tt-section__hint">
            Consultas en lenguaje natural sobre tu flota y tus órdenes. Respuestas sujetas a tu ámbito de rol.
          </p>
        </div>
        <span className="tt-badge tt-badge--neutral">F5</span>
      </div>

      {messages.length === 0 ? (
        <p className="tt-ai__empty">Aún no hay consultas. Ejemplo: “¿qué vehículos llevan más de 10 minutos detenidos?”</p>
      ) : (
        <ul className="tt-ai__messages" role="log" aria-live="polite" aria-relevant="additions text">
          {messages.map((message) => (
            <li
              key={message.id}
              className={`tt-ai__message${message.role === 'user' ? ' tt-ai__message--user' : ''}`}
            >
              <p>
                <span className="tt-visually-hidden">{message.role === 'user' ? 'Tú: ' : 'Asistente: '}</span>
                {message.text}
              </p>
            </li>
          ))}
        </ul>
      )}

      {error ? (
        <p className="tt-form__error" role="alert">
          {error}
        </p>
      ) : null}

      <form className="tt-form" onSubmit={handleSubmit}>
        <div className="tt-field">
          <label className="tt-field__label" htmlFor="ai-question">
            Tu consulta
          </label>
          <input
            id="ai-question"
            className="tt-input"
            type="text"
            value={question}
            disabled={unavailable}
            placeholder={unavailable ? 'Disponible en F5' : 'Escribe una pregunta sobre tu operación'}
            onChange={(event) => setQuestion(event.target.value)}
          />
        </div>
        <div className="tt-form__actions">
          <button className="tt-button" type="submit" disabled={pending || unavailable || !question.trim()}>
            {pending ? 'Consultando…' : 'Preguntar'}
          </button>
          {pending ? (
            <span className="tt-muted" role="status">
              Esperando respuesta…
            </span>
          ) : null}
        </div>
      </form>
    </section>
  )
}
