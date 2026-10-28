/**
 * Metadatos estables del dispositivo. `deviceSeq` debe ser monótono por
 * instalación porque forma parte de la clave de deduplicación del backend.
 */
export interface DeviceMetadataPort {
  getDeviceId(): Promise<string>
  nextDeviceSequence(): Promise<number>
}
