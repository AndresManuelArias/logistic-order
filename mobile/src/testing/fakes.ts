import type { Order, OrderPage, OrderStatus, TokenPair, User } from '../contracts/types'
import { AppError } from '../domain/errors'
import { canTransition } from '../domain/order'
import type { QueuedPoint } from '../domain/point'
import type { PendingBatch, PendingTransition } from '../domain/sync'
import type { AuthApi, SessionRepository, StoredSession } from '../ports/auth'
import type { Clock, Logger } from '../ports/clock'
import type { ConnectivityListener, ConnectivityPort } from '../ports/connectivity'
import type { DeviceMetadataPort } from '../ports/device'
import type {
  LocationTrackingState,
  LocationTrackerPort,
  RawLocationSample,
} from '../ports/location'
import type { CachedOrder, OrderCachePort, OrderTransport } from '../ports/orders'
import type { DeliveryProofRecord, DeliveryProofStorePort, ProofFileStorePort } from '../ports/proofs'
import type { BulkSendInput, BulkSendResult, PointQueueStore, TelemetryTransport } from '../ports/telemetry'
import type {
  EnqueueTransitionInput,
  EnqueueTransitionResult,
  TransitionQueuePort,
} from '../ports/transitions'

export const silentLogger: Logger = {
  debug: () => undefined,
  info: () => undefined,
  warn: () => undefined,
  error: () => undefined,
}

export class FakeClock implements Clock {
  constructor(private current = 1_700_000_000_000) {}
  now(): number {
    return this.current
  }
  advance(ms: number): void {
    this.current += ms
  }
  set(ms: number): void {
    this.current = ms
  }
}

export class FakeConnectivity implements ConnectivityPort {
  private readonly listeners = new Set<ConnectivityListener>()

  constructor(public online = true) {}

  async isOnline(): Promise<boolean> {
    return this.online
  }

  subscribe(listener: ConnectivityListener): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  setOnline(online: boolean): void {
    this.online = online
    for (const listener of this.listeners) listener(online)
  }
}

export class FakeDeviceMetadata implements DeviceMetadataPort {
  private seq = 0

  constructor(public deviceId = 'dev-test') {}

  async getDeviceId(): Promise<string> {
    return this.deviceId
  }

  async nextDeviceSequence(): Promise<number> {
    this.seq += 1
    return this.seq
  }

  get currentSequence(): number {
    return this.seq
  }
}

interface StoredPoint {
  point: QueuedPoint
  state: 'pending' | 'batched' | 'synced' | 'rejected'
  batchId: string | null
  attempts: number
  lastError: string | null
}

interface StoredBatch extends PendingBatch {
  state: 'open' | 'closed' | 'rejected'
}

export class MemoryPointQueue implements PointQueueStore {
  private readonly points = new Map<string, StoredPoint>()
  private readonly batches = new Map<string, StoredBatch>()

  async enqueue(point: QueuedPoint): Promise<void> {
    this.points.set(point.id, {
      point,
      state: 'pending',
      batchId: null,
      attempts: 0,
      lastError: null,
    })
  }

  async pendingPoints(ownerId: string, limit: number): Promise<QueuedPoint[]> {
    return [...this.points.values()]
      .filter((entry) => entry.point.ownerId === ownerId && entry.state === 'pending')
      .sort((a, b) => a.point.deviceSeq - b.point.deviceSeq)
      .slice(0, limit)
      .map((entry) => entry.point)
  }

  async pointsByIds(ownerId: string, ids: readonly string[]): Promise<QueuedPoint[]> {
    return ids
      .map((id) => this.points.get(id))
      .filter(
        (entry): entry is StoredPoint => entry !== undefined && entry.point.ownerId === ownerId,
      )
      .map((entry) => entry.point)
  }

  async openBatch(ownerId: string): Promise<PendingBatch | null> {
    const open = [...this.batches.values()]
      .filter((batch) => batch.ownerId === ownerId && batch.state === 'open')
      .sort((a, b) => a.createdAt - b.createdAt)
    return open[0] ?? null
  }

