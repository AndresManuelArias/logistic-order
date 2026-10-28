import { EMPTY_SYNC_STATUS, type SyncStatusSnapshot } from '../../domain/sync'

type Listener = (snapshot: SyncStatusSnapshot) => void

/**
 * Estado observable de sincronización. Mantiene una instantánea inmutable para
 * poder consumirse con `useSyncExternalStore` sin re-renderizados espurios.
 */
export class SyncStatusStore {
  private snapshot: SyncStatusSnapshot
  private readonly listeners = new Set<Listener>()

  constructor(initial: SyncStatusSnapshot = EMPTY_SYNC_STATUS) {
    this.snapshot = { ...initial }
  }

  readonly getSnapshot = (): SyncStatusSnapshot => this.snapshot

  readonly subscribe = (listener: Listener): (() => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  update(patch: Partial<SyncStatusSnapshot>): void {
    const keys = Object.keys(patch) as (keyof SyncStatusSnapshot)[]
    const changed = keys.some((key) => this.snapshot[key] !== patch[key])
    if (!changed) return

    this.snapshot = { ...this.snapshot, ...patch }
    for (const listener of this.listeners) listener(this.snapshot)
  }

  reset(): void {
    this.update({ ...EMPTY_SYNC_STATUS })
  }
}
