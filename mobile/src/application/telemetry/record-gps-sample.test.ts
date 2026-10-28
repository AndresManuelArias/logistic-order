import { describe, expect, it } from 'vitest'

import { GPS_POLICY } from '../../config/policies'
import { FakeClock, FakeDeviceMetadata, MemoryPointQueue } from '../../testing/fakes'
import { RecordGpsSample } from './record-gps-sample'

const OWNER = 'driver-1'
const NOW = 1_700_000_000_000

function createHarness() {
  const clock = new FakeClock(NOW)
  const points = new MemoryPointQueue()
  const device = new FakeDeviceMetadata('dev-test')
  const service = new RecordGpsSample({ points, device, clock, policy: GPS_POLICY })
  return { clock, points, device, service }
}

describe('RecordGpsSample', () => {
  it('persiste la muestra con secuencia de dispositivo y propietario', async () => {
    const { service, points, device } = createHarness()

    const result = await service.execute(OWNER, {
      lat: 4.7108123,
      lon: -74.0721987,
      recordedAt: NOW,
      speedKph: 19.44,
      accuracyM: 6.5,
      heading: 359.98,
    })

    expect(result.stored).toBe(true)
    if (!result.stored) return
    expect(result.point).toMatchObject({
      ownerId: OWNER,
      deviceSeq: 1,
      lat: 4.710812,
      lon: -74.072199,
      heading: 0,
    })
    expect(points.all()).toHaveLength(1)
    expect(device.currentSequence).toBe(1)
  })

  it('incrementa la secuencia de forma monótona', async () => {
    const { service } = createHarness()

    const first = await service.execute(OWNER, { lat: 4.7, lon: -74.07, recordedAt: NOW })
    const second = await service.execute(OWNER, { lat: 4.7, lon: -74.07, recordedAt: NOW + 15_000 })

    expect(first.stored && first.point.deviceSeq).toBe(1)
    expect(second.stored && second.point.deviceSeq).toBe(2)
  })

  it('descarta muestras con mala precisión sin consumir secuencia', async () => {
    const { service, points, device } = createHarness()

    const result = await service.execute(OWNER, {
      lat: 4.7,
      lon: -74.07,
      recordedAt: NOW,
      accuracyM: GPS_POLICY.maxAccuracyMeters + 1,
    })

    expect(result).toMatchObject({ stored: false, code: 'LOW_ACCURACY' })
    expect(points.all()).toHaveLength(0)
    expect(device.currentSequence).toBe(0)
  })

  it('descarta timestamps futuros y coordenadas inválidas', async () => {
    const { service, points } = createHarness()

    const future = await service.execute(OWNER, {
      lat: 4.7,
      lon: -74.07,
      recordedAt: NOW + 10 * 60_000,
    })
    const invalid = await service.execute(OWNER, { lat: 95, lon: -74.07, recordedAt: NOW })

    expect(future).toMatchObject({ stored: false, code: 'FUTURE_TIMESTAMP' })
    expect(invalid).toMatchObject({ stored: false, code: 'INVALID_COORDINATES' })
    expect(points.all()).toHaveLength(0)
  })
})