  async createBatch(batch: PendingBatch): Promise<void> {
    this.batches.set(batch.id, { ...batch, state: 'open' })
    for (const id of batch.pointIds) {
      const entry = this.points.get(id)
      if (entry) {
        entry.state = 'batched'
        entry.batchId = batch.id
      }
    }
  }

  async markBatchSynced(ownerId: string, batchId: string): Promise<void> {
    const batch = this.batches.get(batchId)
    if (!batch || batch.ownerId !== ownerId) return
    batch.state = 'closed'
    for (const id of batch.pointIds) {
      const entry = this.points.get(id)
      if (entry) {
        entry.state = 'synced'
        entry.lastError = null
      }
    }
  }

  async markBatchRetry(
    ownerId: string,
    batchId: string,
    nextAttemptAt: number,
    error: string,
  ): Promise<void> {
    const batch = this.batches.get(batchId)
    if (!batch || batch.ownerId !== ownerId) return
    batch.attempts += 1
    batch.nextAttemptAt = nextAttemptAt
    batch.lastError = error
  }

  async rejectBatch(
    ownerId: string,
    batchId: string,
    pointIds: readonly string[],
    code: string,
  ): Promise<void> {
    const batch = this.batches.get(batchId)
    if (batch && batch.ownerId === ownerId) batch.state = 'rejected'
    for (const id of pointIds) {
      const entry = this.points.get(id)
      if (entry) {
        entry.state = 'rejected'
        entry.lastError = code
      }
    }
  }

  async counts(ownerId: string): Promise<{ pending: number; rejected: number }> {
    let pending = 0
    let rejected = 0
    for (const entry of this.points.values()) {
      if (entry.point.ownerId !== ownerId) continue
      if (entry.state === 'pending' || entry.state === 'batched') pending += 1
      if (entry.state === 'rejected') rejected += 1
    }
    return { pending, rejected }
  }

  // Utilidades de prueba
  all(): QueuedPoint[] {
    return [...this.points.values()].map((entry) => entry.point)
  }

  stateOf(pointId: string): StoredPoint['state'] | undefined {
    return this.points.get(pointId)?.state
  }

  batchList(): StoredBatch[] {
    return [...this.batches.values()]
  }

  batchesFor(ownerId: string): StoredBatch[] {
    return this.batchList().filter((batch) => batch.ownerId === ownerId)
  }
}

/**
 * Doble del backend de telemetría: implementa la deduplicación real
 * (`Idempotency-Key` + `(time, device_seq)`) para probar el criterio de
 * aceptación sin servidor.
 */
export class FakeTelemetryServer implements TelemetryTransport {
  readonly requests: BulkSendInput[] = []
  private readonly receipts = new Map<string, BulkSendResult>()
  private readonly stored = new Set<string>()
  private failure: { error: AppError; applyFirst: boolean } | null = null

  constructor(private readonly clock: Clock) {}

  failNext(error: AppError, applyFirst = false): void {
    this.failure = { error, applyFirst }
  }

  async sendBulk(input: BulkSendInput): Promise<BulkSendResult> {
    this.requests.push(input)

    if (this.failure) {
      const { error, applyFirst } = this.failure
      this.failure = null
      if (applyFirst) this.apply(input)
      throw error
    }

    const previous = this.receipts.get(input.idempotencyKey)
    if (previous) return { ...previous, alreadyProcessed: true }

    return this.apply(input)
  }

  private apply(input: BulkSendInput): BulkSendResult {
    let accepted = 0
    let duplicates = 0

    for (const point of input.points) {
      const key = `${point.vehicle_id ?? 'self'}|${point.time}|${point.device_seq ?? -1}`
      if (this.stored.has(key)) {
        duplicates += 1
        continue
      }
      this.stored.add(key)
      accepted += 1
    }

    const result: BulkSendResult = {
      accepted,
      duplicates,
      alreadyProcessed: false,
      processedAt: new Date(this.clock.now()).toISOString(),
    }
    this.receipts.set(input.idempotencyKey, result)
    return result
  }

