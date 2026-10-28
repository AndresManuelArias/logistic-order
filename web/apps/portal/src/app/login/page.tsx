'use client'

import { homeForRole, toApiError } from '@tiendas/tracking-core'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { Suspense, useEffect, useState, type FormEvent } from 'react'
import { useAuth } from '@/components/auth-provider'
import { LoadingPanel } from '@tiendas/tracking-ui'

function loginMessage(error: unknown): string {
  const api = toApiError(error)
  if (api.status === 401) return 'Correo o contraseña incorrectos.'
  if (api.status === 400 || api.status === 422) return 'Revisa el correo y la contraseña.'
  if (api.status === 429) return 'Demasiados intentos. Espera un momento antes de reintentar.'
  if (api.status === 502) return 'El servidor no está disponible. Inténtalo en unos segundos.'
  if (api.isNetwork) return 'No pudimos conectar con el servidor. Revisa tu conexión.'
  return api.message || 'No se pudo iniciar sesión.'
}

function LoginForm() {
  const { status, user, login } = useAuth()
  const router = useRouter()
  const searchParams = useSearchParams()
  const next = searchParams.get('next')
  const expired = searchParams.get('expired') === '1'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)

  // Con sesión activa no tiene sentido quedarse en el login: se respeta `next`
  // solo si pertenece al panel del rol (evita saltos entre roles).
  useEffect(() => {
    if (status !== 'authenticated' || !user) return
    const home = homeForRole(user.role)
    const target = next && (next === home || next.startsWith(`${home}/`)) ? next : home
    router.replace(target)
  }, [status, user, next, router])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    setPending(true)
    setError(null)
    try {
      await login(email.trim(), password)
      // La redirección la hace el efecto al cambiar el estado de sesión.
    } catch (reason) {
      setError(loginMessage(reason))
      setPending(false)
    }
  }

  if (status === 'loading') {
    return (
      <div className="tt-auth">
        <div className="tt-auth__card">
          <LoadingPanel label="Comprobando sesión…" />
        </div>
      </div>
    )
  }

  return (
    <div className="tt-auth">
      <div className="tt-auth__card">
        <Link className="tt-auth__brand" href="/">
          <span className="tt-brand__mark" aria-hidden="true">
            TT
          </span>
          Tiendas-Track
        </Link>

        <div>
          <h1 className="tt-auth__title">Iniciar sesión</h1>
          <p className="tt-auth__subtitle">
            Accede al panel de tu rol: flota del proveedor, vista dual de la tienda o entregas del conductor.
          </p>
        </div>

        {expired ? (
          <p className="tt-form__notice" role="status">
            Tu sesión caducó por seguridad. Vuelve a iniciar sesión.
          </p>
        ) : null}

        <form className="tt-form" onSubmit={handleSubmit} noValidate>
          <div className="tt-field">
            <label className="tt-field__label" htmlFor="login-email">
              Correo electrónico
            </label>
            <input
              id="login-email"
              className="tt-input"
              type="email"
              name="email"
              autoComplete="username"
              inputMode="email"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              aria-invalid={error ? true : undefined}
            />
          </div>

          <div className="tt-field">
            <label className="tt-field__label" htmlFor="login-password">
              Contraseña
            </label>
            <input
              id="login-password"
              className="tt-input"
              type="password"
              name="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              aria-invalid={error ? true : undefined}
            />
          </div>

          {error ? (
            <p className="tt-form__error" role="alert">
              {error}
            </p>
          ) : null}

          <button className="tt-button" type="submit" disabled={pending || !email || !password}>
            {pending ? 'Entrando…' : 'Entrar'}
          </button>
        </form>

        <p className="tt-auth__foot">
          ¿Eres cliente final? El seguimiento llega por un enlace de invitado de tu tienda.
        </p>
      </div>
    </div>
  )
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <div className="tt-auth">
          <div className="tt-auth__card">
            <LoadingPanel />
          </div>
        </div>
      }
    >
      <LoginForm />
    </Suspense>
  )
}
