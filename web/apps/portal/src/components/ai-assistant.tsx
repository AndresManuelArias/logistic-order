'use client'

// Panel del asistente operativo (F5). El backend expone POST /api/v1/ai/query
// con un catálogo cerrado de herramientas; el portal muestra la respuesta y las
// herramientas consultadas, y degrada con un mensaje claro cuando el agente no
// está configurado o el proveedor no responde. Nunca inventa respuestas.
import { toApiError, type AiToolInvocation } from '@tiendas/tracking-core'
import { useRef, useState, type FormEvent } from 'react'
import { useAuth } from './auth-provider'

interface Message {
  id: number
  role: 'user' | 'assistant'
  text: string
  tools?: AiToolInvocation[]
  model?: string
}

// Etiquetas legibles del catálogo de herramientas del backend.
const TOOL_LABEL: Record<string, string> = {
  get_stopped_vehicles: 'Vehículos detenidos',
  get_order_status: 'Estado de orden',
  get_fleet_eta: 'ETA de flota',
  get_late_deliveries: 'Entregas vencidas',
}

function toolLabel(name: string): string {
  return TOOL_LABEL[name] ?? name
}

// Traduce un fallo del endpoint a un mensaje accionable para el usuario.
// `configured=false` deshabilita el formulario: reintentar no sirve mientras
// falte AI_API_KEY en el entorno.
function describeError(reason: unknown): { message: string; configured: boolean } {
  const error = toApiError(reason)

  if (error.isForbidden) {
    return { message: 'Tu rol no puede consultar el asistente.', configured: true }
  }
  if (error.isNetwork) {
    return { message: 'No se pudo contactar con el servidor. Revisa tu conexión.', configured: true }
  }

  switch (error.code) {
    case 'AI_NOT_CONFIGURED':
      return {
        message:
          'El asistente no está configurado en este entorno (falta AI_API_KEY). El resto de la plataforma sigue operativo.',
        configured: false,
      }
    case 'CIRCUIT_OPEN':
      return {
        message: 'El proveedor del asistente no está disponible ahora. Reintenta en unos segundos.',
        configured: true,
      }
    case 'AI_TIMEOUT':
      return { message: 'El asistente tardó demasiado en responder. Intenta de nuevo.', configured: true }
    case 'AI_PROVIDER_ERROR':
    case 'AI_INVALID_RESPONSE':
      return { message: 'El asistente no pudo completar la consulta. Intenta de nuevo.', configured: true }
    case 'INVALID_QUESTION':
    case 'INVALID_BODY':
      return { message: 'La consulta no es válida. Escribe una pregunta más corta y concreta.', configured: true }
    default:
      return {
        message: error.message || 'No se pudo consultar al asistente.',
        configured: true,
      }
  }
}

export function AiAssistant() {
  const { api } = useAuth()
  const nextId = useRef(1)
  const [question, setQuestion] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [configured, setConfigured] = useState(true)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const text = question.trim()
    if (!text || pending || !configured) return

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
          tools: answer.tools,
          model: answer.model,
        },
      ])
    } catch (reason) {
      const described = describeError(reason)
      if (!described.configured) setConfigured(false)
      setError(described.message)
    } finally {
      setPending(false)
    }
  }

  const canAsk = configured && !pending && question.trim().length > 0

  return (
    <section className="tt-card tt-ai" aria-labelledby="ai-title">
      <div className="tt-section__head">
        <div>
          <h2 className="tt-section__title" id="ai-title">
            Asistente operativo
          </h2>
          <p className="tt-section__hint">
            Consultas en lenguaje natural sobre tu flota y tus órdenes. Respuestas limitadas a tu ámbito de rol y
            basadas solo en datos del sistema.
          </p>
        </div>
        <span className={`tt-badge ${configured ? 'tt-badge--neutral' : 'tt-badge--danger'}`}>
          {configured ? 'Solo lectura' : 'No configurado'}
        </span>
      </div>

      {messages.length === 0 ? (
        <p className="tt-ai__empty">
          Aún no hay consultas. Ejemplo: “¿qué vehículos llevan más de 20 minutos detenidos?”
        </p>
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
              {message.tools?.length ? (
                <p className="tt-ai__tools">
                  <span className="tt-visually-hidden">Herramientas consultadas: </span>
                  {message.tools.map((tool, index) => (
                    <span
                      key={`${tool.name}-${index}`}
                      className={`tt-badge ${tool.ok ? 'tt-badge--neutral' : 'tt-badge--danger'}`}
                      title={
                        tool.ok
                          ? `${toolLabel(tool.name)} · ${tool.duration_ms} ms`
                          : `${toolLabel(tool.name)}: ${tool.error ?? 'error'}`
                      }
                    >
                      {toolLabel(tool.name)}
                      {tool.ok ? '' : ' · error'}
                    </span>
                  ))}
                </p>
              ) : null}
              {message.role === 'assistant' && message.model ? (
                <p className="tt-ai__meta">Modelo: {message.model}</p>
              ) : null}
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
            maxLength={500}
            disabled={!configured}
            placeholder={
              configured ? 'Ej. ¿Qué vehículos llevan más de 20 minutos detenidos?' : 'Asistente no configurado'
            }
            onChange={(event) => setQuestion(event.target.value)}
          />
        </div>
        <div className="tt-form__actions">
          <button className="tt-button" type="submit" disabled={!canAsk}>
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