  get storedCount(): number {
    return this.stored.size
  }

  get receiptCount(): number {
    return this.receipts.size
  }
}

export class MemoryOrderCache implements OrderCachePort {
  private readonly entries = new Map<string, CachedOrder>()

  private key(ownerId: string, orderId: string): string {
    return `${ownerId}:${orderId}`
  }

  async list(ownerId: string): Promise<CachedOrder[]> {
    return [...this.entries.entries()]
      .filter(([key]) => key.startsWith(`${ownerId}:`))
      .map(([, value]) => value)
  }

  async get(ownerId: string, orderId: string): Promise<CachedOrder | null> {
    return this.entries.get(this.key(ownerId, orderId)) ?? null
  }

  async upsertMany(ownerId: string, orders: readonly Order[], at: number): Promise<void> {
    for (const order of orders) {
      this.entries.set(this.key(ownerId, order.id), { order: structuredClone(order), cachedAt: at })
    }
  }

  async upsert(ownerId: string, order: Order, at: number): Promise<void> {
    this.entries.set(this.key(ownerId, order.id), { order: structuredClone(order), cachedAt: at })
  }
}

export class FakeOrderServer implements OrderTransport {
  readonly orders = new Map<string, Order>()
  readonly calls: string[] = []
  private failure: AppError | null = null

  constructor(initial: Order[] = []) {
    for (const order of initial) this.orders.set(order.id, structuredClone(order))
  }

  failNext(error: AppError): void {
    this.failure = error
  }

  seed(order: Order): void {
    this.orders.set(order.id, structuredClone(order))
  }

  private consumeFailure(): void {
    if (!this.failure) return
    const error = this.failure
    this.failure = null
    throw error
  }

  async listOrders(params?: { limit?: number; offset?: number }): Promise<OrderPage> {
    this.calls.push('list')
    this.consumeFailure()
    const items = [...this.orders.values()].map((order) => structuredClone(order))
    const offset = params?.offset ?? 0
    const limit = params?.limit ?? 100
    return { items: items.slice(offset, offset + limit), total: items.length }
  }

  async getOrder(orderId: string): Promise<Order> {
    this.calls.push(`get:${orderId}`)
    this.consumeFailure()
    const order = this.orders.get(orderId)
    if (!order) throw new AppError('NOT_FOUND', 'Orden no encontrada', { status: 404 })
    return structuredClone(order)
  }

  async changeOrderStatus(orderId: string, status: OrderStatus): Promise<Order> {
    this.calls.push(`patch:${orderId}:${status}`)
    this.consumeFailure()
    const order = this.orders.get(orderId)
    if (!order) throw new AppError('NOT_FOUND', 'Orden no encontrada', { status: 404 })
    if (!canTransition(order.status, status)) {
      throw new AppError('CONFLICT', `Transición ilegal ${order.status} → ${status}`, { status: 409 })
    }
    order.status = status
    this.orders.set(orderId, order)
    return structuredClone(order)
  }
}

interface StoredTransition extends PendingTransition {
  state: 'pending' | 'applied' | 'rejected'
  resolutionCode: string | null
}

export class MemoryTransitionQueue implements TransitionQueuePort {
  private readonly transitions = new Map<string, StoredTransition>()

  async enqueue(
    ownerId: string,
    input: EnqueueTransitionInput,
    at: number,
  ): Promise<EnqueueTransitionResult> {
    const existing = [...this.transitions.values()].find(
      (transition) =>
        transition.ownerId === ownerId &&
        transition.orderId === input.orderId &&
        transition.state === 'pending' &&
        transition.targetStatus === input.targetStatus,
    )
    if (existing) return { transition: { ...existing }, deduped: true }

    const transition: StoredTransition = {
      id: input.id,
      ownerId,
      orderId: input.orderId,
      expectedStatus: input.expectedStatus,
      targetStatus: input.targetStatus,
      idempotencyKey: input.idempotencyKey,
      requestedAt: at,
      attempts: 0,
      nextAttemptAt: at,
      lastError: null,
      state: 'pending',
      resolutionCode: null,
    }
    this.transitions.set(transition.id, transition)
    return { transition: { ...transition }, deduped: false }
  }

