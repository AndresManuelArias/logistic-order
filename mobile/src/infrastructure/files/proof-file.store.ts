import { Directory, File, Paths } from 'expo-file-system'

import type { ProofFileStorePort } from '../../ports/proofs'

const PROOF_DIRECTORY = 'delivery-proofs'

function extensionFrom(uri: string): string {
  const withoutQuery = uri.split('?')[0] ?? ''
  const candidate = withoutQuery.split('.').pop()?.toLowerCase() ?? ''
  return /^[a-z0-9]{2,5}$/.test(candidate) ? candidate : 'jpg'
}

/**
 * Copia la foto desde la caché del selector a `Documents/`, que no es purgado
 * por el sistema operativo. La prueba de entrega debe sobrevivir reinicios.
 */
export class ExpoProofFileStore implements ProofFileStorePort {
  persist(proofId: string, sourceUri: string): Promise<string> {
    const directory = new Directory(Paths.document, PROOF_DIRECTORY)
    if (!directory.exists) {
      directory.create({ intermediates: true, idempotent: true })
    }

    const destination = new File(directory, `${proofId}.${extensionFrom(sourceUri)}`)
    if (destination.exists) destination.delete()

    const source = new File(sourceUri)
    source.copy(destination)
    return Promise.resolve(destination.uri)
  }
}
