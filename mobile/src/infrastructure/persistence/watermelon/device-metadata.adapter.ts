import { Q } from '@nozbe/watermelondb'
import type Database from '@nozbe/watermelondb/Database'

import type { DeviceMetadataPort } from '../../../ports/device'
import { newLocalId } from '../../../shared/ids'
import { AppMetaModel } from './models'

const DEVICE_ID_KEY = 'device_id'
const DEVICE_SEQ_KEY = 'device_seq'

/**
 * Identidad estable del dispositivo y secuencia monótona.
 *
 * La secuencia se serializa en una cadena de promesas: dos capturas
 * concurrentes (foreground + background task) no pueden leer el mismo valor.
 */
export class WatermelonDeviceMetadata implements DeviceMetadataPort {
  private readonly meta
  private deviceIdPromise: Promise<string> | null = null
  private sequenceTail: Promise<unknown> = Promise.resolve()

  constructor(private readonly database: Database) {
    this.meta = database.get<AppMetaModel>('app_meta')
  }

  getDeviceId(): Promise<string> {
    if (!this.deviceIdPromise) {
      this.deviceIdPromise = this.loadOrCreateDeviceId()
    }
    return this.deviceIdPromise
  }

  nextDeviceSequence(): Promise<number> {
    const run = this.sequenceTail.then(async () => {
      const current = await this.readValue(DEVICE_SEQ_KEY)
      const parsed = current ? Number.parseInt(current, 10) : Number.NaN
      const next = Number.isFinite(parsed) && parsed >= 0 ? parsed + 1 : 1
      await this.writeValue(DEVICE_SEQ_KEY, String(next))
      return next
    })
    // La cola sigue funcionando aunque una escritura falle.
    this.sequenceTail = run.catch(() => undefined)
    return run
  }

  private async loadOrCreateDeviceId(): Promise<string> {
    const existing = await this.readValue(DEVICE_ID_KEY)
    if (existing && existing.length > 0) return existing
    const generated = newLocalId('dev_')
    await this.writeValue(DEVICE_ID_KEY, generated)
    return generated
  }

  private async readValue(key: string): Promise<string | null> {
    const rows = await this.meta.query(Q.where('id', key), Q.take(1)).fetch()
    return rows[0]?.value ?? null
  }

  private async writeValue(key: string, value: string): Promise<void> {
    await this.database.write(async () => {
      const rows = await this.meta.query(Q.where('id', key), Q.take(1)).fetch()
      const existing = rows[0]
      if (existing) {
        await existing.update((record) => {
          record.value = value
        })
        return
      }
      await this.meta.create((record) => {
        record._raw.id = key
        record.value = value
      })
    })
  }
}