  async next(ownerId: string, limit: number): Promise<PendingTransition[]> {
    return [...this.transitions.values()]
      .filter((transition) => transition.ownerId === ownerId && transition.state === 'pending')
      .sort((a, b) => a.requestedAt - b.requestedAt)
      .slice(0, limit)
      .map((transition) => ({ ...transition }))
  }

  async findPending(ownerId: string, orderId: string): Promise<PendingTransition | null> {
    const pending = [...this.transitions.values()]
      .filter(
        (transition) =>
          transition.ownerId === ownerId &&
          transition.orderId === orderId &&
          transition.state === 'pending',
      )
      .sort((a, b) => b.requestedAt - a.requestedAt)
    return pending[0] ? { ...pending[0] } : null
  }

  async listPending(ownerId: string): Promise<PendingTransition[]> {
    return [...this.transitions.values()]
      .filter((transition) => transition.ownerId === ownerId && transition.state === 'pending')
      .sort((a, b) => a.requestedAt - b.requestedAt)
      .map((transition) => ({ ...transition }))
  }

  async counts(ownerId: string): Promise<{ pending: number; failed: number }> {
    let pending = 0
    let failed = 0
    for (const transition of this.transitions.values()) {
      if (transition.ownerId !== ownerId) continue
      if (transition.state === 'pending') pending += 1
      if (transition.state === 'rejected') failed += 1
    }
    return { pending, failed }
  }

  async markApplied(ownerId: string, transitionId: string): Promise<void> {
    this.resolve(ownerId, transitionId, 'applied', 'APPLIED')
  }

  async markRejected(ownerId: string, transitionId: string, code: string): Promise<void> {
    this.resolve(ownerId, transitionId, 'rejected', code)
  }

  async markRetry(
    ownerId: string,
    transitionId: string,
    nextAttemptAt: number,
    error: string,
  ): Promise<void> {
    const transition = this.transitions.get(transitionId)
    if (!transition || transition.ownerId !== ownerId) return
    transition.attempts += 1
    transition.nextAttemptAt = nextAttemptAt
    transition.lastError = error
  }

  private resolve(
    ownerId: string,
    transitionId: string,
    state: 'applied' | 'rejected',
    code: string,
  ): void {
    const transition = this.transitions.get(transitionId)
    if (!transition || transition.ownerId !== ownerId) return
    transition.state = state
    transition.resolutionCode = code
  }

  // Utilidades de prueba
  all(): StoredTransition[] {
    return [...this.transitions.values()].map((transition) => ({ ...transition }))
  }

  stateOf(transitionId: string): StoredTransition['state'] | undefined {
    return this.transitions.get(transitionId)?.state
  }
}

export class MemoryProofStore implements DeliveryProofStorePort {
  readonly records: DeliveryProofRecord[] = []

  async save(record: DeliveryProofRecord): Promise<void> {
    this.records.push(record)
  }

  async listByOrder(ownerId: string, orderId: string): Promise<DeliveryProofRecord[]> {
    return this.records.filter(
      (record) => record.ownerId === ownerId && record.orderId === orderId,
    )
  }
}

export class FakeProofFileStore implements ProofFileStorePort {
  readonly calls: Array<{ proofId: string; sourceUri: string }> = []
  failure: AppError | null = null

  async persist(proofId: string, sourceUri: string): Promise<string> {
    if (this.failure) throw this.failure
    this.calls.push({ proofId, sourceUri })
    return `file:///documents/delivery-proofs/${proofId}.jpg`
  }
}

export class MemorySessionRepository implements SessionRepository {
  stored: StoredSession | null = null

  async read(): Promise<StoredSession | null> {
    return this.stored ? structuredClone(this.stored) : null
  }

  async write(session: StoredSession): Promise<void> {
    this.stored = structuredClone(session)
  }

  async clear(): Promise<void> {
    this.stored = null
  }
}

