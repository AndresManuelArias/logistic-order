import type { GpsPolicy } from '../../config/policies'
import { normalizeSample, validateSample, type GpsSampleInput, type QueuedPoint } from '../../domain/point'
import type { Clock } from '../../ports/clock'
import type { DeviceMetadataPort } from '../../ports/device'
import type { PointQueueStore } from '../../ports/telemetry'
import { newLocalId } from '../../shared/ids'

export interface RecordGpsSampleDeps {
  points: PointQueueStore
  device: DeviceMetadataPort
  clock: Clock
  policy: GpsPolicy
}

export type CaptureOutcome =
  | { stored: true; point: QueuedPoint }
  | { stored: false; code: string; message: string }

/**
 * Caso de uso de captura: valida, asigna `device_seq` y **persiste antes de
 * cualquier intento de red**. La sincronización es responsabilidad del
 * coordinador, de modo que una captura nunca se pierde por falta de conexión.
 */
export class RecordGpsSample {
  constructor(private readonly deps: RecordGpsSampleDeps) {}

  async execute(ownerId: string, sample: GpsSampleInput): Promise<CaptureOutcome> {
    const now = this.deps.clock.now()
    const validation = validateSample(sample, {
      now,
      maxAccuracyMeters: this.deps.policy.maxAccuracyMeters,
    })
    if (!validation.ok) {
      return { stored: false, code: validation.code, message: validation.message }
    }

    const normalized = normalizeSample(sample)
    const deviceSeq = await this.deps.device.nextDeviceSequence()
    const point: QueuedPoint = {
      id: newLocalId('pt_'),
      ownerId,
      deviceSeq,
      ...normalized,
    }

    await this.deps.points.enqueue(point, now)
    return { stored: true, point }
  }
}
