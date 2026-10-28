import { describe, expect, it, vi } from 'vitest'

import { SyncStatusStore } from './sync-status-store'

describe('SyncStatusStore', () => {
  it('notifica cambios y expone una instantánea estable', () => {
    const store = new SyncStatusStore()
    const listener = vi.fn()
    const unsubscribe = store.subscribe(listener)

    const before = store.getSnapshot()
    store.update({ pendingPoints: 3, online: true })
    const after = store.getSnapshot()

    expect(after).not.toBe(before)
    expect(after.pendingPoints).toBe(3)
    expect(after.online).toBe(true)
    expect(listener).toHaveBeenCalledTimes(1)
    expect(listener).toHaveBeenCalledWith(after)

    unsubscribe()
    store.update({ syncing: true })
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('no notifica si el parche no cambia nada', () => {
    const store = new SyncStatusStore()
    const listener = vi.fn()
    store.subscribe(listener)

    store.update({ pendingPoints: 0 })
    expect(listener).not.toHaveBeenCalled()

    store.update({ pendingPoints: 1 })
    store.update({ pendingPoints: 1 })
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('reset vuelve al estado vacío', () => {
    const store = new SyncStatusStore()
    store.update({ pendingPoints: 9, lastError: 'boom', sessionExpired: true })
    store.reset()
    expect(store.getSnapshot()).toMatchObject({
      pendingPoints: 0,
      lastError: null,
      sessionExpired: false,
    })
  })
})