export interface FakeAuthOptions {
  user?: User
  loginError?: AppError
  refreshError?: AppError
  /** Simula un contrato que omite `user` en `TokenPair` (fuerza `me()`). */
  omitUserInPair?: boolean
}

export class FakeAuthApi implements AuthApi {
  loginCalls = 0
  refreshCalls = 0
  logoutCalls = 0
  meCalls = 0
  meAccessTokens: Array<string | undefined> = []
  lastLogoutToken: string | null = null
  private readonly user: User
  private readonly omitUserInPair: boolean
  private loginError: AppError | null
  private refreshError: AppError | null
  private tokenVersion = 0

  constructor(options: FakeAuthOptions = {}) {
    this.user = options.user ?? makeUser()
    this.omitUserInPair = options.omitUserInPair ?? false
    this.loginError = options.loginError ?? null
    this.refreshError = options.refreshError ?? null
  }

  setRefreshError(error: AppError | null): void {
    this.refreshError = error
  }

  setLoginError(error: AppError | null): void {
    this.loginError = error
  }

  async login(email: string, password: string): Promise<TokenPair> {
    this.loginCalls += 1
    if (this.loginError) throw this.loginError
    if (!email.includes('@') || password.length === 0) {
      throw new AppError('UNAUTHENTICATED', 'Credenciales inválidas', { status: 401 })
    }
    return this.pair()
  }

  async refresh(_refreshToken: string): Promise<TokenPair> {
    this.refreshCalls += 1
    if (this.refreshError) throw this.refreshError
    return this.pair()
  }

  async logout(refreshToken: string): Promise<void> {
    this.logoutCalls += 1
    this.lastLogoutToken = refreshToken
  }

  async me(accessToken?: string): Promise<User> {
    this.meCalls += 1
    this.meAccessTokens.push(accessToken)
    return this.user
  }

  private pair(): TokenPair {
    this.tokenVersion += 1
    return {
      access_token: `access-${this.tokenVersion}`,
      refresh_token: `refresh-${this.tokenVersion}`,
      expires_in: 900,
      token_type: 'Bearer',
      ...(this.omitUserInPair ? {} : { user: this.user }),
    }
  }
}

export class FakeLocationTracker implements LocationTrackerPort {
  mode: LocationTrackingState['mode'] = 'off'
  startCalls = 0
  stopCalls = 0

  constructor(private readonly onStart?: () => Promise<void>) {}

  async getState(): Promise<LocationTrackingState> {
    return {
      mode: this.mode,
      foreground: 'granted',
      background: this.mode === 'background' ? 'granted' : 'denied',
      message: 'fake',
    }
  }

  async start(): Promise<LocationTrackingState> {
    this.startCalls += 1
    if (this.onStart) await this.onStart()
    this.mode = 'background'
    return this.getState()
  }

  async stop(): Promise<LocationTrackingState> {
    this.stopCalls += 1
    this.mode = 'off'
    return this.getState()
  }

  async suspend(): Promise<void> {}
  async resume(): Promise<void> {}
}

export function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    email: 'conductor1@tiendas.dev',
    full_name: 'Carlos Conductor',
    role: 'DRIVER',
    ...overrides,
  } as User
}

export function makeOrder(overrides: Partial<Order> = {}): Order {
  return {
    id: '22222222-2222-4222-8222-222222222222',
    external_ref: 'PED-TEST-1',
    store_id: '33333333-3333-4333-8333-333333333333',
    status: 'CREATED',
    origin: { lat: 4.7108, lon: -74.0721 },
    destination: { lat: 4.6578, lon: -74.0561 },
    destination_address: 'Cra 7 # 72-40, Bogotá',
    distance_meters: 6200,
    placed_at: '2026-10-28T10:00:00.000Z',
    ...overrides,
  } as Order
}

export function makeSample(overrides: Partial<RawLocationSample> = {}): RawLocationSample {
  return {
    lat: 4.7108,
    lon: -74.0721,
    timestamp: 1_700_000_000_000,
    accuracyM: 12,
    speedMps: 8,
    heading: 90,
    ...overrides,
  }
}
