import { describe, expect, it } from 'vitest'

import { AppError } from '../../domain/errors'
import {
  FakeAuthApi,
  FakeClock,
  MemorySessionRepository,
  makeUser,
  silentLogger,
} from '../../testing/fakes'
import { SessionService } from './session-service'

const NOW = 1_700_000_000_000

function createHarness(options: { refreshError?: AppError } = {}) {
  const clock = new FakeClock(NOW)
  const repository = new MemorySessionRepository()
  const api = new FakeAuthApi({ refreshError: options.refreshError })
  const service = new SessionService({ repository, api, clock, logger: silentLogger })
  return { clock, repository, api, service }
}

describe('SessionService', () => {
  it('inicia sesión, persiste los tokens y publica la instantánea', async () => {
    const { service, repository } = createHarness()
    const snapshots: string[] = []
    service.subscribe((snapshot) => snapshots.push(snapshot.status))

    const user = await service.login('conductor1@tiendas.dev', 'Conductor123')

    expect(user.role).toBe('DRIVER')
    expect(service.currentUserId()).toBe(user.id)
    expect(repository.stored?.accessToken).toBe('access-1')
    expect(repository.stored?.refreshToken).toBe('refresh-1')
    expect(service.getSnapshot()).toMatchObject({ status: 'authenticated', user })
    expect(snapshots).toEqual(['authenticated'])
  })

  it('rechaza cuentas que no son de conductor sin guardar sesión', async () => {
    const { service, repository, api } = createHarness()
    api.setLoginError(null)
    const storeUser = makeUser({ role: 'STORE' })
    const storeApi = new FakeAuthApi({ user: storeUser })
    const restricted = new SessionService({
      repository,
      api: storeApi,
      clock: new FakeClock(NOW),
      logger: silentLogger,
    })

    await expect(restricted.login('tienda1@tiendas.dev', 'Tienda12345')).rejects.toMatchObject({
      code: 'FORBIDDEN',
    })
    expect(repository.stored).toBeNull()
    expect(restricted.getSnapshot().status).toBe('unknown')
    expect(service.getSnapshot().status).toBe('unknown')
    // El refresh emitido se revoca aunque la app no publique la sesión.
    expect(storeApi.logoutCalls).toBe(1)
  })

  it('consulta el perfil con el token recién emitido si el contrato lo omite', async () => {
    const clock = new FakeClock(NOW)
    const repository = new MemorySessionRepository()
    const api = new FakeAuthApi({ omitUserInPair: true })
    const service = new SessionService({ repository, api, clock, logger: silentLogger })

    const user = await service.login('conductor1@tiendas.dev', 'Conductor123')

    expect(user.role).toBe('DRIVER')
    expect(api.meCalls).toBe(1)
    expect(api.meAccessTokens).toEqual(['access-1'])
    expect(repository.stored?.user).toEqual(user)
  })

  it('devuelve el access token vigente sin refrescar', async () => {
    const { service, api } = createHarness()
    await service.login('conductor1@tiendas.dev', 'Conductor123')

    await expect(service.ensureAccessToken()).resolves.toBe('access-1')
    expect(api.refreshCalls).toBe(0)
  })

  it('refresca una sola vez aunque haya peticiones concurrentes', async () => {
    const { service, api, clock } = createHarness()
    await service.login('conductor1@tiendas.dev', 'Conductor123')
    clock.advance(901_000)

    const tokens = await Promise.all([
      service.ensureAccessToken(),
      service.ensureAccessToken(),
      service.ensureAccessToken(),
    ])

    expect(api.refreshCalls).toBe(1)
    expect(tokens).toEqual(['access-2', 'access-2', 'access-2'])
  })

  it('mantiene la sesión si el refresh falla por red (modo offline)', async () => {
    const { service, clock, repository } = createHarness({
      refreshError: new AppError('NETWORK', 'sin conexión'),
    })
    await service.login('conductor1@tiendas.dev', 'Conductor123')
    clock.advance(901_000)

    await expect(service.ensureAccessToken()).rejects.toMatchObject({ code: 'NETWORK' })

    expect(service.getSnapshot().status).toBe('authenticated')
    expect(service.currentUserId()).not.toBeNull()
    expect(repository.stored).not.toBeNull()
  })

  it('cierra la sesión si el refresh token fue revocado', async () => {
    const { service, clock, repository, api } = createHarness({
      refreshError: new AppError('UNAUTHENTICATED', 'refresh revocado', { status: 401 }),
    })
    await service.login('conductor1@tiendas.dev', 'Conductor123')
    clock.advance(901_000)

    await expect(service.ensureAccessToken()).resolves.toBeNull()

    expect(api.refreshCalls).toBe(1)
    expect(service.getSnapshot().status).toBe('expired')
    expect(service.currentUserId()).toBeNull()
    expect(repository.stored).toBeNull()
  })

  it('hidrata una sesión válida desde el almacenamiento seguro', async () => {
    const { service, repository } = createHarness()
    const user = makeUser()
    repository.stored = {
      accessToken: 'access-x',
      refreshToken: 'refresh-x',
      accessExpiresAt: NOW + 60_000,
      user,
    }

    await service.hydrate()

    expect(service.getSnapshot()).toMatchObject({ status: 'authenticated', user })
    await expect(service.ensureAccessToken()).resolves.toBe('access-x')
  })

  it('permite uso offline con una sesión expirada persistida', async () => {
    const { service, repository } = createHarness({
      refreshError: new AppError('NETWORK', 'sin conexión'),
    })
    const user = makeUser()
    repository.stored = {
      accessToken: 'access-old',
      refreshToken: 'refresh-old',
      accessExpiresAt: NOW - 5_000,
      user,
    }

    await service.hydrate()

    expect(service.getSnapshot()).toMatchObject({ status: 'authenticated', user })
  })

  it('limpia la sesión local aunque el servidor no responda al logout', async () => {
    const { service, repository, api } = createHarness()
    await service.login('conductor1@tiendas.dev', 'Conductor123')
    api.logout = async () => {
      throw new AppError('NETWORK', 'sin red')
    }

    await service.logout()

    expect(service.getSnapshot().status).toBe('anonymous')
    expect(repository.stored).toBeNull()
  })

  it('descarta una sesión persistida corrupta', async () => {
    const { service, repository } = createHarness()
    repository.read = async () => {
      throw new AppError('UNKNOWN', 'corrupto')
    }

    await service.hydrate()

    expect(service.getSnapshot().status).toBe('anonymous')
  })
})
